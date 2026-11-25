package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// demoCmd sends realistic sample data (errors, logs, transactions, cron
// check-ins) to a project through the public ingestion endpoints:
//
//	rootcause demo --dsn https://<key>@errors.example.com/1
func demoCmd(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	dsn := fs.String("dsn", "", "project DSN to send demo data to")
	platform := fs.String("platform", "python", "python | javascript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	u, err := url.Parse(*dsn)
	if err != nil || u.User == nil || u.Host == "" {
		return errors.New("--dsn is required (copy it from the project page)")
	}
	key := u.User.Username()
	project := u.Path[strings.LastIndex(u.Path, "/")+1:]
	base := u.Scheme + "://" + u.Host + strings.TrimSuffix(u.Path[:strings.LastIndex(u.Path, "/")], "/")
	endpoint := fmt.Sprintf("%s/api/%s/envelope/", base, project)
	client := &http.Client{Timeout: 15 * time.Second}

	send := func(items ...any) error {
		var b bytes.Buffer
		b.WriteString("{}\n")
		for _, it := range items {
			payload, _ := json.Marshal(it)
			typ := "event"
			if m, ok := it.(map[string]any); ok {
				if t, ok := m["__type"].(string); ok {
					typ = t
					delete(m, "__type")
					payload, _ = json.Marshal(m)
				}
			}
			fmt.Fprintf(&b, "{\"type\":%q,\"length\":%d}\n", typ, len(payload))
			b.Write(payload)
			b.WriteString("\n")
		}
		req, _ := http.NewRequest(http.MethodPost, endpoint, &b)
		req.Header.Set("X-Sentry-Auth", "Sentry sentry_version=7, sentry_client=rootcause-demo/1.0, sentry_key="+key)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("server responded %d", resp.StatusCode)
		}
		return nil
	}

	now := time.Now()
	releases := []string{"shop@2.3.0", "shop@2.4.0", "shop@2.4.1"}
	users := []string{"u-1001", "u-1002", "u-1003", "u-1004", "u-1005", "u-1006"}
	envs := []string{"production", "production", "production", "staging"}
	pick := func(xs []string) string { return xs[rand.IntN(len(xs))] }
	sent := 0

	frames := func(inApp ...[4]string) []any {
		out := []any{}
		if *platform == "python" {
			out = append(out, map[string]any{"filename": "flask/app.py", "module": "flask.app", "function": "full_dispatch_request", "lineno": 867, "in_app": false})
		} else {
			out = append(out, map[string]any{"filename": "node_modules/react-dom/cjs/react-dom.production.js", "function": "commitRoot", "lineno": 1, "colno": 120554, "in_app": false})
		}
		for _, f := range inApp {
			out = append(out, map[string]any{"filename": f[0], "module": strings.TrimSuffix(strings.ReplaceAll(f[0], "/", "."), ".py"),
				"function": f[1], "lineno": 40 + rand.IntN(60), "in_app": true,
				"pre_context": []string{"    order = load_order(order_id)", "    items = order.items"}, "context_line": f[2],
				"post_context": []string{"    return render(total)", ""}, "vars": map[string]any{"order_id": 4000 + rand.IntN(999), "currency": "EUR", "api_key": "sk_live_51HxDemoKeyShouldBeFiltered"}})
		}
		return out
	}
	type errSpec struct {
		typ, value string
		frames     []any
		level      string
		weight     int
	}
	var specs []errSpec
	if *platform == "python" {
		specs = []errSpec{
			{"ZeroDivisionError", "division by zero", frames([4]string{"shop/pricing.py", "average_price", "    return total / len(items)"}), "error", 14},
			{"KeyError", "'shipping_address'", frames([4]string{"shop/checkout.py", "build_shipment", "    addr = payload['shipping_address']"}), "error", 9},
			{"psycopg.OperationalError", "connection to server at \"db.internal\" (10.0.0.5), port 5432 failed: timeout expired", frames([4]string{"shop/db.py", "connect", "    conn = psycopg.connect(DATABASE_URL)"}), "fatal", 4},
			{"PaymentDeclined", "card declined: insufficient_funds (order 4471)", frames([4]string{"shop/payments.py", "charge", "    raise PaymentDeclined(resp.code)"}), "warning", 6},
		}
	} else {
		specs = []errSpec{
			{"TypeError", "Cannot read properties of undefined (reading 'map')", frames([4]string{"src/components/Cart.tsx", "CartItems", "  return cart.items.map((i) => <Item key={i.id} {...i} />);"}), "error", 12},
			{"ChunkLoadError", "Loading chunk 742 failed.", frames([4]string{"src/routes/lazy.ts", "loadRoute", "  const mod = await import(`./pages/${name}`);"}), "error", 5},
			{"Error", "Request failed with status code 502", frames([4]string{"src/api/client.ts", "request", "  throw new Error(`Request failed with status code ${res.status}`);"}), "error", 7},
		}
	}

	for _, sp := range specs {
		for i := 0; i < sp.weight; i++ {
			traceID := strings.ReplaceAll(uuid.NewString(), "-", "")
			ev := map[string]any{
				"event_id":    strings.ReplaceAll(uuid.NewString(), "-", ""),
				"timestamp":   float64(now.Add(-time.Duration(rand.IntN(20*60)) * time.Minute).Unix()),
				"platform":    *platform,
				"level":       sp.level,
				"release":     pick(releases),
				"environment": pick(envs),
				"server_name": fmt.Sprintf("web-%d", 1+rand.IntN(3)),
				"user":        map[string]any{"id": pick(users), "ip_address": fmt.Sprintf("203.0.113.%d", rand.IntN(250))},
				"tags":        map[string]any{"region": pick([]string{"eu-west-1", "us-east-1"}), "feature": pick([]string{"checkout", "cart", "search"})},
				"exception":   map[string]any{"values": []any{map[string]any{"type": sp.typ, "value": sp.value, "stacktrace": map[string]any{"frames": sp.frames}, "mechanism": map[string]any{"type": "generic", "handled": rand.IntN(3) > 0}}}},
				"request":     map[string]any{"method": "POST", "url": "https://shop.example.com/api/checkout", "headers": map[string]any{"Authorization": "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJkZW1vIn0.c2lnbmF0dXJl", "User-Agent": "Mozilla/5.0"}, "data": map[string]any{"card_number": "4242 4242 4242 4242", "quantity": 2}},
				"contexts":    map[string]any{"trace": map[string]any{"trace_id": traceID, "span_id": "a1b2c3d4e5f60718"}, "runtime": map[string]any{"name": map[bool]string{true: "CPython", false: "Chrome"}[*platform == "python"], "version": "3.12.4"}, "os": map[string]any{"name": "Linux", "version": "6.8"}},
				"breadcrumbs": map[string]any{"values": []any{
					map[string]any{"timestamp": float64(now.Unix() - 3), "category": "http", "type": "http", "data": map[string]any{"method": "GET", "url": "/api/cart", "status_code": 200}},
					map[string]any{"timestamp": float64(now.Unix() - 2), "category": "query", "message": "SELECT * FROM orders WHERE id = %s"},
					map[string]any{"timestamp": float64(now.Unix() - 1), "category": "auth", "message": "user signed in", "level": "info"},
				}},
			}
			start := ev["timestamp"].(float64) - 0.3 // the request began just before the error
			tx := map[string]any{"__type": "transaction", "type": "transaction", "event_id": strings.ReplaceAll(uuid.NewString(), "-", ""),
				"transaction": "POST /api/checkout", "start_timestamp": start, "timestamp": start + 0.3 + rand.Float64()*1.5,
				"release": ev["release"], "environment": ev["environment"],
				"contexts": map[string]any{"trace": map[string]any{"trace_id": traceID, "span_id": "a1b2c3d4e5f60718", "op": "http.server", "status": "internal_error"}},
				"spans": []any{
					map[string]any{"span_id": "b1", "parent_span_id": "a1b2c3d4e5f60718", "op": "db.query", "description": "SELECT * FROM orders WHERE id = $1", "start_timestamp": start + 0.01, "timestamp": start + 0.09},
					map[string]any{"span_id": "b2", "parent_span_id": "a1b2c3d4e5f60718", "op": "http.client", "description": "POST https://api.payments.example/charge", "start_timestamp": start + 0.1, "timestamp": start + 0.28},
				}}
			logItem := map[string]any{"__type": "log", "items": []any{
				map[string]any{"timestamp": start, "level": "info", "body": "checkout started", "trace_id": traceID, "attributes": map[string]any{"order_id": map[string]any{"value": 4000 + i, "type": "integer"}}},
				map[string]any{"timestamp": start + 0.2, "level": "error", "body": sp.typ + ": " + sp.value, "trace_id": traceID, "attributes": map[string]any{"service": map[string]any{"value": "shop-api", "type": "string"}}},
			}}
			if err := send(ev, tx, logItem); err != nil {
				return err
			}
			sent++
		}
	}

	// Healthy traffic for the performance page.
	routes := []struct {
		name string
		base float64
	}{{"GET /api/products", 0.04}, {"GET /api/cart", 0.07}, {"POST /api/cart/items", 0.12}, {"GET /api/search", 0.25}, {"POST /api/checkout", 0.6}}
	for i := 0; i < 120; i++ {
		r := routes[rand.IntN(len(routes))]
		start := float64(now.Add(-time.Duration(rand.IntN(3600))*time.Second).UnixMilli()) / 1000
		dur := r.base * (0.6 + rand.ExpFloat64())
		status := "ok"
		if rand.IntN(40) == 0 {
			status = "internal_error"
		}
		tx := map[string]any{"__type": "transaction", "type": "transaction", "event_id": strings.ReplaceAll(uuid.NewString(), "-", ""),
			"transaction": r.name, "start_timestamp": start, "timestamp": start + dur, "release": pick(releases), "environment": "production",
			"contexts": map[string]any{"trace": map[string]any{"trace_id": strings.ReplaceAll(uuid.NewString(), "-", ""), "span_id": "c0ffee0000000001", "op": "http.server", "status": status}},
			"spans":    []any{map[string]any{"span_id": "d1", "parent_span_id": "c0ffee0000000001", "op": "db.query", "description": "SELECT id, name, price FROM products WHERE active", "start_timestamp": start + dur*0.1, "timestamp": start + dur*0.6}}}
		if err := send(tx); err != nil {
			return err
		}
	}

	// Cron monitors via check-ins with config (creates the monitors).
	for _, c := range []struct {
		slug, sched string
		status      string
	}{{"nightly-invoices", "0 2 * * *", "ok"}, {"sync-inventory", "*/15 * * * *", "error"}} {
		ci := map[string]any{"__type": "check_in", "check_in_id": uuid.NewString(), "monitor_slug": c.slug, "status": c.status, "duration": 4.2,
			"monitor_config": map[string]any{"schedule": map[string]any{"type": "crontab", "value": c.sched}, "checkin_margin": 5, "max_runtime": 30, "timezone": "UTC"}}
		if err := send(ci); err != nil {
			return err
		}
	}

	fmt.Printf("Sent %d errors with traces and logs, 120 transactions and 2 cron check-ins to project %s.\n", sent, project)
	return nil
}
