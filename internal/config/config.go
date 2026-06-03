// Package config loads and saves r2fast settings. Credentials live in a
// per-user file (default ~/.config/r2fast/config.toml, mode 0600) or in
// environment variables — never in the repo.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	AccountID       string `toml:"account_id"`
	AccessKeyID     string `toml:"access_key_id"`
	SecretAccessKey string `toml:"secret_access_key"`
	Bucket          string `toml:"bucket"`
	Endpoint        string `toml:"endpoint"`         // optional; derived from AccountID when empty
	PublicBaseURL   string `toml:"public_base_url"`  // custom domain used for download links
	Prefix          string `toml:"prefix"`           // base key prefix (blank = bucket root)
	DefaultTTL      string `toml:"default_ttl"`      // e.g. "7d", "30d", "none"
	RandomSuffix    bool   `toml:"random_suffix"`    // add a random token to keys by default
	Expiry          string `toml:"expiry"`           // "lifecycle" (default) or "worker"
	ExpirePrefix    string `toml:"expire_prefix"`    // worker-mode key prefix (default "e")
	PartSizeMB      int64  `toml:"part_size_mb"`     // multipart part size
	Concurrency     int    `toml:"concurrency"`      // multipart upload concurrency
}

// DefaultDir returns the directory holding config.toml.
func DefaultDir() string {
	if d := os.Getenv("R2FAST_CONFIG_DIR"); d != "" {
		return d
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "r2fast")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "r2fast")
}

// Path returns the full path to config.toml.
func Path() string { return filepath.Join(DefaultDir(), "config.toml") }

// ResolvedEndpoint returns the S3 API endpoint, deriving it from the account
// ID when not set explicitly.
func (c *Config) ResolvedEndpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)
}

// BasePrefix returns the trimmed key prefix (may be empty).
func (c *Config) BasePrefix() string { return strings.Trim(c.Prefix, "/") }

// ExpiryMode returns "auto" (the default), "lifecycle", or "worker".
func (c *Config) ExpiryMode() string {
	switch strings.ToLower(c.Expiry) {
	case "worker":
		return "worker"
	case "lifecycle":
		return "lifecycle"
	default:
		return "auto"
	}
}

// ExpiringPrefix returns the worker-mode key prefix (default "e").
func (c *Config) ExpiringPrefix() string {
	if p := strings.Trim(c.ExpirePrefix, "/"); p != "" {
		return p
	}
	return "e"
}

// Load reads config.toml (if present) and overlays R2FAST_* env vars.
func Load() (*Config, error) {
	c := &Config{}
	path := Path()
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	applyEnv(c)
	if c.PartSizeMB <= 0 {
		c.PartSizeMB = 16
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 8
	}
	if c.DefaultTTL == "" {
		c.DefaultTTL = "7d"
	}
	if c.Expiry == "" {
		c.Expiry = "auto"
	}
	if c.ExpirePrefix == "" {
		c.ExpirePrefix = "e"
	}
	return c, nil
}

func applyEnv(c *Config) {
	set := func(dst *string, key string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	set(&c.AccountID, "R2FAST_ACCOUNT_ID")
	set(&c.AccessKeyID, "R2FAST_ACCESS_KEY_ID")
	set(&c.SecretAccessKey, "R2FAST_SECRET_ACCESS_KEY")
	set(&c.Bucket, "R2FAST_BUCKET")
	set(&c.Endpoint, "R2FAST_ENDPOINT")
	set(&c.PublicBaseURL, "R2FAST_PUBLIC_BASE_URL")
	set(&c.Prefix, "R2FAST_PREFIX")
	set(&c.Expiry, "R2FAST_EXPIRY")
	set(&c.ExpirePrefix, "R2FAST_EXPIRE_PREFIX")
}

// Validate checks that the fields required to talk to R2 are present.
func (c *Config) Validate() error {
	var missing []string
	if c.AccountID == "" && c.Endpoint == "" {
		missing = append(missing, "account_id")
	}
	if c.AccessKeyID == "" {
		missing = append(missing, "access_key_id")
	}
	if c.SecretAccessKey == "" {
		missing = append(missing, "secret_access_key")
	}
	if c.Bucket == "" {
		missing = append(missing, "bucket")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing config (%s) — run `r2fast config init`", strings.Join(missing, ", "))
	}
	return nil
}

// Save writes config.toml with 0600 permissions.
func (c *Config) Save() error {
	if err := os.MkdirAll(DefaultDir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(Path(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}
