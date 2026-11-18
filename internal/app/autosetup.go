package app

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VLESS auto-setup: one click turns a set of domains into
//   - a decoy website with Let's Encrypt certificates on every domain;
//   - VLESS + Reality (Vision) on :443 that "steals" the TLS handshake from that site;
//   - VLESS + XHTTP behind the same site on a secret path, reachable through Cloudflare.

type autoStep struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"` // wait | run | ok | warn | bad | skip
	Detail string `json:"detail"`
}

type autoRunState struct {
	mu       sync.Mutex
	Running  bool           `json:"running"`
	Steps    []*autoStep    `json:"steps"`
	Error    string         `json:"error"`
	Started  int64          `json:"started"`
	Finished int64          `json:"finished"`
	Result   map[string]any `json:"result"`
	Warnings []string       `json:"warnings"`
	byKey    map[string]*autoStep
}

var autoState = &autoRunState{Steps: []*autoStep{}, Warnings: []string{}}

func (s *autoRunState) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	steps := make([]autoStep, 0, len(s.Steps))
	for _, st := range s.Steps {
		steps = append(steps, *st)
	}
	return map[string]any{"running": s.Running, "steps": steps, "error": s.Error, "started": s.Started, "finished": s.Finished,
		"result": s.Result, "warnings": append([]string{}, s.Warnings...)}
}

func (s *autoRunState) set(key, status, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.byKey[key]; st != nil {
		st.Status = status
		if detail != "" {
			st.Detail = detail
		}
	}
}

func (s *autoRunState) warn(msg string) {
	s.mu.Lock()
	s.Warnings = append(s.Warnings, msg)
	s.mu.Unlock()
}

type autoDomainReq struct {
	Name  string `json:"name"`
	Proxy bool   `json:"proxy"` // Cloudflare proxy (orange cloud) — used for CDN links
}

type autoReq struct {
	Domains      []autoDomainReq `json:"domains"`
	SNI          string          `json:"sni"`
	Port         int             `json:"port"`
	CDN          bool            `json:"cdn"`
	GrantAll     bool            `json:"grantAll"`
	CreateClient bool            `json:"createClient"`
	Email        string          `json:"email"`
	CFToken      string          `json:"cfToken"`
	CFManage     bool            `json:"cfManage"`
}

type dnsInfo struct {
	Name   string   `json:"name"`
	IPs    []string `json:"ips"`
	Status string   `json:"status"` // direct | cloudflare | other | none | invalid
	Error  string   `json:"error,omitempty"`
}

func normDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s = strings.SplitN(s, "/", 2)[0]
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return strings.TrimSuffix(s, ".")
}

func (a *App) dnsCheck(name, serverIP string) dnsInfo {
	info := dnsInfo{Name: name, IPs: []string{}}
	if !domainRe.MatchString(name) {
		info.Status, info.Error = "invalid", "некорректный домен"
		return info
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, name)
	if err != nil || len(addrs) == 0 {
		info.Status, info.Error = "none", "нет A-записи"
		return info
	}
	cf, direct := false, false
	for _, ad := range addrs {
		info.IPs = append(info.IPs, ad.IP.String())
		if isCloudflareIP(ad.IP) {
			cf = true
		}
		if serverIP != "" && ad.IP.String() == serverIP {
			direct = true
		}
	}
	switch {
	case direct:
		info.Status = "direct"
	case cf:
		info.Status = "cloudflare"
	default:
		info.Status = "other"
	}
	return info
}

func (a *App) autoServerIP() string {
	ip := a.serverIP()
	if ip == "" {
		a.Store.Read(func(d *Data) {
			if net.ParseIP(d.Settings.PublicHost) != nil {
				ip = d.Settings.PublicHost
			}
		})
	}
	return ip
}

// autoPortCheck tells whether the public Reality port can be used.
func (a *App) autoPortCheck(port int) (bool, string) {
	if port < 1 || port > 65535 {
		return false, "порт должен быть от 1 до 65535"
	}
	reason := ""
	ours := false
	a.Store.Read(func(d *Data) {
		except := ""
		for _, in := range d.Inbounds {
			if in.Auto == "reality" {
				except = in.ID
				ours = in.Port == port && in.Enabled
			}
		}
		reason = portUsedByPanel(d, port, except)
		if reason == "" && d.Settings.Fake.TLSPort == port {
			reason = "служебный порт фейк-сайта"
		}
	})
	if reason != "" {
		return false, reason
	}
	if !ours && !portFree(port, "tcp") {
		return false, "порт занят другой программой на сервере (например, nginx)"
	}
	return true, ""
}

func (a *App) apiAutoGet(w http.ResponseWriter, r *http.Request) error {
	var st FakeSiteSettings
	var inbounds []map[string]any
	var certs []map[string]any
	var clients int
	var sample *Client
	var links []linkItem
	var subURL string
	a.Store.Read(func(d *Data) {
		st = d.Settings.Fake
		clients = len(d.Clients)
		for _, in := range d.Inbounds {
			if in.Auto != "" {
				inbounds = append(inbounds, map[string]any{"id": in.ID, "auto": in.Auto, "remark": in.Remark, "port": in.Port, "enabled": in.Enabled,
					"network": in.Network, "security": in.Security, "listen": in.Listen, "frontHosts": in.FrontHosts})
			}
		}
		for _, name := range st.Domains {
			var best *Domain
			for _, dm := range d.Domains {
				if dm.Name != name {
					continue
				}
				if best == nil || (dm.Status == "active" && best.Status != "active") || (dm.Mode == "letsencrypt" && best.Mode != "letsencrypt" && dm.Status == best.Status) {
					best = dm
				}
			}
			c := map[string]any{"name": name, "status": "none", "mode": "", "expiresAt": int64(0), "error": "", "id": ""}
			if best != nil {
				c["status"], c["mode"], c["expiresAt"], c["error"], c["id"] = best.Status, best.Mode, best.ExpiresAt, best.Error, best.ID
				_, c["issuing"] = issuing.Load(best.ID)
			}
			certs = append(certs, c)
		}
		if st.Enabled {
			for _, c := range d.Clients {
				for _, in := range d.Inbounds {
					if in.Auto != "" && c.HasInbound(in.ID) {
						sample = c
						break
					}
				}
				if sample != nil {
					break
				}
			}
			if sample != nil {
				for _, l := range a.clientLinks(d, sample, false) {
					if l.Node != d.Settings.NodeName || l.External {
						continue
					}
					if in := d.InboundByID(l.ep.ID); in != nil && in.Auto != "" {
						links = append(links, l)
					}
				}
				subURL = a.subBaseURLLocked(d) + sample.SubID
			}
		}
	})
	tokenSet := st.CFToken != ""
	st.CFToken = ""
	res := map[string]any{"settings": st, "cfTokenSet": tokenSet, "site": a.fakeStatus(), "state": autoState.snapshot(),
		"serverIp": a.autoServerIP(), "inbounds": inbounds, "certs": certs, "clients": clients, "links": links, "subUrl": subURL}
	if sample != nil {
		res["sampleClient"] = sample.Name
	}
	if inbounds == nil {
		res["inbounds"] = []any{}
	}
	if certs == nil {
		res["certs"] = []any{}
	}
	if links == nil {
		res["links"] = []any{}
	}
	ok(w, res)
	return nil
}

func (a *App) apiAutoCheck(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Domains []string `json:"domains"`
		Port    int      `json:"port"`
		CFToken string   `json:"cfToken"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ip := a.autoServerIP()
	names := []string{}
	for _, d := range req.Domains {
		if n := normDomain(d); n != "" && !containsStr(names, n) {
			names = append(names, n)
		}
	}
	if len(names) > 10 {
		return errf(400, "Не больше 10 доменов")
	}
	out := make([]dnsInfo, len(names))
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Add(1)
		go func(i int, n string) { defer wg.Done(); out[i] = a.dnsCheck(n, ip) }(i, n)
	}
	wg.Wait()
	if req.Port == 0 {
		req.Port = 443
	}
	free, reason := a.autoPortCheck(req.Port)
	res := map[string]any{"serverIp": ip, "domains": out, "port": map[string]any{"port": req.Port, "free": free, "reason": reason,
		"cdnOk": containsInt(cfHTTPSPorts, req.Port)}}
	token := strings.TrimSpace(req.CFToken)
	if token == "" {
		a.Store.Read(func(d *Data) { token = d.Settings.Fake.CFToken })
	}
	if token != "" {
		cf := map[string]any{"ok": true, "zones": map[string]string{}}
		if err := cfVerify(token); err != nil {
			cf["ok"], cf["error"] = false, err.Error()
		} else {
			zones := map[string]string{}
			for _, n := range names {
				if _, z, err := cfFindZone(token, n); err == nil {
					zones[n] = z
				} else {
					zones[n] = ""
				}
			}
			cf["zones"] = zones
		}
		res["cf"] = cf
	}
	ok(w, res)
	return nil
}

// autoRemark avoids duplicate names in subscriptions when the user already has
// a hand-made inbound called the same.
func autoRemark(d *Data, base string) string {
	name := base
	for i := 2; ; i++ {
		taken := false
		for _, in := range d.Inbounds {
			if in.Remark == name {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
		name = fmt.Sprintf("%s %d", base, i)
	}
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (a *App) apiAutoRun(w http.ResponseWriter, r *http.Request) error {
	var req autoReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	seen := map[string]bool{}
	doms := []autoDomainReq{}
	for _, d := range req.Domains {
		d.Name = normDomain(d.Name)
		if d.Name == "" || seen[d.Name] {
			continue
		}
		if !domainRe.MatchString(d.Name) {
			return errf(400, "Некорректный домен: %s", d.Name)
		}
		seen[d.Name] = true
		if isFreeDomain(d.Name) {
			d.Proxy = false // sslip.io / freemyip … names can't be put behind Cloudflare
		}
		doms = append(doms, d)
	}
	if len(doms) == 0 {
		return errf(400, "Укажите хотя бы один домен — на нём будет фейк-сайт")
	}
	if len(doms) > 10 {
		return errf(400, "Не больше 10 доменов")
	}
	req.Domains = doms
	req.SNI = normDomain(req.SNI)
	if !seen[req.SNI] {
		req.SNI = doms[0].Name
	}
	if req.Port == 0 {
		req.Port = 443
	}
	if free, reason := a.autoPortCheck(req.Port); !free {
		return errf(409, "Порт %d: %s", req.Port, reason)
	}
	req.Email = strings.TrimSpace(req.Email)
	req.CFToken = strings.TrimSpace(req.CFToken)
	if req.CFManage && req.CFToken == "" {
		a.Store.Read(func(d *Data) { req.CFToken = d.Settings.Fake.CFToken })
		if req.CFToken == "" {
			return errf(400, "Укажите API-токен Cloudflare или выключите автонастройку DNS")
		}
	}
	autoState.mu.Lock()
	if autoState.Running {
		autoState.mu.Unlock()
		return errf(409, "Автонастройка уже идёт")
	}
	autoState.Running, autoState.Error, autoState.Result, autoState.Finished = true, "", nil, 0
	autoState.Started = nowMs()
	autoState.Warnings = []string{}
	autoState.Steps = []*autoStep{}
	autoState.byKey = map[string]*autoStep{}
	add := func(k, t string) {
		st := &autoStep{Key: k, Title: t, Status: "wait"}
		autoState.Steps = append(autoState.Steps, st)
		autoState.byKey[k] = st
	}
	add("dns", "Проверка DNS")
	if req.CFManage {
		add("cf", "Cloudflare: DNS-записи и режим SSL")
	}
	add("site", "Фейк-сайт")
	add("inbounds", "Инбаунды VLESS и запуск Xray")
	add("certs", "Сертификаты Let's Encrypt")
	if req.CFManage {
		add("cfssl", "Cloudflare: режим Full (strict)")
	}
	add("check", "Самопроверка")
	autoState.mu.Unlock()
	go a.runAutoSetup(req)
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) runAutoSetup(req autoReq) {
	fail := func(key string, err error) {
		autoState.set(key, "bad", err.Error())
		autoState.mu.Lock()
		autoState.Running, autoState.Error, autoState.Finished = false, err.Error(), nowMs()
		autoState.mu.Unlock()
		a.Store.Update(func(d *Data) error {
			d.AddEvent("bad", "Автонастройка VLESS не завершена: "+err.Error(), "")
			return nil
		})
	}
	defer func() {
		if rec := recover(); rec != nil {
			fail("check", fmt.Errorf("внутренняя ошибка: %v", rec))
		}
	}()
	names := []string{}
	cdnHosts := []string{}
	for _, d := range req.Domains {
		names = append(names, d.Name)
		if d.Proxy {
			cdnHosts = append(cdnHosts, d.Name)
		}
	}
	// SNI goes first in Reality serverNames
	sort.SliceStable(names, func(i, j int) bool { return names[i] == req.SNI && names[j] != req.SNI })
	if len(cdnHosts) == 0 && !isFreeDomain(req.SNI) {
		cdnHosts = []string{req.SNI}
	}
	ip := a.autoServerIP()

	// 1. DNS
	autoState.set("dns", "run", "")
	notes := []string{}
	allOK := true
	for _, n := range names {
		info := a.dnsCheck(n, ip)
		switch info.Status {
		case "direct":
			notes = append(notes, n+" → этот сервер")
		case "cloudflare":
			notes = append(notes, n+" → Cloudflare (прокси включён)")
		case "other":
			allOK = false
			notes = append(notes, n+" → "+strings.Join(info.IPs, ", ")+" (не этот сервер)")
		default:
			allOK = false
			notes = append(notes, n+": нет A-записи")
		}
	}
	if allOK || req.CFManage {
		autoState.set("dns", "ok", strings.Join(notes, "; "))
	} else {
		autoState.set("dns", "warn", strings.Join(notes, "; "))
		autoState.warn("Не все домены указывают на сервер " + ip + " — для них не выпустится сертификат. Создайте A-записи и нажмите «Перевыпустить» в разделе «Домены» (или запустите автонастройку ещё раз).")
	}

	// 2. Cloudflare DNS
	zoneIDs := map[string]string{}
	if req.CFManage {
		autoState.set("cf", "run", "")
		if ip == "" || net.ParseIP(ip).To4() == nil {
			fail("cf", fmt.Errorf("не удалось определить публичный IPv4 сервера"))
			return
		}
		if err := cfVerify(req.CFToken); err != nil {
			fail("cf", err)
			return
		}
		res := []string{}
		for _, d := range req.Domains {
			if isFreeDomain(d.Name) {
				res = append(res, d.Name+": бесплатный домен, DNS ведёт его сервис")
				continue
			}
			zid, zone, err := cfFindZone(req.CFToken, d.Name)
			if err != nil {
				fail("cf", err)
				return
			}
			zoneIDs[zone] = zid
			msg, err := cfUpsertA(req.CFToken, zid, d.Name, ip, d.Proxy)
			if err != nil {
				fail("cf", fmt.Errorf("%s: %v", d.Name, err))
				return
			}
			p := "без прокси"
			if d.Proxy {
				p = "прокси Cloudflare"
			}
			res = append(res, d.Name+": "+msg+" ("+p+")")
		}
		for zone, zid := range zoneIDs {
			// "full" accepts the temporary self-signed certificate until Let's Encrypt issues the real one
			if err := cfSetSSL(req.CFToken, zid, "full"); err != nil {
				res = append(res, zone+": не удалось включить SSL Full — "+err.Error())
			}
		}
		autoState.set("cf", "ok", strings.Join(res, "; "))
	}

	// 3. decoy site
	autoState.set("site", "run", "")
	var fs FakeSiteSettings
	a.Store.Update(func(d *Data) error {
		f := &d.Settings.Fake
		if f.TLSPort == 0 || !portFreeLocal(f.TLSPort) && !fakeServing(f.TLSPort) {
			f.TLSPort = randomLocalPort(d)
		}
		if f.XHTTPPath == "" {
			f.XHTTPPath = "/" + strings.ToLower(RandomString(4)) + "/" + strings.ToLower(RandomString(12))
		}
		if f.WSPath == "" || f.WSPath == f.XHTTPPath {
			f.WSPath = "/" + strings.ToLower(RandomString(5)) + "/" + strings.ToLower(RandomString(11))
		}
		f.Enabled, f.Domains, f.SNI, f.CDNDomains, f.Port, f.CDN = true, names, req.SNI, cdnHosts, req.Port, req.CDN
		if req.Email != "" {
			f.Email = req.Email
		}
		if req.CFToken != "" {
			f.CFToken = req.CFToken
		}
		f.SetupAt = nowMs()
		fs = *f
		return nil
	})
	if err := a.StartFakeSite(); err != nil {
		fail("site", err)
		return
	}
	siteNote := "работает на 127.0.0.1:" + strconv.Itoa(fs.TLSPort)
	if fakeRunning80() {
		siteNote += ", порт 80 — редирект на HTTPS и проверки Let's Encrypt"
	} else {
		siteNote += "; порт 80 занят другой программой — Let's Encrypt может не пройти проверку"
		autoState.warn("Порт 80 занят другой программой — освободите его, иначе сертификаты Let's Encrypt не выпустятся.")
	}
	autoState.set("site", "ok", siteNote)

	// 4. inbounds
	autoState.set("inbounds", "run", "")
	realityID, cdnID, clientName, err := a.autoInbounds(req, names, cdnHosts, fs)
	if err != nil {
		fail("inbounds", err)
		return
	}
	go OpenFirewallPort(req.Port, "tcp")
	go OpenFirewallPort(80, "tcp")
	note := "VLESS Reality (Vision) на порту " + strconv.Itoa(req.Port) + ", SNI " + req.SNI
	if cdnID != "" {
		note += "; VLESS XHTTP через CDN: " + strings.Join(cdnHosts, ", ")
	}
	if clientName != "" {
		note += "; создан клиент " + clientName
	}
	autoState.set("inbounds", "ok", note)

	// 5. certificates
	autoState.set("certs", "run", "")
	certNotes, certFails := a.autoCerts(names, fs.Email)
	if certFails == 0 {
		autoState.set("certs", "ok", strings.Join(certNotes, "; "))
	} else {
		autoState.set("certs", "warn", strings.Join(certNotes, "; "))
		autoState.warn("Не все сертификаты выпущены — пока сайт отдаёт самоподписанный. Панель будет повторять выпуск каждые 30 минут; статус — в разделе «Домены».")
	}

	// 6. Cloudflare strict
	if req.CFManage {
		autoState.set("cfssl", "run", "")
		if certFails > 0 {
			autoState.set("cfssl", "skip", "оставлен режим Full — включим Strict, когда будут сертификаты (повторите автонастройку)")
		} else {
			res := []string{}
			st := "ok"
			for zone, zid := range zoneIDs {
				if err := cfSetSSL(req.CFToken, zid, "strict"); err != nil {
					res = append(res, zone+": "+err.Error())
					st = "warn"
				} else {
					res = append(res, zone+": Full (strict)")
				}
			}
			autoState.set("cfssl", st, strings.Join(res, "; "))
		}
	}

	// 7. self-check
	autoState.set("check", "run", "")
	checks, bad := a.autoSelfCheck()
	if bad {
		autoState.set("check", "warn", strings.Join(checks, "; "))
	} else {
		autoState.set("check", "ok", strings.Join(checks, "; "))
	}

	a.Store.Update(func(d *Data) error {
		d.AddEvent("ok", "Автонастройка VLESS: Reality на "+strconv.Itoa(req.Port)+", фейк-сайт "+strings.Join(names, ", "), "")
		return nil
	})
	go a.SyncNodes()
	autoState.mu.Lock()
	autoState.Running, autoState.Finished = false, nowMs()
	autoState.Result = map[string]any{"realityId": realityID, "cdnId": cdnID, "client": clientName}
	autoState.mu.Unlock()
}

func portFreeLocal(port int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func fakeServing(port int) bool {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.tlsSrv != nil && fake.tlsPort == port
}

func randomLocalPort(d *Data) int {
	for i := 0; i < 200; i++ {
		p := 20000 + int(randInt(40000))
		if portUsedByPanel(d, p, "") == "" && portFreeLocal(p) && portFree(p, "tcp") {
			return p
		}
	}
	return 0
}

// autoInbounds creates or updates the two auto inbounds, grants them and applies Xray.
func (a *App) autoInbounds(req autoReq, names, cdnHosts []string, fs FakeSiteSettings) (realityID, cdnID, clientName string, err error) {
	var snapIn, snapCl []byte
	ip := a.autoServerIP()
	err = a.Store.Update(func(d *Data) error {
		snapIn, _ = json.Marshal(d.Inbounds)
		snapCl, _ = json.Marshal(d.Clients)
		var reality, cdn, cdnws *Inbound
		for _, in := range d.Inbounds {
			switch in.Auto {
			case "reality":
				reality = in
			case "cdn":
				cdn = in
			case "cdnws":
				cdnws = in
			}
		}
		// Reality
		isNew := reality == nil
		if isNew {
			reality = &Inbound{ID: RandomHex(6), CreatedAt: nowMs(), Sniffing: true, Auto: "reality", Remark: autoRemark(d, "Reality")}
		}
		reality.Enabled = true
		reality.Protocol, reality.Network, reality.Security = "vless", "tcp", "reality"
		reality.Port, reality.Listen = req.Port, ""
		reality.RealityTarget = "127.0.0.1:" + strconv.Itoa(fs.TLSPort)
		reality.RealitySNI = append([]string{}, names...)
		reality.Fingerprint = "chrome"
		reality.SpiderX = "/"
		reality.Flow = "xtls-rprx-vision"
		// Reality must reach the server directly: never a domain that may be behind the Cloudflare proxy
		reality.ExternalHost, reality.ExternalPort = ip, 0
		if isNew {
			reality.Tag = ""
		}
		if err := a.normalizeInbound(d, reality, isNew); err != nil {
			return err
		}
		if isNew {
			d.Inbounds = append([]*Inbound{reality}, d.Inbounds...)
		}
		realityID = reality.ID
		// CDN: XHTTP (Xray clients) + WebSocket (sing-box, Clash, NekoBox…) behind the decoy site
		var wsID string
		if req.CDN {
			x, err := a.upsertFront(d, cdn, "cdn", "CDN", "xhttp", fs.XHTTPPath, cdnHosts, realityID)
			if err != nil {
				return err
			}
			cdnID = x.ID
			w, err := a.upsertFront(d, cdnws, "cdnws", "CDN WS", "ws", fs.WSPath, cdnHosts, cdnID)
			if err != nil {
				return err
			}
			wsID = w.ID
		} else {
			for _, old := range []*Inbound{cdn, cdnws} {
				if old != nil {
					dropInbound(d, old.ID)
				}
			}
		}
		ids := []string{realityID}
		for _, id := range []string{cdnID, wsID} {
			if id != "" {
				ids = append(ids, id)
			}
		}
		if req.GrantAll {
			for _, c := range d.Clients {
				for _, id := range ids {
					if !c.HasInbound(id) {
						c.Inbounds = append(c.Inbounds, id)
					}
				}
			}
		}
		if req.CreateClient && len(d.Clients) == 0 {
			c := newClient(d)
			c.Name = autoName(d)
			for _, in := range d.Inbounds {
				if in.Enabled {
					c.Inbounds = append(c.Inbounds, in.ID)
				}
			}
			d.Clients = append(d.Clients, c)
			clientName = c.Name
			d.AddEvent("ok", c.Name+" создан автонастройкой (бессрочно)", c.Name)
		}
		return nil
	})
	if err != nil {
		return "", "", "", err
	}
	if !a.Xray.Installed() {
		return realityID, cdnID, clientName, nil
	}
	if aerr := a.Xray.Apply(); aerr != nil {
		a.Store.Update(func(d *Data) error {
			var ins []*Inbound
			var cls []*Client
			if json.Unmarshal(snapIn, &ins) == nil && json.Unmarshal(snapCl, &cls) == nil {
				d.Inbounds, d.Clients = ins, cls
			}
			return nil
		})
		_ = a.Xray.Apply()
		return "", "", "", fmt.Errorf("Xray не принял конфигурацию — изменения отменены: %v", aerr)
	}
	return realityID, cdnID, clientName, nil
}

// migrateAutoSetup upgrades setups made by older versions: Reality links must
// carry the server IP, and the WebSocket CDN entry for sing-box/Clash clients
// must exist next to XHTTP.
func (a *App) migrateAutoSetup() {
	var need, needWS bool
	a.Store.Read(func(d *Data) {
		if !d.Settings.Fake.Enabled {
			return
		}
		hasCDN, hasWS := false, false
		for _, in := range d.Inbounds {
			switch in.Auto {
			case "reality":
				if in.ExternalHost == "" || net.ParseIP(in.ExternalHost) == nil {
					need = true
				}
			case "cdn":
				hasCDN = true
			case "cdnws":
				hasWS = true
			}
		}
		needWS = d.Settings.Fake.CDN && hasCDN && !hasWS
	})
	if !need && !needWS {
		return
	}
	ip := ""
	if need {
		if ip = a.autoServerIP(); ip == "" {
			need = false
		}
	}
	changed := false
	a.Store.Update(func(d *Data) error {
		f := &d.Settings.Fake
		var cdn, ws *Inbound
		for _, in := range d.Inbounds {
			switch in.Auto {
			case "reality":
				if need && in.ExternalHost != ip {
					in.ExternalHost, changed = ip, true
				}
			case "cdn":
				cdn = in
			case "cdnws":
				ws = in
			}
		}
		if needWS && cdn != nil && ws == nil {
			if f.WSPath == "" {
				f.WSPath = "/" + strings.ToLower(RandomString(5)) + "/" + strings.ToLower(RandomString(11))
			}
			w, err := a.upsertFront(d, nil, "cdnws", "CDN WS", "ws", f.WSPath, cdn.FrontHosts, cdn.ID)
			if err == nil {
				for _, c := range d.Clients {
					if c.HasInbound(cdn.ID) && !c.HasInbound(w.ID) {
						c.Inbounds = append(c.Inbounds, w.ID)
					}
				}
				changed = true
			}
		}
		return nil
	})
	if changed {
		if a.Xray.Installed() {
			if err := a.Xray.Apply(); err != nil {
				log.Printf("autosetup migrate: %v", err)
			}
		}
		go a.SyncNodes()
	}
}

// upsertFront creates or updates a local inbound hidden behind the decoy site.
func (a *App) upsertFront(d *Data, in *Inbound, auto, remark, network, path string, hosts []string, afterID string) (*Inbound, error) {
	isNew := in == nil
	if isNew {
		in = &Inbound{ID: RandomHex(6), CreatedAt: nowMs(), Sniffing: true, Auto: auto, Remark: autoRemark(d, remark)}
		in.Port = randomLocalPort(d)
		if in.Port == 0 {
			return nil, fmt.Errorf("не нашлось свободного локального порта")
		}
	}
	in.Enabled = true
	in.Protocol, in.Network, in.Security = "vless", network, "none"
	in.Listen = "127.0.0.1"
	in.Path, in.Host = path, ""
	if network == "xhttp" {
		in.XHTTPMode = "auto"
	}
	in.Front, in.FrontHosts = "fake", append([]string{}, hosts...)
	in.ExternalHost, in.ExternalPort = "", 0
	if isNew {
		in.Tag = ""
	}
	if err := a.normalizeInbound(d, in, isNew); err != nil {
		return nil, err
	}
	in.Path = path
	if isNew {
		idx := 0
		for i, x := range d.Inbounds {
			if x.ID == afterID {
				idx = i + 1
			}
		}
		d.Inbounds = append(d.Inbounds[:idx], append([]*Inbound{in}, d.Inbounds[idx:]...)...)
	}
	return in, nil
}

func dropInbound(d *Data, id string) {
	for i, in := range d.Inbounds {
		if in.ID == id {
			d.Inbounds = append(d.Inbounds[:i], d.Inbounds[i+1:]...)
			break
		}
	}
	for _, c := range d.Clients {
		c.Inbounds = removeStr(c.Inbounds, id)
	}
}

// autoCerts makes sure every decoy domain has a Let's Encrypt certificate.
func (a *App) autoCerts(names []string, email string) ([]string, int) {
	notes := []string{}
	fails := 0
	for _, n := range names {
		var id string
		fresh := false
		a.Store.Update(func(d *Data) error {
			for _, dm := range d.Domains {
				if dm.Name == n && dm.Mode == "letsencrypt" {
					id = dm.ID
					if dm.Status == "active" && dm.ExpiresAt-nowMs() > 10*86400000 {
						fresh = true
					}
					if email != "" && dm.Email == "" {
						dm.Email = email
					}
					dm.AutoRenew = true
				}
			}
			if id == "" {
				dm := &Domain{ID: RandomHex(6), Name: n, Kind: "domain", Mode: "letsencrypt", Status: "pending", Email: email, AutoRenew: true, CreatedAt: nowMs()}
				d.Domains = append(d.Domains, dm)
				id = dm.ID
			}
			return nil
		})
		if fresh {
			notes = append(notes, n+": сертификат уже есть")
			continue
		}
		if err := a.issueDomain(id, false); err != nil {
			fails++
			notes = append(notes, n+": "+err.Error())
		} else {
			notes = append(notes, n+": выпущен")
		}
	}
	return notes, fails
}

// autoSelfCheck connects to the public Reality port like a browser would.
func (a *App) autoSelfCheck() ([]string, bool) {
	var st FakeSiteSettings
	var cdnPort, wsPort int
	a.Store.Read(func(d *Data) {
		st = d.Settings.Fake
		for _, in := range d.Inbounds {
			if in.Auto == "cdn" && in.Enabled {
				cdnPort = in.Port
			}
			if in.Auto == "cdnws" && in.Enabled {
				wsPort = in.Port
			}
		}
	})
	out := []string{}
	bad := false
	if !a.Xray.Status().Running {
		out = append(out, "Xray не запущен")
		return out, true
	}
	time.Sleep(700 * time.Millisecond)
	for _, name := range st.Domains {
		code, issuer, err := fakeProbe("127.0.0.1:"+strconv.Itoa(st.Port), name, "/")
		if err != nil {
			bad = true
			out = append(out, name+": "+err.Error())
			continue
		}
		if code != 200 {
			bad = true
			out = append(out, fmt.Sprintf("%s: сайт ответил %d", name, code))
			continue
		}
		out = append(out, name+": сайт открывается через порт "+strconv.Itoa(st.Port)+", сертификат — "+issuer)
	}
	if st.CDN && cdnPort > 0 {
		if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(cdnPort), 2*time.Second); err != nil {
			bad = true
			out = append(out, "XHTTP-инбаунд не слушает порт "+strconv.Itoa(cdnPort))
		} else {
			c.Close()
			out = append(out, "XHTTP за сайтом готов")
		}
	}
	if st.CDN && wsPort > 0 && len(st.CDNDomains) > 0 {
		// full path: Reality port → fallback to the site → WebSocket upgrade to Xray
		if code, _, err := fakeProbeWS("127.0.0.1:"+strconv.Itoa(st.Port), st.CDNDomains[0], st.WSPath); err != nil || code != 101 {
			bad = true
			if err == nil {
				err = fmt.Errorf("ответ %d вместо 101", code)
			}
			out = append(out, "WebSocket за сайтом не отвечает: "+err.Error())
		} else {
			out = append(out, "WebSocket за сайтом готов")
		}
	}
	return out, bad
}

// fakeProbeWS sends a WebSocket upgrade through the public port.
func fakeProbeWS(addr, sni, path string) (int, string, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: sni, InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}})
	if err != nil {
		return 0, "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
	key := base64.StdEncoding.EncodeToString([]byte(RandomString(16)))
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nUser-Agent: Mozilla/5.0\r\n\r\n", path, sni, key)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return 0, "", err
	}
	resp.Body.Close()
	return resp.StatusCode, "", nil
}

// fakeProbe performs an HTTPS request with the given SNI and returns the
// status code and the certificate issuer.
func fakeProbe(addr, sni, path string) (int, string, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: sni, InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}})
	if err != nil {
		return 0, "", fmt.Errorf("TLS не устанавливается: %v", err)
	}
	defer conn.Close()
	issuer := "самоподписанный"
	if cs := conn.ConnectionState(); len(cs.PeerCertificates) > 0 {
		c := cs.PeerCertificates[0]
		if c.Issuer.String() != c.Subject.String() {
			issuer = strings.Join(c.Issuer.Organization, " ")
			if issuer == "" {
				issuer = c.Issuer.CommonName
			}
		}
	}
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0\r\nConnection: close\r\n\r\n", path, sni)
	buf := make([]byte, 64)
	n, _ := io.ReadAtLeast(conn, buf, 12)
	line := string(buf[:n])
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0, issuer, fmt.Errorf("пустой ответ")
	}
	code, _ := strconv.Atoi(parts[1])
	return code, issuer, nil
}

func (a *App) apiAutoState(w http.ResponseWriter, r *http.Request) error {
	ok(w, autoState.snapshot())
	return nil
}

func (a *App) apiAutoCheckNow(w http.ResponseWriter, r *http.Request) error {
	checks, bad := a.autoSelfCheck()
	ok(w, map[string]any{"checks": checks, "ok": !bad})
	return nil
}

func (a *App) apiAutoSite(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		CustomHTML string `json:"customHtml"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(req.CustomHTML) > 2<<20 {
		return errf(400, "Страница больше 2 МБ")
	}
	if s := strings.TrimSpace(req.CustomHTML); s != "" && !strings.Contains(strings.ToLower(s), "<html") && !strings.Contains(strings.ToLower(s), "<!doctype") {
		return errf(400, "Это не похоже на HTML-страницу")
	}
	a.Store.Update(func(d *Data) error { d.Settings.Fake.CustomHTML = req.CustomHTML; return nil })
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiAutoPreview(w http.ResponseWriter, r *http.Request) error {
	var custom string
	a.Store.Read(func(d *Data) { custom = d.Settings.Fake.CustomHTML })
	if r.URL.Query().Get("default") == "1" {
		custom = ""
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	r2.Method = http.MethodGet
	serveFakePage(w, r2, custom)
	return nil
}

func (a *App) apiAutoToken(w http.ResponseWriter, r *http.Request) error {
	a.Store.Update(func(d *Data) error { d.Settings.Fake.CFToken = ""; return nil })
	ok(w, map[string]any{"ok": true})
	return nil
}

// apiAutoDelete removes the auto inbounds and stops the decoy site (certificates stay in «Домены»).
func (a *App) apiAutoDelete(w http.ResponseWriter, r *http.Request) error {
	if autoState.snapshot()["running"] == true {
		return errf(409, "Автонастройка ещё идёт")
	}
	a.Store.Update(func(d *Data) error {
		keep := []*Inbound{}
		removed := []string{}
		for _, in := range d.Inbounds {
			if in.Auto != "" {
				removed = append(removed, in.ID)
				continue
			}
			keep = append(keep, in)
		}
		d.Inbounds = keep
		for _, c := range d.Clients {
			for _, id := range removed {
				c.Inbounds = removeStr(c.Inbounds, id)
			}
		}
		d.Settings.Fake.Enabled = false
		d.AddEvent("warn", "Автонастройка VLESS удалена: инбаунды и фейк-сайт выключены", "")
		return nil
	})
	a.StopFakeSite()
	if a.Xray.Installed() {
		if err := a.Xray.Apply(); err != nil {
			return errf(400, "%v", err)
		}
	}
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}
