package app

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const acmeDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"

// in-memory http-01 challenge responses (token -> keyAuth)
var acmeTokens sync.Map

func (a *App) serveACMEChallenge(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
	if v, okk := acmeTokens.Load(token); okk {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(v.(string)))
		return
	}
	http.NotFound(w, r)
}

type acmeClient struct {
	dir   map[string]any
	key   *ecdsa.PrivateKey
	kid   string
	nonce string
	hc    *http.Client
}

func b64u(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (c *acmeClient) jwk() map[string]string {
	pub := c.key.PublicKey
	return map[string]string{"crv": "P-256", "kty": "EC", "x": b64u(pad32(pub.X.Bytes())), "y": b64u(pad32(pub.Y.Bytes()))}
}

func pad32(b []byte) []byte {
	if len(b) >= 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func (c *acmeClient) thumbprint() string {
	j := c.jwk()
	s := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s","y":"%s"}`, j["crv"], j["kty"], j["x"], j["y"])
	h := sha256.Sum256([]byte(s))
	return b64u(h[:])
}

func (c *acmeClient) dirURL(k string) string {
	if v, okk := c.dir[k].(string); okk {
		return v
	}
	return ""
}

func (c *acmeClient) getNonce() (string, error) {
	if c.nonce != "" {
		n := c.nonce
		c.nonce = ""
		return n, nil
	}
	resp, err := c.hc.Head(c.dirURL("newNonce"))
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	n := resp.Header.Get("Replay-Nonce")
	if n == "" {
		return "", errors.New("ACME: нет nonce")
	}
	return n, nil
}

type acmeProblem struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
}

// post sends a JWS-signed request. payload nil => POST-as-GET.
func (c *acmeClient) post(url string, payload any, useJWK bool) (*http.Response, []byte, error) {
	for attempt := 0; attempt < 3; attempt++ {
		nonce, err := c.getNonce()
		if err != nil {
			return nil, nil, err
		}
		prot := map[string]any{"alg": "ES256", "nonce": nonce, "url": url}
		if useJWK {
			prot["jwk"] = c.jwk()
		} else {
			prot["kid"] = c.kid
		}
		pb, _ := json.Marshal(prot)
		var payloadB64 string
		if payload != nil {
			bb, _ := json.Marshal(payload)
			payloadB64 = b64u(bb)
		}
		signIn := b64u(pb) + "." + payloadB64
		h := sha256.Sum256([]byte(signIn))
		r, s, err := ecdsa.Sign(rand.Reader, c.key, h[:])
		if err != nil {
			return nil, nil, err
		}
		sig := append(pad32(r.Bytes()), pad32(s.Bytes())...)
		body, _ := json.Marshal(map[string]string{"protected": b64u(pb), "payload": payloadB64, "signature": b64u(sig)})
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/jose+json")
		req.Header.Set("User-Agent", "KVK-Panel/"+Version)
		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, nil, err
		}
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if n := resp.Header.Get("Replay-Nonce"); n != "" {
			c.nonce = n
		}
		if resp.StatusCode >= 400 {
			var p acmeProblem
			_ = json.Unmarshal(rb, &p)
			if strings.HasSuffix(p.Type, ":badNonce") {
				continue
			}
			return resp, rb, fmt.Errorf("Let's Encrypt: %s", orDefault(p.Detail, resp.Status))
		}
		return resp, rb, nil
	}
	return nil, nil, errors.New("ACME: badNonce")
}

func (a *App) newACMEClient(email string) (*acmeClient, error) {
	c := &acmeClient{hc: &http.Client{Timeout: 30 * time.Second}}
	b, err := httpGetBytes(acmeDirectoryURL, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("Let's Encrypt недоступен: %v", err)
	}
	if err := json.Unmarshal(b, &c.dir); err != nil {
		return nil, err
	}
	var keyPEM, kid string
	a.Store.Read(func(d *Data) { keyPEM, kid = d.ACMEKey, d.ACMEKid })
	if keyPEM != "" {
		if blk, _ := pem.Decode([]byte(keyPEM)); blk != nil {
			if k, err := x509.ParseECPrivateKey(blk.Bytes); err == nil {
				c.key = k
			}
		}
	}
	if c.key == nil {
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		c.key = k
		der, _ := x509.MarshalECPrivateKey(k)
		keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
		kid = ""
		a.Store.Update(func(d *Data) error { d.ACMEKey = keyPEM; d.ACMEKid = ""; return nil })
	}
	c.kid = kid
	if c.kid == "" {
		pl := map[string]any{"termsOfServiceAgreed": true}
		if email != "" {
			pl["contact"] = []string{"mailto:" + email}
		}
		resp, _, err := c.post(c.dirURL("newAccount"), pl, true)
		if err != nil {
			return nil, err
		}
		c.kid = resp.Header.Get("Location")
		if c.kid == "" {
			return nil, errors.New("ACME: не получен идентификатор аккаунта")
		}
		k := c.kid
		a.Store.Update(func(d *Data) error { d.ACMEKid = k; return nil })
	}
	return c, nil
}

// ensurePort80 makes sure something answers http-01 on port 80.
// If the port is free a temporary server is started; returns a stop func.
func (a *App) ensurePort80() (func(), error) {
	var panelPort, subPort int
	a.Store.Read(func(d *Data) { panelPort, subPort = d.Settings.PanelPort, d.Settings.SubPort })
	if (panelPort == 80 || subPort == 80) && a.PanelScheme() == "http" {
		return func() {}, nil
	}
	ln, err := net.Listen("tcp", ":80")
	if err != nil {
		return nil, fmt.Errorf("порт 80 занят другой программой — он нужен Let's Encrypt для проверки. Освободите порт 80 (например, остановите nginx) и повторите")
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			a.serveACMEChallenge(w, r)
			return
		}
		http.NotFound(w, r)
	}), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	return func() { _ = srv.Close() }, nil
}

// issueLetsEncrypt obtains a certificate for a domain name or IP address.
func (a *App) issueLetsEncrypt(name, kind, email string) (certPEM, keyPEM []byte, err error) {
	stop, err := a.ensurePort80()
	if err != nil {
		return nil, nil, err
	}
	defer stop()
	c, err := a.newACMEClient(email)
	if err != nil {
		return nil, nil, err
	}
	idType := "dns"
	if kind == "ip" {
		idType = "ip"
	}
	order := map[string]any{"identifiers": []any{map[string]string{"type": idType, "value": name}}}
	if kind == "ip" {
		// Let's Encrypt issues IP certificates only with the short-lived profile
		order["profile"] = "shortlived"
	}
	resp, body, err := c.post(c.dirURL("newOrder"), order, false)
	if err != nil && kind == "ip" && strings.Contains(err.Error(), "profile") {
		delete(order, "profile")
		resp, body, err = c.post(c.dirURL("newOrder"), order, false)
	}
	if err != nil {
		return nil, nil, err
	}
	orderURL := resp.Header.Get("Location")
	var ord struct {
		Status         string   `json:"status"`
		Authorizations []string `json:"authorizations"`
		Finalize       string   `json:"finalize"`
		Certificate    string   `json:"certificate"`
	}
	if err := json.Unmarshal(body, &ord); err != nil {
		return nil, nil, err
	}
	thumb := c.thumbprint()
	for _, az := range ord.Authorizations {
		_, ab, err := c.post(az, nil, false)
		if err != nil {
			return nil, nil, err
		}
		var authz struct {
			Status     string `json:"status"`
			Challenges []struct {
				Type   string `json:"type"`
				URL    string `json:"url"`
				Token  string `json:"token"`
				Status string `json:"status"`
			} `json:"challenges"`
		}
		_ = json.Unmarshal(ab, &authz)
		if authz.Status == "valid" {
			continue
		}
		var chURL, token string
		for _, ch := range authz.Challenges {
			if ch.Type == "http-01" {
				chURL, token = ch.URL, ch.Token
			}
		}
		if chURL == "" {
			return nil, nil, errors.New("Let's Encrypt не предложил проверку http-01")
		}
		acmeTokens.Store(token, token+"."+thumb)
		defer acmeTokens.Delete(token)
		if _, _, err := c.post(chURL, map[string]any{}, false); err != nil {
			return nil, nil, err
		}
		deadline := time.Now().Add(90 * time.Second)
		for {
			time.Sleep(2 * time.Second)
			_, ab, err := c.post(az, nil, false)
			if err != nil {
				return nil, nil, err
			}
			var st struct {
				Status     string `json:"status"`
				Challenges []struct {
					Type  string       `json:"type"`
					Error *acmeProblem `json:"error"`
				} `json:"challenges"`
			}
			_ = json.Unmarshal(ab, &st)
			if st.Status == "valid" {
				break
			}
			if st.Status == "invalid" {
				msg := "проверка не пройдена"
				for _, ch := range st.Challenges {
					if ch.Error != nil && ch.Error.Detail != "" {
						msg = ch.Error.Detail
					}
				}
				return nil, nil, fmt.Errorf("Let's Encrypt: %s. Проверьте, что %s указывает на этот сервер и порт 80 открыт", msg, name)
			}
			if time.Now().After(deadline) {
				return nil, nil, errors.New("Let's Encrypt не ответил вовремя")
			}
		}
	}
	// CSR
	certKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.CertificateRequest{}
	if kind == "ip" {
		tpl.IPAddresses = []net.IP{net.ParseIP(name)}
	} else {
		tpl.Subject = pkix.Name{CommonName: name}
		tpl.DNSNames = []string{name}
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, tpl, certKey)
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := c.post(ord.Finalize, map[string]string{"csr": b64u(csr)}, false); err != nil {
		return nil, nil, err
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		_, ob, err := c.post(orderURL, nil, false)
		if err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(ob, &ord)
		if ord.Status == "valid" && ord.Certificate != "" {
			break
		}
		if ord.Status == "invalid" {
			return nil, nil, errors.New("Let's Encrypt отклонил заказ")
		}
		if time.Now().After(deadline) {
			return nil, nil, errors.New("Let's Encrypt не выдал сертификат вовремя")
		}
		time.Sleep(2 * time.Second)
	}
	_, certPEM, err = c.post(ord.Certificate, nil, false)
	if err != nil {
		return nil, nil, err
	}
	der, _ := x509.MarshalECPrivateKey(certKey)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	return certPEM, keyPEM, nil
}

func selfSigned(name, kind string) ([]byte, []byte, error) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name, Organization: []string{"KVK Panel"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if kind == "ip" {
		tpl.IPAddresses = []net.IP{net.ParseIP(name)}
	} else {
		tpl.DNSNames = []string{name}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, k.Public(), k)
	if err != nil {
		return nil, nil, err
	}
	kd, _ := x509.MarshalECPrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}), nil
}

// certInfo parses the leaf certificate.
func certInfo(certPEM []byte) (*x509.Certificate, error) {
	for {
		var blk *pem.Block
		blk, certPEM = pem.Decode(certPEM)
		if blk == nil {
			return nil, errors.New("сертификат не найден в PEM")
		}
		if blk.Type == "CERTIFICATE" {
			return x509.ParseCertificate(blk.Bytes)
		}
	}
}

var _ = crypto.SHA256
var _ = log.Printf
