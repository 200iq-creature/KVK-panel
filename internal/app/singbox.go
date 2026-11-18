package app

import (
	"strconv"
	"strings"
)

// sing-box subscription (format=singbox): a complete client config for
// sing-box 1.12+ (official SFA/SFI/SFM/SFW apps, Karing, Throne, Hiddify
// "import as sing-box"). XHTTP, MASQUE and AmneziaWG have no sing-box client
// and are left out, as is REALITY on Xray 26.9.8+ servers (see
// realityMLKEMSince); everything else, including TUIC and Hysteria2, is in.

func singboxTLS(ep NodeInbound, defALPN []string) M {
	t := M{"enabled": true}
	if ep.SNI != "" {
		t["server_name"] = ep.SNI
	}
	switch ep.Security {
	case "reality":
		t["utls"] = M{"enabled": true, "fingerprint": singboxFP(ep.FP)}
		t["reality"] = M{"enabled": true, "public_key": ep.PublicKey, "short_id": ep.ShortID}
		return t
	}
	if ep.AllowInsecure {
		// self-signed certificates (IP addresses): the share links pin the
		// certificate hash, sing-box has no equivalent — skip verification
		t["insecure"] = true
	}
	if alpn := orStrings(ep.ALPN, defALPN); len(alpn) > 0 {
		t["alpn"] = alpn
	}
	return t
}

func singboxFP(fp string) string {
	switch fp {
	case "", "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized":
		return orDefault(fp, "chrome")
	}
	return "chrome"
}

// singboxTransport returns the V2Ray transport, ok=false when sing-box cannot
// speak it (XHTTP, HTTP-obfuscated TCP).
func singboxTransport(ep NodeInbound) (M, bool) {
	switch orDefault(ep.Network, "tcp") {
	case "tcp", "raw":
		if ep.HeaderType == "http" {
			return nil, false
		}
		return nil, true
	case "ws":
		t := M{"type": "ws", "path": orDefault(ep.Path, "/"), "max_early_data": 2048, "early_data_header_name": "Sec-WebSocket-Protocol"}
		if ep.HostHdr != "" {
			t["headers"] = M{"Host": ep.HostHdr}
		}
		return t, true
	case "httpupgrade":
		t := M{"type": "httpupgrade", "path": orDefault(ep.Path, "/")}
		if ep.HostHdr != "" {
			t["host"] = ep.HostHdr
		}
		return t, true
	case "grpc":
		return M{"type": "grpc", "service_name": ep.ServiceName}, true
	}
	return nil, false
}

func singboxOutbound(l linkItem, c *Client) M {
	ep := l.ep
	o := M{"tag": l.Name, "server": ep.Host, "server_port": ep.Port}
	stream := func() bool {
		tr, ok := singboxTransport(ep)
		if !ok {
			return false
		}
		if tr != nil {
			o["transport"] = tr
		}
		if ep.Security == "tls" || ep.Security == "reality" {
			o["tls"] = singboxTLS(ep, nil)
		}
		return true
	}
	switch ep.Protocol {
	case "vless":
		o["type"] = "vless"
		o["uuid"] = c.UUID
		if ep.Flow != "" {
			o["flow"] = ep.Flow
		}
		if !stream() {
			return nil
		}
	case "vmess":
		o["type"] = "vmess"
		o["uuid"] = c.UUID
		o["security"] = "auto"
		o["alter_id"] = 0
		if !stream() {
			return nil
		}
	case "trojan":
		o["type"] = "trojan"
		o["password"] = c.Password
		if !stream() {
			return nil
		}
	case "shadowsocks":
		o["type"] = "shadowsocks"
		o["method"] = ep.SSMethod
		pass := c.Password
		if ss2022KeyLen(ep.SSMethod) > 0 {
			pass = ep.SSPassword + ":" + ssUserKey(ep.SSMethod, c.Password, ep.ID)
		}
		o["password"] = pass
	case "hysteria2":
		o["type"] = "hysteria2"
		o["password"] = c.Password
		if ep.ObfsPassword != "" {
			o["obfs"] = M{"type": "salamander", "password": ep.ObfsPassword}
		}
		o["tls"] = singboxTLS(ep, []string{"h3"})
	case "tuic":
		o["type"] = "tuic"
		o["uuid"] = c.UUID
		o["password"] = c.Password
		o["congestion_control"] = orDefault(ep.TUICCongestion, "bbr")
		o["udp_relay_mode"] = orDefault(ep.TUICUDPMode, "native")
		if ep.TUICZeroRTT {
			o["zero_rtt_handshake"] = true
		}
		o["tls"] = singboxTLS(ep, []string{"h3"})
	default:
		return nil // xhttp/masque/amneziawg: no sing-box client
	}
	return o
}

// realityMLKEMSince is the first Xray-core release whose REALITY server rejects
// a ClientHello without an X25519MLKEM768 key share (XTLS/REALITY 8cdf7bf9c7f0).
// sing-box (up to at least 1.14.x) strips that key share, so its REALITY
// handshakes against such servers always fail with "reality verification failed".
const realityMLKEMSince = "26.9.8"

func xrayRealityStrict(ver string) bool {
	ver = strings.TrimPrefix(strings.TrimSpace(ver), "v")
	return ver == "" || !versionNewer(realityMLKEMSince, ver)
}

// realityStrict tells, per endpoint, whether the server's Xray needs the
// X25519MLKEM768 key share (local inbounds and every linked server).
func (a *App) realityStrict() func(ep NodeInbound) bool {
	local := xrayRealityStrict(a.Xray.Version())
	nodes := map[string]bool{}
	a.Store.Read(func(d *Data) {
		for _, n := range d.Nodes {
			nodes[n.ID] = xrayRealityStrict(n.Xray)
		}
	})
	return func(ep NodeInbound) bool {
		if nid, _, ok := strings.Cut(ep.ID, ":"); ok {
			if v, known := nodes[nid]; known {
				return v
			}
		}
		return local
	}
}

func buildSingbox(links []linkItem, c *Client, sub SubSettings, strict func(NodeInbound) bool) M {
	proxies := []any{}
	names := []string{}
	autoNames := []string{}
	used := map[string]bool{"direct": true, "PROXY": true}
	for _, l := range links {
		if l.External {
			continue
		}
		if l.ep.Security == "reality" && strict != nil && strict(l.ep) {
			continue // would always fail, see realityMLKEMSince
		}
		o := singboxOutbound(l, c)
		if o == nil {
			continue
		}
		// outbound tags must be unique
		tag := l.Name
		for i := 2; used[tag]; i++ {
			tag = l.Name + " (" + strconv.Itoa(i) + ")"
		}
		used[tag] = true
		o["tag"] = tag
		proxies = append(proxies, o)
		names = append(names, tag)
		if len(sub.AutoHosts) == 0 || containsStr(sub.AutoHosts, l.ep.Host) {
			autoNames = append(autoNames, tag)
		}
	}
	outbounds := []any{}
	sel := []string{}
	auto := orDefault(sub.AutoName, "Автовыбор")
	if len(autoNames) > 1 && !used[auto] {
		sel = append(sel, auto)
	}
	sel = append(sel, names...)
	if len(sel) == 0 {
		sel = []string{"direct"}
	}
	outbounds = append(outbounds, M{"type": "selector", "tag": "PROXY", "outbounds": sel, "default": sel[0], "interrupt_exist_connections": true})
	if len(autoNames) > 1 && !used[auto] {
		outbounds = append(outbounds, M{"type": "urltest", "tag": auto, "outbounds": autoNames,
			"url": "https://www.gstatic.com/generate_204", "interval": "3m", "tolerance": 50, "interrupt_exist_connections": true})
	}
	outbounds = append(outbounds, proxies...)
	outbounds = append(outbounds, M{"type": "direct", "tag": "direct"})

	rules := []any{
		M{"action": "sniff"},
		M{"protocol": "dns", "action": "hijack-dns"},
		M{"ip_is_private": true, "outbound": "direct"},
	}
	if sub.HappRoutingRU {
		rules = append(rules, M{"domain_suffix": []string{"ru", "su", "xn--p1ai", "рф"}, "outbound": "direct"})
	}
	return M{
		"log": M{"level": "warn", "timestamp": true},
		"dns": M{
			"servers": []any{
				M{"type": "https", "tag": "remote", "server": "1.1.1.1", "detour": "PROXY"},
				M{"type": "local", "tag": "local"},
			},
			"final":    "remote",
			"strategy": "prefer_ipv4",
		},
		"inbounds": []any{
			M{"type": "tun", "tag": "tun-in", "address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"mtu": 9000, "auto_route": true, "strict_route": true, "stack": "mixed"},
			M{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 2080},
		},
		"outbounds": outbounds,
		"route": M{
			"rules":                   rules,
			"final":                   "PROXY",
			"auto_detect_interface":   true,
			"default_domain_resolver": "local",
		},
	}
}
