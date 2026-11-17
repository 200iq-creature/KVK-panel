package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

func genX25519() (priv, pub string) {
	k, _ := ecdh.X25519().GenerateKey(rand.Reader)
	return base64.RawURLEncoding.EncodeToString(k.Bytes()), base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
}

func pubFromPriv(priv string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(priv, "="))
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(priv)
		if err != nil {
			return "", err
		}
	}
	k, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

func portFree(port int, proto string) bool {
	if proto == "udp" {
		c, err := net.ListenPacket("udp", ":"+strconv.Itoa(port))
		if err != nil {
			return false
		}
		c.Close()
		return true
	}
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func randomFreePort(d *Data) int {
	for i := 0; i < 200; i++ {
		p := 10000 + int(randInt(50000))
		if portUsedByPanel(d, p, "") == "" && portFree(p, "tcp") && portFree(p, "udp") {
			return p
		}
	}
	return 0
}

func randInt(n int64) int64 {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var x uint64
	for _, c := range b {
		x = x<<8 | uint64(c)
	}
	return int64(x % uint64(n))
}

// portUsedByPanel returns a description if the port is used by the panel or another inbound.
func portUsedByPanel(d *Data, port int, exceptID string) string {
	return portUsedByPanelL4(d, port, exceptID, "both")
}

// portUsedByPanelL4 is portUsedByPanel for a listener of the given kind
// (tcp, udp or both): a TCP and a UDP service may share a port number.
func portUsedByPanelL4(d *Data, port int, exceptID, l4 string) string {
	if !l4Overlap(l4, "tcp") {
		for _, in := range d.Inbounds {
			if in.ID != exceptID && in.Port == port && l4Overlap(inboundL4(in), l4) {
				return "порт занят инбаундом «" + in.Remark + "»"
			}
		}
		if d.Settings.BridgePort == port {
			return "служебный порт моста TUIC/AmneziaWG"
		}
		return ""
	}
	if d.Settings.BridgePort > 0 && port == d.Settings.BridgePort {
		return "служебный порт моста TUIC/AmneziaWG"
	}
	if port == d.Settings.PanelPort {
		return "порт занят панелью"
	}
	if d.Settings.SubPort > 0 && port == d.Settings.SubPort {
		return "порт занят подпиской"
	}
	if port == d.Settings.XrayAPIPort {
		return "служебный порт API xray"
	}
	for _, in := range d.Inbounds {
		if in.ID != exceptID && in.Port == port && l4Overlap(inboundL4(in), l4) {
			return "порт занят инбаундом «" + in.Remark + "»"
		}
	}
	if fs := d.Settings.Fake; fs.Enabled {
		if fs.TLSPort > 0 && port == fs.TLSPort {
			return "служебный порт фейк-сайта"
		}
		if port == 80 && fakeRunning80() {
			return "порт 80 занят фейк-сайтом (редирект на HTTPS и проверка Let's Encrypt)"
		}
	}
	tp := d.Settings.TgProxy
	if exceptID != "tg-mtproto" && tp.MTEnabled && tp.MTPort == port {
		return "порт занят MTProto-прокси"
	}
	if exceptID != "tg-socks" && tp.SocksEnabled && tp.SocksPort == port {
		return "порт занят SOCKS5-прокси"
	}
	return ""
}

var tagRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,48}$`)

type inboundView struct {
	*Inbound
	Clients     int              `json:"clients"`
	Online      int              `json:"online"`
	ClientNames []map[string]any `json:"clientList"`
	CertName    string           `json:"certName"`
	EmbError    string           `json:"embError,omitempty"` // TUIC / AmneziaWG failed to start
}

func (a *App) apiInbounds(w http.ResponseWriter, r *http.Request) error {
	now := nowMs()
	var res []inboundView
	var ports []int
	nodeInb := []map[string]any{}
	embErr := a.embeddedErrors()
	a.Store.Read(func(d *Data) {
		// inbounds of linked servers — granted per client like local ones
		for _, n := range d.Nodes {
			seen := map[string]bool{}
			for _, ep := range n.Inbounds {
				if seen[ep.ID] {
					continue
				}
				seen[ep.ID] = true
				cnt := 0
				for _, c := range d.Clients {
					if c.HasInbound(ep.ID) {
						cnt++
					}
				}
				remark := ep.Remark
				if ep.Front {
					remark = strings.SplitN(remark, " · ", 2)[0]
				}
				nodeInb = append(nodeInb, map[string]any{"id": ep.ID, "remark": remark, "protocol": ep.Protocol, "network": ep.Network,
					"security": ep.Security, "port": ep.Port, "enabled": n.Enabled, "node": n.Name, "nodeId": n.ID, "clients": cnt})
			}
		}
		for _, in := range d.Inbounds {
			cp := *in
			v := inboundView{Inbound: &cp, ClientNames: []map[string]any{}, EmbError: embErr[in.ID]}
			for _, c := range d.Clients {
				if c.HasInbound(in.ID) {
					v.Clients++
					online := now-c.LastOnline < 150*1000
					if online {
						v.Online++
					}
					v.ClientNames = append(v.ClientNames, map[string]any{"id": c.ID, "name": c.Name, "status": c.Status(now), "online": online})
				}
			}
			if dm := d.DomainByID(in.CertID); dm != nil {
				v.CertName = dm.Name
			}
			res = append(res, v)
			ports = append(ports, in.Port)
		}
	})
	if res == nil {
		res = []inboundView{}
	}
	ok(w, map[string]any{"inbounds": res, "nodeInbounds": nodeInb, "xray": a.Xray.Status()})
	return nil
}

func (a *App) normalizeInbound(d *Data, in *Inbound, isNew bool) error {
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	switch in.Protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "hysteria2", "tuic", "amneziawg", "masque":
	default:
		return errf(400, "Неизвестный протокол")
	}
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Remark == "" {
		in.Remark = strings.ToUpper(in.Protocol)
	}
	if in.Port == 0 && isNew {
		in.Port = randomFreePort(d)
	}
	if in.Port < 1 || in.Port > 65535 {
		return errf(400, "Порт должен быть от 1 до 65535")
	}
	if msg := portUsedByPanelL4(d, in.Port, in.ID, inboundL4(in)); msg != "" {
		return errf(409, "Порт %d: %s", in.Port, msg)
	}
	if in.Tag == "" {
		in.Tag = fmt.Sprintf("in-%s-%d", in.Protocol, in.Port)
	}
	if !tagRe.MatchString(in.Tag) || in.Tag == "api" {
		return errf(400, "Тег: латиница, цифры, точка, дефис, подчёркивание (до 48 символов)")
	}
	for _, o := range d.Inbounds {
		if o.ID != in.ID && o.Tag == in.Tag {
			in.Tag = fmt.Sprintf("%s-%s", in.Tag, RandomString(4))
		}
	}
	if in.Network == "" {
		in.Network = "tcp"
	}
	switch in.Network {
	case "tcp", "ws", "grpc", "xhttp", "httpupgrade":
	default:
		return errf(400, "Неизвестный транспорт")
	}
	if in.Security == "" {
		in.Security = "none"
	}
	switch in.Protocol {
	case "shadowsocks":
		in.Network = "tcp"
		in.Security = "none"
		if in.SSMethod == "" {
			in.SSMethod = "2022-blake3-aes-128-gcm"
		}
		switch in.SSMethod {
		case "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305", "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305":
		default:
			return errf(400, "Неподдерживаемый метод Shadowsocks")
		}
		if n := ss2022KeyLen(in.SSMethod); n > 0 {
			if b, err := base64.StdEncoding.DecodeString(in.SSPassword); err != nil || len(b) != n {
				in.SSPassword = newSSServerKey(in.SSMethod)
			}
		}
	case "hysteria2":
		in.Network = "tcp"
		in.Security = "tls"
	case "tuic":
		in.Network = "tcp"
		in.Security = "tls"
		switch in.TUICCongestion {
		case "":
			in.TUICCongestion = "bbr"
		case "bbr", "cubic", "new_reno":
		default:
			return errf(400, "TUIC: контроль перегрузки — bbr, cubic или new_reno")
		}
		switch in.TUICUDPMode {
		case "":
			in.TUICUDPMode = "native"
		case "native", "quic":
		default:
			return errf(400, "TUIC: режим UDP — native или quic")
		}
		in.ALPN = uniqStrings(in.ALPN)
		if len(in.ALPN) == 0 {
			in.ALPN = []string{"h3"}
		}
	case "masque":
		in.Network = "tcp"
		in.Security = "tls"
		in.ALPN = masqueALPN(in.ALPN)
		if in.Path == "" {
			in.Path = masqueDefaultPath
		}
		if !strings.HasPrefix(in.Path, "/") || strings.ContainsAny(in.Path, "{} ") {
			return errf(400, "MASQUE: путь должен начинаться с / (без пробелов и фигурных скобок)")
		}
		if in.MTU != 0 && (in.MTU < 1280 || in.MTU > 65535) {
			return errf(400, "MASQUE: MTU от 1280 до 65535 (0 — по умолчанию)")
		}
		pool := []string{}
		v4, v6 := 0, 0
		for _, p := range in.MasquePool {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			pf, err := netip.ParsePrefix(p)
			if err != nil {
				return errf(400, "MASQUE: адрес %q — нужен вид 10.14.0.1/24 или fd14::1/64", p)
			}
			if pf.Addr().Is4() {
				v4++
			} else {
				v6++
			}
			pool = append(pool, pf.String())
		}
		if v4 > 1 || v6 > 1 {
			return errf(400, "MASQUE: не больше одной IPv4 и одной IPv6 подсети")
		}
		if len(pool) == 0 {
			pool = append(pool, defaultMasquePool...)
		}
		in.MasquePool = pool
	case "amneziawg":
		in.Network = "tcp"
		in.Security = "none"
		if err := normalizeAWG(in); err != nil {
			return err
		}
		for _, o := range d.Inbounds {
			if o.ID != in.ID && o.Protocol == "amneziawg" && o.AWG != nil {
				a, _ := netip.ParsePrefix(o.AWG.Network)
				b, _ := netip.ParsePrefix(in.AWG.Network)
				if a.Overlaps(b) {
					return errf(400, "AmneziaWG: подсеть %s пересекается с инбаундом «%s»", in.AWG.Network, o.Remark)
				}
			}
		}
	case "vmess":
		if in.Security == "reality" {
			return errf(400, "VMess не поддерживает Reality — выберите TLS или без защиты")
		}
	}
	if in.Security == "reality" {
		if in.Network == "ws" || in.Network == "httpupgrade" {
			return errf(400, "Reality работает только с транспортами TCP, gRPC и XHTTP")
		}
		in.RealityTarget = strings.TrimSpace(in.RealityTarget)
		if in.RealityTarget == "" {
			in.RealityTarget = "www.microsoft.com:443"
		}
		if !strings.Contains(in.RealityTarget, ":") {
			in.RealityTarget += ":443"
		}
		in.RealitySNI = uniqStrings(in.RealitySNI)
		if in.PrivateKey == "" {
			in.PrivateKey, in.PublicKey = genX25519()
		} else {
			pub, err := pubFromPriv(in.PrivateKey)
			if err != nil {
				return errf(400, "Некорректный приватный ключ Reality")
			}
			in.PublicKey = pub
		}
		in.ShortIDs = uniqStrings(in.ShortIDs)
		if len(in.ShortIDs) == 0 {
			in.ShortIDs = []string{RandomHex(8)}
		}
		for _, s := range in.ShortIDs {
			if len(s)%2 != 0 || len(s) > 16 || !regexp.MustCompile(`^[0-9a-f]*$`).MatchString(s) {
				return errf(400, "shortId: чётное число hex-символов, до 16")
			}
		}
		if in.SpiderX == "" {
			in.SpiderX = "/"
		}
	}
	if in.Security == "tls" {
		dm := d.DomainByID(in.CertID)
		if dm == nil {
			return errf(400, "Для TLS выберите сертификат (добавьте домен в разделе «Домены»)")
		}
		if dm.Status != "active" {
			return errf(400, "Сертификат %s ещё не выпущен", dm.Name)
		}
		if dm.Mode == "selfsigned" {
			in.AllowInsecure = true
		}
	}
	if in.Protocol == "vless" && in.Network == "tcp" && (in.Security == "reality" || in.Security == "tls") {
		if in.Flow == "" && isNew {
			in.Flow = "xtls-rprx-vision"
		}
	} else {
		in.Flow = ""
	}
	if in.Network == "grpc" && in.ServiceName == "" {
		in.ServiceName = RandomString(10)
	}
	if in.Protocol != "masque" && (in.Network == "ws" || in.Network == "xhttp" || in.Network == "httpupgrade") && (in.Path == "" || in.Path == "/") && isNew {
		in.Path = "/" + RandomString(10)
	}
	if in.Path != "" && !strings.HasPrefix(in.Path, "/") {
		in.Path = "/" + in.Path
	}
	in.ALPN = uniqStrings(in.ALPN)
	if in.Protocol != "masque" && in.Protocol != "amneziawg" {
		in.MTU = 0
		in.MasquePool = nil
	}
	if in.Protocol != "amneziawg" {
		in.AWG = nil
	}
	if in.Protocol != "tuic" {
		in.TUICCongestion, in.TUICUDPMode, in.TUICZeroRTT = "", "", false
	}
	return nil
}

// checkPortsFree verifies the OS-level ports an inbound needs.
func checkPortsFree(in *Inbound) error {
	l4 := inboundL4(in)
	if l4Overlap(l4, "tcp") && !portFree(in.Port, "tcp") {
		return errf(409, "Порт %d уже занят другой программой на сервере", in.Port)
	}
	if l4Overlap(l4, "udp") && !portFree(in.Port, "udp") {
		return errf(409, "UDP-порт %d уже занят другой программой на сервере", in.Port)
	}
	return nil
}

func (a *App) apiInboundCreate(w http.ResponseWriter, r *http.Request) error {
	var in Inbound
	if err := readJSON(r, &in); err != nil {
		return err
	}
	var grantAll bool
	if r.URL.Query().Get("grant") == "1" {
		grantAll = true
	}
	var created Inbound
	err := a.Store.Update(func(d *Data) error {
		in.ID = RandomHex(6)
		in.CreatedAt = nowMs()
		in.Enabled = true
		in.Sniffing = true
		if err := a.normalizeInbound(d, &in, true); err != nil {
			return err
		}
		if err := checkPortsFree(&in); err != nil {
			return err
		}
		d.Inbounds = append(d.Inbounds, &in)
		if grantAll {
			for _, c := range d.Clients {
				c.Inbounds = append(c.Inbounds, in.ID)
			}
		}
		d.AddEvent("ok", "Инбаунд «"+in.Remark+"» создан на порту "+strconv.Itoa(in.Port), "")
		created = in
		return nil
	})
	if err != nil {
		return err
	}
	err = a.Xray.Apply()
	if err == nil {
		if e := a.embeddedErrors()[created.ID]; e != "" {
			err = fmt.Errorf("%s не запустился: %s", protoTitle(created.Protocol), e)
		}
	}
	if err != nil {
		// roll back the inbound if xray (or the embedded server) rejects it
		a.Store.Update(func(d *Data) error {
			for i, x := range d.Inbounds {
				if x.ID == created.ID {
					d.Inbounds = append(d.Inbounds[:i], d.Inbounds[i+1:]...)
				}
			}
			for _, c := range d.Clients {
				c.Inbounds = removeStr(c.Inbounds, created.ID)
			}
			return nil
		})
		_ = a.Xray.Apply()
		return errf(400, "%v", err)
	}
	go OpenFirewallPort(created.Port, inboundFirewallProto(&created))
	go a.SyncNodes()
	ok(w, created)
	return nil
}

func (a *App) apiInboundUpdate(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var req Inbound
	if err := readJSON(r, &req); err != nil {
		return err
	}
	var backup Inbound
	err := a.Store.Update(func(d *Data) error {
		in := d.InboundByID(id)
		if in == nil {
			return errf(404, "Инбаунд не найден")
		}
		backup = *in
		req.ID = in.ID
		req.CreatedAt = in.CreatedAt
		req.Up, req.Down = in.Up, in.Down
		req.Enabled = in.Enabled
		req.Sniffing = true
		if strings.EqualFold(req.Protocol, "amneziawg") && in.AWG != nil {
			// keep the server key unless a new one was given explicitly
			if req.AWG == nil {
				cp := *in.AWG
				req.AWG = &cp
			} else if strings.TrimSpace(req.AWG.PrivateKey) == "" {
				req.AWG.PrivateKey = in.AWG.PrivateKey
			}
		}
		if req.Port != in.Port || req.Protocol != in.Protocol {
			if err := a.normalizeInbound(d, &req, false); err != nil {
				return err
			}
			// only listeners the old inbound did not already hold are checked
			nl := inboundL4(&req)
			needTCP, needUDP := l4Overlap(nl, "tcp"), l4Overlap(nl, "udp")
			if req.Port == in.Port {
				ol := inboundL4(in)
				needTCP = needTCP && !l4Overlap(ol, "tcp")
				needUDP = needUDP && !l4Overlap(ol, "udp")
			}
			if needTCP && !portFree(req.Port, "tcp") {
				return errf(409, "Порт %d уже занят другой программой на сервере", req.Port)
			}
			if needUDP && !portFree(req.Port, "udp") {
				return errf(409, "UDP-порт %d уже занят другой программой на сервере", req.Port)
			}
		} else if err := a.normalizeInbound(d, &req, false); err != nil {
			return err
		}
		*in = req
		d.AddEvent("info", "Инбаунд «"+in.Remark+"» изменён", "")
		return nil
	})
	if err != nil {
		return err
	}
	err = a.Xray.Apply()
	if err == nil {
		if e := a.embeddedErrors()[id]; e != "" {
			err = fmt.Errorf("%s не запустился: %s", protoTitle(req.Protocol), e)
		}
	}
	if err != nil {
		a.Store.Update(func(d *Data) error {
			if in := d.InboundByID(id); in != nil {
				*in = backup
			}
			return nil
		})
		_ = a.Xray.Apply()
		return errf(400, "%v", err)
	}
	a.Store.Read(func(d *Data) {
		if in := d.InboundByID(id); in != nil {
			go OpenFirewallPort(in.Port, inboundFirewallProto(in))
		}
	})
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiInboundDelete(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	err := a.Store.Update(func(d *Data) error {
		for i, in := range d.Inbounds {
			if in.ID == id {
				d.Inbounds = append(d.Inbounds[:i], d.Inbounds[i+1:]...)
				for _, c := range d.Clients {
					c.Inbounds = removeStr(c.Inbounds, id)
				}
				d.AddEvent("warn", "Инбаунд «"+in.Remark+"» удалён", "")
				return nil
			}
		}
		return errf(404, "Инбаунд не найден")
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiInboundToggle(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var state bool
	err := a.Store.Update(func(d *Data) error {
		in := d.InboundByID(id)
		if in == nil {
			return errf(404, "Инбаунд не найден")
		}
		in.Enabled = !in.Enabled
		state = in.Enabled
		return nil
	})
	if err != nil {
		return err
	}
	if err := a.Xray.Apply(); err != nil {
		return err
	}
	go a.SyncNodes()
	ok(w, map[string]any{"enabled": state})
	return nil
}

func (a *App) apiInboundMove(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var req struct {
		Dir int `json:"dir"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	err := a.Store.Update(func(d *Data) error {
		for i, in := range d.Inbounds {
			if in.ID == id {
				j := i + req.Dir
				if j < 0 || j >= len(d.Inbounds) {
					return nil
				}
				d.Inbounds[i], d.Inbounds[j] = d.Inbounds[j], d.Inbounds[i]
				return nil
			}
		}
		return errf(404, "Инбаунд не найден")
	})
	if err != nil {
		return err
	}
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiInboundGrant(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var req struct {
		All bool `json:"all"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	n := 0
	err := a.Store.Update(func(d *Data) error {
		in := d.InboundByID(id)
		if in == nil {
			return errf(404, "Инбаунд не найден")
		}
		for _, c := range d.Clients {
			has := c.HasInbound(id)
			if req.All && !has {
				c.Inbounds = append(c.Inbounds, id)
				n++
			} else if !req.All && has {
				c.Inbounds = removeStr(c.Inbounds, id)
				n++
			}
		}
		if req.All {
			d.AddEvent("info", fmt.Sprintf("«%s» выдан всем клиентам (+%d)", in.Remark, n), "")
		} else {
			d.AddEvent("warn", fmt.Sprintf("«%s» забран у всех клиентов (−%d)", in.Remark, n), "")
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	ok(w, map[string]any{"count": n})
	return nil
}

func (a *App) apiInboundAddClient(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	name := strings.TrimSpace(req.Name)
	err := a.Store.Update(func(d *Data) error {
		in := d.InboundByID(id)
		if in == nil {
			return errf(404, "Инбаунд не найден")
		}
		if name != "" {
			if c := d.ClientByName(name); c != nil {
				if c.HasInbound(id) {
					return errf(409, "У %s уже есть это подключение", name)
				}
				c.Inbounds = append(c.Inbounds, id)
				return nil
			}
		}
		c := newClient(d)
		if name == "" {
			name = autoName(d)
		}
		if err := validName(name); err != nil {
			return err
		}
		c.Name = name
		c.Inbounds = []string{id}
		d.Clients = append(d.Clients, c)
		d.AddEvent("ok", c.Name+" создан (бессрочно)", c.Name)
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiInboundRemoveClient(w http.ResponseWriter, r *http.Request) error {
	id, cid := r.PathValue("id"), r.PathValue("cid")
	err := a.Store.Update(func(d *Data) error {
		c := d.ClientByID(cid)
		if c == nil {
			return errf(404, "Клиент не найден")
		}
		c.Inbounds = removeStr(c.Inbounds, id)
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiInboundExampleLink(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var link string
	a.Store.Read(func(d *Data) {
		in := d.InboundByID(id)
		if in == nil {
			return
		}
		var c *Client
		for _, x := range d.Clients {
			if x.HasInbound(id) {
				c = x
				break
			}
		}
		if c == nil {
			c = &Client{Name: "example", UUID: NewUUID(), Password: RandomString(16)}
		}
		ep := a.publicInbound(d, in)
		li := newLinkItem(ep, c, formatName(d.Sub.NameFormat, c.Name, d.Settings.NodeName, in.Remark), d.Settings.NodeName)
		link = li.Link
		if link == "" {
			link = li.Config
		}
	})
	if link == "" {
		return errf(404, "Инбаунд не найден")
	}
	ok(w, map[string]any{"link": link})
	return nil
}

func (a *App) apiX25519(w http.ResponseWriter, r *http.Request) error {
	priv, pub := genX25519()
	ok(w, map[string]any{"privateKey": priv, "publicKey": pub, "shortId": RandomHex(8)})
	return nil
}

func (a *App) apiPortCheck(w http.ResponseWriter, r *http.Request) error {
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))
	except := r.URL.Query().Get("except")
	if port < 1 || port > 65535 {
		return errf(400, "Некорректный порт")
	}
	var msg string
	var ownPort bool
	l4 := "tcp"
	if p := r.URL.Query().Get("protocol"); p != "" {
		l4 = inboundL4(&Inbound{Protocol: p, ALPN: strings.Split(r.URL.Query().Get("alpn"), ",")})
	}
	a.Store.Read(func(d *Data) {
		msg = portUsedByPanelL4(d, port, except, l4)
		if in := d.InboundByID(except); in != nil && in.Port == port && l4Overlap(inboundL4(in), l4) {
			ownPort = true
		}
	})
	free := msg == ""
	if free && !ownPort && ((l4Overlap(l4, "tcp") && !portFree(port, "tcp")) || (l4Overlap(l4, "udp") && !portFree(port, "udp"))) {
		free = false
		msg = "порт занят другой программой на сервере"
	}
	if r.URL.Query().Get("random") == "1" {
		a.Store.Read(func(d *Data) { port = randomFreePort(d) })
		free, msg = true, ""
	}
	ok(w, map[string]any{"port": port, "free": free, "reason": msg})
	return nil
}
