package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (a *App) apiSettings(w http.ResponseWriter, r *http.Request) error {
	res := map[string]any{}
	a.Store.Read(func(d *Data) {
		s := d.Settings
		res["panelPort"] = s.PanelPort
		res["basePath"] = s.BasePath
		res["subPath"] = s.SubPath
		res["subPort"] = s.SubPort
		res["apiToken"] = s.APIToken
		res["nodeName"] = s.NodeName
		res["countryCode"] = s.CountryCode
		res["publicHost"] = s.PublicHost
		res["defaultFp"] = s.DefaultFP
		res["autoUpdate"] = s.AutoUpdate
		res["showTrafficAll"] = s.ShowTrafficAll
		res["newResetPeriod"] = s.NewResetPeriod
		res["newResetDays"] = s.NewResetDays
		res["autoCleanDays"] = s.AutoCleanDays
		res["username"] = d.Admin.Username
		res["panelUrl"] = a.panelURLLocked(d)
		res["apiUrl"] = a.panelURLLocked(d) + "api"
		res["latestVersion"] = s.LatestVersion
		res["lastUpdateCheck"] = s.LastUpdateCheck
		res["https"] = s.PanelCertID != ""
	})
	res["version"] = Version
	res["branch"] = Branch
	res["updateBranch"] = a.UpdateBranch()
	res["repo"] = RepoSlug
	ok(w, res)
	return nil
}

func (a *App) apiSettingsUpdate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		NodeName       *string `json:"nodeName"`
		CountryCode    *string `json:"countryCode"`
		DefaultFP      *string `json:"defaultFp"`
		AutoUpdate     *bool   `json:"autoUpdate"`
		ShowTrafficAll *bool   `json:"showTrafficAll"`
		NewResetPeriod *string `json:"newResetPeriod"`
		NewResetDays   *int    `json:"newResetDays"`
		AutoCleanDays  *int    `json:"autoCleanDays"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	err := a.Store.Update(func(d *Data) error {
		s := &d.Settings
		if req.NodeName != nil {
			n := strings.TrimSpace(*req.NodeName)
			if n == "" {
				return errf(400, "Название ноды не может быть пустым")
			}
			s.NodeName = n
		}
		if req.CountryCode != nil {
			s.CountryCode = strings.ToUpper(strings.TrimSpace(*req.CountryCode))
		}
		if req.DefaultFP != nil {
			switch *req.DefaultFP {
			case "chrome", "firefox", "safari", "ios", "edge", "android", "random", "randomized", "qq", "360":
				s.DefaultFP = *req.DefaultFP
			default:
				return errf(400, "Неизвестный отпечаток")
			}
		}
		if req.AutoUpdate != nil {
			s.AutoUpdate = *req.AutoUpdate
		}
		if req.ShowTrafficAll != nil {
			s.ShowTrafficAll = *req.ShowTrafficAll
		}
		if req.NewResetPeriod != nil {
			s.NewResetPeriod = *req.NewResetPeriod
		}
		if req.NewResetDays != nil {
			s.NewResetDays = *req.NewResetDays
		}
		if req.AutoCleanDays != nil {
			if *req.AutoCleanDays < 0 {
				return errf(400, "Некорректное число дней")
			}
			s.AutoCleanDays = *req.AutoCleanDays
		}
		return nil
	})
	if err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

var pathRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)

func (a *App) apiSettingsPanel(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Port     int    `json:"port"`
		BasePath string `json:"basePath"`
		SubPath  string `json:"subPath"`
		SubPort  *int   `json:"subPort"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	var newURL string
	changed := false
	err := a.Store.Update(func(d *Data) error {
		s := &d.Settings
		if req.Port != 0 && req.Port != s.PanelPort {
			if req.Port < 1 || req.Port > 65535 {
				return errf(400, "Порт должен быть от 1 до 65535")
			}
			for _, in := range d.Inbounds {
				if in.Port == req.Port {
					return errf(409, "Порт занят инбаундом «%s»", in.Remark)
				}
			}
			if !portFree(req.Port, "tcp") {
				return errf(409, "Порт %d занят другой программой", req.Port)
			}
			s.PanelPort = req.Port
			changed = true
		}
		if req.BasePath != "" && req.BasePath != s.BasePath {
			bp := strings.Trim(req.BasePath, "/")
			if !pathRe.MatchString(bp) {
				return errf(400, "Путь: 3–64 символа — латиница, цифры, - и _")
			}
			if bp == s.SubPath {
				return errf(400, "Путь панели совпадает с путём подписки")
			}
			s.BasePath = bp
			changed = true
		}
		if req.SubPath != "" && req.SubPath != s.SubPath {
			sp := strings.Trim(req.SubPath, "/")
			if !pathRe.MatchString(sp) && sp != "sub" {
				return errf(400, "Путь подписки: 3–64 символа — латиница, цифры, - и _")
			}
			if sp == s.BasePath {
				return errf(400, "Путь подписки совпадает с путём панели")
			}
			s.SubPath = sp
			changed = true
		}
		if req.SubPort != nil && *req.SubPort != s.SubPort {
			p := *req.SubPort
			if p != 0 {
				if p < 1 || p > 65535 || p == s.PanelPort {
					return errf(400, "Некорректный порт подписки")
				}
				if !portFree(p, "tcp") {
					return errf(409, "Порт %d занят", p)
				}
			}
			s.SubPort = p
			changed = true
		}
		if changed {
			d.AddEvent("warn", "Изменены порт/путь панели", "")
		}
		newURL = a.panelURLLocked(d)
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		a.Store.Read(func(d *Data) {
			OpenFirewallPort(d.Settings.PanelPort, "tcp")
			if d.Settings.SubPort > 0 {
				OpenFirewallPort(d.Settings.SubPort, "tcp")
			}
		})
		go func() {
			time.Sleep(800 * time.Millisecond)
			a.RequestPanelRestart()
		}()
	}
	ok(w, map[string]any{"ok": true, "url": newURL, "restart": changed})
	return nil
}

func (a *App) apiSettingsCredentials(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Current     string `json:"current"`
		Username    string `json:"username"`
		NewPassword string `json:"newPassword"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	var user string
	var ver int
	err := a.Store.Update(func(d *Data) error {
		if !CheckPassword(d.Admin.PassHash, req.Current) {
			return errf(403, "Текущий пароль неверный")
		}
		u := strings.TrimSpace(req.Username)
		if u != "" {
			if len(u) < 3 || len(u) > 32 || strings.ContainsAny(u, " \t") {
				return errf(400, "Логин: 3–32 символа без пробелов")
			}
			d.Admin.Username = u
		}
		if req.NewPassword != "" {
			if len(req.NewPassword) < 8 {
				return errf(400, "Пароль: минимум 8 символов")
			}
			d.Admin.PassHash = HashPassword(req.NewPassword)
		}
		d.Admin.SessionVersion++
		d.AddEvent("warn", "Изменены логин/пароль администратора", "")
		user, ver = d.Admin.Username, d.Admin.SessionVersion
		return nil
	})
	if err != nil {
		return err
	}
	a.setSessionCookie(w, r, user, ver)
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiSettingsToken(w http.ResponseWriter, r *http.Request) error {
	var tok string
	a.Store.Update(func(d *Data) error {
		d.Settings.APIToken = RandomHex(32)
		tok = d.Settings.APIToken
		d.AddEvent("warn", "API-токен перевыпущен — старый больше не действует", "")
		return nil
	})
	ok(w, map[string]any{"apiToken": tok})
	return nil
}

func (a *App) apiSettingsResetAll(w http.ResponseWriter, r *http.Request) error {
	n := 0
	a.Store.Update(func(d *Data) error {
		for _, c := range d.Clients {
			c.ResetPeriod = d.Settings.NewResetPeriod
			c.ResetDays = d.Settings.NewResetDays
			if c.ResetPeriod != "" && c.LastReset == 0 {
				c.LastReset = nowMs()
			}
			n++
		}
		d.AddEvent("info", fmt.Sprintf("Период авто-сброса применён ко всем клиентам (%d)", n), "")
		return nil
	})
	ok(w, map[string]any{"count": n})
	return nil
}

// ---------- panel self-update ----------

var branchRe = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._/-]*$`)

// UpdateBranch is the branch the installer will update from: the one it
// remembered in DATA/branch, otherwise the branch this binary was built from.
func (a *App) UpdateBranch() string {
	if b, err := os.ReadFile(filepath.Join(a.DataDir, "branch")); err == nil {
		if v := strings.TrimSpace(string(b)); branchRe.MatchString(v) {
			return v
		}
	}
	if branchRe.MatchString(Branch) {
		return Branch
	}
	return "main"
}

// fetchLatestPanelVersion reads the VERSION file of the branch the panel
// updates from (the installer builds that branch; releases are cut from main
// with the same VERSION).
func fetchLatestPanelVersion(branch string) (string, error) {
	vb, err := httpGetBytes("https://raw.githubusercontent.com/"+RepoSlug+"/"+branch+"/VERSION?t="+strconv.FormatInt(nowMs(), 10), 1024)
	if err != nil {
		// raw.githubusercontent.com is blocked in some networks — ask the API
		if jb, aerr := httpGetBytes("https://api.github.com/repos/"+RepoSlug+"/contents/VERSION?ref="+url.QueryEscape(branch), 64<<10); aerr == nil {
			var f struct {
				Content string `json:"content"`
			}
			if json.Unmarshal(jb, &f) == nil {
				if dec, derr := base64.StdEncoding.DecodeString(strings.ReplaceAll(f.Content, "\n", "")); derr == nil {
					vb, err = dec, nil
				}
			}
		}
	}
	if err == nil {
		if v := strings.TrimPrefix(strings.TrimSpace(string(vb)), "v"); v != "" {
			return v, nil
		}
		err = fmt.Errorf("пустой VERSION в ветке %s", branch)
	}
	if branch != "main" {
		return "", err
	}
	b, rerr := httpGetBytes("https://api.github.com/repos/"+RepoSlug+"/releases/latest", 1<<20)
	if rerr != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if jerr := json.Unmarshal(b, &rel); jerr != nil || rel.TagName == "" {
		return "", err
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

func versionNewer(latest, cur string) bool {
	pl := strings.Split(strings.TrimPrefix(latest, "v"), ".")
	pc := strings.Split(strings.TrimPrefix(cur, "v"), ".")
	for i := 0; i < 3; i++ {
		var a, b int
		if i < len(pl) {
			fmt.Sscanf(pl[i], "%d", &a)
		}
		if i < len(pc) {
			fmt.Sscanf(pc[i], "%d", &b)
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func (a *App) apiPanelUpdateCheck(w http.ResponseWriter, r *http.Request) error {
	branch := a.UpdateBranch()
	latest, err := fetchLatestPanelVersion(branch)
	if err != nil {
		return errf(502, "Не удалось проверить обновления (ветка %s): %v", branch, err)
	}
	a.Store.Update(func(d *Data) error {
		d.Settings.LatestVersion = latest
		d.Settings.LastUpdateCheck = nowMs()
		return nil
	})
	ok(w, map[string]any{"current": Version, "latest": latest, "branch": branch, "update": versionNewer(latest, Version)})
	return nil
}

// RunSelfUpdate launches the installer in update mode outside of the panel process.
func (a *App) RunSelfUpdate() error {
	branch := a.UpdateBranch()
	// the branch name is validated by branchRe, so it is safe inside the script
	script := "curl -fsSL https://raw.githubusercontent.com/" + RepoSlug + "/" + branch + "/install.sh -o /tmp/kvk-install.sh && bash /tmp/kvk-install.sh update " + branch
	if _, err := exec.LookPath("systemd-run"); err == nil {
		cmd := exec.Command("systemd-run", "--unit=kvk-panel-update-"+RandomString(5), "--no-block", "/bin/bash", "-c", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	cmd := exec.Command("/bin/bash", "-c", "nohup bash -c '"+script+"' >/tmp/kvk-update.log 2>&1 &")
	return cmd.Run()
}

func (a *App) apiPanelUpdate(w http.ResponseWriter, r *http.Request) error {
	if os.Geteuid() != 0 {
		return errf(400, "Обновление из панели требует запуска от root")
	}
	if err := a.RunSelfUpdate(); err != nil {
		return err
	}
	a.Store.Update(func(d *Data) error {
		d.AddEvent("info", "Запущено обновление панели", "")
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}

// ---------- subscription settings ----------

func (a *App) apiSubSettings(w http.ResponseWriter, r *http.Request) error {
	var res map[string]any
	a.Store.Read(func(d *Data) {
		ex := ""
		var exClient *Client
		if len(d.Clients) > 0 {
			exClient = d.Clients[0]
			ex = a.subBaseURLLocked(d) + exClient.SubID
		}
		res = map[string]any{"sub": d.Sub, "example": ex, "node": d.Settings.NodeName, "host": a.panelHostLocked(d), "subBase": a.subBaseURLLocked(d)}
		if exClient != nil {
			res["exampleClient"] = exClient.Name
		}
	})
	ok(w, res)
	return nil
}

func (a *App) apiSubSettingsUpdate(w http.ResponseWriter, r *http.Request) error {
	var req SubSettings
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if len([]rune(req.Description)) > 200 {
		return errf(400, "Описание: до 200 символов")
	}
	if req.UpdateHours < 0 || req.UpdateHours > 168 {
		return errf(400, "Интервал обновления: от 1 до 168 часов")
	}
	if strings.TrimSpace(req.JSONTemplate) != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(req.JSONTemplate), &m); err != nil {
			return errf(400, "Шаблон JSON некорректен: %v", err)
		}
	}
	req.AutoHosts = uniqStrings(req.AutoHosts)
	extCount := 0
	for _, l := range strings.Split(req.ExternalLinks, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if !strings.Contains(l, "://") {
			return errf(400, "Внешняя ссылка без схемы: %s", truncate(l, 40))
		}
		extCount++
	}
	if extCount > 50 {
		return errf(400, "Внешних ссылок: не больше 50")
	}
	if req.PageView != "classic" {
		req.PageView = "simple"
	}
	a.Store.Update(func(d *Data) error {
		d.Sub = req
		if d.Sub.NameFormat == "" {
			d.Sub.NameFormat = "{name} · {label}"
		}
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}
