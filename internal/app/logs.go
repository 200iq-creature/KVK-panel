package app

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) logFile(kind string) string {
	if kind == "error" || kind == "errors" {
		return filepath.Join(a.logsDir(), "error.log")
	}
	return filepath.Join(a.logsDir(), "access.log")
}

func (a *App) apiLogsState(w http.ResponseWriter, r *http.Request) error {
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	now := nowMs()
	res := map[string]any{"recording": st.LogUntil > now, "until": st.LogUntil, "level": st.LogLevel, "access": st.LogAccess}
	for _, k := range []string{"access", "error"} {
		if fi, err := os.Stat(a.logFile(k)); err == nil {
			res[k+"Size"] = fi.Size()
		} else {
			res[k+"Size"] = 0
		}
	}
	ok(w, res)
	return nil
}

func (a *App) apiLogsStart(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Minutes int    `json:"minutes"`
		Level   string `json:"level"`
		Access  *bool  `json:"access"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if req.Minutes < 5 || req.Minutes > 60 {
		return errf(400, "Запись: от 5 до 60 минут")
	}
	switch req.Level {
	case "debug", "info", "warning", "error":
	case "":
		req.Level = "warning"
	default:
		return errf(400, "Неизвестный уровень")
	}
	until := time.Now().Add(time.Duration(req.Minutes) * time.Minute).UnixMilli()
	a.Store.Update(func(d *Data) error {
		d.Settings.LogUntil = until
		d.Settings.LogLevel = req.Level
		d.Settings.LogAccess = req.Access == nil || *req.Access
		d.AddEvent("info", "Включена запись логов Xray на "+strconv.Itoa(req.Minutes)+" мин", "")
		return nil
	})
	if err := a.Xray.Apply(); err != nil {
		return err
	}
	ok(w, map[string]any{"until": until})
	return nil
}

func (a *App) apiLogsStop(w http.ResponseWriter, r *http.Request) error {
	a.Store.Update(func(d *Data) error { d.Settings.LogUntil = 0; return nil })
	if err := a.Xray.Apply(); err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

// tailLines reads the last n lines (matching q) from a file without loading it fully.
func tailLines(path string, n int, q string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	fi, _ := f.Stat()
	const maxRead = 8 << 20
	start := int64(0)
	if fi.Size() > maxRead {
		start = fi.Size() - maxRead
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	ql := strings.ToLower(q)
	ring := make([]string, 0, n)
	first := start > 0
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			continue
		}
		if ql != "" && !strings.Contains(strings.ToLower(line), ql) {
			continue
		}
		if len(ring) == n {
			ring = ring[1:]
		}
		ring = append(ring, line)
	}
	return ring, nil
}

func (a *App) apiLogs(w http.ResponseWriter, r *http.Request) error {
	kind := r.URL.Query().Get("kind")
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 5000 {
		n = 500
	}
	q := r.URL.Query().Get("q")
	var lines []string
	var err error
	if kind == "core" {
		for _, l := range a.Xray.RecentLog() {
			if q == "" || strings.Contains(strings.ToLower(l), strings.ToLower(q)) {
				lines = append(lines, l)
			}
		}
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
	} else {
		lines, err = tailLines(a.logFile(kind), n, q)
		if err != nil {
			return err
		}
	}
	if lines == nil {
		lines = []string{}
	}
	ok(w, map[string]any{"lines": lines})
	return nil
}

func (a *App) apiLogsClear(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Kind string `json:"kind"`
	}
	_ = readJSON(r, &req)
	kinds := []string{"access", "error"}
	if req.Kind != "" {
		kinds = []string{req.Kind}
	}
	for _, k := range kinds {
		if fileExists(a.logFile(k)) {
			_ = os.Truncate(a.logFile(k), 0)
		}
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiLogsDownload(w http.ResponseWriter, r *http.Request) error {
	kind := r.URL.Query().Get("kind")
	p := a.logFile(kind)
	if !fileExists(p) {
		return errf(404, "Лог пуст")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="xray-`+filepath.Base(p)+`"`)
	http.ServeFile(w, r, p)
	return nil
}

// expireLogs turns logging off when the recording window is over and caps file sizes.
func (a *App) expireLogs() {
	var until int64
	a.Store.Read(func(d *Data) { until = d.Settings.LogUntil })
	if until > 0 && until <= nowMs() {
		a.Store.Update(func(d *Data) error {
			d.Settings.LogUntil = 0
			d.AddEvent("info", "Запись логов Xray завершена", "")
			return nil
		})
		if a.Xray.Installed() {
			_ = a.Xray.Apply()
		}
	}
	for _, k := range []string{"access", "error"} {
		p := a.logFile(k)
		if fi, err := os.Stat(p); err == nil && fi.Size() > 200<<20 {
			_ = os.Truncate(p, 0)
		}
	}
}
