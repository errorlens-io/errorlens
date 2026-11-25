# RootCause — Architecture & MVP Plan

> **RootCause** — open-source, self-hosted error & log tracking.
> *Your errors, your servers, your eyes only.*

Status: draft v0.1 · 2026-11-25

---

## 1. Goals

1. **Privacy by construction.** Sensitive data is scrubbed *before* it is written to disk. No telemetry, no phone-home, no third-party scripts in the UI.
2. **Drop-in for existing SDKs.** Teams keep the open-source error SDKs already in their apps and only change the DSN. Every major language works on day one.
3. **One container to run.** `docker run rootcausehq/rootcause` + Postgres. Runs in ~512 MB RAM for small teams.
4. **One codebase, two deployments.** Self-hosted (single org) and RootCause Cloud (paid, multi-tenant) run the same code.
5. **Clean-room implementation.** No code copied from any proprietary or source-available product. Only the public wire protocol (documented by the open-source SDKs) is implemented.

### Non-goals for v1
Session replay, profiling, full APM/tracing UI, native crash (minidump) symbolication, mobile dSYM/ProGuard. These come later or never.

---

## 2. Tech stack (recommended)

| Layer | Choice | Why |
|---|---|---|
| Backend | **Go** | Single static binary, low memory, great concurrency for ingestion, easy Docker image (~30 MB) |
| Database | **PostgreSQL 16+** | Only required dependency. JSONB for payloads, time partitioning for retention, Row-Level Security for multi-tenancy |
| Queue | **Postgres-backed** (`SELECT … FOR UPDATE SKIP LOCKED`) | No Kafka/Redis required. Optional NATS/Redis adapter later for high volume |
| Analytics store (later) | ClickHouse adapter | Opt-in for heavy users; not in MVP |
| Frontend | **React + TypeScript + Vite**, Tailwind | Built into static files and embedded in the Go binary (`embed.FS`) |
| Migrations | `goose` or `golang-migrate` | Plain SQL migrations |
| Packaging | Docker image, docker-compose, Helm chart (later) | |

---

## 3. High-level architecture

```
  Your app (SDK)                 Browser (UI)
          │ envelope/store              │
          ▼                             ▼
 ┌──────────────────────────────────────────────┐
 │              rootcause (single binary)       │
 │                                              │
 │  ┌────────────┐   ┌──────────┐  ┌──────────┐ │
 │  │ Ingest API │──▶│ Scrubber │─▶│  Queue   │ │
 │  │ auth, rate │   │ (PII)    │  │ (PG)     │ │
 │  │ limit, gz  │   └──────────┘  └────┬─────┘ │
 │  └────────────┘                      ▼       │
 │                              ┌──────────────┐│
 │  ┌────────────┐              │  Processor   ││
 │  │  Web API + │              │ normalize,   ││
 │  │  UI (SPA)  │              │ fingerprint, ││
 │  └─────┬──────┘              │ group, store ││
 │        │                     └──────┬───────┘│
 │  ┌─────┴──────┐  ┌───────────┐      │        │
 │  │ Auth/RBAC  │  │ Alerter   │◀─────┘        │
 │  │ Audit log  │  │ email/    │               │
 │  └────────────┘  │ slack/hook│               │
 │                  └───────────┘               │
 └──────────────────────┬───────────────────────┘
                        ▼
                  PostgreSQL
```

The binary can run all roles, or be split with `--role=ingest|worker|web` for scaling. Same image either way.

**Critical rule:** the scrubber runs in the ingest path *before* the event is queued. Raw unscrubbed payloads never touch disk.

---

## 4. Ingestion (SDK protocol compatibility)

### DSN
```
https://<public_key>@rootcause.example.com/<project_id>
```

### Endpoints
| Endpoint | Purpose | MVP |
|---|---|---|
| `POST /api/{project_id}/envelope/` | Modern SDKs (all item types) | ✅ |
| `POST /api/{project_id}/store/` | Legacy SDKs (single event JSON) | ✅ |
| `POST /api/{project_id}/security/` | CSP reports | later |
| `POST /api/{project_id}/minidump/` | Native crashes | no |
| `POST /v1/logs`, `/v1/traces` | OpenTelemetry (OTLP/HTTP) | phase 2 |

### Auth
Accept the key from any of: the SDK auth header, the public-key query param, or the envelope header `dsn` (exact protocol names live in `internal/ingest/protocol.go`). Validate against `project_keys`. Handle CORS for browser SDKs (allowed origins per project).

### Envelope item types
| Item | MVP behavior |
|---|---|
| `event` | Process & store |
| `log` | Process & store |
| `client_report` | Count dropped events (stats) |
| `attachment` | Store (size-limited, scrubbed) — phase 2 |
| `transaction`, `span` | Accept with 200, discard (phase 4) |
| `session`, `sessions` | Accept, aggregate counts — phase 2 |
| `replay_*`, `profile`, `check_in` | Accept with 200, discard |

Always return 200 for well-formed but unsupported items so SDKs don't retry forever. Support `gzip`, `deflate`, `br` request bodies. Enforce max body size (default 1 MB compressed / 20 MB decompressed). Return `429` with `Retry-After` and the SDK rate-limit header when quota is exceeded.

---

## 5. Privacy & security (the product's core promise)

### 5.1 Scrubbing at ingest (default ON)
- **Key denylist** (case-insensitive substring match on any JSON key, headers, cookies, query params, form data, stack-frame local variables): `password, passwd, secret, token, api_key, apikey, auth, authorization, cookie, session, csrf, xsrf, private_key, credit, card, cvv, ssn, iban`.
- **Value patterns** (regex on string values): credit cards (Luhn-validated), JWTs, `Bearer …`, AWS access keys, GitHub/Stripe/Slack tokens, PEM private keys, IBAN, emails (optional), IPv4/IPv6 (optional).
- **IP handling:** per project — store, truncate (/24, /48), or drop.
- **Custom rules** per project: extra keys, regexes, "safe fields" allowlist.
- Replacement value: `[Filtered]`. Record which rules fired (counts only) so users can audit.
- Stack-frame `vars` collection: option to drop entirely.

### 5.2 Encryption
- **In transit:** TLS everywhere (bundled Caddy option in compose for automatic HTTPS).
- **At rest, app-level (optional, recommended for Cloud):** event payloads encrypted with AES-256-GCM using a per-organization data key (DEK). DEKs are wrapped by a master key (KEK) from env var, file, HashiCorp Vault, or AWS/GCP KMS.
- **Cloud BYOK (Enterprise):** customer supplies the KEK; revoking it makes their data unreadable, even to us.
- Searchable metadata (issue title, level, timestamps, tags) stays unencrypted for querying; full payload (stack vars, request, breadcrumbs, extra) is encrypted.

### 5.3 Access
- Passwords: argon2id. Sessions: HttpOnly, Secure, SameSite=Lax cookies. TOTP 2FA. OIDC/SAML SSO (phase 3).
- RBAC: `owner`, `admin`, `member`, `viewer`; project-level team access.
- **Audit log** (append-only): logins, settings changes, key creation, *and every view of a full event payload* (who, when, which event).
- Scoped API tokens for automation.

### 5.4 Trust guarantees
- Zero telemetry. No update-check unless the admin enables it.
- UI loads no external fonts, scripts, or analytics.
- Reproducible builds + signed images (cosign) + SBOM published per release.

---

## 6. Processing: normalization & grouping

1. **Normalize**: fill defaults, cap sizes (message 8 KB, breadcrumbs 100, frames 250), trim strings, parse user-agent into browser/OS tags.
2. **Fingerprint** (first match wins):
   1. SDK-supplied `fingerprint` (supports `{{ default }}` expansion).
   2. Project custom fingerprint rules (e.g. `error.type:DatabaseError -> db-error`).
   3. **Stack trace**: in-app frames only (fallback all frames) → `module/filename + function` per frame, with normalization (strip line numbers, hashes in filenames, anonymous function noise, webpack chunk hashes).
   4. **Exception**: `type` + normalized message (replace numbers, UUIDs, hex, emails, paths with placeholders).
   5. **Message**: normalized `logentry.message` template (prefer un-formatted template).
3. `hash = sha256(components)` → upsert into `issues` by `(project_id, hash)`.
4. Update issue: `last_seen`, `times_seen`, regression detection (resolved issue receives new event → reopen + alert).
5. Store event, tags, and bump hourly counters.

Grouping quality is the hardest part of the product. Build a **test corpus** of real SDK payloads (Python, JS, Node, Java, Go, PHP, .NET, Ruby) early and snapshot-test grouping.

---

## 7. Data model (core tables)

Every tenant-owned table has `org_id` and is protected by Postgres RLS.

```
organizations   (id, name, slug, plan, created_at)
users           (id, email, password_hash, totp_secret, created_at)
memberships     (org_id, user_id, role)
teams / team_members / project_teams
projects        (id, org_id, name, slug, platform, retention_days, scrub_config jsonb)
project_keys    (id, org_id, project_id, public_key, rate_limit, is_active)
issues          (id, org_id, project_id, hash, title, culprit, level, status,
                 first_seen, last_seen, times_seen, assignee_id, first_release, last_release)
events          (id uuid, org_id, project_id, issue_id, timestamp, level, release,
                 environment, platform, tags jsonb, payload bytea/jsonb, received_at)
                 -- PARTITION BY RANGE (received_at), daily
logs            (id, org_id, project_id, timestamp, severity, body, attributes jsonb, trace_id)
                 -- PARTITION BY RANGE (timestamp), daily
issue_counts    (org_id, project_id, issue_id, hour, count)
releases        (id, org_id, project_id, version, created_at)
alert_rules / alert_channels / alert_history
audit_log       (id, org_id, actor_id, action, target, ip, created_at)  -- append-only
usage_counters  (org_id, project_id, day, events_accepted, events_dropped, logs_bytes)
job_queue       (id, kind, payload, run_at, attempts, locked_until)
```

**Retention:** drop whole daily partitions older than `retention_days` (cheap, no vacuum storm).
**RLS:** each request does `SET LOCAL app.org_id = …`; policies `USING (org_id = current_setting('app.org_id')::bigint)`. A bug in app code can't leak across tenants.

---

## 8. Multi-tenancy & RootCause Cloud

| Tier | Isolation |
|---|---|
| Self-hosted | One org (or many, if they want) — same schema |
| Cloud Starter/Team | Shared database, RLS per org, per-org DEK encryption |
| Cloud Enterprise | Dedicated database (or dedicated cluster/region), BYOK, SSO |

Cloud-only extras (kept in a separate private module or config flags — the open-source core stays complete):
- Stripe billing, plans, usage metering from `usage_counters`
- Per-org quotas & rate limits enforced at ingest
- Org signup, email verification, abuse protection
- Data region selection (EU / US)

**Pricing idea (simple, predictable):** flat monthly tiers by event volume, no separate meters for every feature. Self-hosted is free forever.

---

## 9. UI (MVP screens)

1. Login / signup / 2FA
2. Projects list + "Setup" page showing the DSN and copy-paste SDK snippet per platform
3. **Issues list**: filters (status, level, environment, release, time), search, sort by last seen / frequency, sparkline
4. **Issue detail**: title, counts chart, tags breakdown, latest event with stack trace (in-app frames highlighted, context lines), breadcrumbs, request, user, contexts; prev/next event; resolve / ignore / assign
5. **Logs explorer**: time range, severity, full-text search, attribute filters, live tail
6. Alerts: rules + channels (email, Slack, Discord, generic webhook)
7. Settings: members & roles, project keys, scrubbing rules (with "test a payload" preview), retention, audit log viewer

---

## 10. Configuration & deployment

```bash
docker run -d -p 8000:8000 \
  -e ROOTCAUSE_DATABASE_URL=postgres://… \
  -e ROOTCAUSE_SECRET_KEY=… \
  -e ROOTCAUSE_PUBLIC_URL=https://errors.example.com \
  rootcausehq/rootcause:latest
```

- `docker-compose.yml` with Postgres (+ optional Caddy for TLS).
- First boot: run migrations automatically, print one-time admin setup link.
- Config: env vars, optional `rootcause.yaml`.
- Health: `/healthz`, `/readyz`; Prometheus `/metrics` (local only, off by default).
- Backups: documented `pg_dump` recipe + optional built-in scheduled backup to S3-compatible storage.
- Helm chart in phase 3.

**Performance targets (to validate with load tests):** 512 MB RAM idle footprint; ≥ 500 events/s on 2 vCPU + Postgres; p99 ingest latency < 50 ms (ingest only enqueues).

---

## 11. Repository layout

```
rootcause/
├── cmd/rootcause/          # main.go (serve, migrate, createuser, …)
├── internal/
│   ├── ingest/             # HTTP endpoints, envelope parser, auth, rate limit
│   ├── scrub/              # PII scrubber + rules
│   ├── process/            # normalize, fingerprint, grouping
│   ├── store/              # Postgres repositories, RLS session handling
│   ├── queue/              # PG-backed job queue
│   ├── alert/              # rules engine + channels
│   ├── auth/               # users, sessions, RBAC, 2FA, audit
│   ├── crypto/             # DEK/KEK envelope encryption
│   ├── api/                # JSON API for the UI
│   └── config/
├── migrations/
├── web/                    # React app (embedded at build)
├── testdata/sdk-payloads/  # real SDK envelopes per language for tests
├── deploy/                 # Dockerfile, compose, helm
└── docs/
```

---

## 12. Roadmap

### Phase 0 — Foundation (≈2 weeks)
- Repo, CI (lint, test, build image), license, CONTRIBUTING, SECURITY.md
- Config, Postgres connection, migrations, RLS scaffolding, job queue
- Auth (users, orgs, sessions), minimal UI shell

### Phase 1 — Errors MVP (≈6–8 weeks) → **first public release v0.1**
- Envelope + store endpoints, DSN auth, CORS, compression, size limits
- Scrubber with default rules + per-project config
- Normalization, fingerprinting, issue grouping, regression detection
- Issues list + issue detail UI, resolve/ignore/assign
- Email + Slack + webhook alerts (new issue, regression, spike)
- Retention via partition dropping, audit log
- Docker image + compose, docs site on rootcause.sh
- Test with official SDKs: Python, JavaScript (browser + Node), Go, PHP, Java

### Phase 2 — Logs & developer workflow (≈6 weeks)
- `log` envelope items + OTLP logs ingestion, logs explorer, live tail
- Releases, environments, "first seen in release"
- **JavaScript source maps** (upload via our own CLI, compatible with existing upload tooling)
- Attachments, session counts / crash-free rate
- App-level payload encryption (DEK/KEK)

### Phase 3 — RootCause Cloud (≈6–8 weeks)
- Org signup, Stripe billing, quotas, usage dashboard
- SSO (OIDC/SAML), BYOK, data regions
- Horizontal scaling (`--role` split), Helm chart, load testing
- Security audit / pentest before launch

### Phase 4 — Beyond
- Traces & performance (OTLP traces, transactions), ClickHouse adapter
- Uptime/cron monitors
- AI-free-by-default "suggested root cause" (local models only, opt-in)
- More SDK-specific symbolication (ProGuard, dSYM)

---

## 13. Open decisions

| # | Decision | Recommendation |
|---|---|---|
| 1 | **License** | **AGPL-3.0** for the server (stops others reselling a closed hosted copy, keeps it truly open source) + MIT for any CLI/SDK helpers. Alternative: Apache-2.0 for maximum adoption, accepting that clouds could resell it. |
| 2 | Backend language | Go (fast to build, tiny image). Rust only if the team already knows it. |
| 3 | Cloud-only code | Keep in a private repo/module behind interfaces; open core stays feature-complete for self-hosters. |
| 4 | Own SDKs? | Phase 1–2: reuse the existing MIT-licensed SDKs + OpenTelemetry. Later: publish RootCause-branded SDK packages (MIT allows forking/renaming) so setup docs only show RootCause names. |
| 5 | Contributor agreement | Use DCO (sign-off), not a CLA, to build community trust. |

---

## 14. Risks

| Risk | Mitigation |
|---|---|
| Grouping quality worse than incumbents → noisy issues | Real-payload test corpus, snapshot tests, user-editable fingerprint rules, merge/unmerge issues |
| Upstream SDK protocol changes | Protocol is versioned and SDKs stay backward compatible; integration tests pinned to SDK versions in CI |
| Trademark / affiliation confusion | Never use other products' names or logos anywhere in RootCause; own brand, own UI design |
| Postgres limits at high volume | Partitioning, batching inserts, COPY; ClickHouse adapter for large users |
| Security bug undermines the privacy brand | Scrub-before-disk invariant tested, RLS, fuzzing the envelope parser, external audit before Cloud launch, bug bounty |
| Domain not yet purchased | Buy rootcause.sh before any public announcement |
