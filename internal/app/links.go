package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

// panelHost returns the host clients/admins should use (domain of panel cert or public IP).
func (a *App) panelHostLocked(d *Data) string {
	if d.Settings.PanelCertID != "" {
		if dm := d.DomainByID(d.Settings.PanelCertID); dm != nil && dm.Status == "active" {
			return dm.Name
		}
	}
	if d.Settings.PublicHost != "" {
		return d.Settings.PublicHost
	}
	return "127.0.0.1"
}

func hostPort(host string, port int, scheme string) string {
	h := host
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	if (scheme == "https" && port == 443) || (scheme == "http" && port == 80) {
		return h
	}
	return h + ":" + strconv.Itoa(port)
}

func (a *App) subBaseURLLocked(d *Data) string {
	scheme := "http"
	if d.Settings.PanelCertID != "" {
		if dm := d.DomainByID(d.Settings.PanelCertID); dm != nil && dm.Status == "active" {
			scheme = "https"
		}
	}
	port := d.Settings.PanelPort
	if d.Settings.SubPort > 0 {
		port = d.Settings.SubPort
	}
	return scheme + "://" + hostPort(a.panelHostLocked(d), port, scheme) + "/" + d.Settings.SubPath + "/"
}

func (a *App) subBaseURL() string {
	var s string
	a.Store.Read(func(d *Data) { s = a.subBaseURLLocked(d) })
	return s
}

func (a *App) panelURLLocked(d *Data) string {
	scheme := "http"
	if d.Settings.PanelCertID != "" {
		if dm := d.DomainByID(d.Settings.PanelCertID); dm != nil && dm.Status == "active" {
			scheme = "https"
		}
	}
	return scheme + "://" + hostPort(a.panelHostLocked(d), d.Settings.PanelPort, scheme) + "/" + d.Settings.BasePath + "/"
}

// publicInbound converts an inbound to a public descriptor used for links.
func (a *App) publicInbound(d *Data, in *Inbound) NodeInbound {
	host := d.Settings.PublicHost
	if in.Security == "tls" || in.Protocol == "hysteria2" {
		if in.TLSServerName != "" {
			host = in.TLSServerName
		} else if dm := d.DomainByID(in.CertID); dm != nil {
			host = dm.Name
		}
	}
	if in.ExternalHost != "" {
		host = in.ExternalHost
	}
	port := in.Port
	if in.ExternalPort > 0 {
		port = in.ExternalPort
	}
	ep := NodeInbound{
		ID: in.ID, Tag: in.Tag, Remark: in.Remark, Protocol: in.Protocol, Host: host, Port: port,
		Network: in.Network, Security: in.Security, Path: in.Path, HostHdr: in.Host, ServiceName: in.ServiceName,
		XHTTPMode: in.XHTTPMode, HeaderType: in.HeaderType, SSMethod: in.SSMethod, SSPassword: in.SSPassword,
		ObfsPassword: in.ObfsPassword, AllowInsecure: in.AllowInsecure, ALPN: in.ALPN,
	}
	if in.Protocol == "vless" && in.Network == "tcp" && (in.Security == "reality" || in.Security == "tls") {
		ep.Flow = in.Flow
	}
	if in.AllowInsecure && (in.Security == "tls" || in.Protocol == "hysteria2") {
		if dm := d.DomainByID(in.CertID); dm != nil && dm.CertFile != "" {
			ep.CertSha256 = certFileSha256(dm.CertFile)
		}
	}
	fp := in.Fingerprint
	if fp == "" {
		fp = d.Settings.DefaultFP
	}
	ep.FP = fp
	switch in.Security {
	case "reality":
		if len(in.RealitySNI) > 0 {
			ep.SNI = in.RealitySNI[0]
		} else {
			h := in.RealityTarget
			if hh, _, err := net.SplitHostPort(h); err == nil {
				h = hh
			}
			ep.SNI = h
		}
		ep.PublicKey = in.PublicKey
		if len(in.ShortIDs) > 0 {
			ep.ShortID = in.ShortIDs[0]
		}
		ep.SpiderX = in.SpiderX
	case "tls":
		ep.SNI = in.TLSServerName
		if ep.SNI == "" {
			ep.SNI = host
		}
	}
	if in.Protocol == "hysteria2" {
		ep.Security = "tls"
		ep.Network = "hysteria"
		if ep.SNI == "" {
			ep.SNI = in.TLSServerName
			if ep.SNI == "" {
				ep.SNI = host
			}
		}
	}
	return ep
}

func formatName(tpl, name, node, label string) string {
	if tpl == "" {
		tpl = "{name} · {label}"
	}
	r := strings.NewReplacer("{name}", name, "{node}", node, "{label}", label)
	return strings.TrimSpace(r.Replace(tpl))
}

type linkItem struct {
	Name     string `json:"name"`
	Link     string `json:"link"`
	Protocol string `json:"protocol"`
	Node     string `json:"node"`
	Inbound  string `json:"inbound"`
	Network  string `json:"network"`
	Security string `json:"security"`
	External bool   `json:"external,omitempty"`
	ep       NodeInbound
}

// clientEndpoints lists endpoints available to a client (local + linked nodes).
func (a *App) clientLinks(d *Data, c *Client, includeDisabled bool) []linkItem {
	out := []linkItem{}
	node := d.Settings.NodeName
	for _, in := range d.Inbounds {
		if !in.Enabled && !includeDisabled {
			continue
		}
		if !c.HasInbound(in.ID) {
			continue
		}
		ep := a.publicInbound(d, in)
		nm := formatName(d.Sub.NameFormat, c.Name, node, in.Remark)
		out = append(out, linkItem{Name: nm, Link: buildLink(ep, c, nm), Protocol: in.Protocol, Node: node, Inbound: in.Remark, Network: ep.Network, Security: ep.Security, ep: ep})
	}
	for _, n := range d.Nodes {
		if !n.Enabled {
			continue
		}
		for _, ep := range n.Inbounds {
			nm := formatName(d.Sub.NameFormat, c.Name, n.Name, ep.Remark)
			out = append(out, linkItem{Name: nm, Link: buildLink(ep, c, nm), Protocol: ep.Protocol, Node: n.Name, Inbound: ep.Remark, Network: ep.Network, Security: ep.Security, ep: ep})
		}
	}
	out = append(out, externalLinks(d.Sub.ExternalLinks, c)...)
	return out
}

// externalLinks expands the operator's own links ({uuid}, {name}, {password}
// placeholders) appended to every active client's subscription.
func externalLinks(raw string, c *Client) []linkItem {
	out := []linkItem{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "://")
		if i <= 0 {
			continue
		}
		proto := strings.ToLower(line[:i])
		if proto == "hy2" {
			proto = "hysteria2"
		}
		r := strings.NewReplacer("{uuid}", c.UUID, "{password}", c.Password, "{name}", esc(c.Name))
		line = r.Replace(line)
		name := ""
		if h := strings.Index(line, "#"); h >= 0 {
			frag := line[h+1:]
			if dec, err := url.QueryUnescape(frag); err == nil {
				name = dec
			} else {
				name = frag
			}
			line = line[:h] + "#" + esc(name)
		}
		if name == "" {
			name = proto
		}
		out = append(out, linkItem{Name: name, Link: line, Protocol: proto, Node: "external", Inbound: name, External: true})
	}
	return out
}

func esc(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

func buildLink(ep NodeInbound, c *Client, remark string) string {
	hp := ep.Host
	if strings.Contains(hp, ":") && !strings.HasPrefix(hp, "[") {
		hp = "[" + hp + "]"
	}
	hp += ":" + strconv.Itoa(ep.Port)
	q := url.Values{}
	addTransport := func() {
		net := ep.Network
		if net == "" {
			net = "tcp"
		}
		q.Set("type", net)
		switch net {
		case "tcp":
			if ep.HeaderType == "http" {
				q.Set("headerType", "http")
				q.Set("path", orDefault(ep.Path, "/"))
				if ep.HostHdr != "" {
					q.Set("host", ep.HostHdr)
				}
			}
		case "ws", "httpupgrade":
			q.Set("path", orDefault(ep.Path, "/"))
			if ep.HostHdr != "" {
				q.Set("host", ep.HostHdr)
			}
		case "grpc":
			q.Set("serviceName", ep.ServiceName)
			q.Set("mode", "gun")
		case "xhttp":
			q.Set("path", orDefault(ep.Path, "/"))
			q.Set("mode", orDefault(ep.XHTTPMode, "auto"))
			if ep.HostHdr != "" {
				q.Set("host", ep.HostHdr)
			}
		}
	}
	addSecurity := func() {
		switch ep.Security {
		case "reality":
			q.Set("security", "reality")
			q.Set("pbk", ep.PublicKey)
			q.Set("fp", orDefault(ep.FP, "chrome"))
			q.Set("sni", ep.SNI)
			q.Set("sid", ep.ShortID)
			q.Set("spx", orDefault(ep.SpiderX, "/"))
		case "tls":
			q.Set("security", "tls")
			if ep.SNI != "" {
				q.Set("sni", ep.SNI)
			}
			q.Set("fp", orDefault(ep.FP, "chrome"))
			if len(ep.ALPN) > 0 {
				q.Set("alpn", strings.Join(ep.ALPN, ","))
			}
			if ep.AllowInsecure {
				q.Set("allowInsecure", "1")
				if ep.CertSha256 != "" {
					q.Set("pcs", ep.CertSha256)
				}
			}
		default:
			q.Set("security", "none")
		}
	}
	switch ep.Protocol {
	case "vless":
		q.Set("encryption", "none")
		addTransport()
		addSecurity()
		if ep.Flow != "" {
			q.Set("flow", ep.Flow)
		}
		return "vless://" + c.UUID + "@" + hp + "?" + q.Encode() + "#" + esc(remark)
	case "trojan":
		addTransport()
		addSecurity()
		return "trojan://" + url.PathEscape(c.Password) + "@" + hp + "?" + q.Encode() + "#" + esc(remark)
	case "vmess":
		v := map[string]string{
			"v": "2", "ps": remark, "add": ep.Host, "port": strconv.Itoa(ep.Port), "id": c.UUID, "aid": "0", "scy": "auto",
			"net": orDefault(ep.Network, "tcp"), "type": orDefault(ep.HeaderType, "none"), "host": ep.HostHdr, "path": ep.Path, "tls": "",
		}
		if ep.Network == "grpc" {
			v["path"] = ep.ServiceName
			v["type"] = "gun"
		}
		if ep.Network == "xhttp" {
			v["type"] = orDefault(ep.XHTTPMode, "auto")
		}
		if ep.Security == "tls" {
			v["tls"] = "tls"
			v["sni"] = ep.SNI
			v["fp"] = orDefault(ep.FP, "chrome")
			if len(ep.ALPN) > 0 {
				v["alpn"] = strings.Join(ep.ALPN, ",")
			}
		}
		b, _ := json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b)
	case "shadowsocks":
		pass := c.Password
		if ss2022KeyLen(ep.SSMethod) > 0 {
			pass = ep.SSPassword + ":" + ssUserKey(ep.SSMethod, c.Password, ep.ID)
			return "ss://" + esc(ep.SSMethod+":"+pass) + "@" + hp + "#" + esc(remark)
		}
		return "ss://" + base64.RawURLEncoding.EncodeToString([]byte(ep.SSMethod+":"+pass)) + "@" + hp + "#" + esc(remark)
	case "hysteria2":
		q2 := url.Values{}
		if ep.SNI != "" {
			q2.Set("sni", ep.SNI)
		}
		if ep.AllowInsecure {
			q2.Set("insecure", "1")
			if ep.CertSha256 != "" {
				q2.Set("pinSHA256", ep.CertSha256)
				q2.Set("pcs", ep.CertSha256)
			}
		}
		if ep.ObfsPassword != "" {
			q2.Set("obfs", "salamander")
			q2.Set("obfs-password", ep.ObfsPassword)
		}
		return "hysteria2://" + url.PathEscape(c.Password) + "@" + hp + "/?" + q2.Encode() + "#" + esc(remark)
	}
	return ""
}

// ---------- xray client outbound (JSON subscription) ----------

func clientOutbound(ep NodeInbound, c *Client, tag string) M {
	ob := M{"tag": tag}
	server := M{"address": ep.Host, "port": ep.Port}
	switch ep.Protocol {
	case "vless":
		u := M{"id": c.UUID, "encryption": "none", "level": 0}
		if ep.Flow != "" {
			u["flow"] = ep.Flow
		}
		server["users"] = []any{u}
		ob["protocol"] = "vless"
		ob["settings"] = M{"vnext": []any{server}}
	case "vmess":
		server["users"] = []any{M{"id": c.UUID, "security": "auto", "level": 0}}
		ob["protocol"] = "vmess"
		ob["settings"] = M{"vnext": []any{server}}
	case "trojan":
		server["password"] = c.Password
		ob["protocol"] = "trojan"
		ob["settings"] = M{"servers": []any{server}}
	case "shadowsocks":
		pass := c.Password
		if ss2022KeyLen(ep.SSMethod) > 0 {
			pass = ep.SSPassword + ":" + ssUserKey(ep.SSMethod, c.Password, ep.ID)
		}
		server["method"] = ep.SSMethod
		server["password"] = pass
		ob["protocol"] = "shadowsocks"
		ob["settings"] = M{"servers": []any{server}}
	case "hysteria2":
		ob["protocol"] = "hysteria"
		ob["settings"] = M{"version": 2, "address": ep.Host, "port": ep.Port}
	default:
		return nil
	}
	ss := M{}
	network := orDefault(ep.Network, "tcp")
	if ep.Protocol == "hysteria2" {
		network = "hysteria"
		ss["hysteriaSettings"] = M{"version": 2, "auth": c.Password}
		if ep.ObfsPassword != "" {
			ss["finalmask"] = M{"udp": []any{M{"type": "salamander", "settings": M{"password": ep.ObfsPassword}}}}
		}
	}
	ss["network"] = network
	switch network {
	case "ws":
		ss["wsSettings"] = M{"path": orDefault(ep.Path, "/"), "host": ep.HostHdr}
	case "grpc":
		ss["grpcSettings"] = M{"serviceName": ep.ServiceName}
	case "xhttp":
		ss["xhttpSettings"] = M{"path": orDefault(ep.Path, "/"), "host": ep.HostHdr, "mode": orDefault(ep.XHTTPMode, "auto")}
	case "httpupgrade":
		ss["httpupgradeSettings"] = M{"path": orDefault(ep.Path, "/"), "host": ep.HostHdr}
	case "tcp":
		if ep.HeaderType == "http" {
			ss["tcpSettings"] = M{"header": M{"type": "http", "request": M{"path": []string{orDefault(ep.Path, "/")}, "headers": M{"Host": []string{ep.HostHdr}}}}}
		}
	}
	switch ep.Security {
	case "reality":
		ss["security"] = "reality"
		ss["realitySettings"] = M{"serverName": ep.SNI, "fingerprint": orDefault(ep.FP, "chrome"), "publicKey": ep.PublicKey, "shortId": ep.ShortID, "spiderX": orDefault(ep.SpiderX, "/")}
	case "tls":
		ss["security"] = "tls"
		t := M{"serverName": ep.SNI, "fingerprint": orDefault(ep.FP, "chrome")}
		if ep.AllowInsecure && ep.CertSha256 != "" {
			// Xray 26+: allowInsecure was removed — pin the server certificate instead
			t["pinnedPeerCertSha256"] = ep.CertSha256
		}
		if ep.Protocol == "hysteria2" {
			t["alpn"] = []string{"h3"}
		} else if len(ep.ALPN) > 0 {
			t["alpn"] = ep.ALPN
		}
		ss["tlsSettings"] = t
	default:
		ss["security"] = "none"
	}
	ob["streamSettings"] = ss
	return ob
}

var certHashCache sync.Map // path+mtime -> hash

// certFileSha256 returns hex sha256 of the leaf certificate DER.
func certFileSha256(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	key := path + "|" + fi.ModTime().String()
	if v, ok := certHashCache.Load(key); ok {
		return v.(string)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return ""
	}
	sum := sha256.Sum256(blk.Bytes)
	h := hex.EncodeToString(sum[:])
	certHashCache.Store(key, h)
	return h
}
