// Package config loads ErrorLens's runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
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

	// TrustedProxies are networks whose X-Forwarded-For header is believed.
	TrustedProxies []*net.IPNet
	// AllowPrivateWebhooks lets alert webhooks target private/loopback
	// addresses (useful for self-hosted chat; unsafe for multi-tenant hosting).
	AllowPrivateWebhooks bool
	// SignupEnabled allows anyone to create an account and organization
	// (hosted mode). Self-hosted instances usually keep it off.
	SignupEnabled bool
	// FreeEventQuota is the monthly event quota for new organizations created
	// through sign-up (hosted mode).
	FreeEventQuota int64

	SMTP SMTP

	// EncryptionKey (base64, 32 bytes) enables encryption of stored event
	// payloads. Losing it makes encrypted events unreadable.
	EncryptionKey string
}

// SMTP configures outgoing email (alerts, invitations). Host "" disables email.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
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
		DatabaseURL: get("ERRORLENS_DATABASE_URL", ""),
		ListenAddr:  get("ERRORLENS_LISTEN_ADDR", ":8080"),
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("ERRORLENS_DATABASE_URL is required")
	}

	pub, err := url.Parse(get("ERRORLENS_PUBLIC_URL", "http://localhost:8080"))
	if err != nil || (pub.Scheme != "http" && pub.Scheme != "https") || pub.Host == "" {
		return nil, fmt.Errorf("ERRORLENS_PUBLIC_URL must be an absolute http(s) URL")
	}
	pub.Path = strings.TrimRight(pub.Path, "/")
	cfg.PublicURL = pub

	if err := cfg.LogLevel.UnmarshalText([]byte(get("ERRORLENS_LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("ERRORLENS_LOG_LEVEL: %w", err)
	}

	ttl, err := time.ParseDuration(get("ERRORLENS_SESSION_TTL", "336h"))
	if err != nil || ttl < time.Minute {
		return nil, fmt.Errorf("ERRORLENS_SESSION_TTL must be a duration of at least 1m")
	}
	cfg.SessionTTL = ttl

	for _, s := range strings.Split(get("ERRORLENS_TRUSTED_PROXIES", ""), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if strings.Contains(s, ":") {
				s += "/128"
			} else {
				s += "/32"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("ERRORLENS_TRUSTED_PROXIES: invalid network %q", s)
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, n)
	}

	cfg.AllowPrivateWebhooks = isTrue(get("ERRORLENS_ALLOW_PRIVATE_WEBHOOKS", "false"))
	cfg.SignupEnabled = isTrue(get("ERRORLENS_SIGNUP_ENABLED", "false"))
	cfg.EncryptionKey = get("ERRORLENS_ENCRYPTION_KEY", "")
	q, err := strconv.ParseInt(get("ERRORLENS_FREE_EVENT_QUOTA", "5000"), 10, 64)
	if err != nil || q < 0 {
		return nil, fmt.Errorf("ERRORLENS_FREE_EVENT_QUOTA must be a non-negative number")
	}
	cfg.FreeEventQuota = q

	cfg.SMTP = SMTP{
		Host:     get("ERRORLENS_SMTP_HOST", ""),
		Username: get("ERRORLENS_SMTP_USERNAME", ""),
		Password: get("ERRORLENS_SMTP_PASSWORD", ""),
		From:     get("ERRORLENS_SMTP_FROM", "ErrorLens <errorlens@localhost>"),
	}
	port, err := strconv.Atoi(get("ERRORLENS_SMTP_PORT", "587"))
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("ERRORLENS_SMTP_PORT must be a port number")
	}
	cfg.SMTP.Port = port

	return cfg, nil
}

func isTrue(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
