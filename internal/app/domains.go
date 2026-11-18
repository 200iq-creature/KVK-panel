package app

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var domainRe = regexp.MustCompile(`^(?i)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

var issuing sync.Map // domain id -> true

type domainView struct {
	*Domain
	DaysLeft int      `json:"daysLeft"`
	UsedBy   []string `json:"usedBy"`
	Panel    bool     `json:"panel"`
	Issuing  bool     `json:"issuing"`
	PointsOK *bool    `json:"pointsOk,omitempty"`
	// Site: the decoy website runs on this domain (SNI: it is the Reality mask)
	Site bool `json:"site"`
	SNI  bool `json:"sni"`
	// PanelOf: name of the server whose panel is opened on this domain
	PanelOf string `json:"panelOf,omitempty"`
	// Dyn: a freemyip / myaddr name the panel keeps pointed at the server
	Dyn bool `json:"dyn"`
}

func (a *App) apiDomains(w http.ResponseWriter, r *http.Request) error {
	list := []domainView{}
	var host, panelCert, ip, server string
	a.Store.Read(func(d *Data) {
		host, panelCert = d.Settings.PublicHost, d.Settings.PanelCertID
		server = orDefault(d.Settings.NodeName, "этот сервер")
		panelHost := a.panelHostLocked(d)
		nodeHosts := map[string]string{}
		for _, n := range d.Nodes {
			if u, err := url.Parse(n.URL); err == nil {
				nodeHosts[strings.ToLower(u.Hostname())] = n.Name
			}
		}
		fs := d.Settings.Fake
		for _, dm := range d.Domains {
			cp := *dm
			cp.DynToken = ""
			v := domainView{Domain: &cp, UsedBy: []string{}, Panel: dm.ID == panelCert, Dyn: dm.DynToken != ""}
			v.Site = fs.Enabled && containsStr(fs.Domains, dm.Name)
			v.SNI = v.Site && fs.SNI == dm.Name
			if v.Panel || (dm.Kind == "domain" && strings.EqualFold(panelHost, dm.Name)) {
				v.PanelOf = server
			} else if nm, ok := nodeHosts[strings.ToLower(dm.Name)]; ok {
				v.PanelOf = nm
			}
			if dm.ExpiresAt > 0 {
				v.DaysLeft = int(time.Until(time.UnixMilli(dm.ExpiresAt)).Hours() / 24)
			}
			for _, in := range d.Inbounds {
				if in.CertID == dm.ID {
					v.UsedBy = append(v.UsedBy, in.Remark)
				}
			}
			if d.Settings.SubCertID == dm.ID {
				v.UsedBy = append([]string{"подписка"}, v.UsedBy...)
			}
			_, v.Issuing = issuing.Load(dm.ID)
			list = append(list, v)
		}
	})
	ip = a.serverIP()
	ok(w, map[string]any{"domains": list, "publicHost": host, "serverIp": ip, "panelCertId": panelCert, "panelUrl": a.panelURL(), "server": server})
	return nil
}

func (a *App) panelURL() string {
	var u string
	a.Store.Read(func(d *Data) { u = a.panelURLLocked(d) })
	return u
}

var cachedIP struct {
	sync.Mutex
	ip string
	at time.Time
}

func (a *App) serverIP() string {
	cachedIP.Lock()
	defer cachedIP.Unlock()
	if cachedIP.ip != "" && time.Since(cachedIP.at) < time.Hour {
		return cachedIP.ip
	}
	ip := detectPublicIP()
	if ip != "" {
		cachedIP.ip, cachedIP.at = ip, time.Now()
	}
	return ip
}

func domainKind(name string) string {
	if net.ParseIP(name) != nil {
		return "ip"
	}
	return "domain"
}

func (a *App) apiDomainCreate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name      string `json:"name"`
		Mode      string `json:"mode"`
		Email     string `json:"email"`
		Cert      string `json:"cert"`
		Key       string `json:"key"`
		AutoRenew *bool  `json:"autoRenew"`
		ForPanel  bool   `json:"forPanel"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(req.Name))
	name = strings.TrimPrefix(strings.TrimPrefix(name, "https://"), "http://")
	name = strings.TrimSuffix(strings.SplitN(name, "/", 2)[0], ".")
	kind := domainKind(name)
	if kind == "domain" && !domainRe.MatchString(name) {
		return errf(400, "Введите домен (например vpn.example.com) или IP-адрес")
	}
	if kind == "ip" {
		ip := net.ParseIP(name)
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() {
			if req.Mode == "letsencrypt" {
				return errf(400, "Let's Encrypt выдаёт сертификаты только на публичные IP")
			}
		}
	}
	switch req.Mode {
	case "letsencrypt", "selfsigned", "custom":
	default:
		return errf(400, "Выберите способ получения сертификата")
	}
	dm := &Domain{ID: RandomHex(6), Name: name, Kind: kind, Mode: req.Mode, Status: "pending", Email: strings.TrimSpace(req.Email),
		AutoRenew: req.Mode == "letsencrypt", CreatedAt: nowMs()}
	if req.AutoRenew != nil && req.Mode == "letsencrypt" {
		dm.AutoRenew = *req.AutoRenew
	}
	err := a.Store.Update(func(d *Data) error {
		for _, x := range d.Domains {
			if x.Name == name && x.Mode == req.Mode {
				return errf(409, "%s уже добавлен", name)
			}
		}
		d.Domains = append(d.Domains, dm)
		return nil
	})
	if err != nil {
		return err
	}
	switch req.Mode {
	case "selfsigned":
		c, k, err := selfSigned(name, kind)
		if err == nil {
			err = a.saveCert(dm.ID, c, k)
		}
		if err != nil {
			a.removeDomain(dm.ID)
			return err
		}
	case "custom":
		if _, err := tls.X509KeyPair([]byte(strings.TrimSpace(req.Cert)), []byte(strings.TrimSpace(req.Key))); err != nil {
			a.removeDomain(dm.ID)
			return errf(400, "Сертификат и ключ не подходят друг к другу: %v", err)
		}
		if err := a.saveCert(dm.ID, []byte(strings.TrimSpace(req.Cert)+"\n"), []byte(strings.TrimSpace(req.Key)+"\n")); err != nil {
			a.removeDomain(dm.ID)
			return err
		}
	case "letsencrypt":
		go a.issueDomain(dm.ID, req.ForPanel)
	}
	if req.ForPanel && req.Mode != "letsencrypt" {
		_ = a.setPanelCert(dm.ID)
	}
	ok(w, map[string]any{"id": dm.ID})
	return nil
}

func (a *App) removeDomain(id string) {
	a.Store.Update(func(d *Data) error {
		for i, x := range d.Domains {
			if x.ID == id {
				d.Domains = append(d.Domains[:i], d.Domains[i+1:]...)
				break
			}
		}
		return nil
	})
}

func (a *App) saveCert(id string, certPEM, keyPEM []byte) error {
	info, err := certInfo(certPEM)
	if err != nil {
		return err
	}
	dir := filepath.Join(a.certsDir(), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	cf, kf := filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(cf+".tmp", certPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(kf+".tmp", keyPEM, 0o600); err != nil {
		return err
	}
	_ = os.Rename(kf+".tmp", kf)
	_ = os.Rename(cf+".tmp", cf)
	usedByXray := false
	a.Store.Update(func(d *Data) error {
		dm := d.DomainByID(id)
		if dm == nil {
			return nil
		}
		dm.CertFile, dm.KeyFile = cf, kf
		dm.Status, dm.Error = "active", ""
		dm.IssuedAt = info.NotBefore.UnixMilli()
		dm.ExpiresAt = info.NotAfter.UnixMilli()
		for _, in := range d.Inbounds {
			if in.CertID == id && in.Enabled {
				usedByXray = true
			}
		}
		return nil
	})
	if usedByXray {
		a.ScheduleApply()
	}
	return nil
}

func (a *App) issueDomain(id string, forPanel bool) error {
	if _, busy := issuing.LoadOrStore(id, true); busy {
		return fmt.Errorf("выпуск уже идёт")
	}
	defer issuing.Delete(id)
	var dm Domain
	found := false
	a.Store.Update(func(d *Data) error {
		if x := d.DomainByID(id); x != nil {
			x.LastTry = nowMs()
			if x.Status != "active" {
				x.Status = "pending"
			}
			dm = *x
			found = true
		}
		return nil
	})
	if !found {
		return fmt.Errorf("не найден")
	}
	certPEM, keyPEM, err := a.issueLetsEncrypt(dm.Name, dm.Kind, dm.Email)
	if err == nil {
		err = a.saveCert(id, certPEM, keyPEM)
	}
	if err != nil {
		log.Printf("acme %s: %v", dm.Name, err)
		a.Store.Update(func(d *Data) error {
			if x := d.DomainByID(id); x != nil {
				if x.CertFile == "" {
					x.Status = "error"
				}
				x.Error = err.Error()
			}
			d.AddEvent("bad", "Сертификат для "+dm.Name+" не выпущен: "+err.Error(), "")
			return nil
		})
		a.notify("⚠️ Сертификат для " + dm.Name + " не выпущен: " + err.Error())
		return err
	}
	a.Store.Update(func(d *Data) error {
		d.AddEvent("ok", "Сертификат Let's Encrypt выпущен для "+dm.Name, "")
		return nil
	})
	var isPanel bool
	a.Store.Read(func(d *Data) { isPanel = d.Settings.PanelCertID == id })
	if forPanel && !isPanel {
		_ = a.setPanelCert(id)
	}
	return nil
}

func (a *App) setPanelCert(id string) error {
	var cf, kf string
	err := a.Store.Update(func(d *Data) error {
		dm := d.DomainByID(id)
		if dm == nil {
			return errf(404, "Сертификат не найден")
		}
		if dm.CertFile == "" {
			return errf(400, "Сертификат ещё не выпущен")
		}
		cf, kf = dm.CertFile, dm.KeyFile
		if _, err := tls.LoadX509KeyPair(cf, kf); err != nil {
			return errf(400, "Файлы сертификата повреждены: %v", err)
		}
		d.Settings.PanelCertID = id
		if dm.Kind == "domain" {
			d.Settings.PublicHost = dm.Name
		}
		d.AddEvent("ok", "Панель переведена на HTTPS ("+dm.Name+")", "")
		return nil
	})
	if err != nil {
		return err
	}
	go func() {
		time.Sleep(800 * time.Millisecond)
		a.RequestPanelRestart()
	}()
	return nil
}

func (a *App) apiDomainIssue(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var mode string
	a.Store.Read(func(d *Data) {
		if dm := d.DomainByID(id); dm != nil {
			mode = dm.Mode
		}
	})
	switch mode {
	case "":
		return errf(404, "Не найден")
	case "letsencrypt":
		if _, busy := issuing.Load(id); busy {
			return errf(409, "Выпуск уже идёт")
		}
		go a.issueDomain(id, false)
	case "selfsigned":
		var name, kind string
		a.Store.Read(func(d *Data) { dm := d.DomainByID(id); name, kind = dm.Name, dm.Kind })
		c, k, err := selfSigned(name, kind)
		if err != nil {
			return err
		}
		if err := a.saveCert(id, c, k); err != nil {
			return err
		}
	default:
		return errf(400, "Свой сертификат нельзя перевыпустить — загрузите новый")
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiDomainPanel(w http.ResponseWriter, r *http.Request) error {
	if err := a.setPanelCert(r.PathValue("id")); err != nil {
		return err
	}
	var url string
	time.Sleep(10 * time.Millisecond)
	a.Store.Read(func(d *Data) {
		url = "https://" + hostPort(a.panelHostLocked(d), d.Settings.PanelPort, "https") + "/" + d.Settings.BasePath + "/"
	})
	ok(w, map[string]any{"ok": true, "url": url})
	return nil
}

func (a *App) apiDomainPanelOff(w http.ResponseWriter, r *http.Request) error {
	a.Store.Update(func(d *Data) error {
		d.Settings.PanelCertID = ""
		d.AddEvent("warn", "HTTPS панели выключен", "")
		return nil
	})
	go func() {
		time.Sleep(800 * time.Millisecond)
		a.RequestPanelRestart()
	}()
	var url string
	a.Store.Read(func(d *Data) {
		url = "http://" + hostPort(a.panelHostLocked(d), d.Settings.PanelPort, "http") + "/" + d.Settings.BasePath + "/"
	})
	ok(w, map[string]any{"ok": true, "url": url})
	return nil
}

func (a *App) apiDomainDelete(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	err := a.Store.Update(func(d *Data) error {
		idx := -1
		for i, x := range d.Domains {
			if x.ID == id {
				idx = i
			}
		}
		if idx < 0 {
			return errf(404, "Не найден")
		}
		if d.Settings.PanelCertID == id {
			return errf(409, "Сертификат используется панелью — сначала выключите HTTPS панели")
		}
		if d.Settings.SubCertID == id {
			d.Settings.SubCertID = "" // subscription links go back to the panel's address
		}
		for _, in := range d.Inbounds {
			if in.CertID == id {
				return errf(409, "Сертификат используется инбаундом «%s»", in.Remark)
			}
		}
		d.AddEvent("info", "Удалён сертификат "+d.Domains[idx].Name, "")
		d.Domains = append(d.Domains[:idx], d.Domains[idx+1:]...)
		return nil
	})
	if err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(a.certsDir(), id))
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiHostUpdate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Host string `json:"host"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	h := strings.ToLower(strings.TrimSpace(req.Host))
	if h == "" {
		h = a.serverIP()
	}
	if net.ParseIP(h) == nil && !domainRe.MatchString(h) {
		return errf(400, "Укажите домен или IP")
	}
	a.Store.Update(func(d *Data) error {
		d.Settings.PublicHost = h
		d.AddEvent("info", "Адрес сервера для ссылок: "+h, "")
		return nil
	})
	go a.SyncNodes()
	ok(w, map[string]any{"ok": true, "host": h})
	return nil
}

// renewCerts renews Let's Encrypt certificates when 1/3 of lifetime remains.
func (a *App) renewCerts() {
	a.dynKeepAlive()
	type job struct {
		id string
	}
	var jobs []job
	now := time.Now()
	a.Store.Read(func(d *Data) {
		for _, dm := range d.Domains {
			if dm.Mode != "letsencrypt" || !dm.AutoRenew {
				continue
			}
			// decoy-site domains are retried often: without a real certificate the site looks suspicious
			retry := 6 * time.Hour
			if d.Settings.Fake.Enabled && containsStr(d.Settings.Fake.Domains, dm.Name) {
				retry = 30 * time.Minute
			}
			if dm.LastTry > 0 && now.Sub(time.UnixMilli(dm.LastTry)) < retry && dm.Status != "active" {
				continue
			}
			if dm.ExpiresAt == 0 {
				if dm.Status == "error" && now.Sub(time.UnixMilli(dm.LastTry)) > retry {
					jobs = append(jobs, job{dm.ID})
				}
				continue
			}
			total := dm.ExpiresAt - dm.IssuedAt
			left := dm.ExpiresAt - now.UnixMilli()
			if left < total/3 {
				if dm.LastTry > 0 && now.Sub(time.UnixMilli(dm.LastTry)) < 3*time.Hour {
					continue
				}
				jobs = append(jobs, job{dm.ID})
			}
		}
	})
	for _, j := range jobs {
		_ = a.issueDomain(j.id, false)
	}
}
