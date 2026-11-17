package app

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ================= MTProto (mtg) process manager =================

type mtgManager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	lastErr string
	started time.Time
	cfgHash string
	logs    []string
}

var mtg = &mtgManager{}

func (a *App) mtgBin() string { return filepath.Join(a.BinDir, "mtg") }

func (a *App) mtgVersion() string {
	if !fileExists(a.mtgBin()) {
		return ""
	}
	out, err := runCmd(5*time.Second, a.mtgBin(), "--version")
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	if len(f) > 0 {
		return f[0]
	}
	return strings.TrimSpace(out)
}

func newMTSecret(domain string) string {
	return "ee" + RandomHex(16) + hex.EncodeToString([]byte(domain))
}

func (m *mtgManager) running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

func (m *mtgManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	m.cmd = nil
	m.cfgHash = ""
}

func (m *mtgManager) addLog(l string) {
	m.mu.Lock()
	m.logs = append(m.logs, l)
	if len(m.logs) > 100 {
		m.logs = m.logs[len(m.logs)-100:]
	}
	m.mu.Unlock()
}

// ensureMTG starts/restarts/stops mtg according to settings.
func (a *App) ensureMTG() {
	var tp TgProxySettings
	a.Store.Read(func(d *Data) { tp = d.Settings.TgProxy })
	if !tp.MTEnabled || tp.MTSecret == "" {
		mtg.stop()
		return
	}
	if !fileExists(a.mtgBin()) {
		mtg.mu.Lock()
		mtg.lastErr = "mtg не установлен — нажмите «Установить»"
		mtg.mu.Unlock()
		return
	}
	cfg := fmt.Sprintf("secret = %q\nbind-to = \"0.0.0.0:%d\"\n", tp.MTSecret, tp.MTPort)
	hash := cfg
	mtg.mu.Lock()
	if mtg.cmd != nil && mtg.cfgHash == hash {
		mtg.mu.Unlock()
		return
	}
	if mtg.cmd != nil && mtg.cmd.Process != nil {
		_ = mtg.cmd.Process.Kill()
		mtg.cmd = nil
	}
	mtg.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	p := filepath.Join(a.DataDir, "mtg.toml")
	if err := os.WriteFile(p, []byte(cfg), 0o600); err != nil {
		return
	}
	cmd := exec.Command(a.mtgBin(), "run", p)
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		mtg.mu.Lock()
		mtg.lastErr = err.Error()
		mtg.mu.Unlock()
		return
	}
	mtg.mu.Lock()
	mtg.cmd, mtg.cfgHash, mtg.started, mtg.lastErr = cmd, hash, time.Now(), ""
	mtg.mu.Unlock()
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			mtg.addLog(sc.Text())
		}
	}()
	go func() {
		err := cmd.Wait()
		mtg.mu.Lock()
		if mtg.cmd == cmd {
			mtg.cmd = nil
			mtg.cfgHash = ""
			msg := "mtg остановился"
			if err != nil {
				msg += ": " + err.Error()
			}
			if len(mtg.logs) > 0 {
				msg += " — " + mtg.logs[len(mtg.logs)-1]
			}
			mtg.lastErr = msg
			log.Print(msg)
		}
		mtg.mu.Unlock()
	}()
}

// tgProxyTick restarts mtg if it died and applies proxy-user expiry.
var lastProxyFP string

func (a *App) tgProxyTick() {
	a.ensureMTG()
	now := nowMs()
	var b strings.Builder
	a.Store.Read(func(d *Data) {
		for _, p := range d.ProxyUsers {
			if p.Active(now) {
				b.WriteString(p.ID + ",")
			}
		}
	})
	fp := b.String()
	if lastProxyFP != "" && fp != lastProxyFP {
		a.ScheduleApply()
	}
	lastProxyFP = fp
}

func mtgAssetURL() (string, string, error) {
	b, err := httpGetBytes("https://api.github.com/repos/9seconds/mtg/releases/latest", 2<<20)
	if err != nil {
		return "", "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(b, &rel); err != nil {
		return "", "", err
	}
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	for _, as := range rel.Assets {
		if strings.Contains(as.Name, "linux-"+arch+".tar.gz") {
			return as.URL, rel.TagName, nil
		}
	}
	return "", "", fmt.Errorf("нет сборки mtg для linux-%s", arch)
}

func (a *App) installMTG() error {
	u, tag, err := mtgAssetURL()
	if err != nil {
		return err
	}
	b, err := httpGetBytes(u, 64<<20)
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) == "mtg" && h.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(io.LimitReader(tr, 64<<20))
			if err != nil {
				return err
			}
			mtg.stop()
			tmp := a.mtgBin() + ".new"
			if err := os.WriteFile(tmp, data, 0o755); err != nil {
				return err
			}
			if err := os.Rename(tmp, a.mtgBin()); err != nil {
				return err
			}
			a.Store.Update(func(d *Data) error {
				d.AddEvent("ok", "Установлен mtg "+tag+" для MTProto-прокси", "")
				return nil
			})
			a.ensureMTG()
			return nil
		}
	}
	return fmt.Errorf("в архиве нет бинарника mtg")
}

// socksInbound returns the xray SOCKS5 inbound for Telegram-proxy users.
func (a *App) socksInbound(d *Data) M {
	tp := d.Settings.TgProxy
	if !tp.SocksEnabled {
		return nil
	}
	now := nowMs()
	accounts := []any{}
	for _, p := range d.ProxyUsers {
		if p.Active(now) {
			accounts = append(accounts, M{"user": p.Username, "pass": p.Password})
		}
	}
	if len(accounts) == 0 {
		return nil
	}
	return M{
		"tag": "tgproxy-socks", "listen": "0.0.0.0", "port": tp.SocksPort, "protocol": "socks",
		"settings": M{"auth": "password", "accounts": accounts, "udp": tp.SocksUDP},
		"sniffing": M{"enabled": true, "destOverride": []string{"http", "tls"}},
	}
}

func (a *App) proxyLinks(d *Data, p *ProxyUser) map[string]string {
	host := a.panelHostLocked(d)
	tp := d.Settings.TgProxy
	out := map[string]string{}
	if tp.SocksEnabled && p != nil {
		q := url.Values{"server": {host}, "port": {strconv.Itoa(tp.SocksPort)}, "user": {p.Username}, "pass": {p.Password}}
		out["socks"] = "https://t.me/socks?" + q.Encode()
		out["socksUri"] = "socks5://" + url.QueryEscape(p.Username) + ":" + url.QueryEscape(p.Password) + "@" + hostPort(host, tp.SocksPort, "")
	}
	if tp.MTEnabled && tp.MTSecret != "" {
		q := url.Values{"server": {host}, "port": {strconv.Itoa(tp.MTPort)}, "secret": {tp.MTSecret}}
		out["mtproto"] = "https://t.me/proxy?" + q.Encode()
	}
	return out
}

// ================= API =================

func (a *App) apiTgProxy(w http.ResponseWriter, r *http.Request) error {
	res := map[string]any{}
	now := nowMs()
	a.Store.Read(func(d *Data) {
		res["settings"] = d.Settings.TgProxy
		res["host"] = a.panelHostLocked(d)
		res["links"] = a.proxyLinks(d, nil)
		users := []map[string]any{}
		active := 0
		for i := len(d.ProxyUsers) - 1; i >= 0; i-- {
			p := d.ProxyUsers[i]
			if p.Active(now) {
				active++
			}
			users = append(users, map[string]any{"user": p, "active": p.Active(now), "links": a.proxyLinks(d, p)})
		}
		res["users"] = users
		res["active"] = active
	})
	mtg.mu.Lock()
	res["mtg"] = map[string]any{"running": mtg.cmd != nil, "error": mtg.lastErr, "uptime": func() int64 {
		if mtg.cmd == nil {
			return 0
		}
		return int64(time.Since(mtg.started).Seconds())
	}(), "log": append([]string{}, mtg.logs...)}
	mtg.mu.Unlock()
	res["mtgInstalled"] = fileExists(a.mtgBin())
	res["mtgVersion"] = a.mtgVersion()
	ok(w, res)
	return nil
}

func (a *App) apiTgProxySave(w http.ResponseWriter, r *http.Request) error {
	var req TgProxySettings
	if err := readJSON(r, &req); err != nil {
		return err
	}
	req.MTDomain = strings.ToLower(strings.TrimSpace(req.MTDomain))
	if req.MTDomain == "" || !domainRe.MatchString(req.MTDomain) {
		return errf(400, "Домен маскировки: например www.google.com")
	}
	var prev TgProxySettings
	err := a.Store.Update(func(d *Data) error {
		prev = d.Settings.TgProxy
		for _, chk := range []struct {
			on   bool
			port int
			id   string
			old  int
		}{{req.MTEnabled, req.MTPort, "tg-mtproto", prev.MTPort}, {req.SocksEnabled, req.SocksPort, "tg-socks", prev.SocksPort}} {
			if !chk.on {
				continue
			}
			if chk.port < 1 || chk.port > 65535 {
				return errf(400, "Некорректный порт %d", chk.port)
			}
			if why := portUsedByPanel(d, chk.port, chk.id); why != "" {
				return errf(409, "Порт %d: %s", chk.port, why)
			}
			if chk.port != chk.old && !portFree(chk.port, "tcp") {
				return errf(409, "Порт %d занят другой программой", chk.port)
			}
		}
		if req.MTEnabled && req.SocksEnabled && req.MTPort == req.SocksPort {
			return errf(409, "У MTProto и SOCKS5 должны быть разные порты")
		}
		secret := prev.MTSecret
		if secret == "" || prev.MTDomain != req.MTDomain {
			secret = newMTSecret(req.MTDomain)
		}
		req.MTSecret = secret
		d.Settings.TgProxy = req
		d.AddEvent("info", "Обновлены настройки Telegram-прокси", "")
		return nil
	})
	if err != nil {
		return err
	}
	if prev.SocksEnabled != req.SocksEnabled || prev.SocksPort != req.SocksPort || prev.SocksUDP != req.SocksUDP {
		if err := a.Xray.Apply(); err != nil && a.Xray.Installed() {
			a.Store.Update(func(d *Data) error { d.Settings.TgProxy = prev; return nil })
			_ = a.Xray.Apply()
			return errf(400, "Xray не принял SOCKS5: %v", err)
		}
	}
	if req.MTEnabled {
		go OpenFirewallPort(req.MTPort, "tcp")
	}
	if req.SocksEnabled {
		proto := "tcp"
		if req.SocksUDP {
			proto = "both"
		}
		go OpenFirewallPort(req.SocksPort, proto)
	}
	a.ensureMTG()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiTgProxySecret(w http.ResponseWriter, r *http.Request) error {
	a.Store.Update(func(d *Data) error {
		d.Settings.TgProxy.MTSecret = newMTSecret(d.Settings.TgProxy.MTDomain)
		d.AddEvent("warn", "Секрет MTProto перевыпущен — старые ссылки перестали работать", "")
		return nil
	})
	a.ensureMTG()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiTgProxyInstall(w http.ResponseWriter, r *http.Request) error {
	if err := a.installMTG(); err != nil {
		return errf(502, "Не удалось установить mtg: %v", err)
	}
	ok(w, map[string]any{"ok": true, "version": a.mtgVersion()})
	return nil
}

type proxyUserReq struct {
	Name      string  `json:"name"`
	Days      float64 `json:"days"`
	ExpiresAt *int64  `json:"expiresAt"`
	Enabled   *bool   `json:"enabled"`
	Note      string  `json:"note"`
	Regen     bool    `json:"regen"`
}

func (a *App) createProxyUser(d *Data, name string, days int, tgID int64) *ProxyUser {
	p := &ProxyUser{ID: RandomHex(6), Name: name, Username: "u" + RandomHex(4), Password: RandomString(14), Enabled: true, TgID: tgID, CreatedAt: nowMs()}
	if days > 0 {
		p.ExpiresAt = time.Now().Add(time.Duration(days) * 24 * time.Hour).UnixMilli()
	}
	d.ProxyUsers = append(d.ProxyUsers, p)
	return p
}

func (a *App) apiTgProxyUserCreate(w http.ResponseWriter, r *http.Request) error {
	var req proxyUserReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "proxy-" + RandomHex(3)
	}
	var id string
	a.Store.Update(func(d *Data) error {
		p := a.createProxyUser(d, name, int(req.Days), 0)
		p.Note = req.Note
		id = p.ID
		return nil
	})
	a.ScheduleApply()
	ok(w, map[string]any{"id": id})
	return nil
}

func (a *App) apiTgProxyUserUpdate(w http.ResponseWriter, r *http.Request) error {
	var req proxyUserReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	id := r.PathValue("id")
	err := a.Store.Update(func(d *Data) error {
		for _, p := range d.ProxyUsers {
			if p.ID != id {
				continue
			}
			if n := strings.TrimSpace(req.Name); n != "" {
				p.Name = n
			}
			p.Note = req.Note
			if req.Enabled != nil {
				p.Enabled = *req.Enabled
			}
			if req.ExpiresAt != nil {
				p.ExpiresAt = *req.ExpiresAt
			}
			if req.Days != 0 {
				base := time.Now().UnixMilli()
				if p.ExpiresAt > base {
					base = p.ExpiresAt
				}
				p.ExpiresAt = base + int64(req.Days*86400*1000)
			}
			if req.Regen {
				p.Password = RandomString(14)
			}
			return nil
		}
		return errf(404, "Доступ не найден")
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiTgProxyUserDelete(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	a.Store.Update(func(d *Data) error {
		for i, p := range d.ProxyUsers {
			if p.ID == id {
				d.ProxyUsers = append(d.ProxyUsers[:i], d.ProxyUsers[i+1:]...)
				break
			}
		}
		return nil
	})
	a.ScheduleApply()
	ok(w, map[string]any{"ok": true})
	return nil
}
