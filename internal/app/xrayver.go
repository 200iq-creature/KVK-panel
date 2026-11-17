package app

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

func xrayAssetName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "Xray-linux-64.zip"
	case "386":
		return "Xray-linux-32.zip"
	case "arm64":
		return "Xray-linux-arm64-v8a.zip"
	case "arm":
		return "Xray-linux-arm32-v7a.zip"
	case "s390x":
		return "Xray-linux-s390x.zip"
	case "riscv64":
		return "Xray-linux-riscv64.zip"
	case "mips64le":
		return "Xray-linux-mips64le.zip"
	}
	return "Xray-linux-64.zip"
}

type xrayRelease struct {
	Tag        string `json:"tag"`
	Published  string `json:"published"`
	Prerelease bool   `json:"prerelease"`
}

var relCache struct {
	sync.Mutex
	at   time.Time
	list []xrayRelease
}

func fetchXrayReleases() ([]xrayRelease, error) {
	relCache.Lock()
	defer relCache.Unlock()
	if time.Since(relCache.at) < 10*time.Minute && relCache.list != nil {
		return relCache.list, nil
	}
	b, err := httpGetBytes("https://api.github.com/repos/XTLS/Xray-core/releases?per_page=30", 64<<20)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
		Prerelease  bool   `json:"prerelease"`
		Draft       bool   `json:"draft"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	list := []xrayRelease{}
	for _, r := range raw {
		if r.Draft {
			continue
		}
		list = append(list, xrayRelease{Tag: r.TagName, Published: r.PublishedAt, Prerelease: r.Prerelease})
	}
	relCache.list, relCache.at = list, time.Now()
	return list, nil
}

var installMu sync.Mutex

// downloadXray fetches a release, verifies it and swaps binaries (keeping the previous one as .prev).
func (a *App) downloadXray(tag string) error {
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	asset := xrayAssetName()
	base := "https://github.com/XTLS/Xray-core/releases/download/" + tag + "/"
	zipb, err := httpGetBytes(base+asset, 200<<20)
	if err != nil {
		return fmt.Errorf("не удалось скачать %s: %v", asset, err)
	}
	if dg, err := httpGetBytes(base+asset+".dgst", 1<<20); err == nil {
		want := ""
		for _, line := range strings.Split(string(dg), "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToUpper(l), "SHA2-256") || strings.HasPrefix(strings.ToUpper(l), "SHA256") {
				if i := strings.LastIndex(l, "= "); i >= 0 {
					want = strings.TrimSpace(l[i+2:])
				}
			}
		}
		if want != "" {
			sum := sha256.Sum256(zipb)
			if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
				return fmt.Errorf("контрольная сумма архива не совпала")
			}
		}
	}
	zr, err := zip.NewReader(bytes.NewReader(zipb), int64(len(zipb)))
	if err != nil {
		return err
	}
	var bin []byte
	geo := map[string][]byte{}
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if name != "xray" && name != "geoip.dat" && name != "geosite.dat" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if name == "xray" {
			bin = data
		} else {
			geo[name] = data
		}
	}
	if bin == nil {
		return fmt.Errorf("в архиве нет бинарника xray")
	}
	dst := a.Xray.bin()
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	if out, err := runCmd(15*time.Second, tmp, "version"); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("новый xray не запускается на этом сервере: %s", lastMeaningful(out))
	}
	if fileExists(dst) {
		_ = os.Rename(dst, dst+".prev")
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	for name, data := range geo {
		p := filepath.Join(a.BinDir, name)
		if !fileExists(p) {
			_ = os.WriteFile(p, data, 0o644)
		}
	}
	a.Xray.resetVersion()
	return nil
}

func latestXrayTag() (string, error) {
	b, err := httpGetBytes("https://api.github.com/repos/XTLS/Xray-core/releases/latest", 1<<20)
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(b, &rel); err != nil || rel.TagName == "" {
		return "", fmt.Errorf("не удалось узнать последнюю версию Xray")
	}
	return rel.TagName, nil
}

// InstallXrayOffline is used by the CLI/installer (no running service).
func (a *App) InstallXrayOffline(tag string) error {
	if tag == "" || tag == "latest" {
		t, err := latestXrayTag()
		if err != nil {
			return err
		}
		tag = t
	}
	_ = os.MkdirAll(a.BinDir, 0o755)
	return a.downloadXray(tag)
}

// InstallXray downloads a specific Xray release and restarts it, rolling back on failure.
func (a *App) InstallXray(tag string) error {
	installMu.Lock()
	defer installMu.Unlock()
	prevVer := a.Xray.Version()
	if err := a.downloadXray(tag); err != nil {
		return err
	}
	dst := a.Xray.bin()
	newVer := a.Xray.Version()
	if err := a.Xray.Apply(); err != nil {
		// rollback
		if fileExists(dst + ".prev") {
			_ = os.Rename(dst, dst+".failed")
			_ = os.Rename(dst+".prev", dst)
			a.Xray.resetVersion()
			_ = a.Xray.Apply()
		}
		return fmt.Errorf("xray %s не запустился с текущим конфигом, возвращена прежняя версия: %v", newVer, err)
	}
	a.Store.Update(func(d *Data) error {
		d.Settings.XrayPrevVersion = prevVer
		d.AddEvent("ok", fmt.Sprintf("Xray обновлён: %s → %s", orDefault(prevVer, "—"), newVer), "")
		return nil
	})
	return nil
}

func (a *App) RollbackXray() error {
	installMu.Lock()
	defer installMu.Unlock()
	dst := a.Xray.bin()
	if !fileExists(dst + ".prev") {
		return fmt.Errorf("предыдущей версии нет")
	}
	cur := a.Xray.Version()
	_ = os.Rename(dst, dst+".tmp")
	if err := os.Rename(dst+".prev", dst); err != nil {
		_ = os.Rename(dst+".tmp", dst)
		return err
	}
	_ = os.Rename(dst+".tmp", dst+".prev")
	a.Xray.resetVersion()
	if err := a.Xray.Apply(); err != nil {
		return err
	}
	nv := a.Xray.Version()
	a.Store.Update(func(d *Data) error {
		d.Settings.XrayPrevVersion = cur
		d.AddEvent("info", fmt.Sprintf("Xray откатан: %s → %s", cur, nv), "")
		return nil
	})
	return nil
}

func (a *App) prevXrayVersion() string {
	p := a.Xray.bin() + ".prev"
	if !fileExists(p) {
		return ""
	}
	out, err := runCmd(10*time.Second, p, "version")
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	if len(f) >= 2 {
		return f[1]
	}
	return ""
}

// ---------- handlers ----------

func (a *App) apiXrayStatus(w http.ResponseWriter, r *http.Request) error {
	st := a.Xray.Status()
	ok(w, map[string]any{"status": st, "prev": a.prevXrayVersion(), "log": a.Xray.RecentLog(), "asset": xrayAssetName()})
	return nil
}

func (a *App) apiXrayAction(w http.ResponseWriter, r *http.Request) error {
	switch r.PathValue("action") {
	case "restart":
		if err := a.Xray.Apply(); err != nil {
			return err
		}
	case "stop":
		a.Xray.Stop()
		a.Store.Update(func(d *Data) error { d.AddEvent("warn", "Xray остановлен вручную", ""); return nil })
	case "start":
		if err := a.Xray.Apply(); err != nil {
			return err
		}
	default:
		return errf(404, "unknown action")
	}
	ok(w, a.Xray.Status())
	return nil
}

func (a *App) apiXrayVersions(w http.ResponseWriter, r *http.Request) error {
	list, err := fetchXrayReleases()
	if err != nil {
		return errf(502, "GitHub недоступен: %v", err)
	}
	ok(w, map[string]any{"releases": list, "current": a.Xray.Version(), "prev": a.prevXrayVersion()})
	return nil
}

func (a *App) apiXrayInstall(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Version string `json:"version"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Version) == "" {
		return errf(400, "Выберите версию")
	}
	if err := a.InstallXray(strings.TrimSpace(req.Version)); err != nil {
		return err
	}
	ok(w, a.Xray.Status())
	return nil
}

func (a *App) apiXrayRollback(w http.ResponseWriter, r *http.Request) error {
	if err := a.RollbackXray(); err != nil {
		return err
	}
	ok(w, a.Xray.Status())
	return nil
}

func (a *App) apiXrayConfig(w http.ResponseWriter, r *http.Request) error {
	b, err := os.ReadFile(a.Xray.cfgPath())
	if err != nil {
		cfg, _ := a.BuildXrayConfig()
		b, _ = json.MarshalIndent(cfg, "", "  ")
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

var _ = http.StatusOK

func jsonIndent(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
