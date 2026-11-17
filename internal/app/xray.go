package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type XrayManager struct {
	app      *App
	mu       sync.Mutex
	cmd      *exec.Cmd
	started  time.Time
	wantRun  bool
	lastErr  string
	version  string
	logMu    sync.Mutex
	logLines []string
	restarts int
	exitCh   chan struct{}
}

func (x *XrayManager) bin() string     { return filepath.Join(x.app.BinDir, "xray") }
func (x *XrayManager) cfgPath() string { return filepath.Join(x.app.DataDir, "xray.json") }

func (x *XrayManager) Installed() bool { return fileExists(x.bin()) }

func (x *XrayManager) Version() string {
	x.mu.Lock()
	v := x.version
	x.mu.Unlock()
	if v != "" {
		return v
	}
	v = x.readVersion()
	x.mu.Lock()
	x.version = v
	x.mu.Unlock()
	return v
}

func (x *XrayManager) readVersion() string {
	if !x.Installed() {
		return ""
	}
	out, err := runCmd(10*time.Second, x.bin(), "version")
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	if len(f) >= 2 && f[0] == "Xray" {
		return f[1]
	}
	return ""
}

func (x *XrayManager) resetVersion() {
	x.mu.Lock()
	x.version = ""
	x.mu.Unlock()
}

func (x *XrayManager) addLog(line string) {
	x.logMu.Lock()
	defer x.logMu.Unlock()
	x.logLines = append(x.logLines, line)
	if len(x.logLines) > 400 {
		x.logLines = x.logLines[len(x.logLines)-400:]
	}
}

func (x *XrayManager) RecentLog() []string {
	x.logMu.Lock()
	defer x.logMu.Unlock()
	out := make([]string, len(x.logLines))
	copy(out, x.logLines)
	return out
}

type XrayStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
	Uptime    int64  `json:"uptime"`
	Error     string `json:"error"`
	Pid       int    `json:"pid"`
}

func (x *XrayManager) Status() XrayStatus {
	st := XrayStatus{Installed: x.Installed(), Version: x.Version()}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.cmd != nil && x.cmd.Process != nil {
		st.Running = true
		st.Pid = x.cmd.Process.Pid
		st.Uptime = int64(time.Since(x.started).Seconds())
	}
	st.Error = x.lastErr
	return st
}

// Test validates a config with `xray run -test`.
func (x *XrayManager) Test(cfg []byte) error {
	if !x.Installed() {
		return fmt.Errorf("xray не установлен")
	}
	tmp := filepath.Join(x.app.DataDir, "xray.test.json")
	if err := os.WriteFile(tmp, cfg, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	cmd := exec.Command(x.bin(), "run", "-test", "-c", tmp)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+x.app.BinDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("конфиг xray не прошёл проверку: %s", lastMeaningful(string(out)))
	}
	return nil
}

func lastMeaningful(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" {
			if len(l) > 400 {
				l = l[:400]
			}
			return l
		}
	}
	return "неизвестная ошибка"
}

// Apply builds config, tests it and restarts xray.
func (x *XrayManager) Apply() error {
	x.app.prepareEmbedded()
	defer x.app.syncEmbeddedLogged()
	cfg, err := x.app.BuildXrayConfig()
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if !x.Installed() {
		_ = os.WriteFile(x.cfgPath(), b, 0o600)
		return fmt.Errorf("xray не установлен — установите ядро в «Настройках»")
	}
	if err := x.Test(b); err != nil {
		x.mu.Lock()
		x.lastErr = err.Error()
		x.mu.Unlock()
		return err
	}
	if err := os.WriteFile(x.cfgPath(), b, 0o600); err != nil {
		return err
	}
	return x.Restart()
}

func (x *XrayManager) Start() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.wantRun = true
	return x.startLocked()
}

func (x *XrayManager) startLocked() error {
	if x.cmd != nil {
		return nil
	}
	if !x.Installed() {
		x.lastErr = "xray не установлен"
		return fmt.Errorf("%s", x.lastErr)
	}
	if !fileExists(x.cfgPath()) {
		cfg, err := x.app.BuildXrayConfig()
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		_ = os.WriteFile(x.cfgPath(), b, 0o600)
	}
	cmd := exec.Command(x.bin(), "run", "-c", x.cfgPath())
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+x.app.BinDir)
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		x.lastErr = err.Error()
		return err
	}
	x.cmd = cmd
	x.started = time.Now()
	x.lastErr = ""
	exitCh := make(chan struct{})
	x.exitCh = exitCh
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			x.addLog(sc.Text())
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	go func() {
		err := cmd.Wait()
		x.mu.Lock()
		if x.cmd == cmd {
			x.cmd = nil
			if x.wantRun {
				msg := "xray завершился"
				if err != nil {
					msg += ": " + err.Error()
				}
				logs := x.RecentLog()
				if len(logs) > 0 {
					msg += " — " + logs[len(logs)-1]
				}
				x.lastErr = msg
				x.restarts++
				go func() {
					time.Sleep(3 * time.Second)
					x.mu.Lock()
					defer x.mu.Unlock()
					if x.wantRun && x.cmd == nil {
						_ = x.startLocked()
					}
				}()
			}
		}
		x.mu.Unlock()
		close(exitCh)
	}()
	// re-apply runtime rules after start
	go func() {
		time.Sleep(1500 * time.Millisecond)
		x.app.reapplyDeviceBlocks()
	}()
	return nil
}

func (x *XrayManager) stopLocked() {
	if x.cmd == nil || x.cmd.Process == nil {
		x.cmd = nil
		return
	}
	cmd := x.cmd
	ch := x.exitCh
	x.cmd = nil
	_ = cmd.Process.Signal(os.Interrupt)
	x.mu.Unlock()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-ch
	}
	x.mu.Lock()
}

func (x *XrayManager) Stop() {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.wantRun = false
	x.stopLocked()
}

func (x *XrayManager) Restart() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.wantRun = true
	x.stopLocked()
	return x.startLocked()
}

func (x *XrayManager) Running() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.cmd != nil
}

// ---------- stats via CLI ----------

type statItem struct {
	Name  string
	Value int64
}

func (x *XrayManager) apiAddr() string {
	var port int
	x.app.Store.Read(func(d *Data) { port = d.Settings.XrayAPIPort })
	return "127.0.0.1:" + strconv.Itoa(port)
}

func (x *XrayManager) QueryStats(reset bool) ([]statItem, error) {
	if !x.Running() {
		return nil, nil
	}
	args := []string{"api", "statsquery", "--server=" + x.apiAddr()}
	if reset {
		args = append(args, "-reset")
	}
	out, err := runCmd(8*time.Second, x.bin(), args...)
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, lastMeaningful(out))
	}
	var resp struct {
		Stat []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"stat"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, err
	}
	items := make([]statItem, 0, len(resp.Stat))
	for _, s := range resp.Stat {
		v := strings.Trim(string(s.Value), `"`)
		n, _ := strconv.ParseInt(v, 10, 64)
		items = append(items, statItem{Name: s.Name, Value: n})
	}
	return items, nil
}

// OnlineIPs returns email -> list of IPs (requires xray with statsonlineiplist -all).
func (x *XrayManager) OnlineIPs() (map[string][]string, error) {
	if !x.Running() {
		return nil, nil
	}
	out, err := runCmd(8*time.Second, x.bin(), "api", "statsonlineiplist", "--server="+x.apiAddr(), "-all")
	if err != nil {
		return nil, fmt.Errorf("%s", lastMeaningful(out))
	}
	res := map[string][]string{}
	// -all output: {"users":[{"email":"x","ips":[{"ip":"1.2.3.4","lastSeen":"..."}]}]}
	var generic any
	if err := json.Unmarshal([]byte(out), &generic); err != nil {
		return res, nil
	}
	walkOnline(generic, res)
	return res, nil
}

func walkOnline(v any, res map[string][]string) {
	switch t := v.(type) {
	case map[string]any:
		name, _ := t["name"].(string)
		if name == "" {
			name, _ = t["email"].(string)
		}
		if raw, okk := t["ips"]; okk && name != "" {
			email := name
			if p := strings.Split(name, ">>>"); len(p) >= 2 {
				email = p[1]
			}
			switch ips := raw.(type) {
			case map[string]any:
				for ip := range ips {
					res[email] = append(res[email], ip)
				}
			case []any:
				for _, e := range ips {
					if m, okk := e.(map[string]any); okk {
						if ip, _ := m["ip"].(string); ip != "" {
							res[email] = append(res[email], ip)
						}
					} else if ip, okk := e.(string); okk {
						res[email] = append(res[email], ip)
					}
				}
			}
			if _, seen := res[email]; !seen {
				res[email] = []string{}
			}
			return
		}
		for _, vv := range t {
			walkOnline(vv, res)
		}
	case []any:
		for _, vv := range t {
			walkOnline(vv, res)
		}
	}
}

// BlockIPs blocks source IPs via the routing API (used for device limits).
func (x *XrayManager) BlockIPs(ips []string) error {
	if !x.Running() {
		return nil
	}
	args := []string{"api", "sib", "--server=" + x.apiAddr(), "-outbound=blocked", "-ruletag=kvk-devlimit", "-reset"}
	if len(ips) == 0 {
		ips = []string{"255.255.255.255"}
	}
	args = append(args, ips...)
	out, err := runCmd(8*time.Second, x.bin(), args...)
	if err != nil {
		return fmt.Errorf("%s", lastMeaningful(out))
	}
	return nil
}
