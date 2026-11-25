package config

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"ERRORLENS_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.PublicURL.String() != "http://localhost:8080" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.SecureCookies() {
		t.Fatal("http public URL must not use secure cookies")
	}
	if cfg.SessionTTL != 14*24*time.Hour {
		t.Fatalf("session ttl = %v", cfg.SessionTTL)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"missing db": {},
		"bad url":    {"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": "errors.example.com"},
		"bad level":  {"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_LOG_LEVEL": "loud"},
		"short ttl":  {"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_SESSION_TTL": "5s"},
	}
	for name, m := range cases {
		if _, err := Load(env(m)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestHTTPSPublicURL(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"ERRORLENS_DATABASE_URL": "x",
		"ERRORLENS_PUBLIC_URL":   "https://errors.example.com/",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecureCookies() || cfg.PublicURL.String() != "https://errors.example.com" {
		t.Fatalf("got %s secure=%v", cfg.PublicURL, cfg.SecureCookies())
	}
}
