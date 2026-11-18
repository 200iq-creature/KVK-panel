package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

// «Автонастройка» — the step-by-step setup of the whole link group:
//   - every server gets its own site on its own domain (the domain points at
//     that server's IP) and Reality on it, so each server's SNI is its own site;
//   - every server gets inbounds of every protocol, named «<server> <protocol>»;
//   - the client «main» gets all of them (the link group carries it to the servers);
//   - the subscription gets its own domain if wanted;
//   - the panel moves to its domain last, when the admin presses the button —
//     the new address is shown in advance, the login and password stay the same.

type wizDom struct {
	Mode  string `json:"mode"` // free | exist | own | keep (panel) | panel (sub: same as panel)
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"` // free: word before the IP — site-1-2-3-4.sslip.io
}

type wizSite struct {
	Server string `json:"server"` // "" — this server, else a node id
	Domain wizDom `json:"domain"`
}

type wizReq struct {
	Panel wizDom    `json:"panel"`
	Sub   wizDom    `json:"sub"`
	Sites []wizSite `json:"sites"`
	Email string    `json:"email"`
}

var wizState = &autoRunState{Steps: []*autoStep{}, Warnings: []string{}}

// ---------- servers of the link group ----------

type wizServer struct {
	ID, Name, URL, Token, Version string
	Main                          bool
}

func (a *App) wizServers() []wizServer {
	out := []wizServer{}
	a.Store.Read(func(d *Data) {
		out = append(out, wizServer{Name: orDefault(d.Settings.NodeName, "Основной"), Version: Version, Main: true})
		for _, n := range d.Nodes {
			if n.Enabled {
				out = append(out, wizServer{ID: n.ID, Name: n.Name, URL: n.URL, Token: n.Token, Version: n.Version})
			}
		}
	})
	return out
}

func (a *App) wizServer(id string) (wizServer, bool) {
	for _, s := range a.wizServers() {
		if s.ID == id {
			return s, true
		}
	}
	return wizServer{}, false
}

// verAtLeast: "1.0.7" >= "1.0.6".
func verAtLeast(v, min string) bool {
	p := func(s string) []int {
		out := []int{}
		for _, x := range strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".") {
			n, _ := strconv.Atoi(strings.TrimFunc(x, func(r rune) bool { return r < '0' || r > '9' }))
			out = append(out, n)
		}
		for len(out) < 3 {
			out = append(out, 0)
		}
		return out
	}
	a, b := p(v), p(min)
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

// wizNodeVersion asks the server itself (the stored version is from the last sync).
func (a *App) wizNodeVersion(s wizServer) string {
	var ni nodeInfo
	if err := nodeCall("GET", s.URL, s.Token, "api/node/info", nil, &ni); err != nil {
		return ""
	}
	return ni.Version
}

type localCallKey struct{}

func isLocalCall(r *http.Request) bool { return r.Context().Value(localCallKey{}) != nil }

var (
	localMuxOnce sync.Once
	localMux     *http.ServeMux
)

// localCall runs a panel API call in-process — the same requests the wizard
// sends to linked servers, so every server is set up the same way.
func (a *App) localCall(method, path string, body, out any) error {
	localMuxOnce.Do(func() { localMux = a.apiMux() })
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, "/api/"+path, rd)
	r = r.WithContext(context.WithValue(r.Context(), localCallKey{}, true))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	localMux.ServeHTTP(w, r)
	if w.Code != 200 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &e)
		return fmt.Errorf("%s", orDefault(e.Error, "ошибка "+strconv.Itoa(w.Code)))
	}
	if out != nil {
		return json.Unmarshal(w.Body.Bytes(), out)
	}
	return nil
}

func (a *App) srvCall(s wizServer, method, path string, body, out any) error {
	if s.Main {
		return a.localCall(method, path, body, out)
	}
	return nodeCall(method, s.URL, s.Token, "api/"+path, body, out)
}

type wizDomain struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Mode     string `json:"mode"`
	Status   string `json:"status"`
	Error    string `json:"error"`
	Provider string `json:"provider"`
	Issuing  bool   `json:"issuing"`
	Panel    bool   `json:"panel"`
	Site     bool   `json:"site"`
}

type wizDomains struct {
	Domains  []wizDomain `json:"domains"`
	ServerIP string      `json:"serverIp"`
}

func (a *App) wizListDomains(s wizServer) (wizDomains, error) {
	var res wizDomains
	err := a.srvCall(s, "GET", "domains", nil, &res)
	return res, err
}

// ---------- GET /api/wizard ----------

func (a *App) apiWizGet(w http.ResponseWriter, r *http.Request) error {
	type srvView struct {
		ID       string      `json:"id"`
		Name     string      `json:"name"`
		Main     bool        `json:"main"`
		Version  string      `json:"version"`
		Ready    bool        `json:"ready"` // panel version knows the wizard's requests
		IP       string      `json:"ip"`
		Error    string      `json:"error,omitempty"`
		Domains  []wizDomain `json:"domains"`
		Site     []string    `json:"site"`
		SitePort int         `json:"sitePort"`
	}
	servers := a.wizServers()
	views := make([]srvView, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		views[i] = srvView{ID: s.ID, Name: s.Name, Main: s.Main, Version: s.Version, Ready: s.Main || verAtLeast(s.Version, "1.0.7"), Domains: []wizDomain{}, Site: []string{}}
		wg.Add(1)
		go func(i int, s wizServer) {
			defer wg.Done()
			if !s.Main {
				if v := a.wizNodeVersion(s); v != "" {
					views[i].Version, views[i].Ready = v, verAtLeast(v, "1.0.7")
				}
			}
			ds, err := a.wizListDomains(s)
			if err != nil {
				views[i].Error = err.Error()
				return
			}
			views[i].IP = ds.ServerIP
			if ds.Domains != nil {
				views[i].Domains = ds.Domains
			}
			var auto struct {
				Settings FakeSiteSettings `json:"settings"`
			}
			if a.srvCall(s, "GET", "autosetup", nil, &auto) == nil && auto.Settings.Enabled {
				views[i].Site, views[i].SitePort = auto.Settings.Domains, auto.Settings.Port
			}
		}(i, s)
	}
	wg.Wait()
	var res map[string]any
	a.Store.Read(func(d *Data) {
		st := d.Settings
		panel := map[string]any{"port": st.PanelPort, "basePath": st.BasePath, "url": a.panelURLLocked(d), "onDomain": false}
		if dm := d.DomainByID(st.PanelCertID); dm != nil && dm.Status == "active" && dm.Kind == "domain" {
			panel["onDomain"], panel["domain"] = true, dm.Name
		}
		sub := map[string]any{"port": orInt(st.SubPort, st.PanelPort), "path": st.SubPath, "certId": st.SubCertID, "domain": subHostLocked(d), "url": a.subBaseURLLocked(d)}
		var mainClient map[string]any
		if c := d.ClientByName("main"); c != nil {
			mainClient = map[string]any{"name": c.Name, "inbounds": len(c.Inbounds)}
		}
		res = map[string]any{"servers": views, "panel": panel, "sub": sub, "email": st.Fake.Email, "main": mainClient,
			"isNode": d.Main != nil, "state": wizState.snapshot()}
	})
	ok(w, res)
	return nil
}

func orInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func (a *App) apiWizState(w http.ResponseWriter, r *http.Request) error {
	ok(w, wizState.snapshot())
	return nil
}

// ---------- POST /api/wizard/run ----------

func (a *App) apiWizRun(w http.ResponseWriter, r *http.Request) error {
	var req wizReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	req.Email = strings.TrimSpace(req.Email)
	clean := func(dm *wizDom, what string) error {
		dm.Name = normDomain(dm.Name)
		dm.Label = cleanLabel(dm.Label, 30)
		switch dm.Mode {
		case "keep", "panel", "":
		case "exist":
			if dm.ID == "" {
				return errf(400, "%s: выберите домен из списка", what)
			}
		case "own":
			if !domainRe.MatchString(dm.Name) {
				return errf(400, "%s: введите домен, например vpn.example.com", what)
			}
		case "free":
		default:
			return errf(400, "%s: неизвестный способ «%s»", what, dm.Mode)
		}
		return nil
	}
	if err := clean(&req.Panel, "Домен панели"); err != nil {
		return err
	}
	if req.Panel.Mode == "" {
		req.Panel.Mode = "keep"
	}
	if err := clean(&req.Sub, "Домен подписки"); err != nil {
		return err
	}
	if req.Sub.Mode == "" {
		req.Sub.Mode = "panel"
	}
	seen := map[string]bool{}
	sites := []wizSite{}
	for _, s := range req.Sites {
		if seen[s.Server] {
			continue
		}
		srv, found := a.wizServer(s.Server)
		if !found {
			return errf(404, "Сервер не найден — обновите страницу")
		}
		if s.Domain.Mode == "keep" || s.Domain.Mode == "panel" {
			return errf(400, "Сайт сервера «%s»: выберите домен", srv.Name)
		}
		if err := clean(&s.Domain, "Сайт сервера «"+srv.Name+"»"); err != nil {
			return err
		}
		if s.Domain.Mode == "" {
			s.Domain.Mode = "free"
		}
		seen[s.Server] = true
		sites = append(sites, s)
	}
	if len(sites) == 0 {
		return errf(400, "Выберите домен сайта хотя бы для одного сервера")
	}
	req.Sites = sites
	servers := map[string]wizServer{}
	for _, s := range a.wizServers() {
		servers[s.ID] = s
	}

	wizState.mu.Lock()
	if wizState.Running {
		wizState.mu.Unlock()
		return errf(409, "Автонастройка уже идёт")
	}
	wizState.Running, wizState.Error, wizState.Result, wizState.Finished = true, "", nil, 0
	wizState.Started = nowMs()
	wizState.Warnings = []string{}
	wizState.Steps = []*autoStep{}
	wizState.byKey = map[string]*autoStep{}
	add := func(k, t string) {
		st := &autoStep{Key: k, Title: t, Status: "wait"}
		wizState.Steps = append(wizState.Steps, st)
		wizState.byKey[k] = st
	}
	add("domains", "Домены")
	for _, s := range sites {
		add("site:"+s.Server, "Сайт — "+servers[s.Server].Name)
		add("inb:"+s.Server, "Инбаунды — "+servers[s.Server].Name)
	}
	add("client", "Клиент main на всех серверах")
	add("sub", "Домен подписки")
	add("panel", "Домен панели")
	wizState.mu.Unlock()
	go a.runWizard(req, servers)
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) runWizard(req wizReq, servers map[string]wizServer) {
	st := wizState
	finish := func(errMsg string) {
		st.mu.Lock()
		st.Running, st.Finished = false, nowMs()
		if errMsg != "" {
			st.Error = errMsg
		}
		st.mu.Unlock()
	}
	defer func() {
		if rec := recover(); rec != nil {
			finish(fmt.Sprintf("внутренняя ошибка: %v", rec))
		}
	}()
	main := servers[""]

	// 1. domains: create what is new, everywhere — certificates start issuing right away
	st.set("domains", "run", "")
	notes := []string{}
	type siteDom struct {
		srv  wizServer
		id   string
		name string
	}
	siteDoms := []siteDom{}
	var panelID, panelName, subID, subName string
	var panelOnDomain bool
	var panelCur string
	a.Store.Read(func(d *Data) {
		if dm := d.DomainByID(d.Settings.PanelCertID); dm != nil && dm.Status == "active" && dm.Kind == "domain" {
			panelOnDomain, panelCur = true, dm.Name
		}
	})
	if req.Panel.Mode != "keep" && !panelOnDomain {
		id, name, err := a.wizDomain(main, req.Panel, "panel", req.Email)
		if err != nil {
			st.set("domains", "bad", "панель: "+err.Error())
			finish("Домен панели: " + err.Error())
			return
		}
		panelID, panelName = id, name
		notes = append(notes, "панель — "+name)
	}
	if req.Sub.Mode != "panel" {
		id, name, err := a.wizDomain(main, req.Sub, "sub", req.Email)
		if err != nil {
			st.warn("Домен подписки не создан: " + err.Error() + " — подписка останется на адресе панели")
		} else {
			subID, subName = id, name
			notes = append(notes, "подписка — "+name)
		}
	}
	for _, s := range req.Sites {
		srv := servers[s.Server]
		id, name, err := a.wizDomain(srv, s.Domain, "site", req.Email)
		if err != nil {
			st.warn(srv.Name + ": домен сайта не создан — " + err.Error())
			st.set("site:"+s.Server, "skip", "нет домена")
			st.set("inb:"+s.Server, "skip", "нет домена")
			continue
		}
		siteDoms = append(siteDoms, siteDom{srv, id, name})
		notes = append(notes, srv.Name+": сайт — "+name)
	}
	if len(siteDoms) == 0 {
		st.set("domains", "bad", strings.Join(notes, "; "))
		finish("Не удалось создать ни одного домена для сайта")
		return
	}
	st.set("domains", "ok", strings.Join(notes, "; "))

	// 2-3. per server, in parallel: site + Reality, then the other protocols
	var wg sync.WaitGroup
	for _, sd := range siteDoms {
		wg.Add(1)
		go func(sd siteDom) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					st.set("site:"+sd.srv.ID, "bad", fmt.Sprintf("внутренняя ошибка: %v", rec))
				}
			}()
			a.wizServerSetup(sd.srv, sd.id, sd.name, req.Email)
		}(sd)
	}
	wg.Wait()

	// 4. client «main» with everything, on every server
	st.set("client", "run", "")
	syncMu.Lock()
	a.syncNodesNow()
	syncMu.Unlock()
	var clientName, subToken string
	var granted int
	err := a.Store.Update(func(d *Data) error {
		c := d.ClientByName("main")
		if c == nil {
			c = newClient(d)
			c.Name = "main"
			d.Clients = append(d.Clients, c)
			d.AddEvent("ok", "Клиент main создан автонастройкой (бессрочно)", "main")
		}
		ids := []string{}
		for _, in := range d.Inbounds {
			if in.Enabled {
				ids = append(ids, in.ID)
			}
		}
		for _, n := range d.Nodes {
			if n.Enabled {
				ids = append(ids, n.InboundIDs()...)
			}
		}
		for _, id := range ids {
			if !c.HasInbound(id) {
				c.Inbounds = append(c.Inbounds, id)
			}
		}
		clientName, subToken, granted = c.Name, c.SubID, len(c.Inbounds)
		return nil
	})
	if err != nil {
		st.set("client", "bad", err.Error())
	} else {
		a.ScheduleApply()
		syncMu.Lock()
		a.syncNodesNow()
		syncMu.Unlock()
		live := 1
		a.Store.Read(func(d *Data) {
			for _, n := range d.Nodes {
				if n.Enabled && n.LastError == "" {
					live++
				}
			}
		})
		where := "на этом сервере"
		if live > 1 {
			where = "на " + strconv.Itoa(live) + " " + plural(live, "сервере", "серверах", "серверах")
		}
		st.set("client", "ok", "«main»: "+strconv.Itoa(granted)+" "+plural(granted, "подключение", "подключения", "подключений")+" "+where)
	}

	// 5. subscription domain
	st.set("sub", "run", "")
	switch {
	case req.Sub.Mode == "panel":
		a.Store.Update(func(d *Data) error { d.Settings.SubCertID = ""; return nil })
		st.set("sub", "ok", "на адресе панели")
	case subID == "":
		st.set("sub", "skip", "домен не создан — подписка на адресе панели")
	default:
		status, msg := a.wizWaitCert(main, subID, 4*time.Minute)
		a.Store.Update(func(d *Data) error {
			d.Settings.SubCertID = subID
			d.AddEvent("ok", "Подписка переведена на "+subName, "")
			return nil
		})
		if status == "active" {
			st.set("sub", "ok", subName+" — сертификат готов")
		} else {
			st.set("sub", "warn", subName+": "+msg)
			st.warn("Сертификат для " + subName + " ещё не выпущен — пока ссылки подписки остаются на адресе панели и переключатся сами, когда сертификат появится.")
		}
	}

	// 6. panel domain — prepared, switched only when the admin presses the button
	st.set("panel", "run", "")
	result := map[string]any{"client": clientName}
	switch {
	case panelOnDomain:
		st.set("panel", "skip", "панель уже на домене "+panelCur+" — ничего не меняем")
	case req.Panel.Mode == "keep":
		st.set("panel", "skip", "адрес панели не меняется")
	default:
		status, msg := a.wizWaitCert(main, panelID, 4*time.Minute)
		if status == "active" {
			var port int
			var base string
			a.Store.Read(func(d *Data) { port, base = d.Settings.PanelPort, d.Settings.BasePath })
			result["panelUrl"] = "https://" + hostPort(panelName, port, "https") + "/" + base + "/"
			result["panelCertId"] = panelID
			st.set("panel", "ok", panelName+" — сертификат готов, панель ждёт перехода")
		} else {
			st.set("panel", "warn", panelName+": "+msg)
			st.warn("Сертификат для " + panelName + " не выпущен — панель остаётся на старом адресе. Когда сертификат появится (раздел «Домены»), включите HTTPS панели кнопкой на этом домене.")
		}
	}
	// the subscription link as it will be after the move
	a.Store.Read(func(d *Data) {
		scheme, host := "http", a.panelHostLocked(d)
		if pu, _ := result["panelUrl"].(string); pu != "" || d.Settings.PanelCertID != "" {
			scheme = "https"
			if pu != "" {
				host = panelName
			}
		}
		if sh := subHostLocked(d); sh != "" {
			host = sh
		}
		if subToken != "" {
			result["subUrl"] = scheme + "://" + hostPort(host, orInt(d.Settings.SubPort, d.Settings.PanelPort), scheme) + "/" + d.Settings.SubPath + "/" + subToken
		}
	})
	a.Store.Update(func(d *Data) error {
		d.AddEvent("ok", "Автонастройка завершена: "+strconv.Itoa(len(siteDoms))+" "+plural(len(siteDoms), "сервер", "сервера", "серверов")+", клиент main", "")
		return nil
	})
	st.mu.Lock()
	st.Result = result
	st.mu.Unlock()
	finish("")
}

// wizDomain finds or creates the domain on a server and starts its certificate.
func (a *App) wizDomain(s wizServer, dm wizDom, role, email string) (string, string, error) {
	list, err := a.wizListDomains(s)
	if err != nil {
		return "", "", err
	}
	find := func(name string) *wizDomain {
		var best *wizDomain
		for i := range list.Domains {
			x := &list.Domains[i]
			if x.Name != name || x.Kind != "domain" {
				continue
			}
			if best == nil || (x.Mode == "letsencrypt" && best.Mode != "letsencrypt") {
				best = x
			}
		}
		return best
	}
	kick := func(x *wizDomain) {
		if x.Status != "active" && !x.Issuing && x.Mode == "letsencrypt" {
			_ = a.srvCall(s, "POST", "domains/"+x.ID+"/issue", nil, nil)
		}
	}
	switch dm.Mode {
	case "exist":
		for i := range list.Domains {
			if x := &list.Domains[i]; x.ID == dm.ID {
				if x.Kind != "domain" {
					return "", "", fmt.Errorf("%s — это IP, нужен домен", x.Name)
				}
				kick(x)
				return x.ID, x.Name, nil
			}
		}
		return "", "", fmt.Errorf("домен не найден — обновите страницу")
	case "own":
		if x := find(dm.Name); x != nil {
			kick(x)
			return x.ID, x.Name, nil
		}
		var res struct {
			ID string `json:"id"`
		}
		if err := a.srvCall(s, "POST", "domains", map[string]any{"name": dm.Name, "mode": "letsencrypt", "email": email}, &res); err != nil {
			return "", "", err
		}
		return res.ID, dm.Name, nil
	default: // free: label-1-2-3-4.sslip.io for the server's own IP
		label := orDefault(dm.Label, role)
		if list.ServerIP != "" {
			if name, err := freeIPName("sslip.io", label, list.ServerIP, false); err == nil {
				if x := find(name); x != nil {
					kick(x)
					return x.ID, x.Name, nil
				}
			}
		}
		var res struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Warning string `json:"warning"`
		}
		if err := a.srvCall(s, "POST", "domains/free", map[string]any{"provider": "sslip", "label": label, "email": email}, &res); err != nil {
			return "", "", err
		}
		if res.Warning != "" {
			wizState.warn(s.Name + ": " + res.Warning)
		}
		return res.ID, res.Name, nil
	}
}

// wizWaitCert waits until the certificate is issued (or failed).
func (a *App) wizWaitCert(s wizServer, id string, max time.Duration) (string, string) {
	deadline := time.Now().Add(max)
	last := "сертификат выпускается"
	for {
		list, err := a.wizListDomains(s)
		if err == nil {
			for _, x := range list.Domains {
				if x.ID != id {
					continue
				}
				if x.Status == "active" {
					return "active", ""
				}
				if x.Error != "" {
					last = x.Error
				}
				if x.Status == "error" && !x.Issuing {
					return "error", last
				}
			}
		}
		if time.Now().After(deadline) {
			return "timeout", last
		}
		time.Sleep(3 * time.Second)
	}
}

// wizServerSetup: the site + Reality on the server's own domain, then one
// inbound of every other protocol — all named «<server> <protocol>».
func (a *App) wizServerSetup(s wizServer, domID, name, email string) {
	st := wizState
	sk, ik := "site:"+s.ID, "inb:"+s.ID
	st.set(sk, "run", "")
	if !s.Main {
		if v := a.wizNodeVersion(s); v != "" {
			s.Version = v
		}
	}
	if !s.Main && !verAtLeast(s.Version, "1.0.7") {
		msg := "на сервере панель " + orDefault(s.Version, "старой версии") + " — обновите её до " + Version + " (kvk update) и запустите автонастройку ещё раз"
		st.set(sk, "bad", msg)
		st.set(ik, "skip", "")
		st.warn(s.Name + ": " + msg)
		return
	}
	var auto struct {
		Settings FakeSiteSettings `json:"settings"`
		State    map[string]any   `json:"state"`
	}
	if err := a.srvCall(s, "GET", "autosetup", nil, &auto); err != nil {
		st.set(sk, "bad", err.Error())
		st.set(ik, "skip", "")
		return
	}
	// 1.0.7 made more VLESS variants (Reality Vision with the site behind it, XHTTP/WS
	// behind the site, HTTPUpgrade) — drop exactly those, by the names the wizard gave them
	removed := a.wizDropExtraVless(s)
	if len(removed) > 0 {
		if err := a.srvCall(s, "GET", "autosetup", nil, &auto); err != nil {
			st.set(sk, "bad", err.Error())
			st.set(ik, "skip", "")
			return
		}
	}
	// the server's own domain goes first (it is the SNI); its other site domains stay
	doms := []map[string]any{{"name": name, "proxy": false}}
	for _, x := range auto.Settings.Domains {
		if x != name && len(doms) < 10 {
			proxy := containsStr(auto.Settings.CDNDomains, x) && x != auto.Settings.SNI
			doms = append(doms, map[string]any{"name": x, "proxy": proxy})
		}
	}
	ports := []int{443, 8443, 2053, 2083, 2087, 2096}
	if auto.Settings.Port > 0 {
		ports = append([]int{auto.Settings.Port}, ports...)
	}
	var runErr error
	port := 0
	for i, p := range ports {
		if i == 0 && len(removed) > 0 {
			time.Sleep(2 * time.Second) // Xray lets go of the old Reality port
		}
		// only the site here: VLESS comes as XHTTP Reality, gRPC Reality and WS TLS below
		runErr = a.srvCall(s, "POST", "autosetup/run", map[string]any{"siteOnly": true, "domains": doms, "sni": name, "port": p,
			"cdn": false, "grantAll": false, "createClient": false, "email": email}, nil)
		if runErr == nil {
			port = p
			break
		}
		if !strings.Contains(runErr.Error(), "Порт") && !strings.Contains(runErr.Error(), "409") {
			break
		}
	}
	if runErr != nil {
		st.set(sk, "bad", runErr.Error())
		st.set(ik, "skip", "")
		st.warn(s.Name + ": сайт не настроен — " + runErr.Error())
		return
	}
	// follow the server's own progress
	var state struct {
		Running  bool        `json:"running"`
		Error    string      `json:"error"`
		Warnings []string    `json:"warnings"`
		Steps    []*autoStep `json:"steps"`
	}
	deadline := time.Now().Add(10 * time.Minute)
	for {
		time.Sleep(1500 * time.Millisecond)
		if err := a.srvCall(s, "GET", "autosetup/state", nil, &state); err == nil {
			cur := ""
			for _, x := range state.Steps {
				if x.Status == "run" {
					cur = x.Title
				}
			}
			if state.Running {
				st.set(sk, "run", orDefault(cur, "идёт настройка"))
			}
		}
		if !state.Running || time.Now().After(deadline) {
			break
		}
	}
	if state.Error != "" {
		st.set(sk, "bad", state.Error)
		st.set(ik, "skip", "")
		st.warn(s.Name + ": " + state.Error)
		return
	}
	for _, w := range state.Warnings {
		st.warn(s.Name + ": " + w)
	}
	status := "ok"
	if len(state.Warnings) > 0 {
		status = "warn"
	}
	st.set(sk, status, "сайт "+name+" на порту "+strconv.Itoa(port)+" · SNI "+name)

	// 3. the other protocols
	st.set(ik, "run", "")
	if err := a.srvCall(s, "GET", "autosetup", nil, &auto); err != nil {
		st.set(ik, "bad", err.Error())
		return
	}
	var have struct {
		Inbounds []struct {
			ID       string `json:"id"`
			Remark   string `json:"remark"`
			Protocol string `json:"protocol"`
		} `json:"inbounds"`
	}
	_ = a.srvCall(s, "GET", "inbounds", nil, &have)
	exists := func(remark string) bool {
		for _, in := range have.Inbounds {
			if in.Remark == remark {
				return true
			}
		}
		return false
	}
	// TLS protocols need the certificate: wait for Let's Encrypt, else use a self-signed one
	certID := domID
	if status, msg := a.wizWaitCert(s, domID, 3*time.Minute); status != "active" {
		var res struct {
			ID string `json:"id"`
		}
		list, _ := a.wizListDomains(s)
		for _, x := range list.Domains {
			if x.Name == name && x.Mode == "selfsigned" && x.Status == "active" {
				res.ID = x.ID
			}
		}
		if res.ID == "" {
			if err := a.srvCall(s, "POST", "domains", map[string]any{"name": name, "mode": "selfsigned"}, &res); err != nil {
				res.ID = ""
			}
		}
		if res.ID != "" {
			certID = res.ID
			st.warn(s.Name + ": сертификат Let's Encrypt для " + name + " не выпущен (" + msg + ") — TLS-подключения (VLESS WS TLS, VMess WS TLS, Hysteria2, TUIC, MASQUE) пока на самоподписанном. Когда сертификат появится, выберите его в этих инбаундах.")
		} else {
			certID = ""
		}
	}
	target := "127.0.0.1:" + strconv.Itoa(auto.Settings.TLSPort)
	reality := func(m map[string]any) map[string]any {
		m["security"], m["realityTarget"], m["realitySni"], m["fingerprint"] = "reality", target, []string{name}, "chrome"
		return m
	}
	tlsOn := func(m map[string]any) map[string]any {
		m["security"], m["certId"] = "tls", certID
		return m
	}
	N := s.Name + " "
	specs := []map[string]any{
		// VLESS: only these three
		reality(map[string]any{"remark": N + "VLESS XHTTP Reality", "protocol": "vless", "network": "xhttp", "xhttpMode": "auto"}),
		reality(map[string]any{"remark": N + "VLESS gRPC Reality", "protocol": "vless", "network": "grpc"}),
		tlsOn(map[string]any{"remark": N + "VLESS WS TLS", "protocol": "vless", "network": "ws"}),
		reality(map[string]any{"remark": N + "Trojan Reality", "protocol": "trojan", "network": "tcp"}),
		tlsOn(map[string]any{"remark": N + "VMess WS TLS", "protocol": "vmess", "network": "ws"}),
		{"remark": N + "Shadowsocks", "protocol": "shadowsocks", "network": "tcp", "security": "none"},
		tlsOn(map[string]any{"remark": N + "Hysteria2", "protocol": "hysteria2"}),
		tlsOn(map[string]any{"remark": N + "TUIC", "protocol": "tuic"}),
		{"remark": N + "AmneziaWG", "protocol": "amneziawg", "security": "none"},
		tlsOn(map[string]any{"remark": N + "MASQUE", "protocol": "masque"}),
	}
	made, skipped, failed := []string{}, 0, []string{}
	for _, m := range specs {
		remark := m["remark"].(string)
		short := strings.TrimPrefix(remark, N)
		if exists(remark) {
			skipped++
			continue
		}
		if m["security"] == "tls" && certID == "" {
			failed = append(failed, short+": нет сертификата")
			continue
		}
		m["externalHost"] = name
		var res struct {
			ID   string `json:"id"`
			Port int    `json:"port"`
		}
		err := a.srvCall(s, "POST", "inbounds", m, &res)
		// AmneziaWG: the default subnet may be taken by another AWG inbound — take the next free one
		for i := 67; err != nil && m["protocol"] == "amneziawg" && strings.Contains(err.Error(), "пересекается") && i < 90; i++ {
			aw := defaultAWG() // fresh obfuscation; the server makes its own keys
			aw.PrivateKey, aw.PublicKey, aw.Network = "", "", "10."+strconv.Itoa(i)+".0.0/16"
			m["awg"] = aw
			err = a.srvCall(s, "POST", "inbounds", m, &res)
		}
		if err != nil {
			failed = append(failed, short+": "+err.Error())
			continue
		}
		made = append(made, short)
	}
	detail := []string{}
	if len(made) > 0 {
		detail = append(detail, "созданы: "+strings.Join(made, ", "))
	}
	if len(removed) > 0 {
		detail = append(detail, "убраны лишние: "+strings.Join(removed, ", "))
	}
	if skipped > 0 {
		detail = append(detail, strconv.Itoa(skipped)+" уже были")
	}
	if len(failed) > 0 {
		detail = append(detail, "не созданы: "+strings.Join(failed, "; "))
		for _, f := range failed {
			st.warn(s.Name + " — " + f)
		}
	}
	stt := "ok"
	if len(failed) > 0 {
		stt = "warn"
	}
	st.set(ik, stt, strings.Join(detail, " · "))
}

// wizDropExtraVless deletes the VLESS inbounds an earlier wizard run made that
// are no longer part of the set (only XHTTP Reality, gRPC Reality and WS TLS stay).
// An old «WS TLS» that lived behind the site is replaced by a standalone one.
func (a *App) wizDropExtraVless(s wizServer) []string {
	var have struct {
		Inbounds []struct {
			ID     string `json:"id"`
			Remark string `json:"remark"`
			Auto   string `json:"auto"`
		} `json:"inbounds"`
	}
	if err := a.srvCall(s, "GET", "inbounds", nil, &have); err != nil {
		return nil
	}
	N := s.Name + " "
	extra := map[string]bool{N + "VLESS Reality": true, N + "VLESS XHTTP CDN": true, N + "VLESS XHTTP TLS": true,
		N + "VLESS WS CDN": true, N + "VLESS HTTPUpgrade TLS": true}
	removed := []string{}
	for _, in := range have.Inbounds {
		if extra[in.Remark] || (in.Remark == N+"VLESS WS TLS" && in.Auto != "") {
			if a.srvCall(s, "DELETE", "inbounds/"+in.ID, nil, nil) == nil {
				removed = append(removed, strings.TrimPrefix(in.Remark, N))
			}
		}
	}
	return removed
}

// ---------- POST /api/wizard/switch ----------

// apiWizSwitch moves the panel to the prepared domain. The answer carries the
// new address; the panel restarts a moment later.
func (a *App) apiWizSwitch(w http.ResponseWriter, r *http.Request) error {
	snap := wizState.snapshot()
	res, _ := snap["result"].(map[string]any)
	id, _ := res["panelCertId"].(string)
	u, _ := res["panelUrl"].(string)
	if id == "" || u == "" {
		return errf(400, "Домен панели не подготовлен — запустите автонастройку")
	}
	var cur string
	a.Store.Read(func(d *Data) { cur = d.Settings.PanelCertID })
	if cur != id {
		if err := a.setPanelCert(id); err != nil {
			return err
		}
	}
	ok(w, map[string]any{"url": u})
	return nil
}

// plural: 1 сервер, 2 сервера, 5 серверов.
func plural(n int, one, few, many string) string {
	n %= 100
	if n >= 11 && n <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
