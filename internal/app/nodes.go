package app

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ---- wire types ----

type syncClient struct {
	ID          string `json:"id"`
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	SubID       string `json:"subId"`
	Password    string `json:"password"`
	ExpiresAt   int64  `json:"expiresAt"`
	DeviceLimit int    `json:"deviceLimit"`
	Active      bool   `json:"active"`
	WGKey       string `json:"wgKey,omitempty"`
	WGIndex     int    `json:"wgIndex,omitempty"`
	// Selective: Inbounds lists the node's own inbound ids the client may use.
	// Without it (older main panels) the client gets every enabled inbound.
	Selective bool     `json:"sel,omitempty"`
	Inbounds  []string `json:"inbounds,omitempty"`
}

type syncPayload struct {
	MainName string       `json:"mainName"`
	MainURL  string       `json:"mainUrl"`
	Clients  []syncClient `json:"clients"`
}

type syncTraffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type nodeInfo struct {
	Name     string                 `json:"name"`
	Country  string                 `json:"country"`
	Version  string                 `json:"version"`
	Xray     string                 `json:"xray"`
	CPU      float64                `json:"cpu"`
	Clients  int                    `json:"clients"`
	Inbounds []NodeInbound          `json:"inbounds"`
	Traffic  map[string]syncTraffic `json:"traffic,omitempty"`
	Online   []string               `json:"online,omitempty"`
	// ProfileHash: hash of the server configuration (profile.go)
	ProfileHash string `json:"profileHash,omitempty"`
}

var nodeHTTP = &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nodes often use IP/self-signed certs; auth is by token
	Proxy:           nil,
}}

// parseNodeKey accepts a panel URL + token, or a "link key" (base64 of url|token).
func parseNodeKey(rawURL, token string) (string, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	token = strings.TrimSpace(token)
	if token == "" && !strings.HasPrefix(rawURL, "http") {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(rawURL, "="))
		if err != nil {
			b, err = base64.StdEncoding.DecodeString(rawURL)
		}
		if err == nil {
			parts := strings.SplitN(string(b), "|", 2)
			if len(parts) == 2 {
				rawURL, token = parts[0], parts[1]
			}
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", errf(400, "Вставьте адрес панели сервера (http(s)://IP:порт/путь/) или ключ связки")
	}
	if token == "" {
		return "", "", errf(400, "Нужен API-токен сервера (Настройки → API)")
	}
	u.RawQuery, u.Fragment = "", ""
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u.String(), token, nil
}

func nodeCall(method, base, token, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := nodeHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("нет связи: %v", shortNetErr(err))
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == 401 {
		return fmt.Errorf("неверный API-токен")
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("по адресу нет панели KVK (проверьте путь)")
	}
	if resp.StatusCode != 200 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		return fmt.Errorf("сервер ответил %d %s", resp.StatusCode, e.Error)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("неожиданный ответ сервера")
		}
	}
	return nil
}

func shortNetErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 && len(s)-i < 80 {
		return s[i+2:]
	}
	return s
}

// ---- node side (this server is a node) ----

func (a *App) localNodeInfo(d *Data) nodeInfo {
	ni := nodeInfo{Name: d.Settings.NodeName, Country: d.Settings.CountryCode, Version: Version, Clients: len(d.Clients), Inbounds: []NodeInbound{},
		ProfileHash: buildProfile(d, false).Hash}
	for _, in := range d.Inbounds {
		if in.Enabled {
			ni.Inbounds = append(ni.Inbounds, a.publicInbounds(d, in)...)
		}
	}
	return ni
}

func (a *App) apiNodeInfo(w http.ResponseWriter, r *http.Request) error {
	var ni nodeInfo
	a.Store.Read(func(d *Data) { ni = a.localNodeInfo(d) })
	ni.Xray = a.Xray.Version()
	ni.CPU = a.sys.Get().CPU
	ok(w, ni)
	return nil
}

func (a *App) apiNodeSyncIn(w http.ResponseWriter, r *http.Request) error {
	var p syncPayload
	if err := readJSON(r, &p); err != nil {
		return err
	}
	changed := false
	var ni nodeInfo
	online := a.onlineIPs()
	now := nowMs()
	a.Store.Update(func(d *Data) error {
		if d.Main == nil || d.Main.URL != p.MainURL || d.Main.Name != p.MainName {
			if d.Main == nil {
				d.AddEvent("ok", "Сервер подключён к основному: "+p.MainName, "")
			}
			d.Main = &MainLink{Name: p.MainName, URL: p.MainURL}
			changed = true
		}
		d.Main.LastSync = now
		grant := []string{}
		for _, in := range d.Inbounds {
			if in.Enabled {
				grant = append(grant, in.ID)
			}
		}
		want := map[string]bool{}
		for _, sc := range p.Clients {
			want[sc.ID] = true
			c := d.ClientByID(sc.ID)
			if c != nil && c.Origin != "main" {
				continue // id clash with a local client — ignore
			}
			name := sc.Name
			if x := d.ClientByName(name); x != nil && x.Origin != "main" {
				name = name + "-m"
			}
			if x := d.ClientByName(name); x != nil && x.ID != sc.ID {
				continue
			}
			if c == nil {
				clash := false
				for _, x := range d.Clients {
					if x.UUID == sc.UUID && x.Origin != "main" {
						clash = true
					}
				}
				if clash {
					continue
				}
				c = &Client{ID: sc.ID, Origin: "main", CreatedAt: now, Tags: []string{"связка"}}
				d.Clients = append(d.Clients, c)
				changed = true
			}
			mine := grant
			if sc.Selective {
				mine = []string{}
				for _, id := range grant {
					if containsStr(sc.Inbounds, id) {
						mine = append(mine, id)
					}
				}
			}
			if c.UUID != sc.UUID || c.Name != name || c.Password != sc.Password || c.ExpiresAt != sc.ExpiresAt ||
				c.DeviceLimit != sc.DeviceLimit || c.Banned != !sc.Active || strings.Join(c.Inbounds, ",") != strings.Join(mine, ",") ||
				(sc.WGKey != "" && (c.WGKey != sc.WGKey || c.WGIndex != sc.WGIndex)) {
				changed = true
			}
			c.UUID, c.Name, c.SubID, c.Password, c.ExpiresAt, c.DeviceLimit = sc.UUID, name, sc.SubID, sc.Password, sc.ExpiresAt, sc.DeviceLimit
			c.Banned = !sc.Active
			c.Inbounds = append([]string{}, mine...)
			if sc.WGKey != "" {
				// AmneziaWG: the same key and tunnel address on every server
				c.WGKey, c.WGIndex = sc.WGKey, sc.WGIndex
			}
		}
		kept := d.Clients[:0]
		for _, c := range d.Clients {
			if c.Origin == "main" && !want[c.ID] {
				changed = true
				continue
			}
			kept = append(kept, c)
		}
		d.Clients = kept
		ni = a.localNodeInfo(d)
		ni.Traffic = map[string]syncTraffic{}
		for _, c := range d.Clients {
			if c.Origin != "main" {
				continue
			}
			du, dd := c.Up-c.ReportedUp, c.Down-c.ReportedDown
			if du < 0 || dd < 0 { // local reset happened
				du, dd = c.Up, c.Down
			}
			if du > 0 || dd > 0 {
				ni.Traffic[c.ID] = syncTraffic{du, dd}
			}
			c.ReportedUp, c.ReportedDown = c.Up, c.Down
			if len(online[c.Name]) > 0 || now-c.LastOnline < 90*1000 {
				ni.Online = append(ni.Online, c.ID)
			}
		}
		return nil
	})
	if changed {
		a.ScheduleApply()
	}
	ni.Xray = a.Xray.Version()
	ni.CPU = a.sys.Get().CPU
	ok(w, ni)
	return nil
}

func (a *App) apiMainDetach(w http.ResponseWriter, r *http.Request) error {
	n := 0
	a.Store.Update(func(d *Data) error {
		kept := d.Clients[:0]
		for _, c := range d.Clients {
			if c.Origin == "main" {
				n++
				continue
			}
			kept = append(kept, c)
		}
		d.Clients = kept
		if d.Main != nil {
			d.AddEvent("warn", "Сервер отвязан от основного «"+d.Main.Name+"»", "")
		}
		d.Main = nil
		return nil
	})
	a.ScheduleApply()
	ok(w, map[string]any{"removed": n})
	return nil
}

// ---- main side ----

var syncMu sync.Mutex
var syncPending bool
var syncPendingMu sync.Mutex

// SyncNodes pushes clients to every enabled node and pulls traffic back. Calls are coalesced.
func (a *App) SyncNodes() {
	syncPendingMu.Lock()
	if syncPending {
		syncPendingMu.Unlock()
		return
	}
	syncPending = true
	syncPendingMu.Unlock()
	time.Sleep(500 * time.Millisecond)
	syncPendingMu.Lock()
	syncPending = false
	syncPendingMu.Unlock()
	syncMu.Lock()
	defer syncMu.Unlock()
	a.syncNodesNow()
}

func (a *App) syncNodesNow() {
	type target struct {
		id, url, token string
		payload        syncPayload
	}
	var targets []target
	now := nowMs()
	a.Store.Read(func(d *Data) {
		mainName, mainURL := d.Settings.NodeName, a.panelURLLocked(d)
		for _, n := range d.Nodes {
			if !n.Enabled {
				continue
			}
			p := syncPayload{MainName: mainName, MainURL: mainURL, Clients: []syncClient{}}
			prefix := n.ID + ":"
			for _, c := range d.Clients {
				if c.Origin == "main" {
					continue // don't re-export clients we got from another main
				}
				sc := syncClient{ID: c.ID, UUID: c.UUID, Name: c.Name, SubID: c.SubID, Password: c.Password,
					ExpiresAt: c.ExpiresAt, DeviceLimit: c.DeviceLimit, Active: c.Status(now) == StActive, WGKey: c.WGKey, WGIndex: c.WGIndex}
				if n.Granted {
					// only the inbounds of this server chosen in the client card
					for _, id := range c.Inbounds {
						if strings.HasPrefix(id, prefix) {
							sc.Inbounds = append(sc.Inbounds, strings.TrimPrefix(id, prefix))
						}
					}
					if len(sc.Inbounds) == 0 {
						continue // no access to this server at all
					}
					sc.Selective = true
				}
				p.Clients = append(p.Clients, sc)
			}
			targets = append(targets, target{n.ID, n.URL, n.Token, p})
		}
	})
	if len(targets) == 0 {
		return
	}
	var wg sync.WaitGroup
	type result struct {
		id   string
		ni   *nodeInfo
		err  error
		ping int
	}
	results := make(chan result, len(targets))
	for _, t := range targets {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			start := time.Now()
			var ni nodeInfo
			err := nodeCall("POST", t.url, t.token, "api/node/sync", t.payload, &ni)
			results <- result{t.id, &ni, err, int(time.Since(start).Milliseconds())}
		}(t)
	}
	wg.Wait()
	close(results)
	statusChanged, resync := false, false
	var refresh []string
	a.Store.Update(func(d *Data) error {
		loc := moscow()
		day := time.Now().In(loc).Format("2006-01-02")
		for res := range results {
			n := d.NodeByID(res.id)
			if n == nil {
				continue
			}
			if res.err != nil {
				if n.LastError == "" {
					d.AddEvent("bad", "Нет связи с сервером «"+n.Name+"»: "+res.err.Error(), "")
					a.notify("🔴 Нет связи с сервером «" + n.Name + "»: " + res.err.Error())
				}
				n.LastError = res.err.Error()
				continue
			}
			if n.LastError != "" {
				d.AddEvent("ok", "Связь с сервером «"+n.Name+"» восстановлена", "")
			}
			ni := res.ni
			n.LastError = ""
			n.LastSync = now
			n.PingMs = res.ping
			updated := n.Version != "" && n.Version != ni.Version
			if ni.ProfileHash != "" {
				n.RemoteHash = ni.ProfileHash
				switch {
				case n.Replaced:
					// a new server under the old key: wait for «Восстановить» or «Сохранить»
				case n.ProfileHash == "":
					refresh = append(refresh, n.ID)
				case ni.ProfileHash != n.ProfileHash:
					if !n.Drift && (now-n.EditedAt < 15*60*1000 || updated) {
						// changed from this panel (or the server was updated): save again
						refresh = append(refresh, n.ID)
					} else if !n.Drift {
						n.Drift = true
						d.AddEvent("warn", "Конфигурация сервера «"+n.Name+"» отличается от сохранённой на основном — проверьте в «Связке серверов»", "")
						a.notify("⚠️ Конфигурация сервера «" + n.Name + "» отличается от сохранённой на основном. Откройте «Связку серверов»: сохранить новую или восстановить прежнюю.")
					}
				default:
					if n.Drift {
						n.Drift = false
						d.AddEvent("ok", "Конфигурация сервера «"+n.Name+"» снова совпадает с сохранённой", "")
					}
					if now-n.ProfileAt > 12*3600*1000 {
						refresh = append(refresh, n.ID) // keep renewed certificates in the copy
					}
				}
			}
			n.Version = ni.Version
			n.Xray = ni.Xray
			n.CPU = ni.CPU
			n.Clients = ni.Clients
			if ni.Country != "" {
				n.Country = ni.Country
			}
			if ni.Name != "" && n.Name == "" {
				n.Name = ni.Name
			}
			old, _ := json.Marshal(n.Inbounds)
			oldIDs := n.InboundIDs()
			n.Inbounds = ni.Inbounds
			for i := range n.Inbounds {
				n.Inbounds[i].ID = n.ID + ":" + n.Inbounds[i].ID
			}
			nw, _ := json.Marshal(n.Inbounds)
			if !bytes.Equal(old, nw) {
				statusChanged = true
			}
			if a.grantNodeInbounds(d, n, oldIDs, n.Drift || n.Replaced) {
				statusChanged = true
				resync = true
			}
			for cid, tr := range ni.Traffic {
				if c := d.ClientByID(cid); c != nil {
					c.Up += tr.Up
					c.Down += tr.Down
					c.TotalUp += tr.Up
					c.TotalDown += tr.Down
					d.Daily[day] += tr.Up + tr.Down
					if c.TrafficLimit > 0 && c.Used() >= c.TrafficLimit {
						statusChanged = true
					}
				}
			}
			for _, cid := range ni.Online {
				if c := d.ClientByID(cid); c != nil {
					c.LastOnline = now
				}
			}
		}
		return nil
	})
	if statusChanged {
		a.ScheduleApply()
	}
	if resync {
		// grants changed — push the per-client inbound lists right away
		go a.SyncNodes()
	}
	for _, id := range refresh {
		go func(id string) { _ = a.fetchNodeProfile(id) }(id)
	}
}

// grantNodeInbounds keeps client grants in step with a linked server's inbound
// list: on the first sync every client gets all of its inbounds (that is how
// linking worked before per-client choice); later a new inbound goes to the
// clients that already use that server, and removed inbounds are dropped.
// keep: the server does not match its saved configuration — grants for
// inbounds missing there are kept, so a restore brings them back intact.
func (a *App) grantNodeInbounds(d *Data, n *Node, oldIDs []string, keep bool) bool {
	ids := n.InboundIDs()
	changed := false
	if !n.Granted {
		if len(ids) == 0 {
			return false
		}
		for _, c := range d.Clients {
			if c.Origin == "main" {
				continue
			}
			for _, id := range ids {
				if !c.HasInbound(id) {
					c.Inbounds = append(c.Inbounds, id)
				}
			}
		}
		n.Granted = true
		d.AddEvent("info", "Инбаунды сервера «"+n.Name+"» выданы всем клиентам — доступ можно менять в карточке клиента", "")
		return true
	}
	prefix := n.ID + ":"
	// inbounds from the saved configuration are not new — after a restore on a
	// fresh server they come back with the same ids and keep the old access
	known := append([]string{}, oldIDs...)
	if n.Profile != nil {
		for _, in := range n.Profile.Inbounds {
			known = append(known, prefix+in.ID)
		}
	}
	for _, id := range ids {
		if containsStr(known, id) {
			continue
		}
		for _, c := range d.Clients {
			if c.Origin == "main" || c.HasInbound(id) {
				continue
			}
			uses := false
			for _, x := range c.Inbounds {
				if strings.HasPrefix(x, prefix) {
					uses = true
					break
				}
			}
			if uses {
				c.Inbounds = append(c.Inbounds, id)
				changed = true
			}
		}
	}
	for _, id := range oldIDs {
		if !keep && !containsStr(ids, id) {
			d.dropNodeInbounds(n.ID, ids)
			changed = true
			break
		}
	}
	return changed
}

func (a *App) apiNodes(w http.ResponseWriter, r *http.Request) error {
	res := map[string]any{}
	a.Store.Read(func(d *Data) {
		list := []map[string]any{}
		var lastSync int64
		offline := 0
		synced := 0
		for _, c := range d.Clients {
			if c.Origin != "main" {
				synced++
			}
		}
		for _, n := range d.Nodes {
			if n.LastSync > lastSync {
				lastSync = n.LastSync
			}
			if n.Enabled && n.LastError != "" {
				offline++
			}
			list = append(list, map[string]any{"id": n.ID, "name": n.Name, "url": n.URL, "enabled": n.Enabled, "country": n.Country,
				"version": n.Version, "clients": n.Clients, "cpu": n.CPU, "pingMs": n.PingMs, "lastSync": n.LastSync, "lastError": n.LastError,
				"inbounds": len(n.Inbounds), "addedAt": n.AddedAt, "xray": n.Xray, "drift": n.Drift, "replaced": n.Replaced,
				"profileAt": n.ProfileAt, "hasProfile": n.Profile != nil, "profileOk": n.RemoteHash != ""})
		}
		res["nodes"] = list
		res["self"] = map[string]any{"name": d.Settings.NodeName, "country": d.Settings.CountryCode, "clients": len(d.Clients), "inbounds": len(d.Inbounds), "url": a.panelURLLocked(d)}
		res["main"] = d.Main
		res["lastSync"] = lastSync
		res["offline"] = offline
		res["synced"] = synced
		res["linkKey"] = base64.RawURLEncoding.EncodeToString([]byte(a.panelURLLocked(d) + "|" + d.Settings.APIToken))
	})
	ok(w, res)
	return nil
}

func (a *App) apiNodeTest(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	u, tok, err := parseNodeKey(req.URL, req.Token)
	if err != nil {
		return err
	}
	var ni nodeInfo
	start := time.Now()
	if err := nodeCall("GET", u, tok, "api/node/info", nil, &ni); err != nil {
		return errf(502, "%v", err)
	}
	ok(w, map[string]any{"info": ni, "pingMs": time.Since(start).Milliseconds(), "url": u})
	return nil
}

func (a *App) apiNodeAdd(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		URL     string `json:"url"`
		Token   string `json:"token"`
		Name    string `json:"name"`
		Country string `json:"country"` // empty — as set on the server
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	cc, err := normCountry(req.Country)
	if err != nil {
		return err
	}
	u, tok, err := parseNodeKey(req.URL, req.Token)
	if err != nil {
		return err
	}
	var ni nodeInfo
	if err := nodeCall("GET", u, tok, "api/node/info", nil, &ni); err != nil {
		return errf(502, "Сервер не отвечает: %v", err)
	}
	var selfToken string
	a.Store.Read(func(d *Data) { selfToken = d.Settings.APIToken })
	if tok == selfToken {
		return errf(400, "Это этот же сервер")
	}
	n := &Node{ID: RandomHex(5), Name: orDefault(strings.TrimSpace(req.Name), ni.Name), URL: u, Token: tok, Enabled: true, Country: ni.Country,
		Version: ni.Version, AddedAt: nowMs()}
	err = a.Store.Update(func(d *Data) error {
		for _, x := range d.Nodes {
			if x.URL == u {
				return errf(409, "Этот сервер уже в связке")
			}
		}
		d.Nodes = append(d.Nodes, n)
		d.AddEvent("ok", "Сервер «"+n.Name+"» добавлен в связку", "")
		return nil
	})
	if err != nil {
		return err
	}
	syncMu.Lock()
	a.syncNodesNow()
	syncMu.Unlock()
	_ = a.fetchNodeProfile(n.ID)
	res := map[string]any{"id": n.ID}
	if cc != "" && cc != ni.Country {
		if wn := a.setNodeCountry(n.ID, u, tok, cc); wn != "" {
			res["warning"] = wn
		}
	}
	ok(w, res)
	return nil
}

func (a *App) apiNodesSync(w http.ResponseWriter, r *http.Request) error {
	syncMu.Lock()
	a.syncNodesNow()
	syncMu.Unlock()
	return a.apiNodes(w, r)
}

func (a *App) apiNodeUpdate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
		Token   *string `json:"token"`
		URL     *string `json:"url"`
		Country *string `json:"country"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	cc := ""
	if req.Country != nil {
		var err error
		if cc, err = normCountry(*req.Country); err != nil {
			return err
		}
	}
	id := r.PathValue("id")
	var curURL, curTok, selfTok string
	a.Store.Read(func(d *Data) {
		if n := d.NodeByID(id); n != nil {
			curURL, curTok = n.URL, n.Token
		}
		selfTok = d.Settings.APIToken
	})
	if curURL == "" {
		return errf(404, "Сервер не найден")
	}
	// a new key / address / token: possibly a different server (the old one was
	// banned) — check that it answers before switching to it
	newURL, newTok := curURL, curTok
	if req.URL != nil && strings.TrimSpace(*req.URL) != "" {
		t := ""
		if req.Token != nil {
			t = strings.TrimSpace(*req.Token)
		}
		if t == "" && strings.HasPrefix(strings.TrimSpace(*req.URL), "http") {
			t = curTok
		}
		u, tk, err := parseNodeKey(*req.URL, t)
		if err != nil {
			return err
		}
		newURL, newTok = u, tk
	} else if req.Token != nil && strings.TrimSpace(*req.Token) != "" {
		newTok = strings.TrimSpace(*req.Token)
	}
	switched := newURL != curURL || newTok != curTok
	if switched {
		if newTok == selfTok {
			return errf(400, "Это этот же сервер")
		}
		var ni nodeInfo
		if err := nodeCall("GET", newURL, newTok, "api/node/info", nil, &ni); err != nil {
			return errf(502, "Сервер не отвечает: %v", err)
		}
	}
	replaced := false
	err := a.Store.Update(func(d *Data) error {
		n := d.NodeByID(id)
		if n == nil {
			return errf(404, "Сервер не найден")
		}
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			n.Name = strings.TrimSpace(*req.Name)
		}
		if req.Enabled != nil {
			n.Enabled = *req.Enabled
		}
		if switched {
			for _, x := range d.Nodes {
				if x.ID != n.ID && x.URL == newURL {
					return errf(409, "Этот сервер уже в связке как «%s»", x.Name)
				}
			}
			if newURL != n.URL && n.Profile != nil {
				// another machine: keep the saved configuration until it is restored there
				n.Replaced = true
				d.AddEvent("warn", "Сервер «"+n.Name+"» переключён на "+newURL+" — восстановите на нём сохранённую конфигурацию", "")
			}
			n.URL, n.Token, n.LastError, n.EditedAt = newURL, newTok, "", 0
		}
		replaced = n.Replaced
		return nil
	})
	if err != nil {
		return err
	}
	res := map[string]any{"ok": true, "replaced": replaced}
	if req.Country != nil {
		if wn := a.setNodeCountry(id, newURL, newTok, cc); wn != "" {
			res["warning"] = wn
		}
	}
	go a.SyncNodes()
	ok(w, res)
	return nil
}

// normCountry: "de" → "DE"; empty is allowed (no flag).
func normCountry(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	if len(s) != 2 || s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' {
		return "", errf(400, "Код страны — две латинские буквы, например DE, NL, FI")
	}
	return s, nil
}

// setNodeCountry saves the country code on the linked server itself (it is the
// source of truth: sync takes it from there) and in the local copy.
func (a *App) setNodeCountry(id, url, token, cc string) string {
	a.Store.Update(func(d *Data) error {
		if n := d.NodeByID(id); n != nil {
			n.Country = cc
		}
		return nil
	})
	if err := nodeCall("PUT", url, token, "api/settings", map[string]any{"countryCode": cc}, nil); err != nil {
		return "Страна сохранена здесь, но сервер не принял её (" + err.Error() + ") — после синхронизации может вернуться прежняя"
	}
	return ""
}

func (a *App) apiNodeDelete(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var n *Node
	err := a.Store.Update(func(d *Data) error {
		for i, x := range d.Nodes {
			if x.ID == id {
				n = x
				d.Nodes = append(d.Nodes[:i], d.Nodes[i+1:]...)
				d.dropNodeInbounds(x.ID, nil)
				d.AddEvent("info", "Сервер «"+x.Name+"» отвязан", "")
				return nil
			}
		}
		return errf(404, "Сервер не найден")
	})
	if err != nil {
		return err
	}
	// tell the node to drop our clients (best effort; the server itself is not touched otherwise)
	go func() {
		_ = nodeCall("POST", n.URL, n.Token, "api/node/sync", syncPayload{MainName: "", MainURL: "", Clients: []syncClient{}}, nil)
	}()
	go func() {
		time.Sleep(3 * time.Second)
		_ = nodeCall("POST", n.URL, n.Token, "api/main/detach", map[string]any{}, nil)
	}()
	ok(w, map[string]any{"ok": true})
	return nil
}
