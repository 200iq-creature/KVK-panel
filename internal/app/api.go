package app

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type handlerFn func(w http.ResponseWriter, r *http.Request) error

func (a *App) wrap(h handlerFn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			fail(w, err)
		}
	}
}

// authed: session cookie (with CSRF header for mutations) or API token.
func (a *App) authed(h handlerFn) http.HandlerFunc {
	return a.wrap(func(w http.ResponseWriter, r *http.Request) error {
		if a.checkToken(r) {
			return h(w, r)
		}
		if !a.checkSession(r) {
			return errf(401, "unauthorized")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("X-KVK") != "1" {
			return errf(403, "csrf")
		}
		return h(w, r)
	})
}

func (a *App) tokenOnly(h handlerFn) http.HandlerFunc {
	return a.wrap(func(w http.ResponseWriter, r *http.Request) error {
		if !a.checkToken(r) {
			return errf(401, "unauthorized")
		}
		return h(w, r)
	})
}

func (a *App) apiMux() *http.ServeMux {
	m := http.NewServeMux()
	A := func(pattern string, h handlerFn) { m.HandleFunc(pattern, a.authed(h)) }

	// auth (public)
	m.HandleFunc("POST /api/auth/login", a.wrap(a.apiLogin))
	m.HandleFunc("POST /api/auth/2fa", a.wrap(a.api2FA))
	m.HandleFunc("POST /api/auth/2fa/telegram", a.wrap(a.api2FATelegram))
	m.HandleFunc("POST /api/auth/logout", a.wrap(a.apiLogout))
	m.HandleFunc("GET /api/auth/me", a.wrap(a.apiMe))

	A("GET /api/meta", a.apiMeta)
	A("GET /api/overview", a.apiOverview)
	A("GET /api/events", a.apiEvents)

	// clients
	A("GET /api/clients", a.apiClients)
	A("POST /api/clients", a.apiClientCreate)
	A("GET /api/clients/export", a.apiClientsExport)
	A("POST /api/clients/import", a.apiClientsImport)
	A("POST /api/clients/bulk", a.apiClientsBulk)
	A("GET /api/clients/{id}", a.apiClientGet)
	A("PUT /api/clients/{id}", a.apiClientUpdate)
	A("DELETE /api/clients/{id}", a.apiClientDelete)
	A("POST /api/clients/{id}/action", a.apiClientAction)

	// inbounds
	A("GET /api/inbounds", a.apiInbounds)
	A("POST /api/inbounds", a.apiInboundCreate)
	A("PUT /api/inbounds/{id}", a.apiInboundUpdate)
	A("DELETE /api/inbounds/{id}", a.apiInboundDelete)
	A("POST /api/inbounds/{id}/toggle", a.apiInboundToggle)
	A("POST /api/inbounds/{id}/move", a.apiInboundMove)
	A("POST /api/inbounds/{id}/grant", a.apiInboundGrant)
	A("POST /api/inbounds/{id}/clients", a.apiInboundAddClient)
	A("DELETE /api/inbounds/{id}/clients/{cid}", a.apiInboundRemoveClient)
	A("GET /api/inbounds/{id}/link", a.apiInboundExampleLink)
	A("GET /api/tools/x25519", a.apiX25519)
	A("GET /api/tools/port", a.apiPortCheck)
	A("GET /api/autosetup", a.apiAutoGet)
	A("POST /api/autosetup/check", a.apiAutoCheck)
	A("POST /api/autosetup/run", a.apiAutoRun)
	A("GET /api/autosetup/state", a.apiAutoState)
	A("POST /api/autosetup/selfcheck", a.apiAutoCheckNow)
	A("PUT /api/autosetup/site", a.apiAutoSite)
	A("GET /api/autosetup/preview", a.apiAutoPreview)
	A("DELETE /api/autosetup/token", a.apiAutoToken)
	A("DELETE /api/autosetup", a.apiAutoDelete)

	// xray core
	A("GET /api/xray/status", a.apiXrayStatus)
	A("POST /api/xray/{action}", a.apiXrayAction)
	A("GET /api/xray/versions", a.apiXrayVersions)
	A("POST /api/xray-install", a.apiXrayInstall)
	A("POST /api/xray-rollback", a.apiXrayRollback)
	A("GET /api/xray-config", a.apiXrayConfig)
	A("GET /api/geo", a.apiGeo)
	A("PUT /api/geo", a.apiGeoSettings)
	A("POST /api/geo/update", a.apiGeoUpdate)
	A("POST /api/geo/rollback", a.apiGeoRollback)

	// settings
	A("GET /api/settings", a.apiSettings)
	A("PUT /api/settings", a.apiSettingsUpdate)
	A("POST /api/settings/panel", a.apiSettingsPanel)
	A("POST /api/settings/credentials", a.apiSettingsCredentials)
	A("POST /api/settings/token", a.apiSettingsToken)
	A("POST /api/settings/reset-all", a.apiSettingsResetAll)
	A("GET /api/panel/update", a.apiPanelUpdateCheck)
	A("POST /api/panel/update", a.apiPanelUpdate)

	// subscription
	A("GET /api/sub", a.apiSubSettings)
	A("PUT /api/sub", a.apiSubSettingsUpdate)

	// security
	A("GET /api/security", a.apiSecurity)
	A("POST /api/security/totp/setup", a.apiTOTPSetup)
	A("POST /api/security/totp/enable", a.apiTOTPEnable)
	A("POST /api/security/totp/disable", a.apiTOTPDisable)
	A("PUT /api/security/telegram", a.apiTelegramSave)
	A("POST /api/security/telegram/test", a.apiTelegramTest)

	// domains & certificates
	A("GET /api/domains", a.apiDomains)
	A("POST /api/domains", a.apiDomainCreate)
	A("GET /api/domains/free", a.apiFreeDomainInfo)
	A("POST /api/domains/free", a.apiFreeDomainCreate)
	A("GET /api/domains/free/check", a.apiFreeDomainCheck)
	A("POST /api/domains/{id}/point", a.apiFreeDomainPoint)
	A("POST /api/domains/{id}/issue", a.apiDomainIssue)
	A("POST /api/domains/{id}/panel", a.apiDomainPanel)
	A("DELETE /api/domains/{id}", a.apiDomainDelete)
	A("POST /api/domains-panel-off", a.apiDomainPanelOff)
	A("PUT /api/host", a.apiHostUpdate)

	// filter
	A("GET /api/filter", a.apiFilter)
	A("PUT /api/filter", a.apiFilterSave)
	A("POST /api/filter/check", a.apiFilterCheck)

	// warp
	A("GET /api/warp", a.apiWarp)
	A("POST /api/warp/register", a.apiWarpRegister)
	A("POST /api/warp/license", a.apiWarpLicense)
	A("PUT /api/warp", a.apiWarpSave)
	A("DELETE /api/warp", a.apiWarpDelete)

	// backups
	A("GET /api/backups", a.apiBackups)
	A("POST /api/backups", a.apiBackupCreate)
	A("POST /api/backups/upload", a.apiBackupUpload)
	A("GET /api/backups/{name}", a.apiBackupDownload)
	A("POST /api/backups/{name}/restore", a.apiBackupRestore)
	A("DELETE /api/backups/{name}", a.apiBackupDelete)

	// migration
	A("POST /api/migrate/test", a.apiMigrateTest)
	A("POST /api/migrate/run", a.apiMigrateRun)
	A("GET /api/migrate/status", a.apiMigrateStatus)
	A("GET /api/migrate/export", a.apiMigrateExport)

	// logs
	A("GET /api/logs/state", a.apiLogsState)
	A("POST /api/logs/start", a.apiLogsStart)
	A("POST /api/logs/stop", a.apiLogsStop)
	A("GET /api/logs", a.apiLogs)
	A("POST /api/logs/clear", a.apiLogsClear)
	A("GET /api/logs/download", a.apiLogsDownload)

	// server link (multi-node)
	A("GET /api/nodes", a.apiNodes)
	A("POST /api/nodes", a.apiNodeAdd)
	A("POST /api/nodes/test", a.apiNodeTest)
	A("POST /api/nodes/sync", a.apiNodesSync)
	A("PUT /api/nodes/{id}", a.apiNodeUpdate)
	A("DELETE /api/nodes/{id}", a.apiNodeDelete)
	A("POST /api/main/detach", a.apiMainDetach)
	A("GET /api/nodes/{id}/profile", a.apiNodeProfile)
	A("POST /api/nodes/{id}/accept", a.apiNodeAccept)
	A("POST /api/nodes/{id}/restore", a.apiNodeRestore)
	A("POST /api/nodes/{id}/grant", a.apiNodeGrant)
	A("/api/nodes/{id}/api/{path...}", a.apiNodeProxy)
	A("PUT /api/backups-settings", a.apiBackupSettings)

	// shop bot
	A("GET /api/shop", a.apiShop)
	A("PUT /api/shop", a.apiShopSave)
	A("GET /api/shop/orders", a.apiShopOrders)
	A("POST /api/shop/orders/{id}/confirm", a.apiShopOrderConfirm)
	A("POST /api/shop/orders/{id}/reject", a.apiShopOrderReject)
	A("GET /api/shop/users", a.apiShopUsers)
	A("GET /api/shop/promos", a.apiShopPromos)
	A("POST /api/shop/promos", a.apiShopPromoSave)
	A("DELETE /api/shop/promos/{code}", a.apiShopPromoDelete)
	A("POST /api/shop/broadcast", a.apiShopBroadcast)
	A("GET /api/shop/broadcast", a.apiShopBroadcastStatus)

	// telegram proxy
	A("GET /api/tgproxy", a.apiTgProxy)
	A("PUT /api/tgproxy", a.apiTgProxySave)
	A("POST /api/tgproxy/secret", a.apiTgProxySecret)
	A("POST /api/tgproxy/install", a.apiTgProxyInstall)
	A("POST /api/tgproxy/users", a.apiTgProxyUserCreate)
	A("PUT /api/tgproxy/users/{id}", a.apiTgProxyUserUpdate)
	A("DELETE /api/tgproxy/users/{id}", a.apiTgProxyUserDelete)

	m.HandleFunc("POST /api/node/sync", a.tokenOnly(a.apiNodeSyncIn))
	m.HandleFunc("GET /api/node/info", a.tokenOnly(a.apiNodeInfo))
	m.HandleFunc("GET /api/node/profile", a.tokenOnly(a.apiNodeProfileGet))
	m.HandleFunc("POST /api/node/profile", a.tokenOnly(a.apiNodeProfileApply))

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]any{"error": "not found"})
	})
	return m
}

// ---------- auth handlers ----------

func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, user string, ver int) {
	var base string
	a.Store.Read(func(d *Data) { base = d.Settings.BasePath })
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: a.makeSession(user, ver, 30), Path: "/" + base + "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 30 * 86400,
	})
}

func (a *App) apiLogin(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ip := clientIP(r)
	ua := shortUA(r.UserAgent())
	if a.limiter.blocked(ip) {
		a.Store.Update(func(d *Data) error {
			d.AddLogin(LoginEntry{T: nowMs(), OK: false, User: req.Username, Method: "пароль", IP: ip, UA: ua, Reason: "блок: много попыток"})
			return nil
		})
		return errf(429, "too_many_attempts")
	}
	var adm Admin
	var st Settings
	a.Store.Read(func(d *Data) { adm = d.Admin; st = d.Settings })
	if req.Username != adm.Username || !CheckPassword(adm.PassHash, req.Password) {
		a.limiter.fail(ip)
		a.Store.Update(func(d *Data) error {
			d.AddLogin(LoginEntry{T: nowMs(), OK: false, User: req.Username, Method: "пароль", IP: ip, UA: ua, Reason: "неверный пароль"})
			return nil
		})
		return errf(401, "invalid_credentials")
	}
	tgReady := adm.TgEnabled && st.TgBotToken != "" && st.TgAdminID != ""
	if adm.TOTPEnabled || tgReady {
		methods := []string{}
		if adm.TOTPEnabled {
			methods = append(methods, "totp")
		}
		if tgReady {
			methods = append(methods, "telegram")
		}
		id := a.tickets.put(&ticket{user: adm.Username, ip: ip, ua: ua, created: time.Now()})
		ok(w, map[string]any{"need2fa": true, "ticket": id, "methods": methods})
		return nil
	}
	a.limiter.reset(ip)
	a.setSessionCookie(w, r, adm.Username, adm.SessionVersion)
	a.Store.Update(func(d *Data) error {
		d.AddLogin(LoginEntry{T: nowMs(), OK: true, User: adm.Username, Method: "по паролю", IP: ip, UA: ua})
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) api2FATelegram(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Ticket string `json:"ticket"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	tk := a.tickets.get(req.Ticket)
	if tk == nil {
		return errf(401, "Сессия входа истекла — введите пароль заново")
	}
	if time.Since(tk.tgSent) < 30*time.Second {
		return errf(429, "Код уже отправлен — подождите 30 секунд")
	}
	code := RandomDigits(6)
	msg := "🔐 Вход в KVK Panel\nКод: " + code + "\nIP: " + tk.ip + "\n" + tk.ua + "\n\nЕсли это не вы — смените пароль панели."
	if err := a.tgSend(msg); err != nil {
		return errf(502, "Telegram не принял сообщение: %v", err)
	}
	tk.tgCode = code
	tk.tgSent = time.Now()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) api2FA(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
		Method string `json:"method"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	ip := clientIP(r)
	if a.limiter.blocked(ip) {
		return errf(429, "too_many_attempts")
	}
	tk := a.tickets.get(req.Ticket)
	if tk == nil {
		return errf(401, "Сессия входа истекла — введите пароль заново")
	}
	var adm Admin
	a.Store.Read(func(d *Data) { adm = d.Admin })
	good := false
	method := ""
	code := strings.TrimSpace(req.Code)
	if req.Method == "telegram" {
		good = tk.tgCode != "" && code == tk.tgCode && time.Since(tk.tgSent) < 5*time.Minute
		method = "пароль + Telegram"
	} else {
		good = adm.TOTPEnabled && totpVerify(adm.TOTPSecret, code)
		method = "пароль + код 2FA"
	}
	if !good {
		tk.tries++
		a.limiter.fail(ip)
		if tk.tries >= 5 {
			a.tickets.del(req.Ticket)
		}
		a.Store.Update(func(d *Data) error {
			d.AddLogin(LoginEntry{T: nowMs(), OK: false, User: tk.user, Method: method, IP: ip, UA: tk.ua, Reason: "неверный код"})
			return nil
		})
		return errf(401, "Неверный код")
	}
	a.tickets.del(req.Ticket)
	a.limiter.reset(ip)
	a.setSessionCookie(w, r, adm.Username, adm.SessionVersion)
	a.Store.Update(func(d *Data) error {
		d.AddLogin(LoginEntry{T: nowMs(), OK: true, User: adm.Username, Method: method, IP: ip, UA: tk.ua})
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiLogout(w http.ResponseWriter, r *http.Request) error {
	var base string
	a.Store.Read(func(d *Data) { base = d.Settings.BasePath })
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/" + base + "/", MaxAge: -1, HttpOnly: true})
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiMe(w http.ResponseWriter, r *http.Request) error {
	authed := a.checkSession(r)
	var user, node string
	a.Store.Read(func(d *Data) { user = d.Admin.Username; node = d.Settings.NodeName })
	res := map[string]any{"authed": authed, "version": Version, "branch": Branch, "node": node}
	if authed {
		res["user"] = user
	}
	ok(w, res)
	return nil
}

// ---------- meta / overview ----------

func (a *App) apiMeta(w http.ResponseWriter, r *http.Request) error {
	res := map[string]any{}
	now := nowMs()
	a.Store.Read(func(d *Data) {
		res["user"] = d.Admin.Username
		res["node"] = d.Settings.NodeName
		res["country"] = d.Settings.CountryCode
		res["host"] = d.Settings.PublicHost
		res["clients"] = len(d.Clients)
		active := 0
		for _, c := range d.Clients {
			if c.Status(now) == StActive {
				active++
			}
		}
		res["activeClients"] = active
		res["inbounds"] = len(d.Inbounds)
		res["basePath"] = d.Settings.BasePath
		res["subPath"] = d.Settings.SubPath
		res["defaultFp"] = d.Settings.DefaultFP
		res["https"] = d.Settings.PanelCertID != ""
		res["tags"] = allTags(d)
		res["main"] = d.Main
	})
	res["version"] = Version
	res["branch"] = Branch
	res["xray"] = a.Xray.Status()
	ok(w, res)
	return nil
}

func allTags(d *Data) []string {
	m := map[string]bool{}
	for _, c := range d.Clients {
		for _, t := range c.Tags {
			m[t] = true
		}
	}
	out := []string{}
	for t := range m {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (a *App) apiOverview(w http.ResponseWriter, r *http.Request) error {
	now := nowMs()
	loc := moscow()
	res := map[string]any{}
	a.rt.Lock()
	onlineIPs := a.rt.onlineIPs
	a.rt.Unlock()
	a.Store.Read(func(d *Data) {
		total, active, online, soon, expired, banned, devices := len(d.Clients), 0, 0, 0, 0, 0, 0
		apps := map[string]int{}
		for _, c := range d.Clients {
			s := c.Status(now)
			if s == StActive {
				active++
				if c.ExpiresAt > 0 && c.ExpiresAt-now < 3*86400*1000 {
					soon++
				}
			}
			if s == StExpired || s == StLimited {
				expired++
			}
			if s == StBanned {
				banned++
			}
			if now-c.LastOnline < 150*1000 {
				online++
				devices += len(onlineIPs[c.Name])
			}
			if c.LastSubUA != "" {
				apps[appName(c.LastSubUA)]++
			}
		}
		res["clients"] = map[string]any{"total": total, "active": active, "online": online, "soon": soon, "expired": expired, "banned": banned, "devices": devices}
		res["autoClean"] = d.Settings.AutoCleanDays
		days := []map[string]any{}
		t := time.Now().In(loc)
		for i := 29; i >= 0; i-- {
			day := t.AddDate(0, 0, -i).Format("2006-01-02")
			days = append(days, map[string]any{"day": day, "bytes": d.Daily[day]})
		}
		res["daily"] = days
		res["apps"] = apps
		ev := d.Events
		if len(ev) > 8 {
			ev = ev[len(ev)-8:]
		}
		out := make([]Event, len(ev))
		for i := range ev {
			out[i] = ev[len(ev)-1-i]
		}
		res["events"] = out
		nodes := []map[string]any{}
		for _, n := range d.Nodes {
			st := "в сети"
			if !n.Enabled {
				st = "выключен"
			} else if n.LastError != "" {
				st = "нет связи"
			}
			nodes = append(nodes, map[string]any{"name": n.Name, "country": n.Country, "clients": n.Clients, "cpu": n.CPU, "status": st, "ok": n.Enabled && n.LastError == ""})
		}
		res["nodes"] = nodes
		res["node"] = d.Settings.NodeName
		res["country"] = d.Settings.CountryCode
		res["host"] = d.Settings.PublicHost
		res["localClients"] = len(d.Clients)
	})
	res["sys"] = a.sys.Get()
	res["xray"] = a.Xray.Status()
	ok(w, res)
	return nil
}

func appName(ua string) string {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "happ"):
		return "Happ"
	case strings.Contains(l, "v2raytun"):
		return "v2RayTun"
	case strings.Contains(l, "v2rayng"):
		return "v2rayNG"
	case strings.Contains(l, "hiddify"):
		return "Hiddify"
	case strings.Contains(l, "streisand"):
		return "Streisand"
	case strings.Contains(l, "shadowrocket"):
		return "Shadowrocket"
	case strings.Contains(l, "incy"):
		return "INCY"
	case strings.Contains(l, "nekobox"), strings.Contains(l, "nekoray"):
		return "NekoBox"
	case strings.Contains(l, "clash"), strings.Contains(l, "mihomo"), strings.Contains(l, "stash"):
		return "Clash / Mihomo"
	case strings.Contains(l, "sing-box"), strings.Contains(l, "sfa"), strings.Contains(l, "sfi"):
		return "sing-box"
	case strings.Contains(l, "foxray"):
		return "FoXray"
	case strings.Contains(l, "mozilla"):
		return "Браузер"
	}
	return "Другое"
}

func (a *App) apiEvents(w http.ResponseWriter, r *http.Request) error {
	var ev []Event
	a.Store.Read(func(d *Data) {
		ev = make([]Event, len(d.Events))
		for i := range d.Events {
			ev[i] = d.Events[len(d.Events)-1-i]
		}
	})
	ok(w, ev)
	return nil
}

func dirSize(p string) int64 {
	var n int64
	_ = filepath.Walk(p, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}
