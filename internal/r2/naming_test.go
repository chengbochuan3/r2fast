package r2

import (
	"testing"
	"time"

	"github.com/chengbochuan3/r2fast/internal/config"
)

func TestParseTTL(t *testing.T) {
	h := time.Hour
	d := 24 * time.Hour
	ok := map[string]time.Duration{
		"7d": 7 * d, "30": 30 * d, "1": d,
		"12h": 12 * h, "30m": 30 * time.Minute, "1h30m": 90 * time.Minute, "2h30m": 150 * time.Minute,
		"none": 0, "": 0, "keep": 0, "forever": 0,
	}
	for in, want := range ok {
		got, err := ParseTTL(in)
		if err != nil {
			t.Fatalf("ParseTTL(%q) error: %v", in, err)
		}
		if got.Dur != want {
			t.Errorf("ParseTTL(%q) = %v, want %v", in, got.Dur, want)
		}
	}
	for _, bad := range []string{"abc", "-3d", "7x", "7d5", "h"} {
		if _, err := ParseTTL(bad); err == nil {
			t.Errorf("ParseTTL(%q) should error", bad)
		}
	}
}

func TestWholeDays(t *testing.T) {
	if n, ok := (TTL{7 * 24 * time.Hour}).WholeDays(); !ok || n != 7 {
		t.Errorf("WholeDays(7d) = %d,%v", n, ok)
	}
	if _, ok := (TTL{2 * time.Hour}).WholeDays(); ok {
		t.Error("WholeDays(2h) should be false")
	}
}

func TestBuildKey(t *testing.T) {
	cases := []struct{ prefix, tier, name, want string }{
		{"", "7d", "st5large.tar", "7d/st5large.tar"},
		{"", "", "model.bin", "model.bin"},
		{"data", "e", "a b.zip", "data/e/a_b.zip"},
		{"", "1d", "../../etc/passwd", "1d/passwd"},
	}
	for _, c := range cases {
		if got := BuildKey(c.prefix, c.tier, c.name, false); got != c.want {
			t.Errorf("BuildKey(%q,%q,%q) = %q, want %q", c.prefix, c.tier, c.name, got, c.want)
		}
	}
}

func TestTierPrefix(t *testing.T) {
	if got := TierPrefix("", 7); got != "7d/" {
		t.Errorf("TierPrefix(empty,7) = %q", got)
	}
	if got := TierPrefix("data", 30); got != "data/30d/" {
		t.Errorf("TierPrefix(data,30) = %q", got)
	}
}

func TestPublicURLEncoding(t *testing.T) {
	c := &Client{cfg: &config.Config{PublicBaseURL: "https://files.example.com"}}
	if got, want := c.PublicURL("7d/a b.tar"), "https://files.example.com/7d/a%20b.tar"; got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
}
