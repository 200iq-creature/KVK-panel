package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) apiSecurity(w http.ResponseWriter, r *http.Request) error {
	res := map[string]any{}
	a.Store.Read(func(d *Data) {
		res["totp"] = d.Admin.TOTPEnabled
		res["telegram"] = d.Admin.TgEnabled
		res["tgBotToken"] = maskToken(d.Settings.TgBotToken)
		res["tgConfigured"] = d.Settings.TgBotToken != "" && d.Settings.TgAdminID != ""
		res["tgAdminId"] = d.Settings.TgAdminID
		res["tgNotify"] = d.Settings.TgNotify
		res["https"] = d.Settings.PanelCertID != ""
		if dm := d.DomainByID(d.Settings.PanelCertID); dm != nil {
			res["httpsDomain"] = dm.Name
			res["httpsExpires"] = dm.ExpiresAt
		}
		logs := make([]LoginEntry, len(d.Logins))
		for i := range d.Logins {
			logs[i] = d.Logins[len(d.Logins)-1-i]
		}
		res["logins"] = logs
	})
	res["secure"] = r.TLS != nil
	res["recoveryFile"] = filepath.Join(a.DataDir, "2fa_off")
	ok(w, res)
	return nil
}

func maskToken(t string) string {
	if len(t) < 12 {
		if t == "" {
			return ""
		}
		return "••••"
	}
	return t[:6] + "••••" + t[len(t)-4:]
}

func (a *App) apiTOTPSetup(w http.ResponseWriter, r *http.Request) error {
	secret := newTOTPSecret()
	var user, node string
	a.Store.Update(func(d *Data) error {
		d.Admin.TOTPPending = secret
		user = d.Admin.Username
		node = d.Settings.NodeName
		return nil
	})
	ok(w, map[string]any{"secret": secret, "uri": totpURI(secret, user, "KVK Panel "+node)})
	return nil
}

func (a *App) apiTOTPEnable(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	err := a.Store.Update(func(d *Data) error {
		if d.Admin.TOTPPending == "" {
			return errf(400, "Сначала начните подключение заново")
		}
		if !totpVerify(d.Admin.TOTPPending, req.Code) {
			return errf(400, "Код не подошёл — проверьте время на телефоне и попробуйте ещё раз")
		}
		d.Admin.TOTPSecret = d.Admin.TOTPPending
		d.Admin.TOTPPending = ""
		d.Admin.TOTPEnabled = true
		d.AddEvent("ok", "Включена 2FA через приложение-аутентификатор", "")
		return nil
	})
	if err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiTOTPDisable(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	err := a.Store.Update(func(d *Data) error {
		if d.Admin.TOTPEnabled && !totpVerify(d.Admin.TOTPSecret, req.Code) {
			return errf(400, "Неверный код из приложения")
		}
		d.Admin.TOTPEnabled = false
		d.Admin.TOTPSecret = ""
		d.AddEvent("warn", "2FA через приложение выключена", "")
		return nil
	})
	if err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiTelegramSave(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Token   *string `json:"token"`
		AdminID *string `json:"adminId"`
		Enabled *bool   `json:"enabled"`
		Notify  *bool   `json:"notify"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	err := a.Store.Update(func(d *Data) error {
		if req.Token != nil && !strings.Contains(*req.Token, "••••") {
			d.Settings.TgBotToken = strings.TrimSpace(*req.Token)
		}
		if req.AdminID != nil {
			d.Settings.TgAdminID = strings.TrimSpace(*req.AdminID)
		}
		if req.Notify != nil {
			d.Settings.TgNotify = *req.Notify
		}
		if req.Enabled != nil {
			if *req.Enabled && (d.Settings.TgBotToken == "" || d.Settings.TgAdminID == "") {
				return errf(400, "Укажите токен бота и ваш Telegram ID")
			}
			d.Admin.TgEnabled = *req.Enabled
			if *req.Enabled {
				d.AddEvent("ok", "Включено подтверждение входа в Telegram", "")
			} else {
				d.AddEvent("warn", "Подтверждение входа в Telegram выключено", "")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiTelegramTest(w http.ResponseWriter, r *http.Request) error {
	if err := a.tgSend("✅ KVK Panel: бот подключён. Сюда будут приходить коды входа и уведомления."); err != nil {
		return errf(502, "%v", err)
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) tgSend(text string) error {
	var token, chat string
	a.Store.Read(func(d *Data) { token, chat = d.Settings.TgBotToken, d.Settings.TgAdminID })
	if token == "" || chat == "" {
		return fmt.Errorf("бот не настроен")
	}
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.PostForm("https://api.telegram.org/bot"+token+"/sendMessage", url.Values{"chat_id": {chat}, "text": {text}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var res struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(b, &res)
	if !res.OK {
		if res.Description == "" {
			res.Description = resp.Status
		}
		return fmt.Errorf("%s", res.Description)
	}
	return nil
}

// notify sends a Telegram notification if enabled.
// It never blocks and is safe to call while the store is locked.
func (a *App) notify(text string) {
	go func() {
		var on bool
		a.Store.Read(func(d *Data) { on = d.Settings.TgNotify && d.Settings.TgBotToken != "" && d.Settings.TgAdminID != "" })
		if on {
			_ = a.tgSend(text)
		}
	}()
}

// check2FARecovery disables 2FA if the recovery file exists.
func (a *App) check2FARecovery() {
	p := filepath.Join(a.DataDir, "2fa_off")
	if !fileExists(p) {
		return
	}
	_ = os.Remove(p)
	a.Store.Update(func(d *Data) error {
		d.Admin.TOTPEnabled = false
		d.Admin.TOTPSecret = ""
		d.Admin.TgEnabled = false
		d.AddEvent("warn", "2FA отключена через файл восстановления", "")
		return nil
	})
}
