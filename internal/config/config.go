// Package config loads RootCause's runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	DatabaseURL string
	ListenAddr  string
	PublicURL   *url.URL
	LogLevel    slog.Level
	SessionTTL  time.Duration
}

// SecureCookies reports whether cookies must carry the Secure flag.
func (c *Config) SecureCookies() bool { return c.PublicURL.Scheme == "https" }

// Load reads configuration using getenv (os.Getenv in production).
func Load(getenv func(string) string) (*Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := &Config{
		DatabaseURL: get("ROOTCAUSE_DATABASE_URL", ""),
		ListenAddr:  get("ROOTCAUSE_LISTEN_ADDR", ":8080"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("ROOTCAUSE_DATABASE_URL is required")
	}

	pub, err := url.Parse(get("ROOTCAUSE_PUBLIC_URL", "http://localhost:8080"))
	if err != nil || (pub.Scheme != "http" && pub.Scheme != "https") || pub.Host == "" {
		return nil, fmt.Errorf("ROOTCAUSE_PUBLIC_URL must be an absolute http(s) URL")
	}
	pub.Path = strings.TrimRight(pub.Path, "/")
	cfg.PublicURL = pub

	if err := cfg.LogLevel.UnmarshalText([]byte(get("ROOTCAUSE_LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("ROOTCAUSE_LOG_LEVEL: %w", err)
	}

	ttl, err := time.ParseDuration(get("ROOTCAUSE_SESSION_TTL", "336h"))
	if err != nil || ttl < time.Minute {
		return nil, fmt.Errorf("ROOTCAUSE_SESSION_TTL must be a duration of at least 1m")
	}
	cfg.SessionTTL = ttl

	return cfg, nil
}
