package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type geoSource struct {
	ID, Name             string
	GeoIP, GeoSite       string
	GeoIPSum, GeoSiteSum string
}

var geoSources = map[string]geoSource{
	"loyalsoldier": {ID: "loyalsoldier", Name: "Loyalsoldier",
		GeoIP: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat", GeoIPSum: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat.sha256sum",
		GeoSite: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat", GeoSiteSum: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat.sha256sum"},
	"v2fly": {ID: "v2fly", Name: "v2fly",
		GeoIP: "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat", GeoIPSum: "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat.sha256sum",
		GeoSite: "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat", GeoSiteSum: "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat.sha256sum"},
	"runetfreedom": {ID: "runetfreedom", Name: "runetfreedom",
		GeoIP: "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat", GeoIPSum: "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat.sha256sum",
		GeoSite: "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat", GeoSiteSum: "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat.sha256sum"},
}

var geoMu sync.Mutex

func downloadVerified(url, sumURL string) ([]byte, error) {
	data, err := httpGetBytes(url, 400<<20)
	if err != nil {
		return nil, err
	}
	if len(data) < 1024 {
		return nil, fmt.Errorf("файл %s подозрительно мал", filepath.Base(url))
	}
	if sumURL != "" {
		if sb, err := httpGetBytes(sumURL, 4096); err == nil {
			want := strings.Fields(string(sb))
			if len(want) > 0 {
				sum := sha256.Sum256(data)
				if !strings.EqualFold(hex.EncodeToString(sum[:]), want[0]) {
					return nil, fmt.Errorf("sha256 %s не совпал с релизом", filepath.Base(url))
				}
			}
		}
	}
	return data, nil
}

func (a *App) UpdateGeo(source string) error {
	return a.updateGeo(source, true)
}

// UpdateGeoOffline downloads geo files without touching a running Xray (CLI / installer).
func (a *App) UpdateGeoOffline(source string) error {
	_ = os.MkdirAll(a.BinDir, 0o755)
	return a.updateGeo(source, false)
}

func (a *App) updateGeo(source string, apply bool) error {
	geoMu.Lock()
	defer geoMu.Unlock()
	src, okk := geoSources[source]
	if !okk {
		return fmt.Errorf("неизвестный источник")
	}
	ip, err := downloadVerified(src.GeoIP, src.GeoIPSum)
	if err != nil {
		return err
	}
	site, err := downloadVerified(src.GeoSite, src.GeoSiteSum)
	if err != nil {
		return err
	}
	files := map[string][]byte{"geoip.dat": ip, "geosite.dat": site}
	// backup + atomic swap
	for name, data := range files {
		p := filepath.Join(a.BinDir, name)
		if fileExists(p) {
			_ = copyFile(p, p+".bak", 0o644)
		}
		if err := os.WriteFile(p+".new", data, 0o644); err != nil {
			return err
		}
	}
	for name := range files {
		p := filepath.Join(a.BinDir, name)
		if err := os.Rename(p+".new", p); err != nil {
			return err
		}
	}
	if apply && a.Xray.Installed() {
		if err := a.Xray.Apply(); err != nil {
			for name := range files {
				p := filepath.Join(a.BinDir, name)
				if fileExists(p + ".bak") {
					_ = copyFile(p+".bak", p, 0o644)
				}
			}
			_ = a.Xray.Apply()
			return fmt.Errorf("xray не стартует с новыми базами — вернули прежние: %v", err)
		}
	}
	a.Store.Update(func(d *Data) error {
		d.Settings.GeoLastUpdate = nowMs()
		d.Settings.GeoLastSource = source
		d.AddEvent("ok", "Базы GeoIP/Geosite обновлены ("+src.Name+")", "")
		return nil
	})
	return nil
}

func (a *App) RollbackGeo() error {
	geoMu.Lock()
	defer geoMu.Unlock()
	any := false
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		p := filepath.Join(a.BinDir, name)
		if fileExists(p + ".bak") {
			cur, _ := os.ReadFile(p)
			if err := copyFile(p+".bak", p, 0o644); err != nil {
				return err
			}
			if cur != nil {
				_ = os.WriteFile(p+".bak", cur, 0o644)
			}
			any = true
		}
	}
	if !any {
		return fmt.Errorf("прежних баз нет")
	}
	if err := a.Xray.Apply(); err != nil {
		return err
	}
	a.Store.Update(func(d *Data) error {
		d.AddEvent("info", "Возвращены прежние базы GeoIP/Geosite", "")
		return nil
	})
	return nil
}

func (a *App) apiGeo(w http.ResponseWriter, r *http.Request) error {
	files := []map[string]any{}
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		p := filepath.Join(a.BinDir, name)
		f := map[string]any{"name": name, "exists": false}
		if fi, err := os.Stat(p); err == nil {
			f["exists"] = true
			f["size"] = fi.Size()
			f["mtime"] = fi.ModTime().UnixMilli()
		}
		f["backup"] = fileExists(p + ".bak")
		files = append(files, f)
	}
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	fresh := st.GeoLastUpdate > 0 && time.Since(time.UnixMilli(st.GeoLastUpdate)) < 14*24*time.Hour
	ok(w, map[string]any{"files": files, "source": st.GeoSource, "auto": st.GeoAuto, "weekly": st.GeoWeekly, "hour": st.GeoHour,
		"lastUpdate": st.GeoLastUpdate, "lastSource": st.GeoLastSource, "fresh": fresh})
	return nil
}

func (a *App) apiGeoSettings(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Source string `json:"source"`
		Auto   bool   `json:"auto"`
		Weekly bool   `json:"weekly"`
		Hour   int    `json:"hour"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if _, okk := geoSources[req.Source]; !okk {
		return errf(400, "Неизвестный источник")
	}
	if req.Hour < 0 || req.Hour > 23 {
		req.Hour = 4
	}
	a.Store.Update(func(d *Data) error {
		d.Settings.GeoSource, d.Settings.GeoAuto, d.Settings.GeoWeekly, d.Settings.GeoHour = req.Source, req.Auto, req.Weekly, req.Hour
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiGeoUpdate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Source string `json:"source"`
	}
	_ = readJSON(r, &req)
	if req.Source == "" {
		a.Store.Read(func(d *Data) { req.Source = d.Settings.GeoSource })
	}
	if err := a.UpdateGeo(req.Source); err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiGeoRollback(w http.ResponseWriter, r *http.Request) error {
	if err := a.RollbackGeo(); err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}
