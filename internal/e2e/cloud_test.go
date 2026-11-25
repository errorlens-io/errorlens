package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/rootcausehq/rootcause/internal/api"
	"github.com/rootcausehq/rootcause/internal/auth"
	"github.com/rootcausehq/rootcause/internal/billing"
	"github.com/rootcausehq/rootcause/internal/config"
	"github.com/rootcausehq/rootcause/internal/logs"
	"github.com/rootcausehq/rootcause/internal/process"
	"github.com/rootcausehq/rootcause/internal/queue"
	"github.com/rootcausehq/rootcause/internal/testutil"
	"github.com/rootcausehq/rootcause/internal/usage"
)

// newCloudApp builds the server in hosted mode with optional extras.
func newCloudApp(t *testing.T, env map[string]string, setup func(*api.Server)) *app {
	t.Helper()
	d := testutil.NewDB(t)
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	env["ROOTCAUSE_DATABASE_URL"] = "x"
	env["ROOTCAUSE_PUBLIC_URL"] = srv.URL
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := api.New(cfg, d, log, nil)
	server.Ingest().Meter = usage.NewMeter(d, log)
	if setup != nil {
		setup(server)
	}
	srv.Config.Handler = server.Handler()
	w := queue.NewWorker(d.Pool, log)
	w.Handle(process.JobKind, (&process.Processor{DB: d}).Handle)
	w.Handle(logs.JobKind, logs.Store(d))
	jar, _ := cookiejar.New(nil)
	a := &app{t: t, db: d, srv: srv, worker: w, http: &http.Client{Jar: jar}}
	t.Cleanup(func() { server.Ingest().Meter.Flush(context.Background()) })
	return a
}

func TestSignupQuotaAndAdmin(t *testing.T) {
	var server *api.Server
	a := newCloudApp(t, map[string]string{"ROOTCAUSE_SIGNUP_ENABLED": "true", "ROOTCAUSE_FREE_EVENT_QUOTA": "3"}, func(s *api.Server) { server = s })

	code, body := a.req("POST", "/api/0/auth/signup", map[string]string{"name": "Ana", "email": "ana@example.com", "password": "a long password!", "organizationName": "Ana Co"}, nil)
	if code != 201 {
		t.Fatalf("signup = %d %s", code, body)
	}
	if code, _ := a.req("POST", "/api/0/auth/signup", map[string]string{"email": "ana@example.com", "password": "a long password!", "organizationName": "X"}, nil); code != 409 {
		t.Fatalf("duplicate signup = %d", code)
	}
	_, body = a.req("POST", "/api/0/organizations/ana-co/projects", map[string]string{"name": "App", "platform": "python"}, nil)
	var p struct{ DSN string }
	json.Unmarshal(body, &p)
	key := p.DSN[strings.Index(p.DSN, "//")+2 : strings.Index(p.DSN, "@")]
	seq := p.DSN[strings.LastIndex(p.DSN, "/")+1:]

	codes := []int{}
	for i := 0; i < 5; i++ {
		c, _ := a.req("POST", "/api/"+seq+"/store/", []byte(fmt.Sprintf(`{"message":"m%d"}`, i)), map[string]string{"X-Sentry-Auth": "Sentry sentry_key=" + key})
		codes = append(codes, c)
	}
	if fmt.Sprint(codes) != "[200 200 200 429 429]" {
		t.Fatalf("quota codes = %v", codes)
	}
	server.Ingest().Meter.Flush(context.Background())
	_, body = a.req("GET", "/api/0/organizations/ana-co/billing", nil, nil)
	var b struct {
		Plan    string
		Quota   int64
		Used    int64
		Dropped int64
	}
	json.Unmarshal(body, &b)
	if b.Plan != "free" || b.Quota != 3 || b.Used != 3 || b.Dropped != 2 {
		t.Fatalf("billing = %s", body)
	}

	// A signed-up user is not an instance admin.
	if code, _ := a.req("GET", "/api/0/admin/organizations", nil, nil); code != 403 {
		t.Fatalf("non-admin console access = %d", code)
	}
	// Promote and raise the quota via the console.
	a.db.Pool.Exec(context.Background(), `UPDATE users SET is_superuser = true`)
	_, body = a.req("GET", "/api/0/admin/organizations", nil, nil)
	var orgs []struct {
		ID   string
		Plan string
	}
	json.Unmarshal(body, &orgs)
	if len(orgs) != 1 || orgs[0].Plan != "free" {
		t.Fatalf("admin orgs = %s", body)
	}
	if code, _ := a.req("PUT", "/api/0/admin/organizations/"+orgs[0].ID, map[string]any{"plan": "team", "quota": nil}, nil); code != 200 {
		t.Fatalf("admin update = %d", code)
	}
	// Second org for the same user gets a unique slug.
	code, body = a.req("POST", "/api/0/organizations", map[string]string{"name": "Ana Co"}, nil)
	if code != 201 || !strings.Contains(string(body), `"slug":"ana-co-2"`) {
		t.Fatalf("second org = %d %s", code, body)
	}
}

func TestSignupDisabledBySelfHostedDefault(t *testing.T) {
	a := newCloudApp(t, map[string]string{}, nil)
	if code, _ := a.req("POST", "/api/0/auth/signup", map[string]string{"email": "x@example.com", "password": "a long password!", "organizationName": "X"}, nil); code != 403 {
		t.Fatalf("signup on self-hosted = %d", code)
	}
}

func TestTwoFactorLogin(t *testing.T) {
	a := newCloudApp(t, map[string]string{}, nil)
	a.req("POST", "/api/0/setup", map[string]string{"email": "o@example.com", "password": "a long password!", "organizationName": "Acme"}, nil)

	_, body := a.req("POST", "/api/0/auth/mfa/setup", map[string]string{}, nil)
	var setup struct{ Secret string }
	json.Unmarshal(body, &setup)
	secret, _ := decodeB32(setup.Secret)
	if code, _ := a.req("POST", "/api/0/auth/mfa/confirm", map[string]string{"code": "000000"}, nil); code != 400 {
		t.Fatalf("wrong code confirmed: %d", code)
	}
	code, body := a.req("POST", "/api/0/auth/mfa/confirm", map[string]string{"code": auth.TOTPCode(secret, time.Now())}, nil)
	var conf struct{ RecoveryCodes []string }
	json.Unmarshal(body, &conf)
	if code != 200 || len(conf.RecoveryCodes) != 10 {
		t.Fatalf("confirm = %d %s", code, body)
	}

	a.req("POST", "/api/0/auth/logout", map[string]string{}, nil)
	code, body = a.req("POST", "/api/0/auth/login", map[string]string{"email": "o@example.com", "password": "a long password!"}, nil)
	var step struct {
		MfaRequired bool
		MfaToken    string
	}
	json.Unmarshal(body, &step)
	if code != 200 || !step.MfaRequired {
		t.Fatalf("login step 1 = %d %s", code, body)
	}
	if code, _ := a.req("GET", "/api/0/auth/me", nil, nil); code != 401 {
		t.Fatalf("session issued before second factor: %d", code)
	}
	if code, _ := a.req("POST", "/api/0/auth/login/mfa", map[string]string{"mfaToken": step.MfaToken, "code": "123456"}, nil); code != 401 {
		t.Fatalf("bad code accepted: %d", code)
	}
	// Recovery code works once.
	if code, _ := a.req("POST", "/api/0/auth/login/mfa", map[string]string{"mfaToken": step.MfaToken, "code": conf.RecoveryCodes[0]}, nil); code != 204 {
		t.Fatalf("recovery login = %d", code)
	}
	_, body = a.req("GET", "/api/0/auth/mfa", nil, nil)
	if !strings.Contains(string(body), `"recoveryCodesLeft":9`) {
		t.Fatalf("recovery code not consumed: %s", body)
	}
}

func decodeB32(s string) ([]byte, error) {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	var bits, n uint
	var out []byte
	for _, c := range s {
		bits = bits<<5 | uint(strings.IndexRune(alpha, c))
		n += 5
		if n >= 8 {
			out = append(out, byte(bits>>(n-8)))
			n -= 8
		}
	}
	return out, nil
}

func TestStripeWebhook(t *testing.T) {
	stripe := &billing.Stripe{SecretKey: "sk_test", WebhookSecret: "whsec_test", Plans: []billing.Plan{{ID: "team", PriceID: "price_team", Quota: 100000}}}
	a := newCloudApp(t, map[string]string{"ROOTCAUSE_SIGNUP_ENABLED": "true"}, func(s *api.Server) { s.WithBilling(stripe) })
	a.req("POST", "/api/0/auth/signup", map[string]string{"email": "b@example.com", "password": "a long password!", "organizationName": "Bee"}, nil)
	var orgID string
	a.db.Pool.QueryRow(context.Background(), `SELECT id::text FROM organizations`).Scan(&orgID)

	payload := []byte(fmt.Sprintf(`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{"id":"sub_1","customer":"cus_1","status":"active","metadata":{"org_id":"%s"},"items":{"data":[{"price":{"id":"price_team"}}]}}}}`, orgID))
	if code, _ := a.req("POST", "/webhooks/stripe", payload, map[string]string{"Stripe-Signature": "t=1,v1=00"}); code != 400 {
		t.Fatalf("unsigned webhook = %d", code)
	}
	if code, b := a.req("POST", "/webhooks/stripe", payload, map[string]string{"Stripe-Signature": billing.Sign("whsec_test", payload, time.Now())}); code != 200 {
		t.Fatalf("webhook = %d %s", code, b)
	}
	var plan string
	var quota int64
	a.db.Pool.QueryRow(context.Background(), `SELECT plan, monthly_event_quota FROM organizations`).Scan(&plan, &quota)
	if plan != "team" || quota != 100000 {
		t.Fatalf("plan=%s quota=%d", plan, quota)
	}
	if _, err := stripe.VerifyWebhook(payload, billing.Sign("whsec_test", payload, time.Now().Add(-time.Hour)), time.Now()); err == nil {
		t.Fatal("replayed webhook accepted")
	}
}

// fakeIdP is a minimal OpenID provider for tests.
type fakeIdP struct {
	srv   *httptest.Server
	key   *rsa.PrivateKey
	email string
	nonce string
}

func newFakeIdP(t *testing.T, email string) *fakeIdP {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	f := &fakeIdP{key: k, email: email}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	iss := f.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/authorize", "token_endpoint": iss + "/token",
			"jwks_uri": iss + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &k.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		f.nonce = r.URL.Query().Get("nonce")
		u, _ := url.Parse(r.URL.Query().Get("redirect_uri"))
		q := u.Query()
		q.Set("code", "c0de")
		q.Set("state", r.URL.Query().Get("state"))
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("code_verifier") == "" {
			http.Error(w, "pkce required", 400)
			return
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: k}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		tok, _ := jwt.Signed(signer).Claims(map[string]any{
			"iss": iss, "sub": "idp-" + f.email, "aud": "rootcause", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			"nonce": f.nonce, "email": f.email, "email_verified": true, "name": "Sam SSO",
		}).Serialize()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": tok, "expires_in": 3600})
	})
	return f
}

func TestOIDCLoginProvisionsIntoOrg(t *testing.T) {
	idp := newFakeIdP(t, "sam@corp.example")
	a := newCloudApp(t, map[string]string{}, func(s *api.Server) {
		s.WithOIDC(&api.OIDCConfig{Issuer: idp.srv.URL, ClientID: "rootcause", ClientSecret: "s", Name: "Corp",
			AllowedDomains: []string{"corp.example"}, AutoJoinOrg: "acme", DefaultRole: "member"})
	})
	a.req("POST", "/api/0/setup", map[string]string{"email": "o@example.com", "password": "a long password!", "organizationName": "Acme"}, nil)

	// A fresh browser goes through the SSO redirect dance.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	resp, err := browser.Get(a.srv.URL + "/api/0/auth/oidc/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Request.URL.Path != "/" || resp.Request.URL.Query().Get("sso_error") != "" {
		t.Fatalf("ended at %s", resp.Request.URL)
	}
	me, _ := browser.Get(a.srv.URL + "/api/0/auth/me")
	body, _ := io.ReadAll(me.Body)
	me.Body.Close()
	if me.StatusCode != 200 || !strings.Contains(string(body), `"email":"sam@corp.example"`) || !strings.Contains(string(body), `"role":"member"`) {
		t.Fatalf("sso me = %d %s", me.StatusCode, body)
	}

	// Disallowed domain is refused.
	idp.email = "eve@evil.example"
	jar2, _ := cookiejar.New(nil)
	browser2 := &http.Client{Jar: jar2}
	resp, _ = browser2.Get(a.srv.URL + "/api/0/auth/oidc/start")
	resp.Body.Close()
	if !strings.Contains(resp.Request.URL.Query().Get("sso_error"), "No RootCause account") {
		t.Fatalf("evil domain not refused: %s", resp.Request.URL)
	}
}
