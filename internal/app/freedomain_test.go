package app

import "testing"

func TestFreeIPName(t *testing.T) {
	cases := []struct {
		suffix, label, ip string
		hex               bool
		want              string
	}{
		{"sslip.io", "", "93.184.216.34", false, "93-184-216-34.sslip.io"},
		{"sslip.io", "My Site", "93.184.216.34", false, "my-site-93-184-216-34.sslip.io"},
		{"nip.io", "cdn", "93.184.216.34", true, "cdn-5db8d822.nip.io"},
		{"nip.io", "--a__b--", "1.2.3.4", false, "a-b-1-2-3-4.nip.io"},
	}
	for _, c := range cases {
		got, err := freeIPName(c.suffix, c.label, c.ip, c.hex)
		if err != nil || got != c.want {
			t.Errorf("freeIPName(%q,%q,%q,%v) = %q, %v; want %q", c.suffix, c.label, c.ip, c.hex, got, err, c.want)
		}
		if !domainRe.MatchString(got) {
			t.Errorf("%s does not pass domainRe", got)
		}
	}
	if _, err := freeIPName("sslip.io", "", "2001:db8::1", false); err == nil {
		t.Error("IPv6 must be rejected")
	}
}

func TestParseFreemyip(t *testing.T) {
	tok := "0123456789abcdef01234567"
	for _, in := range []struct{ raw, name, want string }{
		{"https://freemyip.com/update?token=" + tok + "&domain=moy-sait.freemyip.com", "", "moy-sait.freemyip.com"},
		{"https://freemyip.com/help?domain=Moy-Sait.freemyip.com&token=" + tok, "other", "moy-sait.freemyip.com"},
		{"  " + tok + " ", "moy-sait", "moy-sait.freemyip.com"},
		{"token=" + tok + "&domain=x1.freemyip.com&verbose=yes", "", "x1.freemyip.com"},
	} {
		name, token, err := parseFreemyip(in.raw, in.name)
		if err != nil || name != in.want || token != tok {
			t.Errorf("parseFreemyip(%q,%q) = %q %q %v", in.raw, in.name, name, token, err)
		}
	}
	if _, _, err := parseFreemyip(tok, ""); err == nil {
		t.Error("a bare token without a name must fail")
	}
	if _, _, err := parseFreemyip("https://freemyip.com/update?token=zz&domain=a.freemyip.com", ""); err == nil {
		t.Error("bad token must fail")
	}
}

func TestIsFreeDomain(t *testing.T) {
	for n, want := range map[string]bool{"a-1-2-3-4.sslip.io": true, "x.myaddr.dev": true, "sslip.io": false, "example.com": false, "abc.freemyip.com": true} {
		if isFreeDomain(n) != want {
			t.Errorf("isFreeDomain(%s) != %v", n, want)
		}
	}
	if !myaddrKeyRe.MatchString("Ab_-0123456789012345678901234567890123456789"[:43]) {
		t.Error("myaddr key regexp")
	}
}
