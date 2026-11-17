package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxBackupSize = 64 << 20

var backupNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+\.zip$`)

type backupInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Created int64  `json:"created"`
	Kind    string `json:"kind"` // manual | auto | upload | pre-restore
}

func backupKind(name string) string {
	switch {
	case strings.Contains(name, "-auto-"):
		return "auto"
	case strings.Contains(name, "-upload-"):
		return "upload"
	case strings.Contains(name, "-prerestore-"):
		return "pre-restore"
	}
	return "manual"
}

func (a *App) listBackups() []backupInfo {
	out := []backupInfo{}
	ents, _ := os.ReadDir(a.backupsDir())
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, backupInfo{Name: e.Name(), Size: fi.Size(), Created: fi.ModTime().UnixMilli(), Kind: backupKind(e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}

// CreateBackup writes a zip with the database and certificates.
func (a *App) CreateBackup(kind string) (string, error) {
	a.Store.Flush()
	ts := time.Now().In(moscow()).Format("2006-01-02_15-04-05")
	name := fmt.Sprintf("kvk-%s-%s.zip", kind, ts)
	if kind == "manual" {
		name = fmt.Sprintf("kvk-%s.zip", ts)
	}
	_ = os.MkdirAll(a.backupsDir(), 0o700)
	p := filepath.Join(a.backupsDir(), name)
	f, err := os.OpenFile(p+".tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(f)
	add := func(zname string, data []byte) error {
		w, err := zw.Create(zname)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	meta, _ := json.Marshal(map[string]any{"app": "kvk-panel", "version": Version, "created": time.Now().UTC().Format(time.RFC3339)})
	if err := add("meta.json", meta); err != nil {
		f.Close()
		return "", err
	}
	if err := add("db.json", a.Store.Snapshot()); err != nil {
		f.Close()
		return "", err
	}
	_ = filepath.Walk(a.certsDir(), func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(a.certsDir(), path)
		b, err := os.ReadFile(path)
		if err == nil {
			_ = add("certs/"+filepath.ToSlash(rel), b)
		}
		return nil
	})
	if err := zw.Close(); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	if err := os.Rename(p+".tmp", p); err != nil {
		return "", err
	}
	return name, nil
}

func (a *App) pruneAutoBackups(keep int) {
	if keep <= 0 {
		keep = 7
	}
	n := 0
	for _, b := range a.listBackups() {
		if b.Kind != "auto" && b.Kind != "pre-restore" {
			continue
		}
		n++
		if n > keep {
			_ = os.Remove(filepath.Join(a.backupsDir(), b.Name))
		}
	}
}

func (a *App) apiBackups(w http.ResponseWriter, r *http.Request) error {
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	list := a.listBackups()
	var total int64
	for _, b := range list {
		total += b.Size
	}
	ok(w, map[string]any{"backups": list, "total": total, "autoBackup": st.AutoBackup, "backupKeep": st.BackupKeep,
		"backupTg": st.BackupTg, "tgReady": st.TgBotToken != "" && st.TgAdminID != "", "maxSize": maxBackupSize})
	return nil
}

func (a *App) apiBackupSettings(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		AutoBackup bool `json:"autoBackup"`
		BackupKeep int  `json:"backupKeep"`
		BackupTg   bool `json:"backupTg"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.BackupKeep < 1 || req.BackupKeep > 60 {
		req.BackupKeep = 7
	}
	a.Store.Update(func(d *Data) error {
		d.Settings.AutoBackup, d.Settings.BackupKeep, d.Settings.BackupTg = req.AutoBackup, req.BackupKeep, req.BackupTg
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiBackupCreate(w http.ResponseWriter, r *http.Request) error {
	name, err := a.CreateBackup("manual")
	if err != nil {
		return err
	}
	a.Store.Update(func(d *Data) error { d.AddEvent("ok", "Создан бэкап "+name, ""); return nil })
	ok(w, map[string]any{"name": name})
	return nil
}

func validateBackupZip(b []byte) (*Data, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, errf(400, "Это не zip-архив")
	}
	for _, f := range zr.File {
		if f.Name == "db.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			db, err := io.ReadAll(io.LimitReader(rc, maxBackupSize))
			if err != nil {
				return nil, err
			}
			var d Data
			if err := json.Unmarshal(db, &d); err != nil {
				return nil, errf(400, "db.json в архиве повреждён: %v", err)
			}
			if d.Admin.Username == "" || d.Settings.BasePath == "" {
				return nil, errf(400, "Архив не похож на бэкап KVK Panel")
			}
			return &d, nil
		}
	}
	return nil, errf(400, "В архиве нет db.json — это не бэкап KVK Panel")
}

func (a *App) apiBackupUpload(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupSize+1<<20)
	var data []byte
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			return errf(413, "Файл больше 64 МБ")
		}
		var f multipart.File
		f, _, err := r.FormFile("file")
		if err != nil {
			return errf(400, "Файл не получен")
		}
		defer f.Close()
		data, err = io.ReadAll(f)
		if err != nil {
			return err
		}
	} else {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			return errf(413, "Файл больше 64 МБ")
		}
	}
	if len(data) > maxBackupSize {
		return errf(413, "Файл больше 64 МБ")
	}
	if _, err := validateBackupZip(data); err != nil {
		return err
	}
	name := "kvk-upload-" + time.Now().In(moscow()).Format("2006-01-02_15-04-05") + ".zip"
	if err := os.WriteFile(filepath.Join(a.backupsDir(), name), data, 0o600); err != nil {
		return err
	}
	ok(w, map[string]any{"name": name})
	return nil
}

func (a *App) backupPath(r *http.Request) (string, error) {
	name := r.PathValue("name")
	if !backupNameRe.MatchString(name) {
		return "", errf(400, "Некорректное имя")
	}
	p := filepath.Join(a.backupsDir(), name)
	if !fileExists(p) {
		return "", errf(404, "Бэкап не найден")
	}
	return p, nil
}

func (a *App) apiBackupDownload(w http.ResponseWriter, r *http.Request) error {
	p, err := a.backupPath(r)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(p)+`"`)
	http.ServeFile(w, r, p)
	return nil
}

// RestoreBackup replaces the database and certificates with the archive contents.
func (a *App) RestoreBackup(p string) error { return a.restore(p, true) }

func (a *App) restoreOffline(p string) error { return a.restore(p, false) }

func (a *App) restore(p string, online bool) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	d, err := validateBackupZip(b)
	if err != nil {
		return err
	}
	if _, err := a.CreateBackup("prerestore"); err != nil && online {
		return fmt.Errorf("не удалось сохранить текущее состояние: %v", err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	certs := a.certsDir()
	tmpCerts := certs + ".restore"
	_ = os.RemoveAll(tmpCerts)
	_ = os.MkdirAll(tmpCerts, 0o700)
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "certs/") || strings.HasSuffix(f.Name, "/") {
			continue
		}
		rel := filepath.Clean(strings.TrimPrefix(f.Name, "certs/"))
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		dst := filepath.Join(tmpCerts, rel)
		_ = os.MkdirAll(filepath.Dir(dst), 0o700)
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(rc, 8<<20))
		rc.Close()
		_ = os.WriteFile(dst, data, 0o600)
	}
	_ = os.RemoveAll(certs + ".old")
	_ = os.Rename(certs, certs+".old")
	_ = os.Rename(tmpCerts, certs)
	_ = os.RemoveAll(certs + ".old")
	// fix certificate paths to this server's data dir
	for _, dm := range d.Domains {
		if dm.CertFile != "" {
			dm.CertFile = filepath.Join(certs, dm.ID, "fullchain.pem")
			dm.KeyFile = filepath.Join(certs, dm.ID, "privkey.pem")
			if !fileExists(dm.CertFile) {
				dm.CertFile, dm.KeyFile, dm.Status = "", "", "error"
				dm.Error = "файлы сертификата отсутствуют в бэкапе"
			}
		}
	}
	if d.Settings.PanelCertID != "" {
		if dm := d.DomainByID(d.Settings.PanelCertID); dm == nil || dm.CertFile == "" {
			d.Settings.PanelCertID = ""
		}
	}
	d.AddEvent("warn", "Восстановлен бэкап "+filepath.Base(p), "")
	if err := a.Store.Replace(d); err != nil {
		return err
	}
	if !online {
		return nil
	}
	if a.Xray.Installed() {
		if err := a.Xray.Apply(); err != nil {
			a.Store.Update(func(d *Data) error {
				d.AddEvent("bad", "Xray не запустился после восстановления: "+err.Error(), "")
				return nil
			})
		}
	}
	go func() {
		time.Sleep(time.Second)
		a.RequestPanelRestart()
	}()
	return nil
}

func (a *App) apiBackupRestore(w http.ResponseWriter, r *http.Request) error {
	p, err := a.backupPath(r)
	if err != nil {
		return err
	}
	if err := a.RestoreBackup(p); err != nil {
		return err
	}
	var url string
	a.Store.Read(func(d *Data) { url = a.panelURLLocked(d) })
	ok(w, map[string]any{"ok": true, "url": url})
	return nil
}

func (a *App) apiBackupDelete(w http.ResponseWriter, r *http.Request) error {
	p, err := a.backupPath(r)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

// sendBackupTelegram uploads a backup file to the admin chat.
func (a *App) sendBackupTelegram(name string) error {
	var token, chat, node string
	a.Store.Read(func(d *Data) { token, chat, node = d.Settings.TgBotToken, d.Settings.TgAdminID, d.Settings.NodeName })
	if token == "" || chat == "" {
		return fmt.Errorf("бот не настроен")
	}
	b, err := os.ReadFile(filepath.Join(a.backupsDir(), name))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("chat_id", chat)
	_ = mw.WriteField("caption", "🗄 Бэкап KVK Panel · "+node)
	fw, _ := mw.CreateFormFile("document", name)
	_, _ = fw.Write(b)
	mw.Close()
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Post("https://api.telegram.org/bot"+token+"/sendDocument", mw.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("telegram: %s", string(rb))
	}
	return nil
}
