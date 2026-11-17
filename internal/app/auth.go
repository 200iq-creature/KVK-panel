package app

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "kvk_session"

// ---------- sessions (stateless, HMAC-signed) ----------

func (a *App) makeSession(user string, ver int, days int) string {
	exp := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	payload := fmt.Sprintf("%s|%d|%d", user, ver, exp)
	var secret string
	a.Store.Read(func(d *Data) { secret = d.Settings.SessionSecret })
	return base64url(payload) + "." + hmacSign(secret, payload)
}

func base64url(s string) string {
	return strings.TrimRight(base32Enc(s), "=")
}

func base32Enc(s string) string { return base32.HexEncoding.EncodeToString([]byte(s)) }

func base32Dec(s string) (string, error) {
	if m := len(s) % 8; m != 0 {
		s += strings.Repeat("=", 8-m)
	}
	b, err := base32.HexEncoding.DecodeString(s)
	return string(b), err
}

func (a *App) checkSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, err := base32Dec(parts[0])
	if err != nil {
		return false
	}
	var secret, user string
	var ver int
	a.Store.Read(func(d *Data) {
		secret = d.Settings.SessionSecret
		user = d.Admin.Username
		ver = d.Admin.SessionVersion
	})
	if subtle.ConstantTimeCompare([]byte(hmacSign(secret, payload)), []byte(parts[1])) != 1 {
		return false
	}
	f := strings.Split(payload, "|")
	if len(f) != 3 || f[0] != user || f[1] != strconv.Itoa(ver) {
		return false
	}
	exp, _ := strconv.ParseInt(f[2], 10, 64)
	return time.Now().Unix() < exp
}

func (a *App) checkToken(r *http.Request) bool {
	tok := r.Header.Get("X-API-Key")
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok = strings.TrimPrefix(h, "Bearer ")
	}
	if tok == "" {
		return false
	}
	var want string
	a.Store.Read(func(d *Data) { want = d.Settings.APIToken })
	return want != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(want)) == 1
}

// ---------- brute force ----------

type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}} }

func (l *limiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-5 * time.Minute)
	arr := l.hits[ip]
	n := 0
	for _, t := range arr {
		if t.After(cut) {
			arr[n] = t
			n++
		}
	}
	l.hits[ip] = arr[:n]
	return n >= 5
}

func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[ip] = append(l.hits[ip], time.Now())
}

func (l *limiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, ip)
}

// ---------- TOTP ----------

func totpCode(secret string, t time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil {
		return ""
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(t.Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(buf[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", v%1000000)
}

func totpVerify(secret, code string) bool {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return false
	}
	now := time.Now()
	for _, d := range []time.Duration{0, -30 * time.Second, 30 * time.Second} {
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, now.Add(d))), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func newTOTPSecret() string {
	b := make([]byte, 20)
	copy(b, []byte(RandomHex(10)))
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(RandomString(20)))
}

func totpURI(secret, user, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&period=30&digits=6",
		url.PathEscape(issuer), url.PathEscape(user), secret, url.QueryEscape(issuer))
}

// ---------- 2FA tickets ----------

type ticket struct {
	user    string
	ip      string
	ua      string
	created time.Time
	tgCode  string
	tgSent  time.Time
	tries   int
}

type tickets struct {
	mu sync.Mutex
	m  map[string]*ticket
}

func (t *tickets) put(tk *ticket) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]*ticket{}
	}
	for k, v := range t.m {
		if time.Since(v.created) > 10*time.Minute {
			delete(t.m, k)
		}
	}
	id := RandomHex(16)
	t.m[id] = tk
	return id
}

func (t *tickets) get(id string) *ticket {
	t.mu.Lock()
	defer t.mu.Unlock()
	tk := t.m[id]
	if tk == nil || time.Since(tk.created) > 10*time.Minute {
		return nil
	}
	return tk
}

func (t *tickets) del(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, id)
}

// ---------- user agent ----------

func shortUA(ua string) string {
	browser := "Браузер"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge " + uaVer(ua, "Edg/")
	case strings.Contains(ua, "OPR/"):
		browser = "Opera " + uaVer(ua, "OPR/")
	case strings.Contains(ua, "YaBrowser/"):
		browser = "Яндекс " + uaVer(ua, "YaBrowser/")
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox " + uaVer(ua, "Firefox/")
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome " + uaVer(ua, "Chrome/")
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	case ua == "":
		browser = "—"
	default:
		if len(ua) > 30 {
			browser = ua[:30]
		} else {
			browser = ua
		}
	}
	osName := ""
	switch {
	case strings.Contains(ua, "Windows"):
		osName = "Windows"
	case strings.Contains(ua, "Android"):
		osName = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		osName = "iOS"
	case strings.Contains(ua, "Mac OS"):
		osName = "macOS"
	case strings.Contains(ua, "Linux"):
		osName = "Linux"
	}
	if osName != "" {
		return browser + " · " + osName
	}
	return browser
}

func uaVer(ua, key string) string {
	i := strings.Index(ua, key)
	if i < 0 {
		return ""
	}
	s := ua[i+len(key):]
	if j := strings.IndexAny(s, ". "); j > 0 {
		return s[:j]
	}
	return s
}
