// Package e2e exercises the full server: HTTP API + ingestion + workers.
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/errorlens-io/errorlens/internal/api"
	"github.com/errorlens-io/errorlens/internal/config"
	"github.com/errorlens-io/errorlens/internal/crypto"
	"github.com/errorlens-io/errorlens/internal/db"
	"github.com/errorlens-io/errorlens/internal/logs"
	"github.com/errorlens-io/errorlens/internal/process"
	"github.com/errorlens-io/errorlens/internal/queue"
	"github.com/errorlens-io/errorlens/internal/sourcemap"
	"github.com/errorlens-io/errorlens/internal/testutil"
)

type app struct {
	t      *testing.T
	db     *db.DB
	srv    *httptest.Server
	worker *queue.Worker
	http   *http.Client
}

func newApp(t *testing.T, encrypt bool) *app {
	t.Helper()
	d := testutil.NewDB(t)
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"ERRORLENS_DATABASE_URL": "x", "ERRORLENS_PUBLIC_URL": srv.URL}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var kr *crypto.Keyring
	if encrypt {
		key := make([]byte, 32)
		rand.Read(key)
		kr, _ = crypto.NewKeyring(d, key)
	}
	server := api.New(cfg, d, log, nil).WithKeyring(kr)
	srv.Config.Handler = server.Handler()
	w := queue.NewWorker(d.Pool, log)
	w.Handle(process.JobKind, (&process.Processor{DB: d, Keyring: kr, SourceMaps: &sourcemap.Resolver{DB: d}}).Handle)
	w.Handle(logs.JobKind, logs.Store(d))
	jar, _ := cookiejar.New(nil)
	a := &app{t: t, db: d, srv: srv, worker: w, http: &http.Client{Jar: jar}}
	return a
}

func (a *app) drain() {
	for {
		worked, err := a.worker.RunOnce(context.Background())
		if err != nil {
			a.t.Fatal(err)
		}
		if !worked {
			return
		}
	}
}

func (a *app) req(method, path string, body any, headers map[string]string) (int, []byte) {
	a.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
		if headers == nil {
			headers = map[string]string{}
		}
		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	}
	r, _ := http.NewRequest(method, a.srv.URL+path, rd)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := a.http.Do(r)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// bootstrap creates the owner, org "acme" and a project; returns DSN key and project id.
func (a *app) bootstrap(platform string) (key string, seq string) {
	a.req("POST", "/api/0/setup", map[string]string{"email": "o@example.com", "password": "a long password!", "organizationName": "Acme"}, nil)
	_, b := a.req("POST", "/api/0/organizations/acme/projects", map[string]string{"name": "Web", "platform": platform}, nil)
	var p struct{ DSN string }
	json.Unmarshal(b, &p)
	at, slash := strings.Index(p.DSN, "@"), strings.LastIndex(p.DSN, "/")
	return p.DSN[strings.Index(p.DSN, "//")+2 : at], p.DSN[slash+1:]
}

func TestLogsFromEnvelopeAndOTLP(t *testing.T) {
	a := newApp(t, false)
	key, seq := a.bootstrap("node")

	item := `{"items":[{"timestamp":` + fmt.Sprint(float64(nowUnix())) + `,"level":"warn","body":"payment retry token=abc123secret",` +
		`"trace_id":"5b8efff798038103d269b633813fc60c","attributes":{"order_id":{"value":42,"type":"integer"},"password":{"value":"hunter2","type":"string"}}}]}`
	env := "{}\n{\"type\":\"log\",\"item_count\":1,\"content_type\":\"application/vnd.items.log+json\"}\n" + item + "\n"
	code, body := a.req("POST", "/api/"+seq+"/envelope/", []byte(env), map[string]string{"X-Sentry-Auth": "Sentry sentry_key=" + key})
	if code != 200 {
		t.Fatalf("envelope = %d %s", code, body)
	}

	otlp := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}},` +
		`{"key":"service.version","value":{"stringValue":"2.0.0"}}]},"scopeLogs":[{"logRecords":[` +
		`{"timeUnixNano":"` + fmt.Sprint(nowUnix()*1e9) + `","severityNumber":17,"body":{"stringValue":"db timeout"},` +
		`"attributes":[{"key":"db.system","value":{"stringValue":"postgresql"}}]}]}]}]}`
	code, body = a.req("POST", "/v1/logs", []byte(otlp), map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + key})
	if code != 200 {
		t.Fatalf("otlp = %d %s", code, body)
	}
	if code, _ := a.req("POST", "/v1/logs", []byte(otlp), map[string]string{"Content-Type": "application/json", "Authorization": "Bearer nope"}); code != 401 {
		t.Fatalf("otlp bad key = %d", code)
	}
	a.drain()

	code, body = a.req("GET", "/api/0/organizations/acme/logs", nil, nil)
	var lines []api.LogLine
	json.Unmarshal(body, &lines)
	if code != 200 || len(lines) != 2 {
		t.Fatalf("logs = %d %s", code, body)
	}
	all := string(body)
	if strings.Contains(all, "abc123secret") || strings.Contains(all, "hunter2") {
		t.Fatalf("secrets stored in logs: %s", all)
	}
	_, body = a.req("GET", "/api/0/organizations/acme/logs?severity=error", nil, nil)
	json.Unmarshal(body, &lines)
	if len(lines) != 1 || lines[0].Body != "db timeout" || lines[0].Attributes["service.name"] != "checkout" || lines[0].Release != "2.0.0" {
		t.Fatalf("filtered logs = %s", body)
	}
	_, body = a.req("GET", "/api/0/organizations/acme/logs?query=retry", nil, nil)
	json.Unmarshal(body, &lines)
	if len(lines) != 1 || lines[0].Severity != "warn" || lines[0].TraceID == "" {
		t.Fatalf("search logs = %s", body)
	}
}

// vlq encodes source map base64 VLQ values.
func vlq(vals ...int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	for _, v := range vals {
		u := v << 1
		if v < 0 {
			u = (-v << 1) | 1
		}
		for {
			d := u & 31
			u >>= 5
			if u > 0 {
				d |= 32
			}
			out.WriteByte(chars[d])
			if u == 0 {
				break
			}
		}
	}
	return out.String()
}

func TestSourceMapsWithAPIToken(t *testing.T) {
	a := newApp(t, false)
	key, seq := a.bootstrap("javascript")

	// API token for the CLI.
	_, body := a.req("POST", "/api/0/organizations/acme/tokens", map[string]string{"name": "CI", "role": "member"}, nil)
	var tok struct{ Token string }
	json.Unmarshal(body, &tok)
	if !strings.HasPrefix(tok.Token, "el_") {
		t.Fatalf("token = %s", body)
	}

	minified := "function total(c){return c.items.map(i=>i.price)}\n//# sourceMappingURL=app.min.js.map\n"
	col := strings.Index(minified, "c.items")
	original := "// cart.ts\nexport function total(cart) {\n  return cart.items.map((i) => i.price);\n}\n"
	// One segment: generated col `col` -> source 0, line 2 (0-based), col 9, name 0.
	smap := fmt.Sprintf(`{"version":3,"file":"app.min.js","sources":["webpack:///src/cart.ts"],"sourcesContent":[%q],"names":["total"],"mappings":"%s"}`,
		original, vlq(col, 0, 2, 9, 0))

	upload := func(name, content, token string) int {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		mw.WriteField("name", name)
		fw, _ := mw.CreateFormFile("file", "f")
		fw.Write([]byte(content))
		mw.Close()
		code, _ := a.req("POST", "/api/0/organizations/acme/projects/web/releases/web@1.0.0/files", b.Bytes(),
			map[string]string{"Content-Type": mw.FormDataContentType(), "Authorization": "Bearer " + token})
		return code
	}
	// Use a fresh client so only the token authenticates.
	a.http.Jar, _ = cookiejar.New(nil)
	if code := upload("~/static/app.min.js", minified, "el_wrong"); code != 401 {
		t.Fatalf("bad token upload = %d", code)
	}
	if code := upload("~/static/app.min.js", minified, tok.Token); code != 201 {
		t.Fatalf("upload js = %d", code)
	}
	if code := upload("~/static/app.min.js.map", smap, tok.Token); code != 201 {
		t.Fatalf("upload map = %d", code)
	}
	// Tokens can't reach other orgs or user-only endpoints.
	if code, _ := a.req("GET", "/api/0/auth/me", nil, map[string]string{"Authorization": "Bearer " + tok.Token}); code != 403 {
		t.Fatalf("token /me = %d", code)
	}

	ev := fmt.Sprintf(`{"platform":"javascript","release":"web@1.0.0","exception":{"values":[{"type":"TypeError",
		"value":"Cannot read properties of undefined (reading 'map')","stacktrace":{"frames":[
		{"abs_path":"https://cdn.example.com/static/app.min.js","filename":"app.min.js","function":"total","lineno":1,"colno":%d}]}}]}}`, col+1)
	if code, b := a.req("POST", "/api/"+seq+"/store/", []byte(ev), map[string]string{"X-Sentry-Auth": "Sentry sentry_key=" + key}); code != 200 {
		t.Fatalf("store = %d %s", code, b)
	}
	a.drain()

	var payload string
	a.db.Pool.QueryRow(context.Background(), `SELECT payload::text FROM events`).Scan(&payload)
	for _, want := range []string{`"filename": "src/cart.ts"`, `"lineno": 3`, `"context_line": "  return cart.items.map((i) => i.price);"`, `"in_app": true`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("frame not symbolicated (missing %s): %s", want, payload)
		}
	}
	var culprit string
	a.db.Pool.QueryRow(context.Background(), `SELECT culprit FROM issues`).Scan(&culprit)
	if !strings.Contains(culprit, "cart.ts") {
		t.Fatalf("culprit = %q", culprit)
	}
}

func TestEncryptedPayloads(t *testing.T) {
	a := newApp(t, true)
	key, seq := a.bootstrap("python")
	ev := `{"message":"order 991 failed for customer jane-doe","extra":{"note":"very private"}}`
	if code, b := a.req("POST", "/api/"+seq+"/store/", []byte(ev), map[string]string{"X-Sentry-Auth": "Sentry sentry_key=" + key}); code != 200 {
		t.Fatalf("store = %d %s", code, b)
	}
	a.drain()

	var stored string
	var ct []byte
	a.db.Pool.QueryRow(context.Background(), `SELECT payload::text, payload_ct FROM events`).Scan(&stored, &ct)
	if stored != "{}" || len(ct) == 0 || bytes.Contains(ct, []byte("very private")) {
		t.Fatalf("payload not encrypted: %s", stored)
	}
	code, body := a.req("GET", "/api/0/organizations/acme/issues", nil, nil)
	var list struct{ Issues []struct{ ID int64 } }
	json.Unmarshal(body, &list)
	if code != 200 || len(list.Issues) != 1 {
		t.Fatalf("issues = %s", body)
	}
	code, body = a.req("GET", fmt.Sprintf("/api/0/organizations/acme/issues/%d/events/latest", list.Issues[0].ID), nil, nil)
	if code != 200 || !strings.Contains(string(body), "very private") {
		t.Fatalf("decrypted event = %d %s", code, body)
	}
}

func nowUnix() int64 { return timeNow().Unix() }
