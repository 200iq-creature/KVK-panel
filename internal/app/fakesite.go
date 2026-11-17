package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	fakeweb "github.com/200iq-creature/KVK-panel/fake"
)

// The decoy website. The auto-configured Reality inbound uses it as its
// "target": anyone who is not a VLESS client (browsers, scanners, Cloudflare)
// gets a real HTTPS website with a valid Let's Encrypt certificate for the
// domain. The same site reverse-proxies a secret path to the XHTTP inbound,
// so VLESS also works through a CDN (Cloudflare proxy) on the same port.

type fakeSite struct {
	mu      sync.Mutex
	tlsSrv  *http.Server
	httpSrv *http.Server
	tlsPort int
	http80  bool
	err     string

	certMu sync.Mutex
	certs  map[string]*fakeCert // by domain name
	self   map[string]*tls.Certificate
}

type fakeCert struct {
	file string
	mod  time.Time
	cert *tls.Certificate
}

var fake = &fakeSite{certs: map[string]*fakeCert{}, self: map[string]*tls.Certificate{}}

// fakeStatus is shown in the UI.
func (a *App) fakeStatus() map[string]any {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return map[string]any{"running": fake.tlsSrv != nil, "tlsPort": fake.tlsPort, "port80": fake.http80, "error": fake.err}
}

func fakeRunning80() bool {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.httpSrv != nil
}

// StartFakeSite (re)starts the decoy site according to the settings.
func (a *App) StartFakeSite() error {
	var st FakeSiteSettings
	a.Store.Read(func(d *Data) { st = d.Settings.Fake })
	a.StopFakeSite()
	if !st.Enabled || st.TLSPort == 0 {
		return nil
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(st.TLSPort))
	if err != nil {
		fake.err = "внутренний порт " + strconv.Itoa(st.TLSPort) + " занят: " + err.Error()
		return err
	}
	h := a.fakeHandler(false)
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          log.New(nullWriter{}, "", 0),
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{"h2", "http/1.1"},
			GetCertificate: a.fakeCertificate,
		},
	}
	go func() {
		if err := srv.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("fake site: %v", err)
		}
	}()
	fake.tlsSrv, fake.tlsPort, fake.err = srv, st.TLSPort, ""
	// port 80: ACME challenges, redirect to https (or the site itself for CDN "flexible" mode)
	if ln80, err := net.Listen("tcp", ":80"); err == nil {
		s80 := &http.Server{Handler: a.fakeHandler(true), ReadHeaderTimeout: 20 * time.Second, ErrorLog: log.New(nullWriter{}, "", 0)}
		go func() { _ = s80.Serve(ln80) }()
		fake.httpSrv, fake.http80 = s80, true
	} else {
		fake.http80 = false
	}
	return nil
}

func (a *App) StopFakeSite() {
	// never wait for in-flight requests while holding fake.mu: handlers read the store
	fake.mu.Lock()
	servers := []*http.Server{fake.tlsSrv, fake.httpSrv}
	fake.tlsSrv, fake.httpSrv, fake.http80 = nil, nil, false
	fake.mu.Unlock()
	for _, s := range servers {
		if s != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.Shutdown(ctx)
			cancel()
			_ = s.Close()
		}
	}
}

type nullWriter struct{}

func (nullWriter) Write(p []byte) (int, error) { return len(p), nil }

// fakeCertificate picks the certificate for the requested SNI: an issued
// certificate from «Домены» if there is one, otherwise an in-memory self-signed one.
func (a *App) fakeCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
	var main string
	var cf, kf string
	a.Store.Read(func(d *Data) {
		main = d.Settings.Fake.SNI
		if name == "" || !containsStr(d.Settings.Fake.Domains, name) {
			name = main
		}
		cf, kf = bestCertFor(d, name)
	})
	if name == "" {
		name = "localhost"
	}
	if cf != "" {
		if c := loadCached(name, cf, kf); c != nil {
			return c, nil
		}
	}
	fake.certMu.Lock()
	defer fake.certMu.Unlock()
	if c, okk := fake.self[name]; okk {
		return c, nil
	}
	certPEM, keyPEM, err := selfSigned(name, domainKind(name))
	if err != nil {
		return nil, err
	}
	c, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	fake.self[name] = &c
	return &c, nil
}

// bestCertFor returns the cert files of the best active certificate for a name
// (Let's Encrypt / own certificate preferred over self-signed).
func bestCertFor(d *Data, name string) (string, string) {
	rank := func(m string) int {
		switch m {
		case "letsencrypt":
			return 3
		case "custom":
			return 2
		}
		return 1
	}
	best, cf, kf := 0, "", ""
	for _, dm := range d.Domains {
		if dm.Name != name || dm.Status != "active" || dm.CertFile == "" {
			continue
		}
		if r := rank(dm.Mode); r > best {
			best, cf, kf = r, dm.CertFile, dm.KeyFile
		}
	}
	return cf, kf
}

func loadCached(name, cf, kf string) *tls.Certificate {
	st, err := os.Stat(cf)
	if err != nil {
		return nil
	}
	fake.certMu.Lock()
	defer fake.certMu.Unlock()
	if c := fake.certs[name]; c != nil && c.file == cf && c.mod.Equal(st.ModTime()) {
		return c.cert
	}
	pair, err := tls.LoadX509KeyPair(cf, kf)
	if err != nil {
		return nil
	}
	fake.certs[name] = &fakeCert{file: cf, mod: st.ModTime(), cert: &pair}
	return &pair
}

// fakeHandler serves the decoy site. plain=true is the port-80 variant.
func (a *App) fakeHandler(plain bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var st FakeSiteSettings
		var cdnPort, wsPort int
		a.Store.Read(func(d *Data) {
			st = d.Settings.Fake
			for _, in := range d.Inbounds {
				if !in.Enabled {
					continue
				}
				switch in.Auto {
				case "cdn":
					cdnPort = in.Port
				case "cdnws":
					wsPort = in.Port
				}
			}
		})
		p := r.URL.Path
		if strings.HasPrefix(p, "/.well-known/acme-challenge/") {
			a.serveACMEChallenge(w, r)
			return
		}
		if st.CDN && cdnPort > 0 && pathUnder(p, st.XHTTPPath) {
			xhttpProxy(cdnPort).ServeHTTP(w, r)
			return
		}
		if st.CDN && wsPort > 0 && pathUnder(p, st.WSPath) {
			xhttpProxy(wsPort).ServeHTTP(w, r) // ReverseProxy passes the WebSocket upgrade through
			return
		}
		if plain {
			// Cloudflare "Flexible" SSL talks to the origin over http — serve the site, never loop
			if !strings.Contains(r.Header.Get("CF-Visitor"), "https") {
				host := r.Host
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				target := "https://" + host
				if st.Port != 0 && st.Port != 443 {
					target += ":" + strconv.Itoa(st.Port)
				}
				http.Redirect(w, r, target+r.URL.RequestURI(), http.StatusMovedPermanently)
				return
			}
		}
		serveFakePage(w, r, st.CustomHTML)
	})
}

var fakeModTime = time.Now().Add(-72 * time.Hour).Truncate(time.Hour)

func serveFakePage(w http.ResponseWriter, r *http.Request, custom string) {
	w.Header().Set("Server", "nginx")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.TLS != nil {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	sub := fs.FS(fakeweb.FS)
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Cache-Control", "public, max-age=600")
		if strings.TrimSpace(custom) != "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", fakeModTime, bytes.NewReader([]byte(custom)))
			return
		}
		b, _ := fs.ReadFile(sub, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", fakeModTime, bytes.NewReader(b))
		return
	case "/favicon.svg", "/robots.txt":
		b, err := fs.ReadFile(sub, strings.TrimPrefix(r.URL.Path, "/"))
		if err == nil {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeContent(w, r, r.URL.Path, fakeModTime, bytes.NewReader(b))
			return
		}
	case "/favicon.ico":
		http.Redirect(w, r, "/favicon.svg", http.StatusMovedPermanently)
		return
	}
	b, _ := fs.ReadFile(sub, "404.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = w.Write(b)
	}
}

var (
	proxyMu    sync.Mutex
	proxyCache = map[int]*httputil.ReverseProxy{}
)

func pathUnder(p, base string) bool {
	return base != "" && (p == base || strings.HasPrefix(p, strings.TrimSuffix(base, "/")+"/"))
}

// xhttpProxy streams XHTTP requests to the local Xray inbound.
func xhttpProxy(port int) *httputil.ReverseProxy {
	proxyMu.Lock()
	defer proxyMu.Unlock()
	if p := proxyCache[port]; p != nil {
		return p
	}
	target, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 512,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host
			ip := pr.In.Header.Get("CF-Connecting-IP")
			if ip == "" {
				ip, _, _ = net.SplitHostPort(pr.In.RemoteAddr)
			}
			pr.Out.Header.Set("X-Forwarded-For", ip)
			pr.Out.Header.Set("X-Real-IP", ip)
		},
		Transport:     tr,
		FlushInterval: -1,
		ErrorLog:      log.New(nullWriter{}, "", 0),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Server", "nginx")
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		},
	}
	proxyCache[port] = rp
	return rp
}
