package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/errorlens-io/errorlens/internal/api"
	"github.com/errorlens-io/errorlens/internal/config"
	"github.com/errorlens-io/errorlens/internal/testutil"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newEnv(t *testing.T) (*httptest.Server, func() *client) {
	t.Helper()
	d := testutil.NewDB(t)
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"ERRORLENS_DATABASE_URL": "unused", "ERRORLENS_PUBLIC_URL": srv.URL}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}}
	srv.Config.Handler = api.New(cfg, d, slog.New(slog.NewTextHandler(io.Discard, nil)), ui).Handler()
	return srv, func() *client {
		jar, _ := cookiejar.New(nil)
		return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
	}
}

func (c *client) do(method, path string, body any, headers ...string) (int, map[string]any, []any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var obj map[string]any
	var arr []any
	if json.Unmarshal(raw, &obj) != nil {
		_ = json.Unmarshal(raw, &arr)
	}
	return resp.StatusCode, obj, arr
}

const pw = "a long secure password"

func TestSetupLoginProjectsFlow(t *testing.T) {
	_, newClient := newEnv(t)
	c := newClient()

	if code, body, _ := c.do("GET", "/api/0/setup/status", nil); code != 200 || body["needsSetup"] != true {
		t.Fatalf("setup status: %d %v", code, body)
	}
	if code, _, _ := c.do("GET", "/api/0/auth/me", nil); code != 401 {
		t.Fatalf("me before login = %d", code)
	}

	// Weak password rejected.
	code, _, _ := c.do("POST", "/api/0/setup", map[string]string{
		"email": "owner@example.com", "password": "short", "organizationName": "Acme"})
	if code != 400 {
		t.Fatalf("weak password setup = %d", code)
	}

	code, body, _ := c.do("POST", "/api/0/setup", map[string]string{
		"name": "Owner", "email": "Owner@Example.com", "password": pw, "organizationName": "Acme Inc"})
	if code != 201 {
		t.Fatalf("setup = %d %v", code, body)
	}
	if org := body["organization"].(map[string]any); org["slug"] != "acme-inc" {
		t.Fatalf("org = %v", org)
	}

	// Setup is one-time only.
	if code, _, _ := newClient().do("POST", "/api/0/setup", map[string]string{
		"email": "evil@example.com", "password": pw, "organizationName": "Evil"}); code != 409 {
		t.Fatalf("second setup = %d", code)
	}

	// Setup logged us in.
	code, body, _ = c.do("GET", "/api/0/auth/me", nil)
	if code != 200 || body["user"].(map[string]any)["email"] != "owner@example.com" {
		t.Fatalf("me = %d %v", code, body)
	}

	// Create a project and get its DSN.
	code, body, _ = c.do("POST", "/api/0/organizations/acme-inc/projects", map[string]string{"name": "Backend API", "platform": "python"})
	if code != 201 {
		t.Fatalf("create project = %d %v", code, body)
	}
	dsnRe := regexp.MustCompile(`^http://[0-9a-f]{32}@127\.0\.0\.1:\d+/\d+$`)
	if !dsnRe.MatchString(body["dsn"].(string)) {
		t.Fatalf("bad dsn %v", body["dsn"])
	}
	if code, _, _ := c.do("POST", "/api/0/organizations/acme-inc/projects", map[string]string{"name": "Backend API"}); code != 409 {
		t.Fatalf("duplicate project = %d", code)
	}
	if code, _, _ := c.do("POST", "/api/0/organizations/acme-inc/projects", map[string]string{"name": "X", "platform": "cobol"}); code != 400 {
		t.Fatalf("bad platform = %d", code)
	}

	_, _, list := c.do("GET", "/api/0/organizations/acme-inc/projects", nil)
	if len(list) != 1 {
		t.Fatalf("projects = %v", list)
	}
	if code, _, _ := c.do("GET", "/api/0/organizations/other/projects", nil); code != 404 {
		t.Fatalf("unknown org = %d", code)
	}

	_, _, logEntries := c.do("GET", "/api/0/organizations/acme-inc/audit-log", nil)
	actions := map[string]bool{}
	for _, e := range logEntries {
		actions[e.(map[string]any)["action"].(string)] = true
	}
	if !actions["instance.setup"] || !actions["project.create"] {
		t.Fatalf("audit actions = %v", actions)
	}

	// Logout invalidates the session.
	if code, _, _ := c.do("POST", "/api/0/auth/logout", map[string]string{}); code != 204 {
		t.Fatalf("logout = %d", code)
	}
	if code, _, _ := c.do("GET", "/api/0/auth/me", nil); code != 401 {
		t.Fatalf("me after logout = %d", code)
	}

	// Login again.
	if code, _, _ := c.do("POST", "/api/0/auth/login", map[string]string{"email": "owner@example.com", "password": "wrong password!!"}); code != 401 {
		t.Fatalf("bad login = %d", code)
	}
	if code, _, _ := c.do("POST", "/api/0/auth/login", map[string]string{"email": "nobody@example.com", "password": pw}); code != 401 {
		t.Fatalf("unknown user login = %d", code)
	}
	if code, _, _ := c.do("POST", "/api/0/auth/login", map[string]string{"email": "OWNER@example.com", "password": pw}); code != 204 {
		t.Fatalf("login = %d", code)
	}
	if code, _, _ := c.do("GET", "/api/0/auth/me", nil); code != 200 {
		t.Fatalf("me after login = %d", code)
	}
}

func TestCSRFProtection(t *testing.T) {
	_, newClient := newEnv(t)
	c := newClient()
	body := map[string]string{"email": "a@example.com", "password": pw}

	if code, _, _ := c.do("POST", "/api/0/auth/login", body, "Origin", "https://evil.example"); code != 403 {
		t.Fatalf("cross-origin login = %d", code)
	}
	// Form posts (no JSON content type) are rejected.
	req, _ := http.NewRequest("POST", c.base+"/api/0/auth/login", bytes.NewReader([]byte("email=a")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Fatalf("form post = %d", resp.StatusCode)
	}
}

func TestUIAndHealth(t *testing.T) {
	srv, _ := newEnv(t)
	for path, want := range map[string]int{"/healthz": 200, "/readyz": 200, "/": 200, "/some/client/route": 200} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s = %d", path, resp.StatusCode)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s missing CSP header", path)
		}
	}
	resp, _ := http.Get(srv.URL + "/api/0/nope")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("unknown api route = %d", resp.StatusCode)
	}
}

func TestSetupAccountIsVerified(t *testing.T) {
	_, newClient := newEnv(t)
	c := newClient()
	c.do("POST", "/api/0/setup", map[string]string{"email": "v@example.com", "password": pw, "organizationName": "V"})
	_, me, _ := c.do("GET", "/api/0/auth/me", nil)
	if me["user"].(map[string]any)["emailVerified"] != true {
		t.Fatalf("setup user not verified: %v", me)
	}
}
