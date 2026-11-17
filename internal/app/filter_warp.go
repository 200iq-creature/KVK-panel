package app

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ================= filter =================

func (a *App) apiFilter(w http.ResponseWriter, r *http.Request) error {
	var f FilterSettings
	a.Store.Read(func(d *Data) { f = d.Filter })
	if f.Block == nil {
		f.Block = []string{}
	}
	if f.Allow == nil {
		f.Allow = []string{}
	}
	geo := a.geositeTags()
	avail := map[string]bool{}
	for _, t := range []string{"category-ads-all", "category-porn", "malware", "phishing", "cryptominers"} {
		avail[t] = geo == nil || geo[t]
	}
	ok(w, map[string]any{"filter": f, "doh": filterDoH(f), "lists": avail})
	return nil
}

func normalizeList(in []string) []string {
	out := []string{}
	for _, v := range in {
		for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' }) {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return uniqStrings(out)
}

func validateFilter(f *FilterSettings) error {
	switch f.Mode {
	case "dns", "lists", "custom":
	default:
		return errf(400, "Неизвестный режим фильтрации")
	}
	if f.Mode == "custom" {
		if !strings.HasPrefix(f.CustomDoH, "https://") {
			return errf(400, "Адрес DoH должен начинаться с https://")
		}
	}
	f.Block = normalizeList(f.Block)
	f.Allow = normalizeList(f.Allow)
	return nil
}

func (a *App) apiFilterSave(w http.ResponseWriter, r *http.Request) error {
	var f FilterSettings
	if err := readJSON(r, &f); err != nil {
		return err
	}
	if err := validateFilter(&f); err != nil {
		return err
	}
	// validate with xray before saving
	var cfg M
	a.Store.Read(func(d *Data) { cfg = a.buildConfigFrom(d, &f, true) })
	if a.Xray.Installed() {
		b, _ := json.Marshal(cfg)
		if err := a.Xray.Test(b); err != nil {
			return errf(400, "%v", err)
		}
	}
	var prev FilterSettings
	a.Store.Update(func(d *Data) error {
		prev = d.Filter
		d.Filter = f
		if f.Enabled {
			d.AddEvent("ok", "Фильтрация обновлена", "")
		} else {
			d.AddEvent("info", "Фильтрация выключена", "")
		}
		return nil
	})
	if err := a.Xray.Apply(); err != nil && a.Xray.Installed() {
		a.Store.Update(func(d *Data) error { d.Filter = prev; return nil })
		_ = a.Xray.Apply()
		return errf(400, "Xray не принял настройки: %v", err)
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

// dnsWireQuery builds a minimal DNS A query.
func dnsWireQuery(name string) []byte {
	var b bytes.Buffer
	id := make([]byte, 2)
	_, _ = rand.Read(id)
	b.Write(id)
	b.Write([]byte{1, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	for _, l := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		b.WriteByte(byte(len(l)))
		b.WriteString(l)
	}
	b.WriteByte(0)
	b.Write([]byte{0, 1, 0, 1})
	return b.Bytes()
}

// parseDNSAnswer returns rcode and A records.
func parseDNSAnswer(msg []byte) (int, []string, error) {
	if len(msg) < 12 {
		return 0, nil, fmt.Errorf("короткий ответ")
	}
	rcode := int(msg[3] & 0x0f)
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))
	i := 12
	skipName := func() {
		for i < len(msg) {
			l := int(msg[i])
			if l == 0 {
				i++
				return
			}
			if l&0xc0 == 0xc0 {
				i += 2
				return
			}
			i += l + 1
		}
	}
	for q := 0; q < qd; q++ {
		skipName()
		i += 4
	}
	ips := []string{}
	for k := 0; k < an && i < len(msg); k++ {
		skipName()
		if i+10 > len(msg) {
			break
		}
		typ := binary.BigEndian.Uint16(msg[i:])
		rdl := int(binary.BigEndian.Uint16(msg[i+8:]))
		i += 10
		if i+rdl > len(msg) {
			break
		}
		if typ == 1 && rdl == 4 {
			ips = append(ips, net.IP(msg[i:i+4]).String())
		}
		i += rdl
	}
	return rcode, ips, nil
}

func dohResolve(doh, name string) (int, []string, error) {
	q := dnsWireQuery(name)
	c := &http.Client{Timeout: 6 * time.Second}
	req, _ := http.NewRequest("POST", doh, bytes.NewReader(q))
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 65535))
	return parseDNSAnswer(b)
}

func isBlockedAnswer(rcode int, ips []string) bool {
	if rcode == 3 {
		return true
	}
	if len(ips) == 0 {
		return true // NXDOMAIN/NODATA/REFUSED — filtered
	}
	for _, ip := range ips {
		if ip != "0.0.0.0" && ip != "127.0.0.1" && !strings.HasPrefix(ip, "94.140.14.3") {
			return false
		}
	}
	return true
}

func (a *App) apiFilterCheck(w http.ResponseWriter, r *http.Request) error {
	var f FilterSettings
	if err := readJSON(r, &f); err != nil {
		return err
	}
	if err := validateFilter(&f); err != nil {
		return err
	}
	f.Enabled = true
	res := map[string]any{}
	checks := []map[string]any{}
	var cfg M
	a.Store.Read(func(d *Data) { cfg = a.buildConfigFrom(d, &f, true) })
	b, _ := json.Marshal(cfg)
	if a.Xray.Installed() {
		if err := a.Xray.Test(b); err != nil {
			checks = append(checks, map[string]any{"name": "Конфиг Xray", "ok": false, "detail": err.Error()})
		} else {
			checks = append(checks, map[string]any{"name": "Конфиг Xray", "ok": true, "detail": "Xray принимает эти настройки"})
		}
	}
	if f.Mode == "dns" || f.Mode == "custom" {
		doh := filterDoH(f)
		probes := []struct {
			name, domain string
			want         bool
		}{
			{"Обычный сайт", "example.com", false},
		}
		if f.Ads {
			probes = append(probes, struct {
				name, domain string
				want         bool
			}{"Реклама", "doubleclick.net", true})
		}
		if f.Adult {
			probes = append(probes, struct {
				name, domain string
				want         bool
			}{"Сайты 18+", "pornhub.com", true})
		}
		if f.Security && !f.Ads && !f.Adult {
			probes = append(probes, struct {
				name, domain string
				want         bool
			}{"Фишинг (тест)", "malware.testcategory.com", true})
		}
		for _, p := range probes {
			rc, ips, err := dohResolve(doh, p.domain)
			if err != nil {
				checks = append(checks, map[string]any{"name": p.name, "ok": false, "detail": "DNS-сервер недоступен: " + err.Error()})
				continue
			}
			blocked := isBlockedAnswer(rc, ips)
			detail := p.domain + " → " + strings.Join(ips, ", ")
			if blocked {
				detail = p.domain + " → заблокирован"
			}
			checks = append(checks, map[string]any{"name": p.name, "ok": blocked == p.want, "detail": detail})
		}
		res["doh"] = doh
	} else {
		geo := a.geositeTags()
		for _, t := range []struct {
			on   bool
			tag  string
			name string
		}{{f.Ads, "category-ads-all", "Список рекламы"}, {f.Adult, "category-porn", "Список 18+"}, {f.Security, "malware", "Список вредоносных"}} {
			if !t.on {
				continue
			}
			okk := geo == nil || geo[t.tag]
			d := "geosite:" + t.tag + " найден в базе"
			if !okk {
				d = "В текущей базе geosite нет тега " + t.tag + " — смените источник баз в «Настройках»"
			}
			checks = append(checks, map[string]any{"name": t.name, "ok": okk, "detail": d})
		}
	}
	res["checks"] = checks
	ok(w, res)
	return nil
}

// ================= WARP =================

const warpAPI = "https://api.cloudflareclient.com/v0a2158"

func warpDo(method, path, token string, body any) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, warpAPI+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.10-2158")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare недоступен: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var res map[string]any
	_ = json.Unmarshal(b, &res)
	if resp.StatusCode >= 300 {
		msg := resp.Status
		if errs, okk := res["errors"].([]any); okk && len(errs) > 0 {
			if e, okk := errs[0].(map[string]any); okk {
				msg = fmt.Sprint(e["message"])
			}
		}
		return res, fmt.Errorf("Cloudflare: %s", msg)
	}
	return res, nil
}

func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, okk := cur.(map[string]any)
		if !okk {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func digStr(m map[string]any, keys ...string) string {
	v := dig(m, keys...)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func (a *App) apiWarp(w http.ResponseWriter, r *http.Request) error {
	var ws WarpState
	a.Store.Read(func(d *Data) { ws = d.Warp })
	res := map[string]any{
		"registered": ws.ID != "", "enabled": ws.Enabled, "mode": ws.Mode, "sites": ws.Sites, "port": ws.Port,
		"ipv6First": ws.IPv6First, "fallbackOn": ws.FallbackOn, "type": ws.Type, "address4": ws.Address4, "address6": ws.Address6,
		"license": maskToken(ws.License), "createdAt": ws.CreatedAt, "ports": []int{2408, 500, 1701, 4500},
	}
	if res["sites"] == nil || len(ws.Sites) == 0 {
		res["sites"] = []string{}
	}
	if ws.ID != "" && r.URL.Query().Get("refresh") == "1" {
		if acc, err := warpDo("GET", "/reg/"+ws.ID+"/account", ws.Token, nil); err == nil {
			res["account"] = map[string]any{
				"type": acc["account_type"], "premium": acc["premium_data"], "quota": acc["quota"], "warpPlus": acc["warp_plus"],
			}
			if t := fmt.Sprint(acc["account_type"]); t != "" && t != "<nil>" {
				a.Store.Update(func(d *Data) error { d.Warp.Type = t; return nil })
				res["type"] = t
			}
		} else {
			res["accountError"] = err.Error()
		}
	}
	ok(w, res)
	return nil
}

func (a *App) apiWarpRegister(w http.ResponseWriter, r *http.Request) error {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	priv := base64.StdEncoding.EncodeToString(k.Bytes())
	pub := base64.StdEncoding.EncodeToString(k.PublicKey().Bytes())
	body := map[string]any{
		"install_id": "", "fcm_token": "", "tos": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"key": pub, "type": "Android", "model": "PC", "locale": "en_US",
	}
	res, err := warpDo("POST", "/reg", "", body)
	if err != nil {
		return errf(502, "%v", err)
	}
	id := digStr(res, "id")
	token := digStr(res, "token")
	cfgm, _ := res["config"].(map[string]any)
	if id == "" || token == "" || cfgm == nil {
		return errf(502, "Cloudflare вернул неожиданный ответ")
	}
	peerPub := ""
	if peers, okk := cfgm["peers"].([]any); okk && len(peers) > 0 {
		if p, okk := peers[0].(map[string]any); okk {
			peerPub = fmt.Sprint(p["public_key"])
		}
	}
	v4 := digStr(cfgm, "interface", "addresses", "v4")
	v6 := digStr(cfgm, "interface", "addresses", "v6")
	var reserved []int
	if cid := digStr(cfgm, "client_id"); cid != "" {
		if rb, err := base64.StdEncoding.DecodeString(cid); err == nil && len(rb) >= 3 {
			reserved = []int{int(rb[0]), int(rb[1]), int(rb[2])}
		}
	}
	acType := digStr(res, "account", "account_type")
	a.Store.Update(func(d *Data) error {
		mode, port, sites := d.Warp.Mode, d.Warp.Port, d.Warp.Sites
		d.Warp = WarpState{ID: id, Token: token, PrivateKey: priv, PeerPublic: peerPub, Address4: v4, Address6: v6,
			Reserved: reserved, Type: acType, Mode: orDefault(mode, "sites"), Port: port, Sites: sites, CreatedAt: nowMs()}
		if d.Warp.Port == 0 {
			d.Warp.Port = 2408
		}
		d.AddEvent("ok", "Зарегистрирован аккаунт Cloudflare WARP", "")
		return nil
	})
	var req struct {
		License string `json:"license"`
	}
	_ = readJSON(r, &req)
	if strings.TrimSpace(req.License) != "" {
		if err := a.applyWarpLicense(strings.TrimSpace(req.License)); err != nil {
			ok(w, map[string]any{"ok": true, "licenseError": err.Error()})
			return nil
		}
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) applyWarpLicense(lic string) error {
	var ws WarpState
	a.Store.Read(func(d *Data) { ws = d.Warp })
	if ws.ID == "" {
		return errf(400, "Сначала зарегистрируйте WARP")
	}
	if _, err := warpDo("PUT", "/reg/"+ws.ID+"/account", ws.Token, map[string]any{"license": lic}); err != nil {
		return errf(400, "Ключ не принят: %v", err)
	}
	acc, _ := warpDo("GET", "/reg/"+ws.ID+"/account", ws.Token, nil)
	a.Store.Update(func(d *Data) error {
		d.Warp.License = lic
		if acc != nil {
			d.Warp.Type = digStr(acc, "account_type")
		}
		d.AddEvent("ok", "Ключ WARP+ применён", "")
		return nil
	})
	return nil
}

func (a *App) apiWarpLicense(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		License string `json:"license"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len(strings.TrimSpace(req.License)) < 8 {
		return errf(400, "Введите ключ WARP+")
	}
	if err := a.applyWarpLicense(strings.TrimSpace(req.License)); err != nil {
		return err
	}
	a.ScheduleApply()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiWarpSave(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Enabled    bool     `json:"enabled"`
		Mode       string   `json:"mode"`
		Sites      []string `json:"sites"`
		Port       int      `json:"port"`
		IPv6First  bool     `json:"ipv6First"`
		FallbackOn bool     `json:"fallbackOn"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	switch req.Mode {
	case "all", "sites", "fallback":
	default:
		return errf(400, "Неизвестный режим")
	}
	switch req.Port {
	case 2408, 500, 1701, 4500:
	default:
		return errf(400, "Порт WARP: 2408, 500, 1701 или 4500")
	}
	sites := normalizeList(req.Sites)
	if req.Mode == "sites" && req.Enabled && len(sites) == 0 {
		return errf(400, "Добавьте хотя бы один сайт или список")
	}
	var prev WarpState
	err := a.Store.Update(func(d *Data) error {
		if d.Warp.ID == "" && req.Enabled {
			return errf(400, "Сначала зарегистрируйте WARP")
		}
		prev = d.Warp
		// fallbackOn is driven by the connectivity probe, not by the user
		fb := req.Mode == "fallback" && prev.Mode == "fallback" && prev.FallbackOn
		d.Warp.Enabled, d.Warp.Mode, d.Warp.Sites, d.Warp.Port, d.Warp.IPv6First, d.Warp.FallbackOn = req.Enabled, req.Mode, sites, req.Port, req.IPv6First, fb
		if req.Enabled {
			d.AddEvent("ok", "WARP включён ("+req.Mode+")", "")
		} else {
			d.AddEvent("info", "WARP выключен", "")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := a.Xray.Apply(); err != nil && a.Xray.Installed() {
		a.Store.Update(func(d *Data) error { d.Warp = prev; return nil })
		_ = a.Xray.Apply()
		return errf(400, "Xray не принял настройки WARP: %v", err)
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiWarpDelete(w http.ResponseWriter, r *http.Request) error {
	var ws WarpState
	a.Store.Read(func(d *Data) { ws = d.Warp })
	if ws.ID != "" {
		_, _ = warpDo("DELETE", "/reg/"+ws.ID, ws.Token, nil)
	}
	a.Store.Update(func(d *Data) error {
		d.Warp = WarpState{Mode: "sites", Port: 2408, Sites: d.Warp.Sites}
		d.AddEvent("warn", "Аккаунт WARP удалён", "")
		return nil
	})
	a.ScheduleApply()
	ok(w, map[string]any{"ok": true})
	return nil
}

// ---------- WARP fallback mode ----------
// In "fallback" mode traffic goes directly while the server's own exit works;
// after 3 failed probes in a row everything is switched to WARP, and back to
// direct after 5 successful probes.

var warpFB struct {
	mu         sync.Mutex
	running    bool
	fails, oks int
}

func directExitWorks() bool {
	c := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}}
	for _, u := range []string{"https://www.gstatic.com/generate_204", "https://cp.cloudflare.com/generate_204", "https://www.apple.com/library/test/success.html"} {
		resp, err := c.Get(u)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return true
			}
		}
	}
	return false
}

func (a *App) warpFallbackTick() {
	var mode string
	var enabled, on, registered bool
	a.Store.Read(func(d *Data) {
		mode, enabled, on, registered = d.Warp.Mode, d.Warp.Enabled, d.Warp.FallbackOn, d.Warp.ID != ""
	})
	if mode != "fallback" || !enabled || !registered {
		return
	}
	warpFB.mu.Lock()
	if warpFB.running {
		warpFB.mu.Unlock()
		return
	}
	warpFB.running = true
	warpFB.mu.Unlock()
	defer func() { warpFB.mu.Lock(); warpFB.running = false; warpFB.mu.Unlock() }()

	// While WARP carries all traffic the probe still uses the host's own network
	// stack (the panel process is not routed through Xray), so it measures the direct exit.
	works := directExitWorks()
	warpFB.mu.Lock()
	if works {
		warpFB.oks++
		warpFB.fails = 0
	} else {
		warpFB.fails++
		warpFB.oks = 0
	}
	switchOn := !on && warpFB.fails >= 3
	switchOff := on && warpFB.oks >= 5
	warpFB.mu.Unlock()
	if !switchOn && !switchOff {
		return
	}
	a.Store.Update(func(d *Data) error {
		d.Warp.FallbackOn = switchOn
		if switchOn {
			d.AddEvent("warn", "Прямой выход сервера не отвечает — трафик переключён на WARP", "")
		} else {
			d.AddEvent("ok", "Прямой выход сервера снова работает — WARP отключён", "")
		}
		return nil
	})
	if switchOn {
		a.notify("⚠️ Прямой выход сервера не отвечает — трафик клиентов переключён на WARP")
	}
	a.ScheduleApply()
}
