package app

import "testing"

func TestVerAtLeast(t *testing.T) {
	for _, c := range []struct {
		v, min string
		want   bool
	}{{"1.0.7", "1.0.7", true}, {"1.0.6", "1.0.7", false}, {"1.0.10", "1.0.7", true}, {"v1.1", "1.0.7", true}, {"", "1.0.7", false}, {"0.9.9", "1.0.0", false}} {
		if got := verAtLeast(c.v, c.min); got != c.want {
			t.Errorf("verAtLeast(%q,%q)=%v", c.v, c.min, got)
		}
	}
}

func TestAutoLabelOf(t *testing.T) {
	if got := autoLabelOf("Германия VLESS Reality", "VLESS Reality"); got != "Германия" {
		t.Fatalf("got %q", got)
	}
	if got := autoLabelOf("Reality", "VLESS Reality"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestPlural(t *testing.T) {
	for n, w := range map[int]string{1: "сервер", 2: "сервера", 5: "серверов", 11: "серверов", 21: "сервер", 22: "сервера"} {
		if got := plural(n, "сервер", "сервера", "серверов"); got != w {
			t.Errorf("%d: %s", n, got)
		}
	}
}

func TestLabelRemark(t *testing.T) {
	a, b := &Inbound{Remark: "X VLESS Reality"}, &Inbound{}
	d := &Data{Inbounds: []*Inbound{a, b}}
	if got := labelRemark(d, a, "X VLESS Reality"); got != "X VLESS Reality" {
		t.Fatalf("self rename: %q", got)
	}
	if got := labelRemark(d, b, "X VLESS Reality"); got != "X VLESS Reality 2" {
		t.Fatalf("dup: %q", got)
	}
}
