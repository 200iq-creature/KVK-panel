package app

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// jobs runs all periodic background work.
func (a *App) jobs() {
	statsT := time.NewTicker(10 * time.Second)
	onlineT := time.NewTicker(20 * time.Second)
	minuteT := time.NewTicker(time.Minute)
	defer statsT.Stop()
	defer onlineT.Stop()
	defer minuteT.Stop()
	lastActive := a.activeFingerprint()
	go func() {
		time.Sleep(90 * time.Second)
		a.renewCerts()
	}()
	lastCert := time.Now()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-statsT.C:
			a.pollStats()
		case <-onlineT.C:
			a.pollOnline()
		case <-minuteT.C:
			a.safe("expire", func() {
				a.resetPeriods()
				fp := a.activeFingerprint()
				if fp != lastActive {
					lastActive = fp
					a.ScheduleApply()
					go a.SyncNodes()
				}
			})
			a.safe("nodes", func() { a.SyncNodes() })
			a.safe("logs", a.expireLogs)
			a.safe("geo", a.geoSchedule)
			a.safe("clean", a.autoClean)
			a.safe("backup", a.autoBackup)
			a.safe("update", a.autoUpdateCheck)
			a.safe("remind", a.remindExpiring)
			a.safe("tgproxy", a.tgProxyTick)
			go a.safe("warp-fallback", a.warpFallbackTick)
			if time.Since(lastCert) > 30*time.Minute {
				lastCert = time.Now()
				go a.safe("certs", a.renewCerts)
			}
			a.Store.Flush()
		}
	}
}

func (a *App) safe(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("job %s panic: %v", name, r)
		}
	}()
	fn()
}

// activeFingerprint changes whenever the set of clients allowed in xray changes.
func (a *App) activeFingerprint() string {
	now := nowMs()
	var b strings.Builder
	a.Store.Read(func(d *Data) {
		for _, c := range d.Clients {
			if c.Status(now) == StActive {
				b.WriteString(c.ID)
				b.WriteByte(',')
			}
		}
	})
	return b.String()
}

func (a *App) pollStats() {
	items, err := a.Xray.QueryStats(true)
	if err != nil {
		a.rt.Lock()
		a.rt.statsErr = err.Error()
		a.rt.Unlock()
		return
	}
	if items == nil {
		return
	}
	type ud struct{ up, down int64 }
	users := map[string]*ud{}
	inbounds := map[string]*ud{}
	var rx, tx int64
	for _, it := range items {
		p := strings.Split(it.Name, ">>>")
		if len(p) != 4 || it.Value == 0 {
			continue
		}
		var m map[string]*ud
		switch p[0] {
		case "user":
			m = users
		case "inbound":
			if p[1] == "api" {
				continue
			}
			m = inbounds
		case "outbound":
			if p[1] == "api" {
				continue
			}
			if p[3] == "uplink" {
				tx += it.Value
			} else {
				rx += it.Value
			}
			continue
		default:
			continue
		}
		e := m[p[1]]
		if e == nil {
			e = &ud{}
			m[p[1]] = e
		}
		if p[3] == "uplink" {
			e.up += it.Value
		} else {
			e.down += it.Value
		}
	}
	now := time.Now()
	a.rt.Lock()
	el := now.Sub(a.rt.lastStatsAt).Seconds()
	if a.rt.lastStatsAt.IsZero() || el <= 0 || el > 60 {
		el = 10
	}
	a.rt.speedRx = float64(rx) / el
	a.rt.speedTx = float64(tx) / el
	a.rt.lastStatsAt = now
	a.rt.statsErr = ""
	a.rt.Unlock()
	if len(users) == 0 && len(inbounds) == 0 {
		return
	}
	limitHit := false
	day := now.In(moscow()).Format("2006-01-02")
	nowMilli := now.UnixMilli()
	var crossed []string
	a.Store.UpdateLazy(func(d *Data) {
		var total int64
		for name, v := range users {
			c := d.ClientByName(name)
			if c == nil {
				continue
			}
			wasActive := c.Status(nowMilli) == StActive
			c.Up += v.up
			c.Down += v.down
			c.TotalUp += v.up
			c.TotalDown += v.down
			c.LastOnline = nowMilli
			if wasActive && c.Status(nowMilli) == StLimited {
				limitHit = true
				crossed = append(crossed, c.Name)
				d.AddEvent("warn", "Клиент «"+c.Name+"» исчерпал лимит трафика", c.ID)
			}
		}
		for tag, v := range inbounds {
			if in := d.InboundByTag(tag); in != nil {
				in.Up += v.up
				in.Down += v.down
			}
			total += v.up + v.down
		}
		d.Daily[day] += total
		if len(d.Daily) > 400 {
			keys := sortedKeys(d.Daily)
			for _, k := range keys[:len(keys)-400] {
				delete(d.Daily, k)
			}
		}
	})
	if limitHit {
		a.ScheduleApply()
		go a.SyncNodes()
		for _, n := range crossed {
			a.notify("📉 Клиент «" + n + "» исчерпал лимит трафика")
		}
	}
}

func (a *App) pollOnline() {
	ips, err := a.Xray.OnlineIPs()
	a.rt.Lock()
	if err != nil || ips == nil {
		a.rt.ipSupported = err == nil
		if err != nil {
			a.rt.onlineIPs = map[string][]string{}
		}
		a.rt.Unlock()
		return
	}
	a.rt.ipSupported = true
	for k := range ips {
		sort.Strings(ips[k])
	}
	a.rt.onlineIPs = ips
	a.rt.Unlock()
	now := nowMs()
	a.Store.UpdateLazy(func(d *Data) {
		for name, l := range ips {
			if len(l) == 0 {
				continue
			}
			if c := d.ClientByName(name); c != nil {
				c.LastOnline = now
			}
		}
	})
	a.enforceDeviceLimits(ips)
}

// device limit enforcement: IPs above the limit are blocked for a while.
var allowedIPs = map[string][]string{}

func (a *App) enforceDeviceLimits(ips map[string][]string) {
	limits := map[string]int{}
	blockMin := 5
	a.Store.Read(func(d *Data) {
		for _, c := range d.Clients {
			if c.DeviceLimit > 0 {
				limits[c.Name] = c.DeviceLimit
			}
		}
		if d.Settings.DeviceBlockMinutes > 0 {
			blockMin = d.Settings.DeviceBlockMinutes
		}
	})
	a.rt.Lock()
	now := time.Now()
	changed := false
	for ip, until := range a.rt.blockedIPs {
		if now.After(until) {
			delete(a.rt.blockedIPs, ip)
			changed = true
		}
	}
	var events []string
	for name, list := range ips {
		lim := limits[name]
		if lim <= 0 {
			delete(allowedIPs, name)
			continue
		}
		cur := map[string]bool{}
		for _, ip := range list {
			cur[ip] = true
		}
		keep := []string{}
		for _, ip := range allowedIPs[name] {
			if cur[ip] && len(keep) < lim {
				keep = append(keep, ip)
			}
		}
		for _, ip := range list {
			if containsStr(keep, ip) {
				continue
			}
			if _, blocked := a.rt.blockedIPs[ip]; blocked {
				continue
			}
			if len(keep) < lim {
				keep = append(keep, ip)
				continue
			}
			a.rt.blockedIPs[ip] = now.Add(time.Duration(blockMin) * time.Minute)
			changed = true
			events = append(events, fmt.Sprintf("Клиент «%s»: превышен лимит устройств (%d), IP %s заблокирован на %d мин", name, lim, ip, blockMin))
		}
		allowedIPs[name] = keep
	}
	a.rt.Unlock()
	if changed {
		a.reapplyDeviceBlocks()
	}
	if len(events) > 0 {
		a.Store.Update(func(d *Data) error {
			for _, e := range events {
				d.AddEvent("warn", e, "")
			}
			return nil
		})
	}
}

// reapplyDeviceBlocks pushes the current block list to xray (also after restarts).
func (a *App) reapplyDeviceBlocks() {
	a.rt.Lock()
	list := make([]string, 0, len(a.rt.blockedIPs))
	for ip := range a.rt.blockedIPs {
		list = append(list, ip)
	}
	a.rt.Unlock()
	if err := a.Xray.BlockIPs(list); err != nil {
		log.Printf("device block: %v", err)
	}
}

// resetPeriods resets client traffic on their schedule.
func (a *App) resetPeriods() {
	loc := moscow()
	now := time.Now().In(loc)
	changed := false
	a.Store.Update(func(d *Data) error {
		for _, c := range d.Clients {
			if c.ResetPeriod == "" || c.ResetPeriod == "never" {
				continue
			}
			last := time.UnixMilli(c.LastReset).In(loc)
			if c.LastReset == 0 {
				c.LastReset = now.UnixMilli()
				continue
			}
			due := false
			switch c.ResetPeriod {
			case "daily":
				due = now.Format("2006-01-02") != last.Format("2006-01-02")
			case "weekly":
				y1, w1 := now.ISOWeek()
				y2, w2 := last.ISOWeek()
				due = y1 != y2 || w1 != w2
			case "monthly":
				due = now.Month() != last.Month() || now.Year() != last.Year()
			case "days":
				days := c.ResetDays
				if days <= 0 {
					days = 30
				}
				due = now.Sub(last) >= time.Duration(days)*24*time.Hour
			}
			if due {
				wasLimited := c.Status(now.UnixMilli()) == StLimited
				c.Up, c.Down = 0, 0
				c.LastReset = now.UnixMilli()
				if wasLimited {
					changed = true
				}
			}
		}
		return nil
	})
	if changed {
		a.ScheduleApply()
	}
}

func (a *App) geoSchedule() {
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	if !st.GeoAuto {
		return
	}
	now := time.Now().In(moscow())
	if now.Hour() != st.GeoHour {
		return
	}
	if st.GeoWeekly && now.Weekday() != time.Monday {
		return
	}
	if st.GeoLastUpdate > 0 && time.Since(time.UnixMilli(st.GeoLastUpdate)) < 20*time.Hour {
		return
	}
	go func() {
		if err := a.UpdateGeo(st.GeoSource); err != nil {
			a.Store.Update(func(d *Data) error {
				d.Settings.GeoLastUpdate = nowMs() // don't retry every minute
				d.AddEvent("bad", "Автообновление баз не удалось: "+err.Error(), "")
				return nil
			})
		}
	}()
}

func (a *App) autoClean() {
	var days int
	a.Store.Read(func(d *Data) { days = d.Settings.AutoCleanDays })
	if days <= 0 {
		return
	}
	cut := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	removed := []string{}
	a.Store.Update(func(d *Data) error {
		kept := d.Clients[:0]
		for _, c := range d.Clients {
			if c.Origin != "main" && c.ExpiresAt > 0 && c.ExpiresAt < cut {
				removed = append(removed, c.Name)
				continue
			}
			kept = append(kept, c)
		}
		d.Clients = kept
		if len(removed) > 0 {
			d.AddEvent("info", fmt.Sprintf("Автоочистка: удалено клиентов с истёкшим сроком — %d", len(removed)), "")
		}
		return nil
	})
	if len(removed) > 0 {
		a.ScheduleApply()
		go a.SyncNodes()
	}
}

func (a *App) autoBackup() {
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	if !st.AutoBackup {
		return
	}
	now := time.Now().In(moscow())
	if now.Hour() != 3 || (st.LastAutoBackup > 0 && time.Since(time.UnixMilli(st.LastAutoBackup)) < 20*time.Hour) {
		return
	}
	a.Store.Update(func(d *Data) error { d.Settings.LastAutoBackup = nowMs(); return nil })
	name, err := a.CreateBackup("auto")
	if err != nil {
		a.Store.Update(func(d *Data) error {
			d.AddEvent("bad", "Автобэкап не создан: "+err.Error(), "")
			return nil
		})
		return
	}
	a.pruneAutoBackups(st.BackupKeep)
	if st.BackupTg {
		go func() {
			if err := a.sendBackupTelegram(name); err != nil {
				a.Store.Update(func(d *Data) error {
					d.AddEvent("warn", "Бэкап не отправлен в Telegram: "+err.Error(), "")
					return nil
				})
			}
		}()
	}
}

func (a *App) autoUpdateCheck() {
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	if st.LastUpdateCheck > 0 && time.Since(time.UnixMilli(st.LastUpdateCheck)) < 12*time.Hour {
		return
	}
	latest, err := fetchLatestPanelVersion()
	a.Store.Update(func(d *Data) error {
		d.Settings.LastUpdateCheck = nowMs()
		if err == nil {
			d.Settings.LatestVersion = latest
		}
		return nil
	})
	if err != nil || !versionNewer(latest, Version) {
		return
	}
	if st.AutoUpdate && time.Now().In(moscow()).Hour() >= 3 && time.Now().In(moscow()).Hour() <= 6 {
		a.Store.Update(func(d *Data) error {
			d.AddEvent("info", "Автообновление панели до "+latest, "")
			return nil
		})
		_ = a.RunSelfUpdate()
	} else {
		a.notify("⬆️ Доступна новая версия KVK Panel: " + latest)
	}
}

// remindExpiring notifies the admin once a day about subscriptions ending within 3 days.
var lastRemindDay string

func (a *App) remindExpiring() {
	now := time.Now().In(moscow())
	day := now.Format("2006-01-02")
	if now.Hour() != 10 || lastRemindDay == day {
		return
	}
	lastRemindDay = day
	ms := now.UnixMilli()
	names := []string{}
	a.Store.Read(func(d *Data) {
		for _, c := range d.Clients {
			if c.Origin == "main" || c.Status(ms) != StActive || c.ExpiresAt == 0 {
				continue
			}
			if c.ExpiresAt-ms < 3*86400*1000 {
				names = append(names, c.Name)
			}
		}
	})
	if len(names) > 0 {
		if len(names) > 30 {
			names = append(names[:30], "…")
		}
		a.notify(fmt.Sprintf("⏳ Подписка заканчивается в ближайшие 3 дня у %d клиентов:\n%s", len(names), strings.Join(names, ", ")))
	}
	a.shopRemind()
}
