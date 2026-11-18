package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type clientView struct {
	*Client
	Status  string   `json:"status"`
	Online  bool     `json:"online"`
	Devices int      `json:"devices"`
	IPs     []string `json:"ips"`
	SubURL  string   `json:"subUrl"`
	App     string   `json:"app"`
}

func (a *App) viewClient(d *Data, c *Client, now int64, ips map[string][]string, subBase string) clientView {
	cc := *c
	v := clientView{Client: &cc, Status: c.Status(now), Online: now-c.LastOnline < 150*1000}
	v.IPs = ips[c.Name]
	if v.IPs == nil {
		v.IPs = []string{}
	}
	v.Devices = len(v.IPs)
	v.SubURL = subBase + c.SubID
	if c.LastSubUA != "" {
		v.App = appName(c.LastSubUA)
	}
	return v
}

func (a *App) onlineIPs() map[string][]string {
	a.rt.Lock()
	defer a.rt.Unlock()
	return a.rt.onlineIPs
}

func (a *App) apiClients(w http.ResponseWriter, r *http.Request) error {
	now := nowMs()
	ips := a.onlineIPs()
	subBase := a.subBaseURL()
	var list []clientView
	a.Store.Read(func(d *Data) {
		list = make([]clientView, 0, len(d.Clients))
		for i := len(d.Clients) - 1; i >= 0; i-- {
			list = append(list, a.viewClient(d, d.Clients[i], now, ips, subBase))
		}
	})
	ok(w, list)
	return nil
}

type clientReq struct {
	Name        *string   `json:"name"`
	UUID        *string   `json:"uuid"`
	SubID       *string   `json:"subId"`
	Days        *float64  `json:"days"`
	ExpiresAt   *int64    `json:"expiresAt"`
	TrafficGB   *float64  `json:"trafficGb"`
	DeviceLimit *int      `json:"deviceLimit"`
	Telegram    *string   `json:"telegram"`
	Note        *string   `json:"note"`
	Tags        []string  `json:"tags"`
	Inbounds    *[]string `json:"inbounds"`
	ResetPeriod *string   `json:"resetPeriod"`
	ResetDays   *int      `json:"resetDays"`
}

func validName(n string) error {
	if n == "" || len([]rune(n)) > 64 {
		return errf(400, "Имя клиента: от 1 до 64 символов")
	}
	if strings.ContainsAny(n, "<>\"'`\\\n\r\t:") {
		return errf(400, "Имя клиента не должно содержать символы < > \" ' ` \\ :")
	}
	return nil
}

func autoName(d *Data) string {
	for i := len(d.Clients) + 1; ; i++ {
		n := fmt.Sprintf("client%d", i)
		if d.ClientByName(n) == nil {
			return n
		}
	}
}

func (a *App) applyClientReq(d *Data, c *Client, req clientReq, isNew bool) error {
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" && isNew {
			n = autoName(d)
		}
		if err := validName(n); err != nil {
			return err
		}
		if o := d.ClientByName(n); o != nil && o.ID != c.ID {
			return errf(409, "Клиент с именем «%s» уже есть", n)
		}
		c.Name = n
	}
	if req.UUID != nil && strings.TrimSpace(*req.UUID) != "" {
		u := strings.ToLower(strings.TrimSpace(*req.UUID))
		if !IsUUID(u) {
			return errf(400, "Некорректный UUID")
		}
		for _, o := range d.Clients {
			if o.UUID == u && o.ID != c.ID {
				return errf(409, "UUID уже используется клиентом %s", o.Name)
			}
		}
		c.UUID = u
	}
	if req.SubID != nil && strings.TrimSpace(*req.SubID) != "" {
		s := strings.TrimSpace(*req.SubID)
		for _, o := range d.Clients {
			if o.SubID == s && o.ID != c.ID {
				return errf(409, "Такой ID подписки уже занят")
			}
		}
		c.SubID = s
	}
	if req.ExpiresAt != nil {
		c.ExpiresAt = *req.ExpiresAt
	} else if req.Days != nil {
		if *req.Days <= 0 {
			c.ExpiresAt = 0
		} else {
			c.ExpiresAt = time.Now().Add(time.Duration(*req.Days * float64(24*time.Hour))).UnixMilli()
		}
	}
	if req.TrafficGB != nil {
		if *req.TrafficGB <= 0 {
			c.TrafficLimit = 0
		} else {
			c.TrafficLimit = int64(*req.TrafficGB * 1024 * 1024 * 1024)
		}
	}
	if req.DeviceLimit != nil {
		if *req.DeviceLimit < 0 {
			return errf(400, "Лимит устройств не может быть отрицательным")
		}
		c.DeviceLimit = *req.DeviceLimit
	}
	if req.Telegram != nil {
		c.Telegram = strings.TrimSpace(*req.Telegram)
	}
	if req.Note != nil {
		c.Note = strings.TrimSpace(*req.Note)
	}
	if req.Tags != nil {
		c.Tags = uniqStrings(req.Tags)
	}
	if req.Inbounds != nil {
		ids := []string{}
		for _, id := range *req.Inbounds {
			if d.ValidInboundID(id) {
				ids = append(ids, id)
			}
		}
		c.Inbounds = uniqStrings(ids)
	}
	if req.ResetPeriod != nil {
		switch *req.ResetPeriod {
		case "", "hour", "day", "week", "month", "mday", "anniv", "days":
			c.ResetPeriod = *req.ResetPeriod
		default:
			return errf(400, "Неизвестный период сброса")
		}
		if c.ResetPeriod != "" && c.LastReset == 0 {
			c.LastReset = nowMs()
		}
	}
	if req.ResetDays != nil {
		c.ResetDays = *req.ResetDays
	}
	return nil
}

func newClient(d *Data) *Client {
	c := &Client{
		ID: RandomHex(8), UUID: NewUUID(), SubID: RandomHex(8), Password: RandomString(16),
		Tags: []string{}, Inbounds: []string{}, CreatedAt: nowMs(), ResetPeriod: d.Settings.NewResetPeriod, ResetDays: d.Settings.NewResetDays,
	}
	if c.ResetPeriod != "" {
		c.LastReset = c.CreatedAt
	}
	return c
}

func (a *App) apiClientCreate(w http.ResponseWriter, r *http.Request) error {
	var req clientReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	var view clientView
	err := a.Store.Update(func(d *Data) error {
		c := newClient(d)
		if req.Name == nil {
			empty := ""
			req.Name = &empty
		}
		if req.Inbounds == nil {
			all := d.DefaultGrant()
			req.Inbounds = &all
		}
		if err := a.applyClientReq(d, c, req, true); err != nil {
			return err
		}
		d.Clients = append(d.Clients, c)
		days := "бессрочно"
		if c.ExpiresAt > 0 {
			days = fmt.Sprintf("на %d дн.", int((c.ExpiresAt-c.CreatedAt+3600000)/86400000))
		}
		d.AddEvent("ok", fmt.Sprintf("%s создан %s", c.Name, days), c.Name)
		view = a.viewClient(d, c, nowMs(), nil, a.subBaseURLLocked(d))
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, view)
	return nil
}

func (a *App) apiClientGet(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var res map[string]any
	ips := a.onlineIPs()
	a.Store.Read(func(d *Data) {
		c := d.ClientByID(id)
		if c == nil {
			c = d.ClientByName(id)
		}
		if c == nil {
			return
		}
		v := a.viewClient(d, c, nowMs(), ips, a.subBaseURLLocked(d))
		res = map[string]any{"client": v, "links": a.clientLinks(d, c, true)}
	})
	if res == nil {
		return errf(404, "Клиент не найден")
	}
	ok(w, res)
	return nil
}

func (a *App) apiClientUpdate(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var req clientReq
	if err := readJSON(r, &req); err != nil {
		return err
	}
	var view clientView
	err := a.Store.Update(func(d *Data) error {
		c := d.ClientByID(id)
		if c == nil {
			return errf(404, "Клиент не найден")
		}
		if c.Origin != "" {
			return errf(409, "Клиент управляется основным сервером связки — измените его там")
		}
		old := c.Name
		if err := a.applyClientReq(d, c, req, false); err != nil {
			return err
		}
		if old != c.Name {
			d.AddEvent("info", fmt.Sprintf("%s переименован в %s", old, c.Name), c.Name)
		}
		view = a.viewClient(d, c, nowMs(), nil, a.subBaseURLLocked(d))
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, view)
	return nil
}

func (a *App) apiClientDelete(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	err := a.Store.Update(func(d *Data) error {
		for i, c := range d.Clients {
			if c.ID == id {
				d.Clients = append(d.Clients[:i], d.Clients[i+1:]...)
				d.AddEvent("warn", c.Name+" удалён", "")
				return nil
			}
		}
		return errf(404, "Клиент не найден")
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func clientAction(d *Data, c *Client, action string, days float64, tag string) error {
	now := nowMs()
	switch action {
	case "reset-traffic":
		c.Up, c.Down = 0, 0
		c.LastReset = now
		d.AddEvent("info", c.Name+": трафик сброшен", c.Name)
	case "ban":
		c.Banned = true
		d.AddEvent("warn", c.Name+" забанен", c.Name)
	case "unban":
		c.Banned = false
		d.AddEvent("ok", c.Name+" разбанен", c.Name)
	case "freeze":
		if !c.Frozen {
			c.Frozen = true
			c.FrozenAt = now
			d.AddEvent("info", c.Name+" заморожен", c.Name)
		}
	case "unfreeze":
		if c.Frozen {
			if c.ExpiresAt > 0 && c.FrozenAt > 0 {
				c.ExpiresAt += now - c.FrozenAt
			}
			c.Frozen = false
			c.FrozenAt = 0
			d.AddEvent("ok", c.Name+" разморожен — срок продлён на время заморозки", c.Name)
		}
	case "extend":
		if days == 0 {
			return errf(400, "Укажите число дней")
		}
		base := c.ExpiresAt
		if base == 0 {
			return nil // бессрочный — продлевать нечего
		}
		if base < now {
			base = now
		}
		c.ExpiresAt = base + int64(days*86400000)
		d.AddEvent("ok", fmt.Sprintf("%s продлён на %g дн.", c.Name, days), c.Name)
	case "set-days":
		if days <= 0 {
			c.ExpiresAt = 0
		} else {
			c.ExpiresAt = now + int64(days*86400000)
		}
	case "regen-uuid":
		c.UUID = NewUUID()
		c.Password = RandomString(16)
		d.AddEvent("warn", c.Name+": ключи перевыпущены — старые ссылки больше не работают", c.Name)
	case "regen-sub":
		c.SubID = RandomHex(8)
		d.AddEvent("info", c.Name+": новая ссылка подписки", c.Name)
	case "add-tag":
		if tag != "" && !containsStr(c.Tags, tag) {
			c.Tags = append(c.Tags, tag)
		}
	case "remove-tag":
		c.Tags = removeStr(c.Tags, tag)
	case "grant-all":
		for _, in := range d.Inbounds {
			if !c.HasInbound(in.ID) {
				c.Inbounds = append(c.Inbounds, in.ID)
			}
		}
		for _, n := range d.Nodes {
			for _, id := range n.InboundIDs() {
				if !c.HasInbound(id) {
					c.Inbounds = append(c.Inbounds, id)
				}
			}
		}
	default:
		return errf(400, "Неизвестное действие")
	}
	return nil
}

// clientAccess gives or takes inbounds (of this server or linked ones).
func clientAccess(d *Data, c *Client, on bool, ids []string) error {
	if len(ids) == 0 {
		return errf(400, "Не выбран инбаунд")
	}
	known := map[string]bool{}
	for _, in := range d.Inbounds {
		known[in.ID] = true
	}
	for _, n := range d.Nodes {
		for _, id := range n.InboundIDs() {
			known[id] = true
		}
	}
	for _, id := range ids {
		if on {
			if !known[id] {
				return errf(404, "Инбаунд не найден — обновите страницу")
			}
			if !c.HasInbound(id) {
				c.Inbounds = append(c.Inbounds, id)
			}
		} else {
			c.Inbounds = removeStr(c.Inbounds, id)
		}
	}
	return nil
}

func (a *App) apiClientAction(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var req struct {
		Action  string   `json:"action"`
		Days    float64  `json:"days"`
		Tag     string   `json:"tag"`
		Inbound []string `json:"inbounds"` // grant / revoke: inbound ids («node:id» for linked servers)
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	err := a.Store.Update(func(d *Data) error {
		c := d.ClientByID(id)
		if c == nil {
			return errf(404, "Клиент не найден")
		}
		switch req.Action {
		case "grant", "revoke":
			return clientAccess(d, c, req.Action == "grant", req.Inbound)
		case "revoke-all":
			c.Inbounds = []string{}
			d.AddEvent("info", c.Name+": все инбаунды забраны", c.Name)
			return nil
		}
		return clientAction(d, c, req.Action, req.Days, req.Tag)
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiClientsBulk(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		Days   float64  `json:"days"`
		Tag    string   `json:"tag"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	n := 0
	err := a.Store.Update(func(d *Data) error {
		set := map[string]bool{}
		for _, id := range req.IDs {
			set[id] = true
		}
		if req.Action == "delete" {
			keep := d.Clients[:0]
			for _, c := range d.Clients {
				if set[c.ID] {
					n++
					continue
				}
				keep = append(keep, c)
			}
			d.Clients = keep
			d.AddEvent("warn", fmt.Sprintf("Удалено клиентов: %d", n), "")
			return nil
		}
		for _, c := range d.Clients {
			if set[c.ID] {
				if err := clientAction(d, c, req.Action, req.Days, req.Tag); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true, "count": n})
	return nil
}

// ---------- import / export ----------

type clientExport struct {
	Name         string   `json:"name"`
	UUID         string   `json:"uuid"`
	SubID        string   `json:"subId"`
	Password     string   `json:"password"`
	Telegram     string   `json:"telegram,omitempty"`
	Note         string   `json:"note,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	ExpiresAt    int64    `json:"expiresAt"`
	TrafficLimit int64    `json:"trafficLimit"`
	Used         int64    `json:"used"`
	DeviceLimit  int      `json:"deviceLimit"`
	Banned       bool     `json:"banned,omitempty"`
	CreatedAt    int64    `json:"createdAt"`
}

func (a *App) apiClientsExport(w http.ResponseWriter, r *http.Request) error {
	format := r.URL.Query().Get("format")
	var list []clientExport
	a.Store.Read(func(d *Data) {
		for _, c := range d.Clients {
			list = append(list, clientExport{c.Name, c.UUID, c.SubID, c.Password, c.Telegram, c.Note, c.Tags, c.ExpiresAt, c.TrafficLimit, c.Used(), c.DeviceLimit, c.Banned, c.CreatedAt})
		}
	})
	stamp := time.Now().Format("2006-01-02")
	if format == "csv" {
		var buf bytes.Buffer
		buf.WriteString("\ufeff")
		cw := csv.NewWriter(&buf)
		_ = cw.Write([]string{"name", "uuid", "subId", "expires", "trafficLimitGB", "usedGB", "deviceLimit", "telegram", "tags", "banned", "note"})
		for _, c := range list {
			exp := ""
			if c.ExpiresAt > 0 {
				exp = time.UnixMilli(c.ExpiresAt).In(moscow()).Format("2006-01-02 15:04")
			}
			_ = cw.Write([]string{c.Name, c.UUID, c.SubID, exp, gbStr(c.TrafficLimit), gbStr(c.Used), strconv.Itoa(c.DeviceLimit), c.Telegram, strings.Join(c.Tags, ";"), strconv.FormatBool(c.Banned), c.Note})
		}
		cw.Flush()
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="clients-`+stamp+`.csv"`)
		_, _ = w.Write(buf.Bytes())
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="clients-`+stamp+`.json"`)
	b, _ := json.MarshalIndent(list, "", "  ")
	_, _ = w.Write(b)
	return nil
}

func gbStr(b int64) string {
	if b == 0 {
		return "0"
	}
	return strconv.FormatFloat(float64(b)/(1<<30), 'f', 2, 64)
}

func (a *App) apiClientsImport(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Data string `json:"data"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	raw := strings.TrimSpace(strings.TrimPrefix(req.Data, "\ufeff"))
	var items []clientExport
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return errf(400, "Не удалось разобрать JSON: %v", err)
		}
	} else {
		rd := csv.NewReader(strings.NewReader(raw))
		rd.FieldsPerRecord = -1
		rows, err := rd.ReadAll()
		if err != nil || len(rows) < 2 {
			return errf(400, "Ожидается JSON-массив или CSV с заголовком")
		}
		head := map[string]int{}
		for i, h := range rows[0] {
			head[strings.ToLower(strings.TrimSpace(h))] = i
		}
		get := func(row []string, k string) string {
			if i, okk := head[strings.ToLower(k)]; okk && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		for _, row := range rows[1:] {
			it := clientExport{Name: get(row, "name"), UUID: get(row, "uuid"), SubID: get(row, "subId"), Telegram: get(row, "telegram"), Note: get(row, "note")}
			if t := get(row, "tags"); t != "" {
				it.Tags = strings.Split(t, ";")
			}
			if e := get(row, "expires"); e != "" {
				if tm, err := time.ParseInLocation("2006-01-02 15:04", e, moscow()); err == nil {
					it.ExpiresAt = tm.UnixMilli()
				} else if tm, err := time.ParseInLocation("2006-01-02", e, moscow()); err == nil {
					it.ExpiresAt = tm.UnixMilli()
				}
			}
			if g, err := strconv.ParseFloat(get(row, "trafficLimitGB"), 64); err == nil {
				it.TrafficLimit = int64(g * (1 << 30))
			}
			it.DeviceLimit, _ = strconv.Atoi(get(row, "deviceLimit"))
			it.Banned = get(row, "banned") == "true"
			items = append(items, it)
		}
	}
	added, skipped := 0, 0
	err := a.Store.Update(func(d *Data) error {
		all := d.DefaultGrant()
		for _, it := range items {
			if it.UUID != "" {
				dup := false
				for _, c := range d.Clients {
					if strings.EqualFold(c.UUID, it.UUID) {
						dup = true
						break
					}
				}
				if dup {
					skipped++
					continue
				}
			}
			c := newClient(d)
			if IsUUID(it.UUID) {
				c.UUID = strings.ToLower(it.UUID)
			}
			name := strings.TrimSpace(it.Name)
			if name == "" || validName(name) != nil {
				name = autoName(d)
			}
			for d.ClientByName(name) != nil {
				name = name + "-" + RandomString(3)
			}
			c.Name = name
			if it.SubID != "" && d.ClientBySub(it.SubID) == nil {
				c.SubID = it.SubID
			}
			if it.Password != "" {
				c.Password = it.Password
			}
			c.Telegram, c.Note, c.ExpiresAt, c.TrafficLimit, c.DeviceLimit, c.Banned = it.Telegram, it.Note, it.ExpiresAt, it.TrafficLimit, it.DeviceLimit, it.Banned
			if it.Tags != nil {
				c.Tags = uniqStrings(it.Tags)
			}
			c.Inbounds = append([]string{}, all...)
			d.Clients = append(d.Clients, c)
			added++
		}
		if added > 0 {
			d.AddEvent("ok", fmt.Sprintf("Импортировано клиентов: %d", added), "")
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	ok(w, map[string]any{"added": added, "skipped": skipped})
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
