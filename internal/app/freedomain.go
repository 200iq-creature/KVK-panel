package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Free domains: names that cost nothing and need no account.
//
//   - sslip.io, nip.io — wildcard DNS: the server IP is part of the name
//     (web-5db8d822.sslip.io → 93.184.216.34). Nothing to register at all.
//   - freemyip.com, myaddr.tools — a name of your choice. The claim itself is
//     protected by a captcha on their site (one click, no account or e-mail);
//     the panel takes the resulting token, points the name at the server and
//     keeps it alive.

type freeProvider struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Suffix   string `json:"suffix"`
	Kind     string `json:"kind"` // ip | token
	ClaimURL string `json:"claimUrl,omitempty"`
	Site     string `json:"site"`
}

var freeProviders = []freeProvider{
	{ID: "sslip", Name: "sslip.io", Suffix: "sslip.io", Kind: "ip", Site: "https://sslip.io"},
	{ID: "nip", Name: "nip.io", Suffix: "nip.io", Kind: "ip", Site: "https://nip.io"},
	{ID: "freemyip", Name: "freemyip.com", Suffix: "freemyip.com", Kind: "token", ClaimURL: "https://freemyip.com/main", Site: "https://freemyip.com"},
	{ID: "myaddr", Name: "myaddr.tools", Suffix: "myaddr.tools", Kind: "token", ClaimURL: "https://myaddr.tools/claim", Site: "https://myaddr.tools"},
}

func freeProviderByID(id string) *freeProvider {
	for i := range freeProviders {
		if freeProviders[i].ID == id {
			return &freeProviders[i]
		}
	}
	return nil
}

// freeSuffixes: names under these can't be put behind the Cloudflare proxy.
var freeSuffixes = []string{"sslip.io", "nip.io", "freemyip.com", "myaddr.tools", "myaddr.dev", "myaddr.io"}

func isFreeDomain(name string) bool {
	name = strings.ToLower(name)
	for _, s := range freeSuffixes {
		if strings.HasSuffix(name, "."+s) {
			return true
		}
	}
	return false
}

var labelRe = regexp.MustCompile(`[^a-z0-9-]+`)

func cleanLabel(s string, max int) string {
	s = labelRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > max {
		s = s[:max]
	}
	return strings.Trim(s, "-")
}

func ipv4Forms(ip string) (dashed, hx string) {
	p := net.ParseIP(ip).To4()
	if p == nil {
		return "", ""
	}
	return strings.ReplaceAll(p.String(), ".", "-"), hex.EncodeToString(p)
}

// freeIPName builds web-5db8d822.sslip.io (hex) or web-93-184-216-34.sslip.io.
func freeIPName(suffix, label, ip string, useHex bool) (string, error) {
	dashed, hx := ipv4Forms(ip)
	if dashed == "" {
		return "", errf(400, "Нужен IPv4-адрес сервера — у этого сервера его не удалось определить")
	}
	label = cleanLabel(label, 30)
	part := dashed
	if useHex {
		part = hx
	}
	if label == "" {
		return part + "." + suffix, nil
	}
	return label + "-" + part + "." + suffix, nil
}

// publicResolver asks public DNS directly: the local resolver may cache
// "no such name" from before the name was claimed.
func publicLookup(name string) []string {
	var out []string
	for _, srv := range []string{"1.1.1.1:53", "8.8.8.8:53", ""} {
		r := net.DefaultResolver
		if srv != "" {
			s := srv
			r = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 4 * time.Second}
				return d.DialContext(ctx, "udp", s)
			}}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		ips, err := r.LookupHost(ctx, name)
		cancel()
		if err == nil && len(ips) > 0 {
			out = ips
			break
		}
	}
	return out
}

func resolvesTo(name, ip string) bool {
	for _, x := range publicLookup(name) {
		if x == ip {
			return true
		}
	}
	return false
}

var dynHTTP = &http.Client{Timeout: 15 * time.Second}

// dynUpdate points a freemyip / myaddr name at ip.
func dynUpdate(provider, name, token, ip string) error {
	var req *http.Request
	switch provider {
	case "freemyip":
		q := url.Values{"token": {token}, "domain": {name}}
		if ip != "" {
			q.Set("myip", ip)
		}
		req, _ = http.NewRequest("GET", "https://freemyip.com/update?"+q.Encode(), nil)
	case "myaddr":
		if ip == "" {
			ip = "self"
		}
		req, _ = http.NewRequest("POST", "https://myaddr.tools/update", strings.NewReader(url.Values{"key": {token}, "ip": {ip}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	default:
		return fmt.Errorf("неизвестный сервис %s", provider)
	}
	req.Header.Set("User-Agent", "kvk-panel/"+Version)
	var last error
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(time.Duration(i) * 1500 * time.Millisecond) // freemyip fails on fast repeats
		}
		r2 := req.Clone(context.Background())
		if req.Body != nil {
			r2.Body, _ = req.GetBody()
		}
		resp, err := dynHTTP.Do(r2)
		if err != nil {
			last = fmt.Errorf("%s не отвечает: %s", map[string]string{"freemyip": "freemyip.com", "myaddr": "myaddr.tools"}[provider], shortNetErr(err))
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		body := strings.TrimSpace(string(b))
		if resp.StatusCode == 200 && strings.HasPrefix(strings.ToUpper(body), "OK") {
			return nil
		}
		switch {
		case provider == "freemyip" && strings.HasPrefix(strings.ToUpper(body), "ERROR"):
			last = fmt.Errorf("freemyip.com не принял токен — проверьте ссылку (имя и токен)")
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			msg := body
			if len(msg) > 160 {
				msg = msg[:160]
			}
			return fmt.Errorf("%s: %s", provider, orDefault(msg, fmt.Sprintf("HTTP %d", resp.StatusCode)))
		default:
			last = fmt.Errorf("%s: HTTP %d", provider, resp.StatusCode)
		}
	}
	return last
}

// parseFreemyip accepts the update link freemyip shows after the claim, or a bare token.
func parseFreemyip(raw, name string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	token := raw
	if strings.Contains(raw, "token=") {
		q := raw
		if i := strings.Index(q, "?"); i >= 0 {
			q = q[i+1:]
		}
		vals, err := url.ParseQuery(strings.Fields(q + " ")[0])
		if err != nil {
			return "", "", errf(400, "Не похоже на ссылку freemyip.com")
		}
		token = vals.Get("token")
		if d := vals.Get("domain"); d != "" {
			name = d
		}
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name != "" && !strings.HasSuffix(name, ".freemyip.com") {
		name = cleanLabel(name, 63) + ".freemyip.com"
	}
	if !regexp.MustCompile(`^[0-9a-fA-F]{16,64}$`).MatchString(token) {
		return "", "", errf(400, "Вставьте ссылку из freemyip.com целиком — в ней есть token=… и domain=…")
	}
	if name == "" || !domainRe.MatchString(name) {
		return "", "", errf(400, "Укажите имя, которое вы заняли на freemyip.com")
	}
	return name, token, nil
}

// myaddrLookup: the key alone is enough — myaddr.tools tells which name it belongs to.
func myaddrLookup(key string) (string, error) {
	req, _ := http.NewRequest("GET", "https://myaddr.tools/reg?"+url.Values{"key": {key}}.Encode(), nil)
	req.Header.Set("User-Agent", "kvk-panel/"+Version)
	resp, err := dynHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("myaddr.tools не отвечает: %s", shortNetErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		return "", fmt.Errorf("myaddr.tools не знает такой ключ — скопируйте его ещё раз (кнопка Copy на их странице)")
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("myaddr.tools: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out); err != nil || out.Name == "" {
		return "", fmt.Errorf("myaddr.tools вернул непонятный ответ")
	}
	return strings.ToLower(out.Name) + ".myaddr.tools", nil
}

var myaddrKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$|^[0-9a-fA-F]{64}$`)

func parseMyaddr(key string) (string, string, error) {
	key = strings.TrimSpace(key)
	if i := strings.Index(key, "key="); i >= 0 {
		if v, err := url.ParseQuery(strings.Fields(key[i:] + " ")[0]); err == nil {
			key = v.Get("key")
		}
	}
	if !myaddrKeyRe.MatchString(key) {
		return "", "", errf(400, "Вставьте секретный ключ (Your secret key) со страницы myaddr.tools — 43 символа")
	}
	name, err := myaddrLookup(key)
	if err != nil {
		return "", "", errf(502, "%v", err)
	}
	return name, key, nil
}

// apiFreeDomainCheck: is the name still free on freemyip.com?
func (a *App) apiFreeDomainCheck(w http.ResponseWriter, r *http.Request) error {
	label := cleanLabel(strings.TrimSuffix(strings.ToLower(r.URL.Query().Get("name")), ".freemyip.com"), 63)
	if label == "" {
		return errf(400, "Введите имя")
	}
	req, _ := http.NewRequest("GET", "https://freemyip.com/main?"+url.Values{"subdomain": {label}}.Encode(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 kvk-panel/"+Version)
	state := "unknown"
	if resp, err := dynHTTP.Do(req); err == nil {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
		resp.Body.Close()
		t := strings.ToLower(string(b))
		switch {
		case strings.Contains(t, "not available"):
			state = "taken"
		case strings.Contains(t, "is available"):
			state = "free"
		}
	}
	ok(w, map[string]any{"name": label, "domain": label + ".freemyip.com", "state": state,
		"claimUrl": "https://freemyip.com/main?" + url.Values{"subdomain": {label}}.Encode()})
	return nil
}

func (a *App) apiFreeDomainInfo(w http.ResponseWriter, r *http.Request) error {
	ip := a.autoServerIP()
	dashed, hx := ipv4Forms(ip)
	var have []string
	a.Store.Read(func(d *Data) {
		for _, dm := range d.Domains {
			have = append(have, dm.Name)
		}
	})
	ok(w, map[string]any{"providers": freeProviders, "ip": ip, "ipDashed": dashed, "ipHex": hx, "have": orEmpty(have)})
	return nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (a *App) apiFreeDomainCreate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Provider string `json:"provider"`
		Label    string `json:"label"`
		Hex      bool   `json:"hex"`
		Name     string `json:"name"`
		Token    string `json:"token"`
		ForPanel bool   `json:"forPanel"`
		Email    string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	p := freeProviderByID(req.Provider)
	if p == nil {
		return errf(400, "Выберите сервис")
	}
	ip := a.autoServerIP()
	pip := net.ParseIP(ip).To4()
	if pip == nil {
		return errf(400, "Не удалось определить IPv4-адрес сервера")
	}
	if (pip.IsPrivate() || pip.IsLoopback() || pip.IsLinkLocalUnicast()) && os.Getenv("KVK_ALLOW_PRIVATE_IP") == "" {
		return errf(400, "У сервера нет публичного IPv4 (%s) — Let's Encrypt не сможет его проверить", ip)
	}
	var name, token string
	var err error
	switch p.Kind {
	case "ip":
		if name, err = freeIPName(p.Suffix, req.Label, ip, req.Hex); err != nil {
			return err
		}
	case "token":
		if p.ID == "freemyip" {
			name, token, err = parseFreemyip(req.Token, req.Name)
		} else {
			name, token, err = parseMyaddr(req.Token)
		}
		if err != nil {
			return err
		}
	}
	if !domainRe.MatchString(name) || len(name) > 253 {
		return errf(400, "Получилось неправильное имя %s — измените слово", name)
	}
	dup := false
	a.Store.Read(func(d *Data) {
		for _, x := range d.Domains {
			if x.Name == name {
				dup = true
			}
		}
	})
	if dup {
		return errf(409, "%s уже добавлен", name)
	}
	warn := ""
	if token != "" {
		if err := dynUpdate(p.ID, name, token, ip); err != nil {
			return errf(502, "%v", err)
		}
	}
	// the name must lead to this server before Let's Encrypt is asked
	okDNS := false
	for i := 0; i < 6 && !okDNS; i++ {
		if i > 0 {
			time.Sleep(3 * time.Second)
		}
		okDNS = resolvesTo(name, ip)
	}
	if !okDNS {
		if token == "" {
			return errf(502, "%s пока не указывает на %s — сервис %s недоступен из этой сети или блокирует имя. Попробуйте другое слово или другой сервис", name, ip, p.Name)
		}
		warn = "DNS ещё обновляется — сертификат выпустится автоматически, как только " + name + " начнёт указывать на " + ip
	}
	dm := &Domain{ID: RandomHex(6), Name: name, Kind: "domain", Mode: "letsencrypt", Status: "pending", Email: strings.TrimSpace(req.Email),
		AutoRenew: true, CreatedAt: nowMs(), Provider: p.ID, DynToken: token}
	if token != "" {
		dm.DynIP, dm.DynAt = ip, nowMs()
	}
	err = a.Store.Update(func(d *Data) error {
		for _, x := range d.Domains {
			if x.Name == name {
				return errf(409, "%s уже добавлен", name)
			}
		}
		d.Domains = append(d.Domains, dm)
		d.AddEvent("ok", "Создан бесплатный домен "+name+" ("+p.Name+")", "")
		return nil
	})
	if err != nil {
		return err
	}
	go a.issueDomain(dm.ID, req.ForPanel)
	ok(w, map[string]any{"id": dm.ID, "name": name, "warning": warn})
	return nil
}

// apiFreeDomainPoint re-points a freemyip / myaddr name at this server.
func (a *App) apiFreeDomainPoint(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var prov, name, tok string
	a.Store.Read(func(d *Data) {
		for _, dm := range d.Domains {
			if dm.ID == id {
				prov, name, tok = dm.Provider, dm.Name, dm.DynToken
			}
		}
	})
	if tok == "" {
		return errf(400, "Этот домен не управляется панелью")
	}
	ip := a.autoServerIP()
	if err := dynUpdate(prov, name, tok, ip); err != nil {
		a.setDynState(id, "", err.Error())
		return errf(502, "%v", err)
	}
	a.setDynState(id, ip, "")
	go func() {
		time.Sleep(20 * time.Second)
		_ = a.issueDomain(id, false)
	}()
	ok(w, map[string]any{"ok": true, "ip": ip})
	return nil
}

func (a *App) setDynState(id, ip, errText string) {
	a.Store.Update(func(d *Data) error {
		for _, dm := range d.Domains {
			if dm.ID == id {
				if errText == "" {
					dm.DynIP, dm.DynAt, dm.DynError = ip, nowMs(), ""
				} else {
					dm.DynError = errText
				}
			}
		}
		return nil
	})
}

// dynKeepAlive refreshes freemyip / myaddr names before they expire
// (myaddr drops an address not updated for 90 days, freemyip after a year).
// The stored address is sent, not this server's: a copy of the configuration
// restored elsewhere must not steal the name back without being asked.
func (a *App) dynKeepAlive() {
	type job struct{ id, prov, name, tok, ip string }
	var jobs []job
	now := nowMs()
	a.Store.Read(func(d *Data) {
		for _, dm := range d.Domains {
			if dm.DynToken != "" && dm.DynIP != "" && now-dm.DynAt > 20*24*3600*1000 {
				jobs = append(jobs, job{dm.ID, dm.Provider, dm.Name, dm.DynToken, dm.DynIP})
			}
		}
	})
	for _, j := range jobs {
		if err := dynUpdate(j.prov, j.name, j.tok, j.ip); err != nil {
			a.setDynState(j.id, "", err.Error())
			continue
		}
		a.setDynState(j.id, j.ip, "")
	}
}
