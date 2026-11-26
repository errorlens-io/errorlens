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

func TestTenantDomain(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": "https://errorlens.io", "ERRORLENS_TENANT_DOMAIN": "ErrorLens.io.",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TenantDomain != "errorlens.io" {
		t.Fatalf("tenant domain = %q", cfg.TenantDomain)
	}
	if got := cfg.OrgURL("acme", "/issues/7"); got != "https://acme.errorlens.io/issues/7" {
		t.Fatalf("OrgURL = %s", got)
	}
	for origin, want := range map[string]bool{
		"https://acme.errorlens.io":      true,
		"https://a-1.errorlens.io":       true,
		"http://acme.errorlens.io":       false, // scheme must match
		"https://x.acme.errorlens.io":    false, // one label only
		"https://acme.errorlens.io.evil": false,
		"https://evilerrorlens.io":       false,
		"https://-bad.errorlens.io":      false,
		"https://acme.errorlens.io:8443": false,
	} {
		if got := cfg.IsTenantOrigin(origin); got != want {
			t.Errorf("IsTenantOrigin(%q) = %v, want %v", origin, got, want)
		}
	}

	local, err := Load(env(map[string]string{"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": "http://errorlens.localhost:8080", "ERRORLENS_TENANT_DOMAIN": "errorlens.localhost"}))
	if err != nil {
		t.Fatal(err)
	}
	if !local.IsTenantOrigin("http://acme.errorlens.localhost:8080") || local.IsTenantOrigin("http://acme.errorlens.localhost:9999") {
		t.Fatal("port handling")
	}

	plain, _ := Load(env(map[string]string{"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": "https://errors.example.com/el"}))
	if got := plain.OrgURL("acme", "issues/7"); got != "https://errors.example.com/el/acme/issues/7" {
		t.Fatalf("self-hosted OrgURL = %s", got)
	}
	if plain.IsTenantOrigin("https://acme.example.com") {
		t.Fatal("no tenant origins without a tenant domain")
	}

	for name, m := range map[string]map[string]string{
		"not a domain": {"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": "https://errorlens.io", "ERRORLENS_TENANT_DOMAIN": "errorlens.io/x"},
		"other domain": {"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": "https://errorlens.io", "ERRORLENS_TENANT_DOMAIN": "example.com"},
	} {
		if _, err := Load(env(m)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
