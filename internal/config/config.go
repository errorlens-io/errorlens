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

	// TenantDomain (hosted mode), e.g. "errorlens.io": each organization's
	// dashboard lives at <org-slug>.<TenantDomain>, while sign-in, sign-up and
	// ingestion stay on PublicURL. Empty keeps /<org>/ paths on PublicURL.
	TenantDomain string
}

// SMTP configures outgoing email (alerts, invitations). Host "" disables email.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// OrgURL is the absolute dashboard URL of path (no leading slash) inside an
// organization: https://<org>.<TenantDomain>/path in tenant mode, otherwise
// <PublicURL>/<org>/path.
func (c *Config) OrgURL(org, path string) string {
	path = strings.TrimLeft(path, "/")
	if c.TenantDomain != "" {
		host := org + "." + c.TenantDomain
		if port := c.PublicURL.Port(); port != "" {
			host += ":" + port
		}
		return c.PublicURL.Scheme + "://" + host + "/" + path
	}
	return c.PublicURL.String() + "/" + org + "/" + path
}

// IsTenantOrigin reports whether origin (scheme://host) is an organization
// subdomain of TenantDomain, e.g. https://acme.errorlens.io.
func (c *Config) IsTenantOrigin(origin string) bool {
	if c.TenantDomain == "" {
		return false
	}
	prefix := c.PublicURL.Scheme + "://"
	if !strings.HasPrefix(origin, prefix) {
		return false
	}
	host := strings.TrimPrefix(origin, prefix)
	if port := c.PublicURL.Port(); port != "" {
		var ok bool
		if host, ok = strings.CutSuffix(host, ":"+port); !ok {
			return false
		}
	}
	label, ok := strings.CutSuffix(host, "."+c.TenantDomain)
	return ok && validLabel(label)
}

func validLabel(s string) bool {
	if s == "" || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
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
	if d := strings.ToLower(strings.Trim(get("ERRORLENS_TENANT_DOMAIN", ""), ".")); d != "" {
		for _, l := range strings.Split(d, ".") {
			if !validLabel(l) {
				return nil, fmt.Errorf("ERRORLENS_TENANT_DOMAIN must be a domain name like errorlens.io")
			}
		}
		if h := pub.Hostname(); h != d && !strings.HasSuffix(h, "."+d) {
			return nil, fmt.Errorf("ERRORLENS_PUBLIC_URL must be on ERRORLENS_TENANT_DOMAIN")
		}
		cfg.TenantDomain = d
	}
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
