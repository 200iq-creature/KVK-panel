package app

import (
	"context"
	"crypto/tls"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/200iq-creature/KVK-panel/web"
)

type servers struct {
	list []*http.Server
}

func (s *servers) shutdown() {
	for _, srv := range s.list {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
	}
}

// certificate reloader
type certReloader struct {
	mu       sync.Mutex
	certFile string
	keyFile  string
	mod      time.Time
	cert     *tls.Certificate
}

func (c *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fi, err := os.Stat(c.certFile)
	if err != nil {
		if c.cert != nil {
			return c.cert, nil
		}
		return nil, err
	}
	if c.cert == nil || !fi.ModTime().Equal(c.mod) {
		cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		if err != nil {
			if c.cert != nil {
				return c.cert, nil
			}
			return nil, err
		}
		c.cert = &cert
		c.mod = fi.ModTime()
	}
	return c.cert, nil
}

func (a *App) panelTLS() (*certReloader, bool) {
	var cf, kf string
	a.Store.Read(func(d *Data) {
		if d.Settings.PanelCertID != "" {
			cf, kf = a.certPaths(d.Settings.PanelCertID, d)
		}
	})
	if cf == "" || !fileExists(cf) || !fileExists(kf) {
		return nil, false
	}
	if _, err := tls.LoadX509KeyPair(cf, kf); err != nil {
		log.Printf("panel cert invalid: %v", err)
		return nil, false
	}
	return &certReloader{certFile: cf, keyFile: kf}, true
}

// panelCertFor picks the certificate by SNI: the subscription domain gets its
// own certificate, everything else — the panel's.
func (a *App) panelCertFor(panel *certReloader) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	var mu sync.Mutex
	subs := map[string]*certReloader{}
	return func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		name := strings.ToLower(strings.TrimSuffix(h.ServerName, "."))
		var cf, kf string
		a.Store.Read(func(d *Data) {
			if name != "" && name == subHostLocked(d) {
				cf, kf = a.certPaths(d.Settings.SubCertID, d)
			}
		})
		if cf != "" && fileExists(cf) && fileExists(kf) {
			mu.Lock()
			r := subs[cf]
			if r == nil {
				r = &certReloader{certFile: cf, keyFile: kf}
				subs[cf] = r
			}
			mu.Unlock()
			if c, err := r.get(h); err == nil {
				return c, nil
			}
		}
		return panel.get(h)
	}
}

func (a *App) PanelScheme() string {
	if _, okk := a.panelTLS(); okk {
		return "https"
	}
	return "http"
}

func (a *App) startServers() (*servers, error) {
	var st Settings
	a.Store.Read(func(d *Data) { st = d.Settings })
	res := &servers{}
	reloader, useTLS := a.panelTLS()

	mk := func(port int, h http.Handler) error {
		addr := net.JoinHostPort(st.PanelListen, strconv.Itoa(port))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		srv := &http.Server{Handler: h, ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second}
		if useTLS {
			srv.TLSConfig = &tls.Config{GetCertificate: a.panelCertFor(reloader), MinVersion: tls.VersionTLS12}
			ln = tls.NewListener(ln, srv.TLSConfig)
		}
		res.list = append(res.list, srv)
		go func() {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("server %s: %v", addr, err)
			}
		}()
		scheme := "http"
		if useTLS {
			scheme = "https"
		}
		log.Printf("listening on %s://%s", scheme, addr)
		return nil
	}

	main := a.handler(true, st.SubPort == 0 || st.SubPort == st.PanelPort)
	if err := mk(st.PanelPort, main); err != nil {
		return nil, err
	}
	if st.SubPort > 0 && st.SubPort != st.PanelPort {
		if err := mk(st.SubPort, a.handler(false, true)); err != nil {
			log.Printf("sub port %d: %v", st.SubPort, err)
		}
	}
	return res, nil
}

func (a *App) handler(panel, sub bool) http.Handler {
	api := a.apiMux()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var base, subPath string
		a.Store.Read(func(d *Data) { base = d.Settings.BasePath; subPath = d.Settings.SubPath })
		p := r.URL.Path
		if strings.HasPrefix(p, "/.well-known/acme-challenge/") {
			a.serveACMEChallenge(w, r)
			return
		}
		seg := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 2)
		first := seg[0]
		rest := ""
		if len(seg) == 2 {
			rest = seg[1]
		}
		if panel && first == base && base != "" {
			if p == "/"+base {
				http.Redirect(w, r, "/"+base+"/", http.StatusFound)
				return
			}
			if strings.HasPrefix(rest, "api/") {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/" + rest
				api.ServeHTTP(w, r2)
				return
			}
			a.serveSPA(w, r, rest, "/"+base+"/", "index.html")
			return
		}
		if sub && first == subPath && subPath != "" {
			if strings.HasPrefix(rest, "assets/") || rest == "logo.svg" {
				a.serveSPA(w, r, rest, "/"+subPath+"/", "sub.html")
				return
			}
			if rest != "" && !strings.Contains(rest, "/") {
				a.handleSubscription(w, r, rest)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(404)
		_, _ = w.Write([]byte("<html><head><title>404 Not Found</title></head><body><center><h1>404 Not Found</h1></center><hr><center>nginx</center></body></html>"))
	})
}

func distFS() fs.FS {
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return web.Dist
	}
	return sub
}

func (a *App) serveSPA(w http.ResponseWriter, r *http.Request, rest, base, entry string) {
	dfs := distFS()
	if rest != "" && strings.Contains(rest, ".") {
		clean := path.Clean(rest)
		b, err := fs.ReadFile(dfs, clean)
		if err == nil {
			ct := mime.TypeByExtension(path.Ext(clean))
			if ct == "" {
				ct = "application/octet-stream"
			}
			w.Header().Set("Content-Type", ct)
			if strings.HasPrefix(clean, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			_, _ = w.Write(b)
			return
		}
		if strings.HasPrefix(rest, "assets/") {
			http.NotFound(w, r)
			return
		}
	}
	b, err := fs.ReadFile(dfs, entry)
	if err != nil {
		http.Error(w, "frontend not built", 500)
		return
	}
	html := strings.Replace(string(b), "<head>", `<head><base href="`+base+`"><script>window.__BASE__="`+base+`"</script>`, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(html))
}
