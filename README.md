# ErrorLens

**Open-source, self-hosted error tracking, performance monitoring and logs. Your errors, your servers, your eyes only.**

ErrorLens runs entirely on your infrastructure. It works with the error-reporting SDKs you already use
(just change the DSN) and with OpenTelemetry, and it is designed so sensitive data is scrubbed
**before** it is ever written to disk.

## Features

| | |
|---|---|
| **Errors** | Grouping into issues, stack traces with source context, breadcrumbs, tags, releases, environments, regressions (resolved issues reopen), resolve / ignore |
| **Privacy** | Server-side scrubbing of passwords, tokens, API keys, cookies, card numbers, JWTs, private keys and DB-URL passwords before storage · IP truncation · per-project custom rules · optional AES-256-GCM encryption of stored events · audit log of every event view · no telemetry |
| **Performance** | Transactions and spans from SDKs or OpenTelemetry · p50 / p95, throughput and failure rate · trace view joining transactions, errors and logs |
| **Logs** | From SDK loggers or OpenTelemetry · search, severity filter, trace filter, live tail |
| **Monitors** | Uptime checks for URLs · cron monitors with missed-run and timeout detection |
| **Alerts** | New issue, regression, spike and monitor alerts to email, Slack, Discord or signed webhooks |
| **JavaScript** | Source maps per release (`errorlens sourcemaps upload`) |
| **Teams** | Organizations, projects, roles (owner / admin / member / viewer), invitations, API tokens, 2FA, OIDC single sign-on |
| **Hosting** | One container + Postgres · Docker Compose and Helm · web/worker roles · multi-tenant with Postgres Row-Level Security · sign-up, quotas, usage and Stripe billing hooks for running a hosted service |

## Quick start (Docker Compose)

```bash
git clone https://github.com/errorlens-io/errorlens.git
cd errorlens
cp deploy/.env.example deploy/.env    # set a strong POSTGRES_PASSWORD
docker compose -f deploy/docker-compose.yml up -d --build
```

Open <http://localhost:8080>, create the first admin account, then create a project to get its DSN.
To see ErrorLens with data right away:

```bash
docker compose -f deploy/docker-compose.yml exec errorlens errorlens demo --dsn "<your project DSN>"
```

## Sending data

**Python**: `pip install errorlens`

```python
import errorlens
errorlens.init(dsn="https://<key>@errors.example.com/1", release="app@1.0.0", environment="production")
```

**Node.js**: `npm install @errorlens/node` · **Browser**: `npm install @errorlens/browser`

```js
import * as ErrorLens from "@errorlens/node";
ErrorLens.init({ dsn: "https://<key>@errors.example.com/1", release: "app@1.0.0" });
```

Other languages: any client that speaks the standard error-envelope protocol works with the DSN.

**OpenTelemetry**: point an OTLP/HTTP exporter (protobuf or JSON) at ErrorLens with the DSN public key.
Traces go to `/v1/traces`, logs to `/v1/logs`.

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=https://errors.example.com
OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer <dsn public key>"
```

**Cron jobs**: report runs from any script.

```bash
curl -fsS "https://errors.example.com/api/1/monitors/nightly-backup/checkin?key=<dsn public key>&status=ok"
```

**Source maps**: create an API token (Settings → API tokens), then in CI:

```bash
errorlens sourcemaps upload --url https://errors.example.com --token "$ERRORLENS_AUTH_TOKEN" \
  --org acme --project web --release web@1.2.3 --url-prefix "~/static/js" ./build/static/js
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `ERRORLENS_DATABASE_URL` | (required) | Postgres connection URL |
| `ERRORLENS_PUBLIC_URL` | `http://localhost:8080` | External URL; used in DSNs and links. Use `https://` in production |
| `ERRORLENS_LISTEN_ADDR` | `:8080` | Listen address |
| `ERRORLENS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `ERRORLENS_SESSION_TTL` | `336h` | Login session lifetime |
| `ERRORLENS_TRUSTED_PROXIES` | | Proxy IPs/CIDRs whose `X-Forwarded-For` is trusted |
| `ERRORLENS_ENCRYPTION_KEY` | | Base64 32-byte key; encrypts stored event payloads (`openssl rand -base64 32`). Back it up |
| `ERRORLENS_SMTP_HOST` / `_PORT` / `_USERNAME` / `_PASSWORD` / `_FROM` | | Outgoing email for alerts, invitations, password resets |
| `ERRORLENS_ALLOW_PRIVATE_WEBHOOKS` | `false` | Allow alert webhooks and uptime checks to reach private addresses |
| `ERRORLENS_OIDC_ISSUER` / `_CLIENT_ID` / `_CLIENT_SECRET` / `_NAME` | | Single sign-on with any OpenID Connect provider |
| `ERRORLENS_OIDC_AUTO_JOIN_ORG` / `_DEFAULT_ROLE` / `_ALLOWED_DOMAINS` | | Let SSO users from allowed domains join an organization automatically |
| `ERRORLENS_TENANT_DOMAIN` | | Hosted mode: serve each organization at `<org>.<domain>` (e.g. `errorlens.io`); needs wildcard DNS and TLS. `ERRORLENS_PUBLIC_URL` must be on this domain |
| `ERRORLENS_SIGNUP_ENABLED` | `false` | Hosted mode: anyone can create an account and organization |
| `ERRORLENS_FREE_EVENT_QUOTA` | `5000` | Monthly events for organizations created by sign-up |
| `ERRORLENS_STRIPE_SECRET_KEY` / `_WEBHOOK_SECRET` / `_PLANS` | | Hosted billing; plans as `id:price_id:monthly_events,...`; webhook at `/webhooks/stripe` |

Commands: `errorlens serve [--role=all|web|worker] [--no-migrate]`, `errorlens migrate`,
`errorlens sourcemaps upload`, `errorlens demo`, `errorlens version`.

## Kubernetes

```bash
helm install errorlens deploy/helm/errorlens \
  --set publicURL=https://errors.example.com \
  --set database.url='postgres://user:pass@db:5432/errorlens?sslmode=require'
```

The chart runs migrations as a pre-upgrade job, then separate web and worker deployments.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
make dev-db   # Postgres on :5434
make test
make run
```

## Architecture

A single Go binary serves the API, the embedded React UI and a Postgres-backed job queue; no Kafka,
Redis or ClickHouse required. Requests touching tenant data run as a restricted database role with
the organization set on the transaction, and Row-Level Security policies filter every query. Events,
logs and transactions are stored in daily partitions so retention is a cheap `DROP`. Details in
[PLAN.md](PLAN.md).

## License

[AGPL-3.0](LICENSE). The SDK packages in [`sdks/`](sdks) are MIT.
