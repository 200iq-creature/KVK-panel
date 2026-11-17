package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var Version = "1.0.0"

const RepoSlug = "200iq-creature/KVK-panel"

type App struct {
	DataDir string
	BinDir  string
	Store   *Store
	Xray    *XrayManager

	limiter *limiter
	tickets tickets
	sys     *SysMonitor

	rt struct {
		sync.Mutex
		onlineIPs   map[string][]string
		blockedIPs  map[string]time.Time
		ipSupported bool
		speedRx     float64
		speedTx     float64
		lastStatsAt time.Time
		statsErr    string
	}

	applyMu      sync.Mutex
	applyPending bool
	restartPanel chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
}

func strconvItoa(i int) string { return strconv.Itoa(i) }

func New(dataDir, binDir string) (*App, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(binDir, 0o755)
	st, err := NewStore(filepath.Join(dataDir, "db.json"))
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	a := &App{DataDir: dataDir, BinDir: binDir, Store: st, limiter: newLimiter(), restartPanel: make(chan struct{}, 1)}
	a.Xray = &XrayManager{app: a}
	a.sys = newSysMonitor()
	a.rt.onlineIPs = map[string][]string{}
	a.rt.blockedIPs = map[string]time.Time{}
	for _, d := range []string{a.certsDir(), a.backupsDir(), a.logsDir()} {
		_ = os.MkdirAll(d, 0o700)
	}
	return a, nil
}

func (a *App) certsDir() string   { return filepath.Join(a.DataDir, "certs") }
func (a *App) backupsDir() string { return filepath.Join(a.DataDir, "backups") }
func (a *App) logsDir() string    { return filepath.Join(a.DataDir, "logs") }

// Run starts xray, background jobs and the web server; blocks until ctx done.
func (a *App) Run() error {
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.Store.Update(func(d *Data) error {
		if d.Settings.PublicHost == "" {
			if ip := detectPublicIP(); ip != "" {
				d.Settings.PublicHost = ip
			}
		}
		return nil
	})
	if a.Xray.Installed() {
		if err := a.Xray.Apply(); err != nil {
			log.Printf("xray: %v", err)
			_ = a.Xray.Start()
		}
	} else {
		log.Printf("xray binary not found in %s", a.BinDir)
	}
	go a.migrateAutoSetup()
	if err := a.StartFakeSite(); err != nil {
		log.Printf("fake site: %v", err)
	}
	a.check2FARecovery()
	go a.sys.loop(a.ctx)
	go a.jobs()
	go a.StartShop()
	go a.ensureMTG()
	for {
		srv, err := a.startServers()
		if err != nil {
			return err
		}
		select {
		case <-a.restartPanel:
			log.Printf("restarting web server")
			srv.shutdown()
			time.Sleep(300 * time.Millisecond)
		case <-a.ctx.Done():
			srv.shutdown()
			a.Xray.Stop()
			a.StopFakeSite()
			mtg.stop()
			a.Store.Flush()
			return nil
		}
	}
}

func (a *App) Shutdown() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) RequestPanelRestart() {
	select {
	case a.restartPanel <- struct{}{}:
	default:
	}
}

// ScheduleApply debounces config rebuilds triggered by frequent changes.
func (a *App) ScheduleApply() {
	a.applyMu.Lock()
	if a.applyPending {
		a.applyMu.Unlock()
		return
	}
	a.applyPending = true
	a.applyMu.Unlock()
	go func() {
		time.Sleep(700 * time.Millisecond)
		a.applyMu.Lock()
		a.applyPending = false
		a.applyMu.Unlock()
		if err := a.Xray.Apply(); err != nil {
			log.Printf("apply: %v", err)
			a.Store.Update(func(d *Data) error {
				d.AddEvent("bad", "Xray не применил изменения: "+err.Error(), "")
				return nil
			})
		}
	}()
}

func detectPublicIP() string {
	for _, u := range []string{"https://api.ipify.org", "https://ifconfig.me/ip", "https://ipv4.icanhazip.com"} {
		b, err := httpGetBytesTimeout(u, 64, 5*time.Second)
		if err == nil {
			ip := net.ParseIP(trimSpace(string(b)))
			if ip != nil {
				return ip.String()
			}
		}
	}
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).IP.String()
	}
	return ""
}
