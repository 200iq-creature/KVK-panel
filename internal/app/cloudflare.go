package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Cloudflare: IP ranges (to detect proxied domains) and a minimal API client
// used by the VLESS auto-setup to create DNS records and switch SSL mode.

var cfNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18",
		"108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17",
		"162.158.0.0/15", "104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
		"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
		"2a06:98c0::/29", "2c0f:f248::/32",
	} {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

func isCloudflareIP(ip net.IP) bool {
	for _, n := range cfNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// CF ports that Cloudflare proxies for HTTPS
var cfHTTPSPorts = []int{443, 2053, 2083, 2087, 2096, 8443}

type cfResp struct {
	Success bool            `json:"success"`
	Errors  []cfErr         `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func cfDo(token, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "https://api.cloudflare.com/client/v4"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Timeout: 20 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("Cloudflare недоступен: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var r cfResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("Cloudflare: HTTP %d", resp.StatusCode)
	}
	if !r.Success {
		msgs := []string{}
		for _, e := range r.Errors {
			msgs = append(msgs, fmt.Sprintf("%s (%d)", e.Message, e.Code))
		}
		if len(msgs) == 0 {
			msgs = append(msgs, fmt.Sprintf("HTTP %d", resp.StatusCode))
		}
		return fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// cfVerify checks that the token works and can see zones.
func cfVerify(token string) error {
	var zones []struct {
		ID string `json:"id"`
	}
	if err := cfDo(token, "GET", "/zones?per_page=1", nil, &zones); err != nil {
		return err
	}
	if len(zones) == 0 {
		return fmt.Errorf("Cloudflare: токен не видит ни одной зоны — добавьте домен в Cloudflare и выдайте токену права Zone:Read и DNS:Edit")
	}
	return nil
}

// cfFindZone finds the zone a hostname belongs to.
func cfFindZone(token, host string) (id, zone string, err error) {
	parts := strings.Split(strings.Trim(host, "."), ".")
	for i := 0; i+1 < len(parts); i++ {
		cand := strings.Join(parts[i:], ".")
		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := cfDo(token, "GET", "/zones?name="+url.QueryEscape(cand), nil, &zones); err != nil {
			return "", "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, zones[0].Name, nil
		}
	}
	return "", "", fmt.Errorf("домен %s не найден в аккаунте Cloudflare (или у токена нет доступа к зоне)", host)
}

type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// cfUpsertA points host to ip (A record) with the given proxy state; removes
// conflicting CNAME/AAAA records of the same name so traffic reaches this server.
func cfUpsertA(token, zoneID, host, ip string, proxied bool) (string, error) {
	var recs []cfRecord
	if err := cfDo(token, "GET", "/zones/"+zoneID+"/dns_records?per_page=100&name="+url.QueryEscape(host), nil, &recs); err != nil {
		return "", err
	}
	var a *cfRecord
	notes := []string{}
	for i := range recs {
		r := &recs[i]
		switch r.Type {
		case "A":
			if a == nil {
				a = r
			} else if err := cfDo(token, "DELETE", "/zones/"+zoneID+"/dns_records/"+r.ID, nil, nil); err == nil {
				notes = append(notes, "удалена лишняя A-запись "+r.Content)
			}
		case "AAAA", "CNAME":
			if err := cfDo(token, "DELETE", "/zones/"+zoneID+"/dns_records/"+r.ID, nil, nil); err != nil {
				return "", err
			}
			notes = append(notes, "удалена "+r.Type+" "+r.Content)
		}
	}
	body := map[string]any{"type": "A", "name": host, "content": ip, "ttl": 1, "proxied": proxied}
	if a != nil {
		if a.Content == ip && a.Proxied == proxied {
			return strings.Join(append(notes, "A-запись уже верная"), "; "), nil
		}
		if err := cfDo(token, "PATCH", "/zones/"+zoneID+"/dns_records/"+a.ID, body, nil); err != nil {
			return "", err
		}
		return strings.Join(append(notes, "A-запись обновлена"), "; "), nil
	}
	if err := cfDo(token, "POST", "/zones/"+zoneID+"/dns_records", body, nil); err != nil {
		return "", err
	}
	return strings.Join(append(notes, "A-запись создана"), "; "), nil
}

// cfSetSSL sets the zone SSL/TLS mode (off | flexible | full | strict).
func cfSetSSL(token, zoneID, mode string) error {
	return cfDo(token, "PATCH", "/zones/"+zoneID+"/settings/ssl", map[string]string{"value": mode}, nil)
}
