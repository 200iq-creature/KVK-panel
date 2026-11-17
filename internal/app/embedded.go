package app

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Embedded protocols are served by the panel process (not by xray):
// TUIC v5 (sing-quic) and AmneziaWG (amneziawg-go + userspace TCP/IP stack).
// Both hand decrypted traffic to xray through the local VLESS bridge.

func isEmbeddedProto(p string) bool { return p == "tuic" || p == "amneziawg" }

// alwaysTLS: protocols that always run over TLS / QUIC with a certificate.
func alwaysTLS(p string) bool { return p == "hysteria2" || p == "tuic" || p == "masque" }

// inboundL4 returns which transport ports an inbound occupies: tcp, udp or both.
func inboundL4(in *Inbound) string {
	switch in.Protocol {
	case "hysteria2", "tuic", "amneziawg":
		return "udp"
	case "shadowsocks":
		return "both"
	case "masque":
		h2, h3 := masqueVersions(in.ALPN)
		switch {
		case h2 && h3:
			return "both"
		case h2:
			return "tcp"
		}
		return "udp"
	}
	return "tcp"
}

func l4Overlap(a, b string) bool {
	return a == "both" || b == "both" || a == b
}

const masqueDefaultPath = "/.well-known/masque/ip/*/*/"

var defaultMasquePool = []string{"10.14.0.1/24", "fd14::1/64"}

// masqueALPN keeps only h2/h3 (h3 first); empty means h3 only.
func masqueALPN(in []string) []string {
	h2, h3 := masqueVersions(in)
	out := []string{}
	if h3 {
		out = append(out, "h3")
	}
	if h2 {
		out = append(out, "h2")
	}
	return out
}

func (a *App) hasEmbedded(d *Data) bool {
	for _, in := range d.Inbounds {
		if in.Enabled && isEmbeddedProto(in.Protocol) {
			return true
		}
	}
	return false
}

func masqueVersions(alpn []string) (h2, h3 bool) {
	for _, p := range alpn {
		switch p {
		case "h2":
			h2 = true
		case "h3":
			h3 = true
		}
	}
	if !h2 && !h3 {
		h3 = true
	}
	return
}

// embUser is a client allowed on an embedded inbound.
type embUser struct {
	Name     string
	UUID     string
	Password string
	WGKey    string
	WGIndex  int
}

// embSpec is everything a runner needs, copied out of the store.
type embSpec struct {
	In       Inbound
	AWG      AWGSettings
	CertFile string
	KeyFile  string
	Users    []embUser
}

// coreKey changes when the listener has to be recreated (users excluded).
func (s embSpec) coreKey() string {
	in := s.In
	k := fmt.Sprintf("%s|%s|%d|%s|%s|%v|%s|%s|%d", in.Protocol, in.Listen, in.Port, in.TUICCongestion, in.TUICUDPMode, in.TUICZeroRTT,
		strings.Join(in.ALPN, ","), s.CertFile, in.MTU)
	if in.Protocol == "amneziawg" {
		a := s.AWG
		k += fmt.Sprintf("|%s|%s|%d|%d|%d|%d|%d|%d|%d|%s|%s|%s|%s|%s|%s|%s|%s|%s", a.PrivateKey, a.Network, a.Jc, a.Jmin, a.Jmax, a.S1, a.S2, a.S3, a.S4,
			a.H1, a.H2, a.H3, a.H4, a.I1, a.I2, a.I3, a.I4, a.I5)
	}
	return k
}

type embRunner interface {
	Update(spec embSpec) error
	Close()
	CoreKey() string
	// Online reports client name -> remote IP for currently connected clients.
	Online() map[string]string
}

type embState struct {
	mu         sync.Mutex
	bridgePort int
	runners    map[string]embRunner // inbound id
	counters   map[string]*trafficCounter
	errors     map[string]string // inbound id -> last start error
	seen       map[string]map[string]time.Time
}

func (e *embState) counter(id string) *trafficCounter {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.counters == nil {
		e.counters = map[string]*trafficCounter{}
	}
	c := e.counters[id]
	if c == nil {
		c = &trafficCounter{}
		e.counters[id] = c
	}
	return c
}

// markOnline records a client connection from ip (TUIC connections).
func (e *embState) markOnline(name, ip string) {
	if name == "" || ip == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.seen == nil {
		e.seen = map[string]map[string]time.Time{}
	}
	m := e.seen[name]
	if m == nil {
		m = map[string]time.Time{}
		e.seen[name] = m
	}
	m[ip] = time.Now()
}

// prepareEmbedded allocates what the embedded protocols need before the xray
// config is built: the bridge port and AmneziaWG credentials of clients.
func (a *App) prepareEmbedded() {
	need := false
	a.Store.Read(func(d *Data) {
		has := false
		for _, in := range d.Inbounds {
			if in.Enabled && isEmbeddedProto(in.Protocol) {
				has = true
			}
		}
		if has && d.Settings.BridgePort == 0 {
			need = true
		}
		for _, c := range d.Clients {
			if c.WGKey == "" || c.WGIndex < 2 {
				need = true
				break
			}
		}
	})
	if need {
		a.Store.Update(func(d *Data) error {
			has := false
			for _, in := range d.Inbounds {
				if in.Enabled && isEmbeddedProto(in.Protocol) {
					has = true
				}
			}
			if has && d.Settings.BridgePort == 0 {
				d.Settings.BridgePort = randomFreePort(d)
			}
			ensureWGCreds(d)
			return nil
		})
	}
	a.Store.Read(func(d *Data) {
		a.emb.mu.Lock()
		a.emb.bridgePort = d.Settings.BridgePort
		a.emb.mu.Unlock()
	})
}

// ensureWGCreds gives every client an AmneziaWG key and a unique host number.
// Clients synced from a main server keep the main's values (their configs are
// generated there); local clients move away on a clash.
func ensureWGCreds(d *Data) {
	used := map[int]bool{}
	for pass := 0; pass < 2; pass++ {
		for _, c := range d.Clients {
			if (c.Origin == "main") != (pass == 0) || c.WGIndex < 2 {
				continue
			}
			if used[c.WGIndex] {
				if c.Origin != "main" {
					c.WGIndex = 0 // duplicate (e.g. after an import) — reassign
				}
				continue
			}
			used[c.WGIndex] = true
		}
	}
	next := 2
	for _, c := range d.Clients {
		if c.WGKey == "" {
			c.WGKey, _ = newWGKeyPair()
		}
		if c.WGIndex < 2 {
			for used[next] {
				next++
			}
			c.WGIndex = next
			used[next] = true
		}
	}
}

// embeddedSpecs collects specs of enabled embedded inbounds (store must be locked).
func (a *App) embeddedSpecs(d *Data) []embSpec {
	now := nowMs()
	var out []embSpec
	for _, in := range d.Inbounds {
		if !in.Enabled || !isEmbeddedProto(in.Protocol) {
			continue
		}
		sp := embSpec{In: *in}
		sp.In.AWG = nil
		if in.AWG != nil {
			sp.AWG = *in.AWG
		}
		if in.Protocol == "tuic" {
			sp.CertFile, sp.KeyFile = a.certPaths(in.CertID, d)
		}
		for _, c := range d.Clients {
			if c.Status(now) != StActive || !c.HasInbound(in.ID) {
				continue
			}
			sp.Users = append(sp.Users, embUser{Name: c.Name, UUID: c.UUID, Password: c.Password, WGKey: c.WGKey, WGIndex: c.WGIndex})
		}
		out = append(out, sp)
	}
	return out
}

// syncEmbedded starts, updates and stops embedded inbounds to match the store.
func (a *App) syncEmbedded() error {
	var specs []embSpec
	a.Store.Read(func(d *Data) { specs = a.embeddedSpecs(d) })
	want := map[string]embSpec{}
	for _, s := range specs {
		want[s.In.ID] = s
	}
	a.emb.mu.Lock()
	if a.emb.runners == nil {
		a.emb.runners = map[string]embRunner{}
	}
	if a.emb.errors == nil {
		a.emb.errors = map[string]string{}
	}
	var stop []embRunner
	for id, r := range a.emb.runners {
		s, ok := want[id]
		if !ok || r.CoreKey() != s.coreKey() {
			stop = append(stop, r)
			delete(a.emb.runners, id)
		}
	}
	a.emb.mu.Unlock()
	for _, r := range stop {
		r.Close()
	}
	var errs []string
	for _, s := range specs {
		a.emb.mu.Lock()
		r := a.emb.runners[s.In.ID]
		a.emb.mu.Unlock()
		var err error
		if r != nil {
			err = r.Update(s)
		} else {
			switch s.In.Protocol {
			case "tuic":
				r, err = a.startTUIC(s)
			case "amneziawg":
				r, err = a.startAWG(s)
			}
			if err == nil {
				a.emb.mu.Lock()
				a.emb.runners[s.In.ID] = r
				a.emb.mu.Unlock()
			}
		}
		a.emb.mu.Lock()
		if err != nil {
			msg := fmt.Sprintf("%s «%s»: %v", protoTitle(s.In.Protocol), s.In.Remark, err)
			a.emb.errors[s.In.ID] = err.Error()
			errs = append(errs, msg)
			log.Print(msg)
		} else {
			delete(a.emb.errors, s.In.ID)
		}
		a.emb.mu.Unlock()
	}
	a.emb.mu.Lock()
	for id := range a.emb.errors {
		if _, ok := want[id]; !ok {
			delete(a.emb.errors, id)
		}
	}
	a.emb.mu.Unlock()
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// syncEmbeddedLogged syncs embedded inbounds and reports new failures as events.
func (a *App) syncEmbeddedLogged() {
	before := a.embeddedErrors()
	_ = a.syncEmbedded()
	after := a.embeddedErrors()
	var msgs []string
	for id, e := range after {
		if before[id] != e {
			msgs = append(msgs, e)
		}
	}
	if len(msgs) == 0 {
		return
	}
	a.Store.Update(func(d *Data) error {
		for id, e := range after {
			if before[id] == e {
				continue
			}
			name := id
			if in := d.InboundByID(id); in != nil {
				name = protoTitle(in.Protocol) + " «" + in.Remark + "»"
			}
			d.AddEvent("bad", name+" не запустился: "+e, "")
		}
		return nil
	})
}

func (a *App) stopEmbedded() {
	a.emb.mu.Lock()
	rs := a.emb.runners
	a.emb.runners = map[string]embRunner{}
	a.emb.mu.Unlock()
	for _, r := range rs {
		r.Close()
	}
}

// embeddedStatus returns inbound id -> error for embedded inbounds that failed.
func (a *App) embeddedErrors() map[string]string {
	a.emb.mu.Lock()
	defer a.emb.mu.Unlock()
	out := map[string]string{}
	for k, v := range a.emb.errors {
		out[k] = v
	}
	return out
}

func (a *App) embeddedRunning(id string) bool {
	a.emb.mu.Lock()
	defer a.emb.mu.Unlock()
	_, ok := a.emb.runners[id]
	return ok
}

// embeddedOnline merges online clients of embedded inbounds: name -> IPs.
func (a *App) embeddedOnline() map[string][]string {
	a.emb.mu.Lock()
	rs := make([]embRunner, 0, len(a.emb.runners))
	for _, r := range a.emb.runners {
		rs = append(rs, r)
	}
	a.emb.mu.Unlock()
	set := map[string]map[string]bool{}
	add := func(n, ip string) {
		if set[n] == nil {
			set[n] = map[string]bool{}
		}
		set[n][ip] = true
	}
	for _, r := range rs {
		for n, ip := range r.Online() {
			add(n, ip)
		}
	}
	a.emb.mu.Lock()
	cut := time.Now().Add(-75 * time.Second)
	for n, m := range a.emb.seen {
		for ip, t := range m {
			if t.Before(cut) {
				delete(m, ip)
				continue
			}
			add(n, ip)
		}
		if len(m) == 0 {
			delete(a.emb.seen, n)
		}
	}
	a.emb.mu.Unlock()
	out := map[string][]string{}
	for n, m := range set {
		for ip := range m {
			out[n] = append(out[n], ip)
		}
		sort.Strings(out[n])
	}
	return out
}

// takeEmbeddedTraffic returns and resets per-inbound byte counters.
func (a *App) takeEmbeddedTraffic() map[string][2]int64 {
	a.emb.mu.Lock()
	defer a.emb.mu.Unlock()
	out := map[string][2]int64{}
	for id, c := range a.emb.counters {
		u, d := c.up.Swap(0), c.down.Swap(0)
		if u != 0 || d != 0 {
			out[id] = [2]int64{u, d}
		}
	}
	return out
}

func (a *App) ipBlocked(ip string) bool {
	a.rt.Lock()
	defer a.rt.Unlock()
	until, ok := a.rt.blockedIPs[ip]
	return ok && time.Now().Before(until)
}

func protoTitle(p string) string {
	switch p {
	case "tuic":
		return "TUIC"
	case "amneziawg":
		return "AmneziaWG"
	case "masque":
		return "MASQUE"
	case "hysteria2":
		return "Hysteria2"
	}
	return strings.ToUpper(p)
}

// certLoader serves a certificate from files and reloads it after renewal.
type certLoader struct {
	certFile, keyFile string
	mu                sync.Mutex
	mod               time.Time
	cert              *tls.Certificate
}

func (l *certLoader) load() (*tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fi, err := os.Stat(l.certFile)
	if err != nil {
		return nil, err
	}
	if l.cert != nil && fi.ModTime().Equal(l.mod) {
		return l.cert, nil
	}
	c, err := tls.LoadX509KeyPair(l.certFile, l.keyFile)
	if err != nil {
		return nil, err
	}
	l.cert, l.mod = &c, fi.ModTime()
	return l.cert, nil
}

func (l *certLoader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) { return l.load() }

func udpListenAddr(listen string, port int) string {
	return net.JoinHostPort(listen, fmt.Sprint(port))
}
