package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func isBrowserUA(ua string) bool {
	l := strings.ToLower(ua)
	if !strings.Contains(l, "mozilla") {
		return false
	}
	for _, k := range []string{"happ", "v2ray", "clash", "mihomo", "hiddify", "sing-box", "streisand", "shadowrocket", "stash", "nekobox", "incy", "foxray"} {
		if strings.Contains(l, k) {
			return false
		}
	}
	return true
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func (a *App) handleSubscription(w http.ResponseWriter, r *http.Request, subID string) {
	q := r.URL.Query()
	format := q.Get("format")
	ua := r.UserAgent()
	if format == "" && isBrowserUA(ua) {
		var exists bool
		a.Store.Read(func(d *Data) { exists = d.ClientBySub(subID) != nil })
		if !exists {
			http.NotFound(w, r)
			return
		}
		var subPath string
		a.Store.Read(func(d *Data) { subPath = d.Settings.SubPath })
		a.serveSPA(w, r, "", "/"+subPath+"/", "sub.html")
		return
	}
	now := nowMs()
	var (
		c       Client
		found   bool
		links   []linkItem
		sub     SubSettings
		st      Settings
		subURL  string
		mainURL string
	)
	a.Store.Read(func(d *Data) {
		cl := d.ClientBySub(subID)
		if cl == nil {
			return
		}
		found = true
		c = *cl
		links = a.clientLinks(d, cl, false)
		sub = d.Sub
		st = d.Settings
		subURL = a.subBaseURLLocked(d) + cl.SubID
		if d.Main != nil {
			mainURL = d.Main.URL
		}
	})
	if !found {
		http.NotFound(w, r)
		return
	}
	if format != "info" {
		ip := clientIP(r)
		a.Store.UpdateLazy(func(d *Data) {
			if cl := d.ClientBySub(subID); cl != nil {
				cl.LastSubAt = now
				cl.LastSubUA = ua
				cl.LastSubIP = ip
			}
		})
	}
	status := c.Status(now)
	active := status == StActive
	if !active {
		links = nil
	}
	if format == "info" {
		a.subInfo(w, &c, status, links, sub, st, subURL)
		return
	}
	_ = mainURL

	title := sub.Title
	if title == "" {
		title = st.NodeName
	}
	upload, download, total := c.Up, c.Down, c.TrafficLimit
	if total == 0 && !st.ShowTrafficAll {
		upload, download = 0, 0
	}
	expire := int64(0)
	if c.ExpiresAt > 0 {
		expire = c.ExpiresAt / 1000
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("profile-title", "base64:"+b64(title))
	hours := sub.UpdateHours
	if hours <= 0 {
		hours = 12
	}
	h.Set("profile-update-interval", strconv.Itoa(hours))
	h.Set("subscription-userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", upload, download, total, expire))
	h.Set("profile-web-page-url", subURL)
	if sub.PageSupportURL != "" {
		h.Set("support-url", sub.PageSupportURL)
	}
	announce := sub.Description
	if sub.RenewRemind && (!active || (c.ExpiresAt > 0 && c.ExpiresAt-now < 86400000)) {
		if sub.RenewURL != "" {
			announce = "Подписка заканчивается — продлите: " + sub.RenewURL
		} else {
			announce = "Подписка заканчивается — продлите её, чтобы VPN продолжил работать"
		}
		if !active {
			announce = strings.Replace(announce, "заканчивается", "закончилась", 1)
		}
	}
	if announce != "" {
		h.Set("announce", "base64:"+b64(announce))
	}
	if sub.HappRoutingRU {
		h.Set("routing", "happ://routing/onadd/"+b64(happRoutingJSON()))
	}
	h.Set("Content-Disposition", `inline; filename="`+asciiOnly(title)+`"`)

	if format == "" {
		l := strings.ToLower(ua)
		switch {
		case strings.Contains(l, "clash") || strings.Contains(l, "mihomo") || strings.Contains(l, "stash"):
			format = "clash"
		case (strings.Contains(l, "happ") || strings.Contains(l, "v2raytun") || strings.Contains(l, "incy") || strings.Contains(l, "v2rayng")) &&
			(sub.AutoSelect || sub.Fragment || strings.TrimSpace(sub.JSONTemplate) != "" || sub.HappRoutingRU):
			format = "json"
		}
	}
	switch format {
	case "json":
		h.Set("Content-Type", "application/json; charset=utf-8")
		b, _ := json.MarshalIndent(buildJSONSub(links, &c, sub), "", "  ")
		_, _ = w.Write(b)
	case "clash":
		h.Set("Content-Type", "text/yaml; charset=utf-8")
		_, _ = w.Write([]byte(buildClash(links, &c, sub)))
	case "links":
		lines := []string{}
		for _, l := range links {
			lines = append(lines, l.Link)
		}
		_, _ = w.Write([]byte(strings.Join(lines, "\n")))
	default:
		lines := []string{}
		for _, l := range links {
			lines = append(lines, l.Link)
		}
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))))
	}
}

func asciiOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && r >= 32 && r != '"' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "subscription"
	}
	return b.String()
}

func (a *App) subInfo(w http.ResponseWriter, c *Client, status string, links []linkItem, sub SubSettings, st Settings, subURL string) {
	type L struct {
		Name     string `json:"name"`
		Link     string `json:"link"`
		Protocol string `json:"protocol"`
		Node     string `json:"node"`
	}
	ls := []L{}
	for _, l := range links {
		ls = append(ls, L{l.Name, l.Link, l.Protocol, l.Node})
	}
	title := sub.PageTitle
	if title == "" {
		title = sub.Title
	}
	if title == "" {
		title = st.NodeName
	}
	ok(w, map[string]any{
		"name": c.Name, "status": status, "up": c.Up, "down": c.Down, "limit": c.TrafficLimit,
		"expiresAt": c.ExpiresAt, "subUrl": subURL, "links": ls, "title": title,
		"text": sub.PageText, "support": sub.PageSupportURL, "logo": sub.PageLogoURL, "view": sub.PageView,
		"node": st.NodeName, "showTraffic": st.ShowTrafficAll || c.TrafficLimit > 0,
		"renewUrl": sub.RenewURL,
	})
}

func happRoutingJSON() string {
	m := M{
		"Name": "RU direct", "GlobalProxy": "true",
		"RemoteDNSType": "DoH", "RemoteDNSDomain": "https://cloudflare-dns.com/dns-query", "RemoteDNSIP": "1.1.1.1",
		"DomesticDNSType": "DoU", "DomesticDNSDomain": "", "DomesticDNSIP": "77.88.8.8",
		"Geoipurl":    "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat",
		"Geositeurl":  "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat",
		"LastUpdated": strconv.FormatInt(time.Now().Unix(), 10), "DnsHosts": M{},
		"DirectSites": []string{"geosite:category-ru", "geosite:private", "domain:ru", "domain:su", "domain:xn--p1ai"},
		"DirectIp":    []string{"geoip:ru", "geoip:private"},
		"ProxySites":  []string{}, "ProxyIp": []string{}, "BlockSites": []string{}, "BlockIp": []string{},
		"DomainStrategy": "IPIfNonMatch", "FakeDNS": "false",
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func ruDirectRules() []any {
	return []any{
		M{"type": "field", "domain": []string{"geosite:category-ru", "domain:ru", "domain:su", "domain:xn--p1ai"}, "outboundTag": "direct"},
		M{"type": "field", "ip": []string{"geoip:ru"}, "outboundTag": "direct"},
	}
}

func buildJSONSub(links []linkItem, c *Client, sub SubSettings) []any {
	var tpl M
	if strings.TrimSpace(sub.JSONTemplate) != "" {
		_ = json.Unmarshal([]byte(sub.JSONTemplate), &tpl)
	}
	base := func(remarks string) M {
		cfg := M{
			"remarks": remarks,
			"log":     M{"loglevel": "warning"},
			"dns":     M{"servers": []any{"https://1.1.1.1/dns-query", "8.8.8.8"}},
			"inbounds": []any{
				M{"tag": "socks", "port": 10808, "listen": "127.0.0.1", "protocol": "socks", "settings": M{"udp": true, "auth": "noauth"},
					"sniffing": M{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true}},
				M{"tag": "http", "port": 10809, "listen": "127.0.0.1", "protocol": "http", "settings": M{}},
			},
		}
		return cfg
	}
	extraOut := func(outs []any) []any {
		outs = append(outs, M{"tag": "direct", "protocol": "freedom", "settings": M{}}, M{"tag": "block", "protocol": "blackhole", "settings": M{}})
		if sub.Fragment {
			outs = append(outs, M{"tag": "fragment", "protocol": "freedom", "settings": M{"fragment": M{
				"packets": orDefault(sub.FragPackets, "tlshello"), "length": orDefault(sub.FragLength, "100-200"), "interval": orDefault(sub.FragInterval, "10-20"),
			}}})
		}
		return outs
	}
	withFrag := func(ob M) M {
		if sub.Fragment {
			if ss, okk := ob["streamSettings"].(M); okk {
				ss["sockopt"] = M{"dialerProxy": "fragment"}
			}
		}
		return ob
	}
	rulesFor := func(final M) []any {
		rules := []any{}
		if tpl != nil {
			if rt, okk := tpl["routing"].(map[string]any); okk {
				if rr, okk := rt["rules"].([]any); okk {
					rules = append(rules, rr...)
				}
			}
		}
		rules = append(rules, M{"type": "field", "ip": []string{"geoip:private"}, "outboundTag": "direct"})
		if sub.HappRoutingRU {
			rules = append(rules, ruDirectRules()...)
		}
		rules = append(rules, final)
		return rules
	}
	applyTpl := func(cfg M) {
		if tpl == nil {
			return
		}
		for _, k := range []string{"dns", "log", "inbounds", "policy"} {
			if v, okk := tpl[k]; okk {
				cfg[k] = v
			}
		}
		if rt, okk := tpl["routing"].(map[string]any); okk {
			if ds, okk := rt["domainStrategy"].(string); okk {
				cfg["routing"].(M)["domainStrategy"] = ds
			}
		}
		if outs, okk := tpl["outbounds"].([]any); okk {
			cfg["outbounds"] = append(cfg["outbounds"].([]any), outs...)
		}
		if obs, okk := tpl["observatory"].(map[string]any); okk {
			if cur, okk := cfg["observatory"].(M); okk {
				for k, v := range obs {
					cur[k] = v
				}
			}
		}
	}
	res := []any{}
	// auto-select group
	if sub.AutoSelect {
		outs := []any{}
		i := 0
		for _, l := range links {
			if l.Protocol != "vless" || l.External {
				continue
			}
			if len(sub.AutoHosts) > 0 && !containsStr(sub.AutoHosts, l.ep.Host) {
				continue
			}
			ob := clientOutbound(l.ep, c, fmt.Sprintf("proxy-%d", i))
			if ob == nil {
				continue
			}
			outs = append(outs, withFrag(ob))
			i++
		}
		if len(outs) > 0 {
			name := sub.AutoName
			if name == "" {
				name = "Автовыбор"
			}
			cfg := base(name)
			cfg["outbounds"] = extraOut(outs)
			cfg["observatory"] = M{"subjectSelector": []string{"proxy-"}, "probeURL": "https://www.gstatic.com/generate_204", "probeInterval": "1m", "enableConcurrency": true}
			cfg["routing"] = M{"domainStrategy": "AsIs", "balancers": []any{M{"tag": "auto", "selector": []string{"proxy-"}, "strategy": M{"type": "leastPing"}, "fallbackTag": "proxy-0"}},
				"rules": rulesFor(M{"type": "field", "network": "tcp,udp", "balancerTag": "auto"})}
			applyTpl(cfg)
			res = append(res, cfg)
		}
	}
	for _, l := range links {
		if l.External {
			continue
		}
		ob := clientOutbound(l.ep, c, "proxy")
		if ob == nil {
			continue
		}
		cfg := base(l.Name)
		cfg["outbounds"] = extraOut([]any{withFrag(ob)})
		cfg["routing"] = M{"domainStrategy": "AsIs", "rules": rulesFor(M{"type": "field", "network": "tcp,udp", "outboundTag": "proxy"})}
		applyTpl(cfg)
		res = append(res, cfg)
	}
	return res
}

func clashProxy(l linkItem, c *Client) M {
	ep := l.ep
	p := M{"name": l.Name, "server": ep.Host, "port": ep.Port, "udp": true}
	net := orDefault(ep.Network, "tcp")
	setNet := func() bool {
		switch net {
		case "tcp":
			p["network"] = "tcp"
		case "ws":
			p["network"] = "ws"
			o := M{"path": orDefault(ep.Path, "/")}
			if ep.HostHdr != "" {
				o["headers"] = M{"Host": ep.HostHdr}
			}
			p["ws-opts"] = o
		case "httpupgrade":
			p["network"] = "ws"
			o := M{"path": orDefault(ep.Path, "/"), "v2ray-http-upgrade": true}
			if ep.HostHdr != "" {
				o["headers"] = M{"Host": ep.HostHdr}
			}
			p["ws-opts"] = o
		case "grpc":
			p["network"] = "grpc"
			p["grpc-opts"] = M{"grpc-service-name": ep.ServiceName}
		case "xhttp":
			p["network"] = "xhttp"
			o := M{"path": orDefault(ep.Path, "/"), "mode": orDefault(ep.XHTTPMode, "auto")}
			if ep.HostHdr != "" {
				o["host"] = ep.HostHdr
			}
			p["xhttp-opts"] = o
		default:
			return false
		}
		return true
	}
	setTLS := func(sniKey string) {
		switch ep.Security {
		case "reality":
			p["tls"] = true
			p[sniKey] = ep.SNI
			p["client-fingerprint"] = orDefault(ep.FP, "chrome")
			p["reality-opts"] = M{"public-key": ep.PublicKey, "short-id": ep.ShortID}
		case "tls":
			p["tls"] = true
			if ep.SNI != "" {
				p[sniKey] = ep.SNI
			}
			p["client-fingerprint"] = orDefault(ep.FP, "chrome")
			if len(ep.ALPN) > 0 {
				p["alpn"] = ep.ALPN
			}
			if ep.AllowInsecure {
				p["skip-cert-verify"] = true
			}
		}
	}
	switch ep.Protocol {
	case "vless":
		p["type"] = "vless"
		p["uuid"] = c.UUID
		if ep.Flow != "" {
			p["flow"] = ep.Flow
		}
		if !setNet() {
			return nil
		}
		setTLS("servername")
	case "vmess":
		p["type"] = "vmess"
		p["uuid"] = c.UUID
		p["alterId"] = 0
		p["cipher"] = "auto"
		if !setNet() {
			return nil
		}
		setTLS("servername")
	case "trojan":
		p["type"] = "trojan"
		p["password"] = c.Password
		if !setNet() {
			return nil
		}
		setTLS("sni")
	case "shadowsocks":
		p["type"] = "ss"
		p["cipher"] = ep.SSMethod
		pass := c.Password
		if ss2022KeyLen(ep.SSMethod) > 0 {
			pass = ep.SSPassword + ":" + ssUserKey(ep.SSMethod, c.Password, ep.ID)
		}
		p["password"] = pass
	case "hysteria2":
		p["type"] = "hysteria2"
		p["password"] = c.Password
		if ep.SNI != "" {
			p["sni"] = ep.SNI
		}
		if ep.AllowInsecure {
			p["skip-cert-verify"] = true
		}
		if ep.ObfsPassword != "" {
			p["obfs"] = "salamander"
			p["obfs-password"] = ep.ObfsPassword
		}
		p["alpn"] = []string{"h3"}
	default:
		return nil
	}
	return p
}

func buildClash(links []linkItem, c *Client, sub SubSettings) string {
	var b strings.Builder
	b.WriteString("mixed-port: 7890\nallow-lan: false\nmode: rule\nlog-level: warning\nipv6: true\n")
	b.WriteString("dns:\n  enable: true\n  enhanced-mode: fake-ip\n  nameserver:\n    - https://1.1.1.1/dns-query\n    - https://8.8.8.8/dns-query\n")
	b.WriteString("proxies:\n")
	names := []string{}
	autoNames := []string{}
	for _, l := range links {
		if l.External {
			continue
		}
		p := clashProxy(l, c)
		if p == nil {
			continue
		}
		j, _ := json.Marshal(p)
		b.WriteString("  - " + string(j) + "\n")
		names = append(names, l.Name)
		if l.Protocol == "vless" && (len(sub.AutoHosts) == 0 || containsStr(sub.AutoHosts, l.ep.Host)) {
			autoNames = append(autoNames, l.Name)
		}
	}
	if len(names) == 0 {
		b.WriteString("  - {\"name\":\"нет доступных подключений\",\"type\":\"socks5\",\"server\":\"127.0.0.1\",\"port\":1}\n")
		names = append(names, "нет доступных подключений")
	}
	auto := sub.AutoName
	if auto == "" {
		auto = "Автовыбор"
	}
	b.WriteString("proxy-groups:\n")
	sel := []string{}
	if sub.AutoSelect && len(autoNames) > 0 {
		sel = append(sel, auto)
	}
	sel = append(sel, names...)
	j, _ := json.Marshal(M{"name": "PROXY", "type": "select", "proxies": sel})
	b.WriteString("  - " + string(j) + "\n")
	if sub.AutoSelect && len(autoNames) > 0 {
		j, _ := json.Marshal(M{"name": auto, "type": "url-test", "proxies": autoNames, "url": "https://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50})
		b.WriteString("  - " + string(j) + "\n")
	}
	b.WriteString("rules:\n")
	b.WriteString("  - IP-CIDR,127.0.0.0/8,DIRECT,no-resolve\n  - IP-CIDR,10.0.0.0/8,DIRECT,no-resolve\n  - IP-CIDR,172.16.0.0/12,DIRECT,no-resolve\n  - IP-CIDR,192.168.0.0/16,DIRECT,no-resolve\n")
	if sub.HappRoutingRU {
		b.WriteString("  - DOMAIN-SUFFIX,ru,DIRECT\n  - DOMAIN-SUFFIX,su,DIRECT\n  - DOMAIN-SUFFIX,xn--p1ai,DIRECT\n  - GEOSITE,category-ru,DIRECT\n  - GEOIP,RU,DIRECT\n")
	}
	b.WriteString("  - MATCH,PROXY\n")
	return b.String()
}
