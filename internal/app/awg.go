package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
)

// ---------- keys ----------

func newWGKeyPair() (priv, pub string) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", ""
	}
	return base64.StdEncoding.EncodeToString(k.Bytes()), base64.StdEncoding.EncodeToString(k.PublicKey().Bytes())
}

func wgPublic(priv string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(priv))
	if err != nil || len(b) != 32 {
		return "", errors.New("ключ: 32 байта в base64")
	}
	k, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

func wgHex(b64 string) string {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(b) != 32 {
		return ""
	}
	return hex.EncodeToString(b)
}

// ---------- settings ----------

func rnd(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	return lo + int(n.Int64())
}

// newAWGObfuscation generates obfuscation parameters compatible with every
// AmneziaWG client (1.0 field set; S3/S4/I1-I5 stay empty).
func newAWGObfuscation(a *AWGSettings) {
	a.Jc = rnd(3, 6)
	a.Jmin = rnd(40, 60)
	a.Jmax = a.Jmin + rnd(20, 70)
	a.S1 = rnd(15, 150)
	for {
		a.S2 = rnd(15, 150)
		if a.S1+56 != a.S2 {
			break
		}
	}
	a.S3, a.S4 = 0, 0
	seen := map[uint32]bool{}
	hs := make([]string, 4)
	for i := range hs {
		for {
			var b [4]byte
			_, _ = rand.Read(b[:])
			v := binary.BigEndian.Uint32(b[:]) & 0x7fffffff
			if v > 4 && !seen[v] {
				seen[v] = true
				hs[i] = strconv.FormatUint(uint64(v), 10)
				break
			}
		}
	}
	a.H1, a.H2, a.H3, a.H4 = hs[0], hs[1], hs[2], hs[3]
	a.I1, a.I2, a.I3, a.I4, a.I5 = "", "", "", "", ""
}

func defaultAWG() *AWGSettings {
	a := &AWGSettings{Network: "10.66.0.0/16", DNS: []string{"1.1.1.1", "1.0.0.1"}}
	a.PrivateKey, a.PublicKey = newWGKeyPair()
	newAWGObfuscation(a)
	return a
}

var hRangeRe = regexp.MustCompile(`^\d{1,10}(-\d{1,10})?$`)

func parseHRange(s string) (lo, hi uint64, err error) {
	if !hRangeRe.MatchString(s) {
		return 0, 0, errors.New("bad")
	}
	p := strings.SplitN(s, "-", 2)
	lo, err = strconv.ParseUint(p[0], 10, 32)
	if err != nil {
		return
	}
	hi = lo
	if len(p) == 2 {
		hi, err = strconv.ParseUint(p[1], 10, 32)
		if err != nil {
			return
		}
	}
	if hi < lo {
		err = errors.New("bad")
	}
	return
}

// normalizeAWG validates AmneziaWG settings and fills defaults.
func normalizeAWG(in *Inbound) error {
	if in.AWG == nil {
		in.AWG = defaultAWG()
	}
	a := in.AWG
	if strings.TrimSpace(a.PrivateKey) == "" {
		a.PrivateKey, a.PublicKey = newWGKeyPair()
	} else {
		pub, err := wgPublic(a.PrivateKey)
		if err != nil {
			return errf(400, "AmneziaWG: некорректный приватный ключ сервера")
		}
		a.PrivateKey, a.PublicKey = strings.TrimSpace(a.PrivateKey), pub
	}
	if a.Network == "" {
		a.Network = "10.66.0.0/16"
	}
	pfx, err := netip.ParsePrefix(strings.TrimSpace(a.Network))
	if err != nil || !pfx.Addr().Is4() || pfx.Bits() < 8 || pfx.Bits() > 29 {
		return errf(400, "AmneziaWG: подсеть — IPv4 вида 10.66.0.0/16 (маска от /8 до /29)")
	}
	a.Network = pfx.Masked().String()
	dns := []string{}
	for _, s := range a.DNS {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, err := netip.ParseAddr(s); err != nil {
			return errf(400, "AmneziaWG: DNS %q — нужен IP-адрес", s)
		}
		dns = append(dns, s)
	}
	if len(dns) == 0 {
		dns = []string{"1.1.1.1", "1.0.0.1"}
	}
	a.DNS = dns
	if in.MTU == 0 {
		in.MTU = 1280
	}
	if in.MTU < 1280 || in.MTU > 1500 {
		return errf(400, "AmneziaWG: MTU от 1280 до 1500")
	}
	if a.Jc < 0 || a.Jc > 128 || a.Jmin < 0 || a.Jmax < a.Jmin || a.Jmax > 1280 {
		return errf(400, "AmneziaWG: Jc 0–128, 0 ≤ Jmin ≤ Jmax ≤ 1280")
	}
	for _, v := range []int{a.S1, a.S2, a.S3, a.S4} {
		if v < 0 || v > 1280 {
			return errf(400, "AmneziaWG: S1–S4 от 0 до 1280")
		}
	}
	if a.S1+56 == a.S2 && a.S1 != 0 {
		return errf(400, "AmneziaWG: S1 + 56 не должно равняться S2 — пакеты рукопожатия станут одного размера")
	}
	type rg struct{ lo, hi uint64 }
	var rs []rg
	for i, h := range []*string{&a.H1, &a.H2, &a.H3, &a.H4} {
		*h = strings.ReplaceAll(strings.TrimSpace(*h), " ", "")
		if *h == "" {
			*h = strconv.Itoa(i + 1)
		}
		lo, hi, err := parseHRange(*h)
		if err != nil {
			return errf(400, "AmneziaWG: H%d — число или диапазон вида 100-200", i+1)
		}
		for _, r := range rs {
			if lo <= r.hi && r.lo <= hi {
				return errf(400, "AmneziaWG: значения H1–H4 не должны совпадать или пересекаться")
			}
		}
		rs = append(rs, rg{lo, hi})
	}
	for _, p := range []*string{&a.I1, &a.I2, &a.I3, &a.I4, &a.I5} {
		*p = strings.TrimSpace(*p)
		if strings.ContainsAny(*p, "\n\r") {
			return errf(400, "AmneziaWG: I1–I5 — одна строка")
		}
	}
	return nil
}

// awgClientIP returns the tunnel address of a client inside the AWG network.
func awgClientIP(network string, index int) (netip.Addr, bool) {
	pfx, err := netip.ParsePrefix(network)
	if err != nil || !pfx.Addr().Is4() || index < 2 {
		return netip.Addr{}, false
	}
	base := pfx.Masked().Addr().As4()
	v := binary.BigEndian.Uint32(base[:]) + uint32(index)
	size := uint32(1) << (32 - pfx.Bits())
	if uint32(index) >= size-1 { // keep the broadcast address free
		return netip.Addr{}, false
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b), true
}

// awgServerIP is the first host of the network (used as the gateway address).
func awgServerIP(network string) netip.Addr {
	ip, _ := awgClientIP(network, 1)
	return ip
}

// awgObfLines renders the obfuscation fields (same names in .conf and UAPI).
func awgObfLines(a AWGSettings, uapi bool) []string {
	kv := func(k string, v any) string {
		if uapi {
			return strings.ToLower(k) + "=" + fmt.Sprint(v)
		}
		return k + " = " + fmt.Sprint(v)
	}
	l := []string{kv("Jc", a.Jc), kv("Jmin", a.Jmin), kv("Jmax", a.Jmax), kv("S1", a.S1), kv("S2", a.S2)}
	if a.S3 > 0 {
		l = append(l, kv("S3", a.S3))
	}
	if a.S4 > 0 {
		l = append(l, kv("S4", a.S4))
	}
	l = append(l, kv("H1", a.H1), kv("H2", a.H2), kv("H3", a.H3), kv("H4", a.H4))
	for i, v := range []string{a.I1, a.I2, a.I3, a.I4, a.I5} {
		if v != "" {
			l = append(l, kv("I"+strconv.Itoa(i+1), v))
		}
	}
	return l
}

// awgClientConfig renders the AmneziaWG .conf for a client.
func awgClientConfig(ep NodeInbound, c *Client) string {
	if ep.AWG == nil || c.WGKey == "" {
		return ""
	}
	ip, ok := awgClientIP(ep.AWG.Network, c.WGIndex)
	if !ok {
		return ""
	}
	mtu := ep.MTU
	if mtu == 0 {
		mtu = 1280
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString("PrivateKey = " + c.WGKey + "\n")
	b.WriteString("Address = " + ip.String() + "/32\n")
	b.WriteString("DNS = " + strings.Join(ep.AWG.DNS, ", ") + "\n")
	b.WriteString("MTU = " + strconv.Itoa(mtu) + "\n")
	for _, l := range awgObfLines(*ep.AWG, false) {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n[Peer]\n")
	b.WriteString("PublicKey = " + ep.AWG.PublicKey + "\n")
	b.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	b.WriteString("Endpoint = " + hostPort(ep.Host, ep.Port, "") + "\n")
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String()
}

// ---------- runtime ----------

type awgPeer struct {
	user embUser
	ip   netip.Addr
	pub  string // hex
}

type awgRunner struct {
	a     *App
	key   string
	id    string
	dev   *device.Device
	stack *gvStack
	ctr   *trafficCounter
	mu    sync.Mutex
	peers map[string]awgPeer // pub hex -> peer
	byIP  atomic.Pointer[map[netip.Addr]embUser]
	eps   atomic.Pointer[map[string]string] // client name -> endpoint IP (from Online)
}

func (a *App) startAWG(s embSpec) (embRunner, error) {
	if s.AWG.PrivateKey == "" {
		return nil, errors.New("нет ключа сервера")
	}
	mtu := s.In.MTU
	if mtu == 0 {
		mtu = 1280
	}
	r := &awgRunner{a: a, key: s.coreKey(), id: s.In.ID, ctr: a.emb.counter(s.In.ID), peers: map[string]awgPeer{}}
	empty := map[netip.Addr]embUser{}
	r.byIP.Store(&empty)
	st, err := newGVStack(mtu, r.handleTCP, r.handleUDP)
	if err != nil {
		return nil, err
	}
	r.stack = st
	lg := &device.Logger{
		Verbosef: device.DiscardLogf,
		Errorf: func(f string, args ...any) {
			msg := fmt.Sprintf(f, args...)
			if strings.Contains(msg, "handshake") || strings.Contains(msg, "Failed to send") {
				return // noise from scanners and roaming clients
			}
			log.Printf("amneziawg %s: %s", s.In.Remark, msg)
		},
	}
	r.dev = device.NewDevice(st, conn.NewDefaultBind(), lg)
	var cfg strings.Builder
	cfg.WriteString("private_key=" + wgHex(s.AWG.PrivateKey) + "\n")
	cfg.WriteString("listen_port=" + strconv.Itoa(s.In.Port) + "\n")
	for _, l := range awgObfLines(s.AWG, true) {
		cfg.WriteString(l + "\n")
	}
	cfg.WriteString("replace_peers=true\n")
	if err := r.dev.IpcSet(cfg.String()); err != nil {
		r.dev.Close()
		return nil, fmt.Errorf("настройки AmneziaWG: %v", err)
	}
	if err := r.dev.Up(); err != nil {
		r.dev.Close()
		return nil, err
	}
	if err := r.Update(s); err != nil {
		r.dev.Close()
		return nil, err
	}
	return r, nil
}

func (r *awgRunner) CoreKey() string { return r.key }

func (r *awgRunner) Close() {
	r.dev.Close() // also closes the stack (tun device)
}

// Update syncs peers incrementally so other clients keep their sessions.
func (r *awgRunner) Update(s embSpec) error {
	want := map[string]awgPeer{}
	byIP := map[netip.Addr]embUser{}
	for _, u := range s.Users {
		pubB64, err := wgPublic(u.WGKey)
		if err != nil {
			continue
		}
		ip, ok := awgClientIP(s.AWG.Network, u.WGIndex)
		if !ok {
			continue
		}
		if _, dup := byIP[ip]; dup {
			continue
		}
		p := awgPeer{user: u, ip: ip, pub: wgHex(pubB64)}
		want[p.pub] = p
		byIP[ip] = u
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for pub := range r.peers {
		if _, ok := want[pub]; !ok {
			b.WriteString("public_key=" + pub + "\nremove=true\n")
		}
	}
	for pub, p := range want {
		old, ok := r.peers[pub]
		if ok && old.ip == p.ip {
			continue
		}
		b.WriteString("public_key=" + pub + "\nreplace_allowed_ips=true\nallowed_ip=" + p.ip.String() + "/32\n")
	}
	if b.Len() > 0 {
		if err := r.dev.IpcSet(b.String()); err != nil {
			return fmt.Errorf("клиенты AmneziaWG: %v", err)
		}
	}
	r.peers = want
	r.byIP.Store(&byIP)
	return nil
}

// Online lists clients with a recent handshake: name -> endpoint IP.
func (r *awgRunner) Online() map[string]string {
	out := map[string]string{}
	txt, err := r.dev.IpcGet()
	if err != nil {
		return out
	}
	r.mu.Lock()
	peers := r.peers
	r.mu.Unlock()
	var cur, ep string
	var hs int64
	flush := func() {
		if p, ok := peers[cur]; ok && ep != "" && hs > 0 && time.Since(time.Unix(hs, 0)) < 3*time.Minute {
			if ap, err := netip.ParseAddrPort(ep); err == nil {
				out[p.user.Name] = ap.Addr().Unmap().String()
			}
		}
	}
	for _, line := range strings.Split(txt, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			flush()
			cur, ep, hs = v, "", 0
		case "endpoint":
			ep = v
		case "last_handshake_time_sec":
			hs, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	flush()
	r.eps.Store(&out)
	return out
}

func (r *awgRunner) lookup(src netip.Addr) (embUser, bool) {
	m := r.byIP.Load()
	if m == nil {
		return embUser{}, false
	}
	u, ok := (*m)[src.Unmap()]
	if ok {
		// device limit: the panel blocks endpoint IPs above the limit
		if eps := r.eps.Load(); eps != nil {
			if ip := (*eps)[u.Name]; ip != "" && r.a.ipBlocked(ip) {
				return u, false
			}
		}
	}
	return u, ok
}

func (r *awgRunner) handleTCP(src netip.Addr, dst bridgeDest) (*bridgeConn, error) {
	u, ok := r.lookup(src)
	if !ok {
		return nil, errors.New("unknown peer")
	}
	return r.a.dialBridge(u.UUID, false, dst, r.ctr)
}

func (r *awgRunner) handleUDP(src netip.Addr, dst bridgeDest) (*bridgeConn, error) {
	u, ok := r.lookup(src)
	if !ok {
		return nil, errors.New("unknown peer")
	}
	return r.a.dialBridge(u.UUID, true, dst, r.ctr)
}
