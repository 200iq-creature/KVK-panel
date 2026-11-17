package app

import (
	"crypto/sha256"
	"encoding/base64"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type M = map[string]any

func ss2022KeyLen(method string) int {
	switch method {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	}
	return 0
}

// ssUserKey derives a deterministic per-client key for Shadowsocks-2022.
func ssUserKey(method, clientPass, inboundID string) string {
	n := ss2022KeyLen(method)
	if n == 0 {
		return clientPass
	}
	sum := sha256.Sum256([]byte(clientPass + "|" + inboundID))
	return base64.StdEncoding.EncodeToString(sum[:n])
}

func newSSServerKey(method string) string {
	n := ss2022KeyLen(method)
	if n == 0 {
		return RandomString(20)
	}
	return RandomBase64(n)
}

func (a *App) certPaths(certID string, d *Data) (string, string) {
	if dm := d.DomainByID(certID); dm != nil && dm.CertFile != "" {
		return dm.CertFile, dm.KeyFile
	}
	return "", ""
}

func (a *App) BuildXrayConfig() (M, error) {
	var cfg M
	a.Store.Read(func(d *Data) { cfg = a.buildConfigFrom(d, nil, false) })
	return cfg, nil
}

// buildConfigFrom builds the full xray config. When filterOverride is set (filter check),
// it is used instead of stored filter settings.
func (a *App) buildConfigFrom(d *Data, filterOverride *FilterSettings, testMode bool) M {
	now := nowMs()
	st := d.Settings
	flt := d.Filter
	if filterOverride != nil {
		flt = *filterOverride
	}
	dnsFilter := flt.Enabled && (flt.Mode == "dns" || flt.Mode == "custom")

	// ----- log -----
	logCfg := M{"loglevel": "none", "access": "none", "error": "", "dnsLog": false}
	if st.LogUntil > now && !testMode {
		logCfg = M{
			"loglevel": st.LogLevel,
			"error":    filepath.Join(a.logsDir(), "error.log"),
			"access":   "none",
			"dnsLog":   false,
		}
		if st.LogAccess {
			logCfg["access"] = filepath.Join(a.logsDir(), "access.log")
		}
	}

	inbounds := []any{}
	if !testMode {
		inbounds = append(inbounds, M{
			"tag": "api", "listen": "127.0.0.1", "port": st.XrayAPIPort,
			"protocol": "dokodemo-door", "settings": M{"address": "127.0.0.1"},
		})
	}

	activeByInbound := map[string][]*Client{}
	for _, c := range d.Clients {
		if c.Status(now) != StActive {
			continue
		}
		for _, id := range c.Inbounds {
			activeByInbound[id] = append(activeByInbound[id], c)
		}
	}

	if !testMode {
		bridge := []any{}
		inBridge := map[string]bool{}
		for _, in := range d.Inbounds {
			if !in.Enabled {
				continue
			}
			if isEmbeddedProto(in.Protocol) {
				// served by the panel; traffic enters xray through the bridge
				for _, c := range activeByInbound[in.ID] {
					if !inBridge[c.Name] {
						inBridge[c.Name] = true
						bridge = append(bridge, M{"id": c.UUID, "email": c.Name, "level": 0})
					}
				}
				continue
			}
			inbounds = append(inbounds, a.buildInbound(d, in, activeByInbound[in.ID], dnsFilter))
		}
		if st.BridgePort > 0 && a.hasEmbedded(d) {
			inbounds = append(inbounds, M{
				"tag": bridgeTag, "listen": "127.0.0.1", "port": st.BridgePort, "protocol": "vless",
				"settings":       M{"clients": bridge, "decryption": "none"},
				"streamSettings": M{"network": "tcp", "security": "none"},
				"sniffing":       M{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": !dnsFilter},
			})
		}
		if sx := a.socksInbound(d); sx != nil {
			inbounds = append(inbounds, sx)
		}
	}

	// ----- outbounds -----
	direct := M{"tag": "direct", "protocol": "freedom", "settings": M{}}
	if dnsFilter {
		direct["settings"] = M{"domainStrategy": "UseIP"}
	}
	outbounds := []any{direct, M{"tag": "blocked", "protocol": "blackhole", "settings": M{}}}

	w := d.Warp
	warpReady := w.ID != "" && w.PrivateKey != "" && w.Enabled && !testMode
	if warpReady {
		addr := []string{}
		if w.Address4 != "" {
			addr = append(addr, w.Address4+"/32")
		}
		if w.Address6 != "" {
			addr = append(addr, w.Address6+"/128")
		}
		strategy := "ForceIPv4v6"
		if w.IPv6First {
			strategy = "ForceIPv6v4"
		}
		ws := M{
			"secretKey": w.PrivateKey,
			"address":   addr,
			"peers": []any{M{
				"publicKey":  w.PeerPublic,
				"endpoint":   "engage.cloudflareclient.com:" + itoa(w.Port),
				"allowedIPs": []string{"0.0.0.0/0", "::/0"},
			}},
			"mtu":            1280,
			"domainStrategy": strategy,
		}
		if len(w.Reserved) == 3 {
			ws["reserved"] = w.Reserved
		}
		outbounds = append(outbounds, M{"tag": "warp", "protocol": "wireguard", "settings": ws})
	}

	// ----- routing -----
	rules := []any{}
	if !testMode {
		rules = append(rules, M{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"})
	}
	rules = append(rules, M{"type": "field", "ip": privateCIDRs, "outboundTag": "blocked"})

	geoTags := a.geositeTags()
	hasGeoIP := fileExists(filepath.Join(a.BinDir, "geoip.dat"))
	validGeo := func(v string) bool {
		if strings.HasPrefix(v, "geoip:") {
			return hasGeoIP
		}
		if strings.HasPrefix(v, "geosite:") {
			tag := strings.ToLower(strings.TrimPrefix(v, "geosite:"))
			if i := strings.Index(tag, "@"); i >= 0 {
				tag = tag[:i]
			}
			return geoTags[tag]
		}
		return true
	}

	if flt.Enabled {
		allowDom, _ := splitTargets(flt.Allow, validGeo)
		if len(allowDom) > 0 && flt.Mode == "lists" {
			rules = append(rules, M{"type": "field", "domain": allowDom, "outboundTag": "direct"})
		}
		if flt.Mode == "lists" {
			lists := []string{}
			if flt.Ads {
				lists = append(lists, "geosite:category-ads-all")
			}
			if flt.Adult {
				lists = append(lists, "geosite:category-porn")
			}
			if flt.Security {
				for _, t := range []string{"geosite:malware", "geosite:phishing", "geosite:cryptominers"} {
					lists = append(lists, t)
				}
			}
			valid := []string{}
			for _, l := range lists {
				if validGeo(l) {
					valid = append(valid, l)
				}
			}
			if len(valid) > 0 {
				rules = append(rules, M{"type": "field", "domain": valid, "outboundTag": "blocked"})
			}
		}
		bd, bi := splitTargets(flt.Block, validGeo)
		if len(bd) > 0 {
			rules = append(rules, M{"type": "field", "domain": bd, "outboundTag": "blocked"})
		}
		if len(bi) > 0 {
			rules = append(rules, M{"type": "field", "ip": bi, "outboundTag": "blocked"})
		}
	}

	if warpReady {
		switch w.Mode {
		case "all":
			rules = append(rules, M{"type": "field", "network": "tcp,udp", "outboundTag": "warp"})
		case "sites":
			sd, si := splitTargets(w.Sites, validGeo)
			if len(sd) > 0 {
				rules = append(rules, M{"type": "field", "domain": sd, "outboundTag": "warp"})
			}
			if len(si) > 0 {
				rules = append(rules, M{"type": "field", "ip": si, "outboundTag": "warp"})
			}
		case "fallback":
			if w.FallbackOn {
				rules = append(rules, M{"type": "field", "network": "tcp,udp", "outboundTag": "warp"})
			}
		}
	}

	cfg := M{
		"log":       logCfg,
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing":   M{"domainStrategy": "AsIs", "rules": rules},
	}
	if !testMode {
		cfg["api"] = M{"tag": "api", "services": []string{"HandlerService", "StatsService", "RoutingService", "LoggerService"}}
		cfg["stats"] = M{}
		cfg["policy"] = M{
			"levels": M{"0": M{"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true, "handshake": 4, "connIdle": 300}},
			"system": M{"statsInboundUplink": true, "statsInboundDownlink": true, "statsOutboundUplink": true, "statsOutboundDownlink": true},
		}
	}
	if dnsFilter {
		cfg["dns"] = buildFilterDNS(flt, validGeo)
	}
	return cfg
}

var privateCIDRs = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
	"192.168.0.0/16", "198.18.0.0/15", "::1/128", "fc00::/7", "fe80::/10"}

func itoa(i int) string { return strconvItoa(i) }

// splitTargets separates domain-like and ip-like routing targets.
func splitTargets(list []string, valid func(string) bool) (domains []string, ips []string) {
	for _, raw := range list {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		low := strings.ToLower(v)
		switch {
		case strings.HasPrefix(low, "geoip:"):
			if valid(low) {
				ips = append(ips, low)
			}
		case strings.HasPrefix(low, "geosite:"):
			if valid(low) {
				domains = append(domains, low)
			}
		case strings.HasPrefix(low, "domain:"), strings.HasPrefix(low, "full:"), strings.HasPrefix(low, "regexp:"), strings.HasPrefix(low, "keyword:"):
			domains = append(domains, v)
		default:
			if _, _, err := net.ParseCIDR(v); err == nil {
				ips = append(ips, v)
			} else if net.ParseIP(v) != nil {
				ips = append(ips, v)
			} else {
				domains = append(domains, "domain:"+strings.TrimPrefix(strings.TrimPrefix(low, "https://"), "http://"))
			}
		}
	}
	return
}

func filterDoH(f FilterSettings) string {
	if f.Mode == "custom" && f.CustomDoH != "" {
		return f.CustomDoH
	}
	switch {
	case f.Ads && f.Adult:
		return "https://family.adguard-dns.com/dns-query"
	case f.Ads:
		return "https://dns.adguard-dns.com/dns-query"
	case f.Adult:
		return "https://family.cloudflare-dns.com/dns-query"
	case f.Security:
		return "https://security.cloudflare-dns.com/dns-query"
	}
	return "https://dns.adguard-dns.com/dns-query"
}

func buildFilterDNS(f FilterSettings, valid func(string) bool) M {
	doh := filterDoH(f)
	local := strings.Replace(doh, "https://", "https+local://", 1)
	servers := []any{}
	allowDom, _ := splitTargets(f.Allow, valid)
	if len(allowDom) > 0 {
		servers = append(servers, M{"address": "https+local://1.1.1.1/dns-query", "domains": allowDom, "skipFallback": true})
	}
	servers = append(servers, M{"address": local, "timeoutMs": 1500})
	servers = append(servers, M{"address": "1.1.1.1", "timeoutMs": 2000})
	return M{"servers": servers, "queryStrategy": "UseIPv4", "disableCache": false}
}

func (a *App) buildInbound(d *Data, in *Inbound, clients []*Client, dnsFilter bool) M {
	ib := M{"tag": in.Tag, "port": in.Port, "protocol": in.Protocol}
	if in.Listen != "" {
		ib["listen"] = in.Listen
	}
	flow := ""
	if in.Protocol == "vless" && in.Network == "tcp" && (in.Security == "reality" || in.Security == "tls") {
		flow = in.Flow
	}
	list := []any{}
	switch in.Protocol {
	case "vless":
		for _, c := range clients {
			cl := M{"id": c.UUID, "email": c.Name, "level": 0}
			if flow != "" {
				cl["flow"] = flow
			}
			list = append(list, cl)
		}
		ib["settings"] = M{"clients": list, "decryption": "none"}
	case "vmess":
		for _, c := range clients {
			list = append(list, M{"id": c.UUID, "email": c.Name, "level": 0})
		}
		ib["settings"] = M{"clients": list}
	case "trojan":
		for _, c := range clients {
			list = append(list, M{"password": c.Password, "email": c.Name, "level": 0})
		}
		ib["settings"] = M{"clients": list}
	case "shadowsocks":
		if ss2022KeyLen(in.SSMethod) > 0 {
			for _, c := range clients {
				list = append(list, M{"password": ssUserKey(in.SSMethod, c.Password, in.ID), "email": c.Name, "level": 0})
			}
			ib["settings"] = M{"method": in.SSMethod, "password": in.SSPassword, "clients": list, "network": "tcp,udp"}
		} else {
			for _, c := range clients {
				list = append(list, M{"password": c.Password, "method": in.SSMethod, "email": c.Name, "level": 0})
			}
			ib["settings"] = M{"clients": list, "network": "tcp,udp"}
		}
	case "hysteria2":
		ib["protocol"] = "hysteria"
		for _, c := range clients {
			list = append(list, M{"auth": c.Password, "email": c.Name, "level": 0})
		}
		ib["settings"] = M{"version": 2, "clients": list}
	case "masque":
		seen := map[string]bool{}
		for _, c := range clients {
			// xray: login is the email; no ':' and unique case-insensitively
			k := strings.ToLower(c.Name)
			if strings.Contains(c.Name, ":") || seen[k] || c.Password == "" {
				continue
			}
			seen[k] = true
			list = append(list, M{"pass": c.Password, "email": c.Name, "level": 0})
		}
		pool := in.MasquePool
		if len(pool) == 0 {
			pool = defaultMasquePool
		}
		set := M{"clients": list, "address": pool}
		if in.MTU > 0 {
			set["mtu"] = in.MTU
		}
		ib["settings"] = set
	}

	ss := M{}
	network := in.Network
	if in.Protocol == "hysteria2" {
		network = "hysteria"
		ss["hysteriaSettings"] = M{"version": 2}
		if in.ObfsPassword != "" {
			ss["finalmask"] = M{"udp": []any{M{"type": "salamander", "settings": M{"password": in.ObfsPassword}}}}
		}
	}
	if in.Protocol == "masque" {
		network = "masque"
		ss["masqueSettings"] = M{"path": orDefault(in.Path, masqueDefaultPath)}
	}
	if network == "" {
		network = "tcp"
	}
	ss["network"] = network
	switch network {
	case "tcp":
		if in.HeaderType == "http" {
			ss["tcpSettings"] = M{"header": M{"type": "http", "request": M{"path": []string{orDefault(in.Path, "/")}, "headers": M{"Host": []string{orDefault(in.Host, "www.bing.com")}}}}}
		}
	case "ws":
		ss["wsSettings"] = M{"path": orDefault(in.Path, "/"), "host": in.Host}
	case "grpc":
		ss["grpcSettings"] = M{"serviceName": in.ServiceName}
	case "xhttp":
		ss["xhttpSettings"] = M{"path": orDefault(in.Path, "/"), "host": in.Host, "mode": orDefault(in.XHTTPMode, "auto")}
	case "httpupgrade":
		ss["httpupgradeSettings"] = M{"path": orDefault(in.Path, "/"), "host": in.Host}
	}
	security := in.Security
	if alwaysTLS(in.Protocol) {
		security = "tls"
	}
	switch security {
	case "reality":
		ss["security"] = "reality"
		sni := in.RealitySNI
		if len(sni) == 0 {
			host := in.RealityTarget
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			sni = []string{host}
		}
		ss["realitySettings"] = M{
			"show": false, "dest": in.RealityTarget, "xver": 0,
			"serverNames": sni, "privateKey": in.PrivateKey, "shortIds": in.ShortIDs,
		}
	case "tls":
		ss["security"] = "tls"
		cf, kf := a.certPaths(in.CertID, d)
		tls := M{"serverName": in.TLSServerName, "certificates": []any{M{"certificateFile": cf, "keyFile": kf}}}
		alpn := in.ALPN
		if in.Protocol == "hysteria2" {
			alpn = []string{"h3"}
		}
		if in.Protocol == "masque" {
			alpn = masqueALPN(in.ALPN)
		}
		if len(alpn) > 0 {
			tls["alpn"] = alpn
		}
		ss["tlsSettings"] = tls
	default:
		ss["security"] = "none"
	}
	ib["streamSettings"] = ss
	if in.Protocol != "hysteria2" {
		ib["sniffing"] = M{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": !dnsFilter}
	}
	return ib
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// ---------- geosite tags (protobuf parsing of geosite.dat) ----------

var geoCache struct {
	sync.Mutex
	mod  time.Time
	tags map[string]bool
}

func (a *App) geositeTags() map[string]bool {
	p := filepath.Join(a.BinDir, "geosite.dat")
	fi, err := os.Stat(p)
	if err != nil {
		return map[string]bool{} // no geosite.dat: no geosite tags are valid
	}
	geoCache.Lock()
	defer geoCache.Unlock()
	if geoCache.tags != nil && fi.ModTime().Equal(geoCache.mod) {
		return geoCache.tags
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	geoCache.tags = parseGeoTags(b)
	geoCache.mod = fi.ModTime()
	return geoCache.tags
}

func readVarint(b []byte, i int) (uint64, int) {
	var x uint64
	var s uint
	for i < len(b) {
		c := b[i]
		i++
		x |= uint64(c&0x7f) << s
		if c < 0x80 {
			return x, i
		}
		s += 7
	}
	return x, i
}

// parseGeoTags reads repeated entries (field 1) and takes their first string field (country_code).
func parseGeoTags(b []byte) map[string]bool {
	tags := map[string]bool{}
	i := 0
	for i < len(b) {
		key, ni := readVarint(b, i)
		i = ni
		wt := key & 7
		switch wt {
		case 2:
			l, ni := readVarint(b, i)
			i = ni
			end := i + int(l)
			if end > len(b) {
				return tags
			}
			if key>>3 == 1 {
				sub := b[i:end]
				k2, j := readVarint(sub, 0)
				if k2>>3 == 1 && k2&7 == 2 {
					l2, j2 := readVarint(sub, j)
					if j2+int(l2) <= len(sub) {
						tags[strings.ToLower(string(sub[j2:j2+int(l2)]))] = true
					}
				}
			}
			i = end
		case 0:
			_, i = readVarint(b, i)
		case 1:
			i += 8
		case 5:
			i += 4
		default:
			return tags
		}
	}
	return tags
}
