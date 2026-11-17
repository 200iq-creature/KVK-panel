package app

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type migClient struct {
	UUID, Name, SubID, Password, Telegram, Note string
	ExpiresAt                                   int64 // 0 = none
	TrafficLimit                                int64
	DeviceLimit                                 int
	Disabled                                    bool
	Used                                        int64
}

type migReq struct {
	Source   string   `json:"source"` // 3xui | remnawave
	URL      string   `json:"url"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	Token    string   `json:"token"`
	Inbounds []string `json:"inbounds"` // local inbound ids to grant; empty = all enabled
	Insecure bool     `json:"insecure"`
}

var migState struct {
	sync.Mutex
	Running  bool     `json:"running"`
	Stage    string   `json:"stage"`
	Total    int      `json:"total"`
	Added    int      `json:"added"`
	Skipped  int      `json:"skipped"`
	Banned   int      `json:"banned"`
	Error    string   `json:"error"`
	Finished int64    `json:"finished"`
	Log      []string `json:"log"`
}

func migHTTP(insecure bool) *http.Client {
	jar, _ := cookiejar.New(nil)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Timeout: 30 * time.Second, Jar: jar, Transport: tr}
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// fetch3xui logs into 3x-ui and returns all clients from all inbounds.
func fetch3xui(req migReq) ([]migClient, error) {
	base := strings.TrimRight(strings.TrimSpace(req.URL), "/")
	if !strings.HasPrefix(base, "http") {
		return nil, errf(400, "Адрес должен начинаться с http:// или https://")
	}
	hc := migHTTP(req.Insecure)
	form := url.Values{"username": {req.Username}, "password": {req.Password}}
	resp, err := hc.PostForm(base+"/login", form)
	if err != nil {
		return nil, errf(502, "Панель 3x-ui недоступна: %v", err)
	}
	b, _ := readBody(resp)
	var lr struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if json.Unmarshal(b, &lr) != nil {
		return nil, errf(502, "По этому адресу не 3x-ui (проверьте секретный путь панели)")
	}
	if !lr.Success {
		return nil, errf(401, "3x-ui не пустила: %s", orDefault(lr.Msg, "неверный логин или пароль"))
	}
	var list struct {
		Success bool `json:"success"`
		Msg     string
		Obj     []struct {
			ID          int    `json:"id"`
			Remark      string `json:"remark"`
			Protocol    string `json:"protocol"`
			Settings    string `json:"settings"`
			Enable      bool   `json:"enable"`
			ClientStats []struct {
				Email      string `json:"email"`
				Up         int64  `json:"up"`
				Down       int64  `json:"down"`
				Total      int64  `json:"total"`
				ExpiryTime int64  `json:"expiryTime"`
				Enable     bool   `json:"enable"`
			} `json:"clientStats"`
		} `json:"obj"`
	}
	var lb []byte
	for _, p := range []string{"/panel/api/inbounds/list", "/xui/API/inbounds/list"} {
		resp, err := hc.Get(base + p)
		if err != nil {
			return nil, errf(502, "%v", err)
		}
		lb, _ = readBody(resp)
		if resp.StatusCode == 200 && json.Unmarshal(lb, &list) == nil && list.Success {
			break
		}
	}
	if !list.Success {
		return nil, errf(502, "3x-ui не отдала список инбаундов")
	}
	seen := map[string]bool{}
	out := []migClient{}
	for _, in := range list.Obj {
		var st struct {
			Clients []struct {
				ID         string `json:"id"`
				Password   string `json:"password"`
				Email      string `json:"email"`
				TotalGB    int64  `json:"totalGB"`
				ExpiryTime int64  `json:"expiryTime"`
				Enable     *bool  `json:"enable"`
				SubID      string `json:"subId"`
				TgID       any    `json:"tgId"`
				LimitIP    int    `json:"limitIp"`
				Comment    string `json:"comment"`
			} `json:"clients"`
		}
		_ = json.Unmarshal([]byte(in.Settings), &st)
		for _, c := range st.Clients {
			id := c.ID
			if id == "" || !IsUUID(id) {
				id = ""
			}
			key := strings.ToLower(orDefault(id, c.Email))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			mc := migClient{UUID: id, Name: c.Email, SubID: c.SubID, Password: c.Password, TrafficLimit: c.TotalGB, DeviceLimit: c.LimitIP, Note: c.Comment}
			if c.TgID != nil {
				t := fmt.Sprint(c.TgID)
				if t != "0" && t != "" {
					mc.Telegram = t
				}
			}
			if c.ExpiryTime > 0 {
				mc.ExpiresAt = c.ExpiryTime
			} else if c.ExpiryTime < 0 {
				// "start after first use": -duration in ms
				mc.ExpiresAt = time.Now().UnixMilli() - c.ExpiryTime
			}
			mc.Disabled = c.Enable != nil && !*c.Enable || !in.Enable
			for _, cs := range in.ClientStats {
				if cs.Email == c.Email {
					mc.Used = cs.Up + cs.Down
				}
			}
			out = append(out, mc)
		}
	}
	return out, nil
}

func fetchRemnawave(req migReq) ([]migClient, error) {
	base := strings.TrimRight(strings.TrimSpace(req.URL), "/")
	if !strings.HasPrefix(base, "http") {
		return nil, errf(400, "Адрес должен начинаться с https://")
	}
	if strings.TrimSpace(req.Token) == "" {
		return nil, errf(400, "Нужен API-токен Remnawave (Настройки → API-токены)")
	}
	hc := migHTTP(req.Insecure)
	out := []migClient{}
	start, size := 0, 500
	for {
		r, _ := http.NewRequest("GET", base+"/api/users?size="+strconv.Itoa(size)+"&start="+strconv.Itoa(start), nil)
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.Token))
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		resp, err := hc.Do(r)
		if err != nil {
			return nil, errf(502, "Remnawave недоступна: %v", err)
		}
		b, _ := readBody(resp)
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return nil, errf(401, "Remnawave отклонила токен")
		}
		if resp.StatusCode != 200 {
			return nil, errf(502, "Remnawave ответила %d", resp.StatusCode)
		}
		var res struct {
			Response struct {
				Users []struct {
					UUID              string `json:"uuid"`
					ShortUUID         string `json:"shortUuid"`
					Username          string `json:"username"`
					Status            string `json:"status"`
					UsedTrafficBytes  any    `json:"usedTrafficBytes"`
					TrafficLimitBytes any    `json:"trafficLimitBytes"`
					ExpireAt          string `json:"expireAt"`
					VlessUUID         string `json:"vlessUuid"`
					TrojanPassword    string `json:"trojanPassword"`
					TelegramID        any    `json:"telegramId"`
					Description       string `json:"description"`
					HwidDeviceLimit   any    `json:"hwidDeviceLimit"`
					UserTraffic       *struct {
						UsedTrafficBytes any `json:"usedTrafficBytes"`
					} `json:"userTraffic"`
				} `json:"users"`
				Total int `json:"total"`
			} `json:"response"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return nil, errf(502, "Неожиданный ответ Remnawave")
		}
		num := func(v any) int64 {
			switch x := v.(type) {
			case float64:
				return int64(x)
			case string:
				n, _ := strconv.ParseInt(x, 10, 64)
				return n
			}
			return 0
		}
		for _, u := range res.Response.Users {
			mc := migClient{UUID: orDefault(u.VlessUUID, u.UUID), Name: u.Username, SubID: u.ShortUUID, Password: u.TrojanPassword, Note: u.Description,
				TrafficLimit: num(u.TrafficLimitBytes), DeviceLimit: int(num(u.HwidDeviceLimit))}
			mc.Used = num(u.UsedTrafficBytes)
			if u.UserTraffic != nil {
				mc.Used = num(u.UserTraffic.UsedTrafficBytes)
			}
			if t := num(u.TelegramID); t > 0 {
				mc.Telegram = strconv.FormatInt(t, 10)
			}
			if tm, err := time.Parse(time.RFC3339, u.ExpireAt); err == nil {
				mc.ExpiresAt = tm.UnixMilli()
				if tm.Year() >= 2099 {
					mc.ExpiresAt = 0
				}
			}
			mc.Disabled = strings.EqualFold(u.Status, "DISABLED")
			out = append(out, mc)
		}
		start += len(res.Response.Users)
		if len(res.Response.Users) == 0 || start >= res.Response.Total {
			break
		}
	}
	return out, nil
}

func fetchMigration(req migReq) ([]migClient, error) {
	switch req.Source {
	case "3xui":
		return fetch3xui(req)
	case "remnawave":
		return fetchRemnawave(req)
	}
	return nil, errf(400, "Выберите источник")
}

func (a *App) apiMigrateTest(w http.ResponseWriter, r *http.Request) error {
	var req migReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	list, err := fetchMigration(req)
	if err != nil {
		return err
	}
	now := nowMs()
	var dup, expired, disabled, noExp int
	a.Store.Read(func(d *Data) {
		for _, c := range list {
			if a.migDuplicate(d, c) {
				dup++
				continue
			}
			if c.ExpiresAt > 0 && c.ExpiresAt < now {
				expired++
				continue
			}
			if c.Disabled {
				disabled++
			}
			if c.ExpiresAt == 0 {
				noExp++
			}
		}
	})
	ok(w, map[string]any{"total": len(list), "duplicates": dup, "expired": expired, "disabled": disabled, "noExpiry": noExp,
		"willAdd": len(list) - dup - expired})
	return nil
}

func (a *App) migDuplicate(d *Data, c migClient) bool {
	for _, x := range d.Clients {
		if c.UUID != "" && strings.EqualFold(x.UUID, c.UUID) {
			return true
		}
	}
	return false
}

func migLog(s string) {
	migState.Log = append(migState.Log, time.Now().In(moscow()).Format("15:04:05")+"  "+s)
	if len(migState.Log) > 200 {
		migState.Log = migState.Log[len(migState.Log)-200:]
	}
}

func (a *App) apiMigrateRun(w http.ResponseWriter, r *http.Request) error {
	var req migReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	migState.Lock()
	if migState.Running {
		migState.Unlock()
		return errf(409, "Перенос уже идёт")
	}
	migState.Running = true
	migState.Stage = "connect"
	migState.Total, migState.Added, migState.Skipped, migState.Banned = 0, 0, 0, 0
	migState.Error = ""
	migState.Log = nil
	migLog("Подключаемся к " + req.URL)
	migState.Unlock()
	go a.runMigration(req)
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) runMigration(req migReq) {
	finish := func(errText string) {
		migState.Lock()
		migState.Running = false
		migState.Finished = nowMs()
		if errText != "" {
			migState.Error = errText
			migState.Stage = "error"
			migLog("Ошибка: " + errText)
		} else {
			migState.Stage = "done"
			migLog(fmt.Sprintf("Готово: перенесено %d, пропущено %d", migState.Added, migState.Skipped))
		}
		migState.Unlock()
	}
	list, err := fetchMigration(req)
	if err != nil {
		finish(err.Error())
		return
	}
	migState.Lock()
	migState.Total = len(list)
	migState.Stage = "import"
	migLog(fmt.Sprintf("Найдено клиентов: %d", len(list)))
	migState.Unlock()

	now := nowMs()
	tenYears := time.Now().AddDate(10, 0, 0).UnixMilli()
	var added, skipped, banned int
	src := map[string]string{"3xui": "3x-ui", "remnawave": "Remnawave"}[req.Source]
	err = a.Store.Update(func(d *Data) error {
		grant := []string{}
		if len(req.Inbounds) > 0 {
			for _, id := range req.Inbounds {
				if d.InboundByID(id) != nil {
					grant = append(grant, id)
				}
			}
		} else {
			for _, in := range d.Inbounds {
				if in.Enabled {
					grant = append(grant, in.ID)
				}
			}
		}
		for _, mc := range list {
			if a.migDuplicate(d, mc) || (mc.ExpiresAt > 0 && mc.ExpiresAt < now) {
				skipped++
				continue
			}
			name := validMigName(mc.Name)
			if name == "" || d.ClientByName(name) != nil {
				base := orDefault(name, "client")
				for i := 2; ; i++ {
					cand := fmt.Sprintf("%s-%d", base, i)
					if d.ClientByName(cand) == nil {
						name = cand
						break
					}
				}
			}
			c := newClient(d)
			c.Name = name
			if mc.UUID != "" {
				c.UUID = strings.ToLower(mc.UUID)
			}
			if mc.SubID != "" && d.ClientBySub(mc.SubID) == nil && len(mc.SubID) >= 6 {
				c.SubID = mc.SubID
			}
			if mc.Password != "" {
				c.Password = mc.Password
			}
			c.Telegram, c.Note = mc.Telegram, mc.Note
			c.ExpiresAt = mc.ExpiresAt
			if c.ExpiresAt == 0 {
				c.ExpiresAt = tenYears
			}
			c.TrafficLimit = mc.TrafficLimit
			c.DeviceLimit = mc.DeviceLimit
			c.Banned = mc.Disabled
			c.Origin = src
			c.Tags = []string{src}
			c.Inbounds = append([]string{}, grant...)
			if c.Banned {
				banned++
			}
			d.Clients = append(d.Clients, c)
			added++
		}
		d.AddEvent("ok", fmt.Sprintf("Миграция из %s: перенесено %d, пропущено %d", src, added, skipped), "")
		return nil
	})
	migState.Lock()
	migState.Added, migState.Skipped, migState.Banned = added, skipped, banned
	migState.Unlock()
	if err != nil {
		finish(err.Error())
		return
	}
	if added > 0 {
		a.ScheduleApply()
		go a.SyncNodes()
	}
	finish("")
}

func validMigName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r == ' ' {
			r = '_'
		}
		if r < 32 || r == '/' || r == '\\' || r == '"' || r == '\'' {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len([]rune(out)) > 48 {
		out = string([]rune(out)[:48])
	}
	return out
}

func (a *App) apiMigrateStatus(w http.ResponseWriter, r *http.Request) error {
	migState.Lock()
	defer migState.Unlock()
	logs := migState.Log
	if logs == nil {
		logs = []string{}
	}
	ok(w, map[string]any{"running": migState.Running, "stage": migState.Stage, "total": migState.Total, "added": migState.Added,
		"skipped": migState.Skipped, "banned": migState.Banned, "error": migState.Error, "finished": migState.Finished, "log": logs})
	return nil
}

// apiMigrateExport exports clients either as 3x-ui inbound JSON or Remnawave-compatible users JSON.
func (a *App) apiMigrateExport(w http.ResponseWriter, r *http.Request) error {
	format := r.URL.Query().Get("format")
	var out any
	var fname string
	a.Store.Read(func(d *Data) {
		switch format {
		case "remnawave":
			users := []map[string]any{}
			for _, c := range d.Clients {
				st := "ACTIVE"
				if c.Banned {
					st = "DISABLED"
				}
				exp := time.UnixMilli(c.ExpiresAt).UTC().Format(time.RFC3339)
				if c.ExpiresAt == 0 {
					exp = "2099-12-31T00:00:00Z"
				}
				u := map[string]any{"username": c.Name, "vlessUuid": c.UUID, "trojanPassword": c.Password, "shortUuid": c.SubID,
					"status": st, "expireAt": exp, "trafficLimitBytes": c.TrafficLimit, "description": c.Note,
					"hwidDeviceLimit": c.DeviceLimit, "trafficLimitStrategy": "NO_RESET"}
				if c.Telegram != "" {
					if n, err := strconv.ParseInt(c.Telegram, 10, 64); err == nil {
						u["telegramId"] = n
					}
				}
				users = append(users, u)
			}
			out = map[string]any{"users": users}
			fname = "kvk-remnawave-users.json"
		default:
			inbounds := []map[string]any{}
			for _, in := range d.Inbounds {
				clients := []map[string]any{}
				for _, c := range d.Clients {
					if !c.HasInbound(in.ID) {
						continue
					}
					cl := map[string]any{"email": c.Name, "enable": !c.Banned, "expiryTime": c.ExpiresAt, "totalGB": c.TrafficLimit,
						"limitIp": c.DeviceLimit, "subId": c.SubID, "tgId": c.Telegram, "comment": c.Note, "reset": 0}
					switch in.Protocol {
					case "trojan":
						cl["password"] = c.Password
					case "shadowsocks":
						cl["password"] = ssUserKey(in.SSMethod, c.Password, in.ID)
						cl["method"] = ""
					case "vless":
						cl["id"] = c.UUID
						cl["flow"] = in.Flow
					default:
						cl["id"] = c.UUID
					}
					clients = append(clients, cl)
				}
				settings, _ := json.Marshal(map[string]any{"clients": clients, "decryption": "none"})
				inbounds = append(inbounds, map[string]any{"remark": in.Remark, "port": in.Port, "protocol": in.Protocol,
					"enable": in.Enabled, "settings": string(settings)})
			}
			out = map[string]any{"inbounds": inbounds}
			fname = "kvk-3xui-inbounds.json"
		}
	})
	b, _ := json.MarshalIndent(out, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)
	_, _ = w.Write(b)
	return nil
}
