package r2

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TTL is how long an object should live. Dur == 0 means keep forever.
type TTL struct {
	Dur time.Duration
}

// IsKeep reports a permanent (never-expiring) object.
func (t TTL) IsKeep() bool { return t.Dur <= 0 }

// WholeDays returns (days, true) when the TTL is an exact multiple of 24h —
// the only shape the lifecycle (day-based) mode can express.
func (t TTL) WholeDays() (int, bool) {
	const day = 24 * time.Hour
	if t.Dur <= 0 || t.Dur%day != 0 {
		return 0, false
	}
	return int(t.Dur / day), true
}

// Human renders a compact duration like "7d", "2h30m", "45m".
func (t TTL) Human() string {
	d := t.Dur
	if d <= 0 {
		return "never"
	}
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	mins := d / time.Minute
	var b strings.Builder
	if days > 0 {
		fmt.Fprintf(&b, "%dd", days)
	}
	if hours > 0 {
		fmt.Fprintf(&b, "%dh", hours)
	}
	if mins > 0 {
		fmt.Fprintf(&b, "%dm", mins)
	}
	if b.Len() == 0 {
		return "<1m"
	}
	return b.String()
}

// ParseTTL accepts "none", a bare day count ("30"), or unit combos using
// d/h/m: "7d", "12h", "30m", "1h30m".
func ParseTTL(s string) (TTL, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "none", "0", "keep", "permanent", "forever":
		return TTL{}, nil
	}
	if n, err := strconv.Atoi(s); err == nil { // bare number => days
		if n < 1 {
			return TTL{}, fmt.Errorf("invalid ttl %q", s)
		}
		return TTL{Dur: time.Duration(n) * 24 * time.Hour}, nil
	}
	var total time.Duration
	num := ""
	matched := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			num += string(r)
			continue
		}
		if num == "" {
			return TTL{}, fmt.Errorf("invalid ttl %q (use e.g. 7d, 12h, 30m, 1h30m, none)", s)
		}
		n, _ := strconv.Atoi(num)
		switch r {
		case 'd':
			total += time.Duration(n) * 24 * time.Hour
		case 'h':
			total += time.Duration(n) * time.Hour
		case 'm':
			total += time.Duration(n) * time.Minute
		default:
			return TTL{}, fmt.Errorf("invalid ttl unit %q in %q (use d/h/m)", string(r), s)
		}
		num = ""
		matched = true
	}
	if num != "" || !matched || total <= 0 {
		return TTL{}, fmt.Errorf("invalid ttl %q (use e.g. 7d, 12h, 30m, 1h30m, none)", s)
	}
	return TTL{Dur: total}, nil
}

// BuildKey assembles the object key: [basePrefix/][tier/][token/]filename.
// tier is "" for permanent files, "7d" for day-based expiry, or the worker
// expiry prefix (e.g. "e") for precise expiry.
func BuildKey(basePrefix, tier, filename string, randomToken bool) string {
	var segs []string
	if p := strings.Trim(basePrefix, "/"); p != "" {
		segs = append(segs, p)
	}
	if t := strings.Trim(tier, "/"); t != "" {
		segs = append(segs, t)
	}
	if randomToken {
		segs = append(segs, randHex(4))
	}
	segs = append(segs, SanitizeFilename(filename))
	return strings.Join(segs, "/")
}

// TierPrefix is the key prefix matched by the lifecycle rule for a day tier.
func TierPrefix(basePrefix string, days int) string {
	if p := strings.Trim(basePrefix, "/"); p != "" {
		return fmt.Sprintf("%s/%dd/", p, days)
	}
	return fmt.Sprintf("%dd/", days)
}

var unsafeChars = regexp.MustCompile(`[\x00-\x1f<>:"\\|?*]+`)

// SanitizeFilename keeps links clean and path-safe.
func SanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	name = unsafeChars.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.Trim(name, "._")
	if name == "" {
		name = "file"
	}
	return name
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0000"
	}
	return hex.EncodeToString(b)
}

// encodeKey percent-encodes each path segment for use in a URL.
func encodeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
