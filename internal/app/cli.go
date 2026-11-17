package app

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Main is the CLI entry point.
func Main(args []string) int {
	fs := flag.NewFlagSet("kvk-panel", flag.ContinueOnError)
	dataDir := fs.String("data", envOr("KVK_DATA", "/etc/kvk-panel"), "data directory")
	binDir := fs.String("bin", envOr("KVK_BIN", "/usr/local/kvk-panel/bin"), "directory with xray binary and geo files")
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	// allow flags anywhere: split known flags from positional args
	var flagArgs, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				flagArgs = append(flagArgs, a)
				continue
			}
			if (name == "data" || name == "bin") && i+1 < len(args) {
				flagArgs = append(flagArgs, a, args[i+1])
				i++
				continue
			}
		}
		rest = append(rest, a)
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	switch cmd {
	case "version", "-v", "--version":
		fmt.Println(Version)
		return 0
	case "branch":
		fmt.Println(Branch)
		return 0
	case "help", "-h", "--help":
		printHelp()
		return 0
	case "run":
		return runServer(*dataDir, *binDir)
	}
	st, err := NewStore(filepath.Join(*dataDir, "db.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка базы:", err)
		return 1
	}
	a := &App{DataDir: *dataDir, BinDir: *binDir, Store: st}
	a.Xray = &XrayManager{app: a}
	arg := func(i int) string {
		if i < len(rest) {
			return rest[i]
		}
		return ""
	}
	switch cmd {
	case "info":
		a.printInfo(false)
	case "info-json":
		a.printInfo(true)
	case "set-port":
		p, err := strconv.Atoi(arg(0))
		if err != nil || p < 1 || p > 65535 {
			fmt.Fprintln(os.Stderr, "Порт должен быть от 1 до 65535")
			return 1
		}
		st.Update(func(d *Data) error { d.Settings.PanelPort = p; return nil })
		fmt.Println("Порт панели:", p)
		OpenFirewallPort(p, "tcp")
	case "set-path":
		p := strings.Trim(arg(0), "/")
		if p == "" {
			p = RandomString(12)
		}
		if !pathRe.MatchString(p) {
			fmt.Fprintln(os.Stderr, "Путь: 3–64 символа — латиница, цифры, - и _")
			return 1
		}
		st.Update(func(d *Data) error { d.Settings.BasePath = p; return nil })
		fmt.Println("Путь панели: /" + p + "/")
	case "set-user":
		u := arg(0)
		if len(u) < 3 {
			fmt.Fprintln(os.Stderr, "Логин: минимум 3 символа")
			return 1
		}
		st.Update(func(d *Data) error { d.Admin.Username = u; d.Admin.SessionVersion++; return nil })
		fmt.Println("Логин изменён:", u)
	case "set-pass":
		p := arg(0)
		if len(p) < 6 {
			fmt.Fprintln(os.Stderr, "Пароль: минимум 6 символов")
			return 1
		}
		st.Update(func(d *Data) error { d.Admin.PassHash = HashPassword(p); d.Admin.SessionVersion++; return nil })
		fmt.Println("Пароль изменён")
	case "reset-creds":
		u := arg(0)
		if u == "" {
			u = "admin" + RandomDigits(3)
		}
		p := arg(1)
		if p == "" {
			p = RandomString(14)
		}
		st.Update(func(d *Data) error {
			d.Admin.Username, d.Admin.PassHash = u, HashPassword(p)
			d.Admin.SessionVersion++
			d.AddEvent("warn", "Логин и пароль сброшены из консоли сервера", "")
			return nil
		})
		fmt.Println("Логин:  " + u)
		fmt.Println("Пароль: " + p)
	case "2fa-off":
		st.Update(func(d *Data) error {
			d.Admin.TOTPEnabled, d.Admin.TOTPSecret, d.Admin.TgEnabled = false, "", false
			d.AddEvent("warn", "2FA отключена из консоли сервера", "")
			return nil
		})
		fmt.Println("Двухфакторная защита отключена")
	case "https-off":
		st.Update(func(d *Data) error { d.Settings.PanelCertID = ""; return nil })
		fmt.Println("HTTPS панели выключен — панель снова доступна по http")
	case "reset-token":
		var t string
		st.Update(func(d *Data) error { d.Settings.APIToken = RandomHex(32); t = d.Settings.APIToken; return nil })
		fmt.Println("Новый API-токен:", t)
	case "backup":
		name, err := a.CreateBackup("manual")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(filepath.Join(a.backupsDir(), name))
	case "restore":
		p := arg(0)
		if !fileExists(p) {
			fmt.Fprintln(os.Stderr, "Файл не найден:", p)
			return 1
		}
		_ = os.MkdirAll(a.backupsDir(), 0o700)
		if err := a.restoreOffline(p); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Бэкап восстановлен")
	case "xray-install":
		if err := a.InstallXrayOffline(arg(0)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Xray установлен:", a.Xray.readVersion())
	case "geo-update":
		src := arg(0)
		if src == "" {
			a.Store.Read(func(d *Data) { src = d.Settings.GeoSource })
		}
		if err := a.UpdateGeoOffline(src); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Базы geoip.dat и geosite.dat обновлены (" + src + ")")
	case "xray-config":
		cfg, _ := a.BuildXrayConfig()
		b, _ := jsonIndent(cfg)
		fmt.Println(string(b))
	default:
		printHelp()
		return 2
	}
	return 0
}

func printHelp() {
	fmt.Print(`KVK Panel ` + Version + `

Использование: kvk-panel <команда> [--data DIR] [--bin DIR]

  run                  запустить панель (по умолчанию)
  info                 адрес панели, логин и порт
  set-port N           сменить порт панели
  set-path [PATH]      сменить секретный путь (без аргумента — случайный)
  set-user NAME        сменить логин
  set-pass PASS        сменить пароль
  reset-creds [U] [P]  сбросить логин и пароль (случайные, если не заданы)
  2fa-off              отключить двухфакторную защиту
  https-off            выключить HTTPS панели
  reset-token          перевыпустить API-токен
  backup               создать бэкап
  restore FILE         восстановить бэкап
  xray-install [VER]   скачать и установить Xray (последнюю версию без аргумента)
  geo-update [SRC]     скачать geoip/geosite (loyalsoldier, runetfreedom, v2fly)
  xray-config          показать сгенерированный конфиг Xray
  version              версия панели

После изменений перезапустите службу: systemctl restart kvk-panel
`)
}

func (a *App) printInfo(asJSON bool) {
	var d0 Data
	a.Store.Read(func(d *Data) { d0 = *d })
	host := d0.Settings.PublicHost
	if host == "" {
		host = detectPublicIP()
	}
	scheme := "http"
	if d0.Settings.PanelCertID != "" {
		scheme = "https"
		if dm := d0.DomainByID(d0.Settings.PanelCertID); dm != nil && dm.Kind == "domain" {
			host = dm.Name
		}
	}
	url := scheme + "://" + hostPort(host, d0.Settings.PanelPort, scheme) + "/" + d0.Settings.BasePath + "/"
	if asJSON {
		b, _ := jsonIndent(map[string]any{"url": url, "port": d0.Settings.PanelPort, "path": d0.Settings.BasePath, "user": d0.Admin.Username,
			"twofa": d0.Admin.TOTPEnabled || d0.Admin.TgEnabled, "version": Version})
		fmt.Println(string(b))
		return
	}
	fmt.Println("Адрес панели:  " + url)
	fmt.Println("Порт:          " + strconv.Itoa(d0.Settings.PanelPort))
	fmt.Println("Путь:          /" + d0.Settings.BasePath + "/")
	fmt.Println("Логин:         " + d0.Admin.Username)
	if d0.Admin.TOTPEnabled || d0.Admin.TgEnabled {
		fmt.Println("2FA:           включена")
	}
	fmt.Println("Подписки:      " + scheme + "://" + hostPort(host, func() int {
		if d0.Settings.SubPort > 0 {
			return d0.Settings.SubPort
		}
		return d0.Settings.PanelPort
	}(), scheme) + "/" + d0.Settings.SubPath + "/<id>")
	fmt.Println("Версия:        " + Version + " (ветка " + Branch + ")")
}

func runServer(dataDir, binDir string) int {
	log.SetFlags(log.LstdFlags)
	a, err := New(dataDir, binDir)
	if err != nil {
		log.Printf("init: %v", err)
		return 1
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("shutting down")
		a.Shutdown()
	}()
	if err := a.Run(); err != nil {
		log.Printf("run: %v", err)
		return 1
	}
	return 0
}
