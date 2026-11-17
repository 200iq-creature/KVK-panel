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
	ni := nodeInfo{Name: d.Settings.NodeName, Country: d.Settings.CountryCode, Version: Version, Clients: len(d.Clients), Inbounds: []NodeInbound{}}
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
			if c.UUID != sc.UUID || c.Name != name || c.Password != sc.Password || c.ExpiresAt != sc.ExpiresAt ||
				c.DeviceLimit != sc.DeviceLimit || c.Banned != !sc.Active || strings.Join(c.Inbounds, ",") != strings.Join(grant, ",") {
				changed = true
			}
			c.UUID, c.Name, c.SubID, c.Password, c.ExpiresAt, c.DeviceLimit = sc.UUID, name, sc.SubID, sc.Password, sc.ExpiresAt, sc.DeviceLimit
			c.Banned = !sc.Active
			c.Inbounds = append([]string{}, grant...)
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
	type target struct{ id, url, token string }
	var targets []target
	var payload syncPayload
	now := nowMs()
	a.Store.Read(func(d *Data) {
		for _, n := range d.Nodes {
			if n.Enabled {
				targets = append(targets, target{n.ID, n.URL, n.Token})
			}
		}
		if len(targets) == 0 {
			return
		}
		payload.MainName = d.Settings.NodeName
		payload.MainURL = a.panelURLLocked(d)
		for _, c := range d.Clients {
			if c.Origin == "main" {
				continue // don't re-export clients we got from another main
			}
			st := c.Status(now)
			payload.Clients = append(payload.Clients, syncClient{ID: c.ID, UUID: c.UUID, Name: c.Name, SubID: c.SubID, Password: c.Password,
				ExpiresAt: c.ExpiresAt, DeviceLimit: c.DeviceLimit, Active: st == StActive})
		}
	})
	if len(targets) == 0 {
		return
	}
	if payload.Clients == nil {
		payload.Clients = []syncClient{}
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
			err := nodeCall("POST", t.url, t.token, "api/node/sync", payload, &ni)
			results <- result{t.id, &ni, err, int(time.Since(start).Milliseconds())}
		}(t)
	}
	wg.Wait()
	close(results)
	statusChanged := false
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
			n.Version = ni.Version
			n.CPU = ni.CPU
			n.Clients = ni.Clients
			if ni.Country != "" {
				n.Country = ni.Country
			}
			if ni.Name != "" && n.Name == "" {
				n.Name = ni.Name
			}
			old, _ := json.Marshal(n.Inbounds)
			n.Inbounds = ni.Inbounds
			for i := range n.Inbounds {
				n.Inbounds[i].ID = n.ID + ":" + n.Inbounds[i].ID
			}
			nw, _ := json.Marshal(n.Inbounds)
			if !bytes.Equal(old, nw) {
				statusChanged = true
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
				"inbounds": len(n.Inbounds), "addedAt": n.AddedAt})
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
		URL   string `json:"url"`
		Token string `json:"token"`
		Name  string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
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
	ok(w, map[string]any{"id": n.ID})
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
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	id := r.PathValue("id")
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
		if req.Token != nil && *req.Token != "" {
			n.Token = strings.TrimSpace(*req.Token)
		}
		if req.URL != nil && *req.URL != "" {
			u, _, err := parseNodeKey(*req.URL, n.Token)
			if err != nil {
				return err
			}
			n.URL = u
		}
		return nil
	})
	if err != nil {
		return err
	}
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiNodeDelete(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var n *Node
	err := a.Store.Update(func(d *Data) error {
		for i, x := range d.Nodes {
			if x.ID == id {
				n = x
				d.Nodes = append(d.Nodes[:i], d.Nodes[i+1:]...)
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
