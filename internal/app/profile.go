package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// A server profile is everything that makes a linked server "that server":
// inbounds with their keys, domains with certificates, the decoy site,
// filtering and WARP. The main panel keeps the last known profile of every
// linked server, warns when the server drifts away from it, and can push it
// to another (fresh) server when the original one is banned or lost.
// Panel access (port, path, login, API token, HTTPS of the panel) is never
// part of a profile — the replacement server keeps its own.

type profileDomain struct {
	Domain
	CertPEM string `json:"certPem,omitempty"`
	KeyPEM  string `json:"keyPem,omitempty"`
}

type serverProfile struct {
	Name       string           `json:"name"`
	Version    string           `json:"version"`
	Xray       string           `json:"xray"`
	PublicHost string           `json:"publicHost"`
	DefaultFP  string           `json:"defaultFp"`
	Inbounds   []*Inbound       `json:"inbounds"`
	Domains    []profileDomain  `json:"domains"`
	Fake       FakeSiteSettings `json:"fake"`
	Filter     FilterSettings   `json:"filter"`
	Warp       WarpState        `json:"warp"`
	At         int64            `json:"at"`
	Hash       string           `json:"hash"`
}

// buildProfile snapshots the server. It only reads d (and certificate files),
// so it is safe inside Store closures.
func buildProfile(d *Data, withCerts bool) serverProfile {
	p := serverProfile{Name: d.Settings.NodeName, Version: Version, PublicHost: d.Settings.PublicHost, DefaultFP: d.Settings.DefaultFP,
		Inbounds: []*Inbound{}, Domains: []profileDomain{}, Fake: d.Settings.Fake, Filter: d.Filter, Warp: d.Warp, At: nowMs()}
	for _, in := range d.Inbounds {
		var cp Inbound
		b, _ := json.Marshal(in)
		_ = json.Unmarshal(b, &cp)
		cp.Up, cp.Down = 0, 0
		p.Inbounds = append(p.Inbounds, &cp)
	}
	for _, dm := range d.Domains {
		if dm.ID == d.Settings.PanelCertID && !domainUsed(d, dm) {
			continue // only the panel's own HTTPS — stays with the server
		}
		pd := profileDomain{Domain: *dm}
		if withCerts && dm.Status == "active" && dm.CertFile != "" {
			c, err1 := os.ReadFile(dm.CertFile)
			k, err2 := os.ReadFile(dm.KeyFile)
			if err1 == nil && err2 == nil {
				pd.CertPEM, pd.KeyPEM = string(c), string(k)
			}
		}
		p.Domains = append(p.Domains, pd)
	}
	p.Fake.SetupAt = d.Settings.Fake.SetupAt
	p.Hash = profileHash(&p)
	return p
}

// domainUsed: the domain is used by something other than the panel itself.
func domainUsed(d *Data, dm *Domain) bool {
	if d.Settings.Fake.Enabled && containsStr(d.Settings.Fake.Domains, dm.Name) {
		return true
	}
	for _, in := range d.Inbounds {
		if in.CertID == dm.ID {
			return true
		}
	}
	return false
}

// profileHash covers configuration only: traffic counters, certificate state
// (renewals) and the producing versions do not count as a change.
func profileHash(p *serverProfile) string {
	type hd struct {
		ID, Name, Kind, Mode, Email string
		AutoRenew                   bool
	}
	h := struct {
		Name, PublicHost, DefaultFP string
		Inbounds                    []*Inbound
		Domains                     []hd
		Fake                        FakeSiteSettings
		Filter                      FilterSettings
		WarpOn                      bool
		WarpMode                    string
		WarpSites                   []string
		WarpID                      string
		WarpPort                    int
		WarpV6, WarpFallback        bool
	}{Name: p.Name, PublicHost: p.PublicHost, DefaultFP: p.DefaultFP, Inbounds: p.Inbounds, Fake: p.Fake, Filter: p.Filter,
		WarpOn: p.Warp.Enabled, WarpMode: p.Warp.Mode, WarpSites: p.Warp.Sites, WarpID: p.Warp.ID, WarpPort: p.Warp.Port,
		WarpV6: p.Warp.IPv6First, WarpFallback: p.Warp.FallbackOn}
	for _, dm := range p.Domains {
		h.Domains = append(h.Domains, hd{dm.ID, dm.Name, dm.Kind, dm.Mode, dm.Email, dm.AutoRenew})
	}
	b, _ := json.Marshal(h)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:12])
}

// ---- node side ----

func (a *App) apiNodeProfileGet(w http.ResponseWriter, r *http.Request) error {
	var p serverProfile
	a.Store.Read(func(d *Data) { p = buildProfile(d, true) })
	p.Xray = a.Xray.Version()
	ok(w, p)
	return nil
}

func (a *App) apiNodeProfileApply(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Profile *serverProfile `json:"profile"`
		DNS     bool           `json:"dns"`
		From    string         `json:"from"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Profile == nil {
		return errf(400, "Нет конфигурации")
	}
	warns, err := a.applyProfile(req.Profile, req.DNS, req.From)
	if err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true, "warnings": warns})
	return nil
}

// applyProfile replaces this server's inbounds, domains, decoy site, filter
// and WARP with the profile. IP "domains" (certificates for the server IP)
// are moved to this server's IP and re-issued.
func (a *App) applyProfile(p *serverProfile, dns bool, from string) ([]string, error) {
	ip := a.autoServerIP()
	warns := []string{}
	type certJob struct {
		id, name, kind, mode string
		cert, key            []byte
	}
	var jobs []certJob
	var oldIP string
	for _, dm := range p.Domains {
		if dm.Kind == "ip" && net.ParseIP(p.PublicHost) != nil && dm.Name == p.PublicHost {
			oldIP = dm.Name
		}
	}
	var panelPort int
	err := a.Store.Update(func(d *Data) error {
		panelPort = d.Settings.PanelPort
		ins := []*Inbound{}
		for _, in := range p.Inbounds {
			if in.Port == d.Settings.PanelPort && in.Listen != "127.0.0.1" && !strings.HasPrefix(in.Listen, "127.") {
				warns = append(warns, fmt.Sprintf("Инбаунд «%s» пропущен: порт %d занят панелью этого сервера", in.Remark, in.Port))
				continue
			}
			cp := *in
			cp.Up, cp.Down = 0, 0
			if oldIP != "" && ip != "" {
				if cp.ExternalHost == oldIP {
					cp.ExternalHost = ""
				}
				if cp.TLSServerName == oldIP {
					cp.TLSServerName = ip
				}
			}
			ins = append(ins, &cp)
		}
		d.Inbounds = ins
		doms := []*Domain{}
		have := map[string]bool{}
		for _, pd := range p.Domains {
			dm := pd.Domain
			dm.CertFile, dm.KeyFile, dm.Error, dm.Status, dm.IssuedAt, dm.ExpiresAt, dm.LastTry = "", "", "", "pending", 0, 0, 0
			job := certJob{id: dm.ID, name: dm.Name, kind: dm.Kind, mode: dm.Mode}
			if dm.Kind == "ip" {
				if ip != "" && net.ParseIP(dm.Name) != nil && dm.Name != ip {
					dm.Name = ip // a certificate for the old server IP is useless here
					job.name = ip
				}
			} else if pd.CertPEM != "" && pd.KeyPEM != "" {
				job.cert, job.key = []byte(pd.CertPEM), []byte(pd.KeyPEM)
			}
			cp := dm
			doms = append(doms, &cp)
			have[dm.ID] = true
			jobs = append(jobs, job)
		}
		// the panel's own HTTPS certificate stays
		for _, dm := range d.Domains {
			if dm.ID == d.Settings.PanelCertID && !have[dm.ID] {
				doms = append(doms, dm)
			}
		}
		d.Domains = doms
		fs := p.Fake
		if fs.Enabled && (fs.TLSPort == 0 || fs.TLSPort == d.Settings.PanelPort) {
			fs.TLSPort = randomLocalPort(d)
		}
		d.Settings.Fake = fs
		d.Filter = p.Filter
		d.Warp = p.Warp
		if strings.TrimSpace(p.Name) != "" {
			d.Settings.NodeName = p.Name
		}
		if p.DefaultFP != "" {
			d.Settings.DefaultFP = p.DefaultFP
		}
		if p.PublicHost != "" && net.ParseIP(p.PublicHost) == nil {
			d.Settings.PublicHost = p.PublicHost
		}
		ids := map[string]bool{}
		for _, in := range d.Inbounds {
			ids[in.ID] = true
		}
		for _, c := range d.Clients {
			kept := []string{}
			for _, id := range c.Inbounds {
				if ids[id] {
					kept = append(kept, id)
				}
			}
			c.Inbounds = kept
		}
		d.AddEvent("warn", "Конфигурация сервера восстановлена"+map[bool]string{true: " с основного «" + from + "»", false: ""}[from != ""]+
			fmt.Sprintf(": %d инбаундов, %d доменов", len(d.Inbounds), len(doms)), "")
		return nil
	})
	if err != nil {
		return nil, err
	}
	// DNS: point the domains at this server through the Cloudflare API
	if dns && p.Fake.CFToken != "" && ip != "" && net.ParseIP(ip).To4() != nil {
		zones := map[string]string{}
		for _, j := range jobs {
			if j.kind != "domain" || isFreeDomain(j.name) {
				continue
			}
			zid, ok := "", false
			for z, id := range zones {
				if j.name == z || strings.HasSuffix(j.name, "."+z) {
					zid, ok = id, true
				}
			}
			if !ok {
				id, zone, err := cfFindZone(p.Fake.CFToken, j.name)
				if err != nil {
					warns = append(warns, "DNS "+j.name+": "+err.Error())
					continue
				}
				zones[zone], zid = id, id
			}
			proxied := containsStr(p.Fake.CDNDomains, j.name)
			if _, err := cfUpsertA(p.Fake.CFToken, zid, j.name, ip, proxied); err != nil {
				warns = append(warns, "DNS "+j.name+": "+err.Error())
			}
		}
	} else if dns {
		names := []string{}
		for _, j := range jobs {
			if j.kind == "domain" && !isFreeDomain(j.name) {
				names = append(names, j.name)
			}
		}
		if len(names) > 0 && ip != "" {
			warns = append(warns, "Перенаправьте A-записи на "+ip+": "+strings.Join(names, ", ")+" — токена Cloudflare в конфигурации нет")
		}
	}
	// free domains: freemyip / myaddr names follow the server when asked to;
	// sslip.io / nip.io names contain the old IP and can't
	for _, pd := range p.Domains {
		switch {
		case pd.DynToken != "" && dns && ip != "":
			if err := dynUpdate(pd.Provider, pd.Name, pd.DynToken, ip); err != nil {
				warns = append(warns, "DNS "+pd.Name+": "+err.Error())
			} else {
				a.setDynState(pd.ID, ip, "")
			}
		case pd.DynToken != "" && pd.DynIP != ip:
			warns = append(warns, pd.Name+" указывает на старый сервер — нажмите «Направить сюда» в «Доменах» или восстановите с галочкой DNS")
		case (pd.Provider == "sslip" || pd.Provider == "nip") && ip != "":
			if d4, h4 := ipv4Forms(ip); !strings.Contains(pd.Name, d4) && !strings.Contains(pd.Name, h4) {
				warns = append(warns, pd.Name+" привязан к IP старого сервера — создайте новый бесплатный домен в «Доменах»")
			}
		}
	}
	// certificates: saved ones first, the rest is issued again
	for _, j := range jobs {
		if j.cert != nil {
			if err := a.saveCert(j.id, j.cert, j.key); err == nil {
				continue
			}
		}
		switch j.mode {
		case "selfsigned":
			if c, k, err := selfSigned(j.name, j.kind); err == nil {
				_ = a.saveCert(j.id, c, k)
			}
		case "letsencrypt":
			id := j.id
			go func() {
				time.Sleep(5 * time.Second) // give fresh DNS records a moment
				_ = a.issueDomain(id, false)
			}()
		default:
			warns = append(warns, "Свой сертификат "+j.name+" не сохранён в конфигурации — загрузите его в «Доменах»")
		}
	}
	a.StopFakeSite()
	if err := a.StartFakeSite(); err != nil {
		warns = append(warns, "Сайт-прикрытие: "+err.Error())
	}
	var ports []struct {
		port  int
		proto string
	}
	a.Store.Read(func(d *Data) {
		for _, in := range d.Inbounds {
			if in.Enabled && in.Listen != "127.0.0.1" {
				ports = append(ports, struct {
					port  int
					proto string
				}{in.Port, inboundFirewallProto(in)})
			}
		}
	})
	go func() {
		for _, pp := range ports {
			if pp.port != panelPort {
				OpenFirewallPort(pp.port, pp.proto)
			}
		}
		if p.Fake.Enabled {
			OpenFirewallPort(80, "tcp")
		}
	}()
	if want := strings.TrimPrefix(p.Xray, "v"); want != "" && a.Xray.Installed() && strings.TrimPrefix(a.Xray.Version(), "v") != want {
		go func() {
			if err := a.InstallXray("v" + want); err != nil {
				a.Store.Update(func(d *Data) error {
					d.AddEvent("warn", "Xray "+want+" из конфигурации не установился: "+err.Error(), "")
					return nil
				})
			}
		}()
	}
	a.ScheduleApply()
	return warns, nil
}

// ---- main side ----

func (a *App) fetchNodeProfile(id string) error {
	var u, tok string
	a.Store.Read(func(d *Data) {
		if n := d.NodeByID(id); n != nil {
			u, tok = n.URL, n.Token
		}
	})
	if u == "" {
		return errf(404, "Сервер не найден")
	}
	var p serverProfile
	start := nowMs()
	if err := nodeCall("GET", u, tok, "api/node/profile", nil, &p); err != nil {
		if strings.Contains(err.Error(), "нет панели") {
			return fmt.Errorf("сервер не поддерживает сохранение конфигурации — обновите на нём панель (kvk update)")
		}
		return err
	}
	if p.Hash == "" {
		return fmt.Errorf("неожиданный ответ сервера")
	}
	return a.Store.Update(func(d *Data) error {
		n := d.NodeByID(id)
		if n == nil || n.URL != u {
			return nil
		}
		if n.Drift || n.Replaced {
			d.AddEvent("info", "Конфигурация сервера «"+n.Name+"» сохранена заново", "")
		}
		n.Profile, n.ProfileHash, n.ProfileAt = &p, p.Hash, nowMs()
		n.Drift, n.Replaced, n.RemoteHash = false, false, p.Hash
		if n.EditedAt <= start {
			// own edits are in the copy now: any later difference is a change made elsewhere
			n.EditedAt = 0
		}
		return nil
	})
}

var profileRefresh sync.Map // node id -> pending

// nodeEdited is called after the main panel changed something on a linked
// server: the server list is re-synced and the saved profile refreshed.
func (a *App) nodeEdited(id string) {
	a.Store.Update(func(d *Data) error {
		if n := d.NodeByID(id); n != nil {
			n.EditedAt = nowMs()
		}
		return nil
	})
	if _, busy := profileRefresh.LoadOrStore(id, true); busy {
		return
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		profileRefresh.Delete(id)
		syncMu.Lock()
		a.syncNodesNow()
		syncMu.Unlock()
		skip := true
		a.Store.Read(func(d *Data) {
			if n := d.NodeByID(id); n != nil {
				skip = n.Drift || n.Replaced
			}
		})
		if !skip {
			_ = a.fetchNodeProfile(id)
		}
	}()
}

// apiNodeProxy forwards a panel API call to a linked server, so its domains,
// inbounds, site, filter and WARP are managed from here.
func (a *App) apiNodeProxy(w http.ResponseWriter, r *http.Request) error {
	id, p := r.PathValue("id"), strings.TrimLeft(r.PathValue("path"), "/")
	for _, deny := range []string{"node/", "main/", "auth/", "nodes", "settings", "backups", "panel/", "security", "shop", "tgproxy", "migrate", "wizard", "inbounds-copy"} {
		if strings.HasPrefix(p, deny) {
			return errf(403, "Этот раздел меняется только на самом сервере")
		}
	}
	var u, tok, name string
	a.Store.Read(func(d *Data) {
		if n := d.NodeByID(id); n != nil {
			u, tok, name = n.URL, n.Token, n.Name
		}
	})
	if u == "" {
		return errf(404, "Сервер не найден")
	}
	q := r.URL.Query()
	grantAll := false
	if r.Method == "POST" && p == "inbounds" && q.Get("grant") == "1" {
		grantAll = true // clients live here: grant on the main panel, not on the node
		q.Del("grant")
	}
	target := u + "api/" + (&url.URL{Path: p}).EscapedPath()
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	var body io.Reader
	if r.Body != nil && r.Method != "GET" {
		body = io.LimitReader(r.Body, 8<<20)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := nodeHTTP.Do(req)
	if err != nil {
		return errf(502, "Сервер «%s» не отвечает: %s", name, shortNetErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return errf(502, "Сервер «%s»: неверный API-токен — обновите его в «Связке серверов»", name)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode == 404 && !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		return errf(502, "Сервер «%s»: по адресу нет панели KVK", name)
	}
	for _, h := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(b)
	if r.Method != "GET" && resp.StatusCode < 300 {
		// HTTPS of the server's panel changed — follow its new address
		if p == "domains-panel-off" || (strings.HasPrefix(p, "domains/") && strings.HasSuffix(p, "/panel")) {
			var res struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(b, &res) == nil && strings.HasPrefix(res.URL, "http") {
				a.Store.Update(func(d *Data) error {
					if n := d.NodeByID(id); n != nil {
						n.URL = res.URL
					}
					return nil
				})
			}
		}
		a.nodeEdited(id)
		if grantAll {
			var created struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(b, &created) == nil && created.ID != "" {
				go func() {
					time.Sleep(2 * time.Second)
					_ = a.grantNodeInbound(id, created.ID, true)
				}()
			}
		}
	}
	return nil
}

// grantNodeInbound gives (or takes) a linked server's inbound to every client.
func (a *App) grantNodeInbound(nid, inID string, all bool) error {
	gid := nid + ":" + strings.TrimPrefix(inID, nid+":")
	known := func() bool {
		k := false
		a.Store.Read(func(d *Data) {
			if n := d.NodeByID(nid); n != nil {
				k = containsStr(n.InboundIDs(), gid)
			}
		})
		return k
	}
	if all && !known() {
		syncMu.Lock()
		a.syncNodesNow()
		syncMu.Unlock()
		if !known() {
			return errf(404, "Инбаунд ещё не появился на основном сервере — синхронизируйте связку")
		}
	}
	n := 0
	a.Store.Update(func(d *Data) error {
		for _, c := range d.Clients {
			if c.Origin == "main" {
				continue
			}
			if all && !c.HasInbound(gid) {
				c.Inbounds = append(c.Inbounds, gid)
				n++
			} else if !all && c.HasInbound(gid) {
				c.Inbounds = removeStr(c.Inbounds, gid)
				n++
			}
		}
		return nil
	})
	a.ScheduleApply()
	go a.SyncNodes()
	return nil
}

func (a *App) apiNodeGrant(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Inbound string `json:"inbound"`
		All     bool   `json:"all"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Inbound == "" {
		return errf(400, "Не указан инбаунд")
	}
	if err := a.grantNodeInbound(r.PathValue("id"), req.Inbound, req.All); err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func profileSummary(p *serverProfile) map[string]any {
	if p == nil {
		return nil
	}
	ins := []map[string]any{}
	for _, in := range p.Inbounds {
		ins = append(ins, map[string]any{"id": in.ID, "remark": in.Remark, "protocol": in.Protocol, "port": in.Port, "network": in.Network,
			"security": in.Security, "enabled": in.Enabled})
	}
	doms := []map[string]any{}
	for _, dm := range p.Domains {
		doms = append(doms, map[string]any{"name": dm.Name, "kind": dm.Kind, "mode": dm.Mode, "cert": dm.CertPEM != "",
			"site": p.Fake.Enabled && containsStr(p.Fake.Domains, dm.Name)})
	}
	return map[string]any{"name": p.Name, "version": p.Version, "xray": p.Xray, "at": p.At, "hash": p.Hash, "inbounds": ins, "domains": doms,
		"site": map[string]any{"enabled": p.Fake.Enabled, "domains": p.Fake.Domains, "sni": p.Fake.SNI, "port": p.Fake.Port, "cdn": p.Fake.CDNDomains,
			"cfToken": p.Fake.CFToken != "", "customHtml": p.Fake.CustomHTML != ""},
		"filter": p.Filter.Enabled, "warp": p.Warp.Enabled && p.Warp.PrivateKey != ""}
}

func (a *App) apiNodeProfile(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var p *serverProfile
	var name string
	var res map[string]any
	a.Store.Read(func(d *Data) {
		if n := d.NodeByID(id); n != nil {
			p, name = n.Profile, n.Name
			res = map[string]any{"drift": n.Drift, "replaced": n.Replaced, "savedHash": n.ProfileHash, "remoteHash": n.RemoteHash, "editedAt": n.EditedAt}
		}
	})
	if res == nil {
		return errf(404, "Сервер не найден")
	}
	if r.URL.Query().Get("download") == "1" {
		if p == nil {
			return errf(404, "Конфигурация ещё не сохранена")
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="kvk-server-`+asciiOnly(name)+`.json"`)
		_, _ = w.Write(b)
		return nil
	}
	res["profile"] = profileSummary(p)
	ok(w, res)
	return nil
}

// apiNodeAccept stores the server's current configuration as the saved one.
func (a *App) apiNodeAccept(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := a.fetchNodeProfile(id); err != nil {
		return errf(502, "%v", err)
	}
	a.Store.Update(func(d *Data) error {
		if n := d.NodeByID(id); n != nil {
			d.dropNodeInbounds(n.ID, n.InboundIDs())
		}
		return nil
	})
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

// apiNodeRestore pushes the saved configuration to the server (usually a fresh
// one that replaced a banned server: change its key first, then restore).
func (a *App) apiNodeRestore(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DNS bool `json:"dns"`
	}
	_ = readJSON(r, &req)
	id := r.PathValue("id")
	var u, tok, name, from string
	var p *serverProfile
	a.Store.Read(func(d *Data) {
		if n := d.NodeByID(id); n != nil {
			u, tok, name, p = n.URL, n.Token, n.Name, n.Profile
		}
		from = d.Settings.NodeName
	})
	if u == "" {
		return errf(404, "Сервер не найден")
	}
	if p == nil {
		return errf(409, "Для «%s» ещё нет сохранённой конфигурации", name)
	}
	var res struct {
		Warnings []string `json:"warnings"`
	}
	if err := nodeCall("POST", u, tok, "api/node/profile", map[string]any{"profile": p, "dns": req.DNS, "from": from}, &res); err != nil {
		if strings.Contains(err.Error(), "нет панели") {
			return errf(502, "Сервер не поддерживает восстановление — обновите на нём панель (kvk update)")
		}
		return errf(502, "%v", err)
	}
	a.Store.Update(func(d *Data) error {
		if n := d.NodeByID(id); n != nil {
			d.AddEvent("ok", "Конфигурация восстановлена на сервере «"+n.Name+"»", "")
			n.EditedAt = nowMs()
		}
		return nil
	})
	syncMu.Lock()
	a.syncNodesNow()
	syncMu.Unlock()
	if err := a.fetchNodeProfile(id); err != nil {
		res.Warnings = append(res.Warnings, err.Error())
	}
	if res.Warnings == nil {
		res.Warnings = []string{}
	}
	ok(w, map[string]any{"ok": true, "warnings": res.Warnings})
	return nil
}
