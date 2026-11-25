# RootCause

**Open-source, self-hosted error and log tracking. Your errors, your servers, your eyes only.**

RootCause is a privacy-first alternative to hosted error trackers. It runs entirely on your
infrastructure, accepts events from the official Sentry SDKs (just change the DSN), and is
designed so sensitive data is scrubbed before it is ever written to disk.

> **Status: early development (Phase 0).** Accounts, organizations, projects and DSNs work.
> Event ingestion arrives in Phase 1. See [PLAN.md](PLAN.md) for the roadmap.

## Why

- **Nothing leaves your servers.** No telemetry, no phone-home, no third-party scripts in the UI.
- **Works with the SDKs you already use.** Python, JavaScript, Go, Java, PHP, Ruby, .NET and more.
- **Small footprint.** One container plus Postgres. No Kafka, Redis or ClickHouse required.
- **Tenant isolation in the database.** Postgres Row-Level Security keeps organizations apart even if application code has a bug.
- **Append-only audit log** of who did what.

## Quick start (Docker Compose)

```bash
git clone https://github.com/rootcausehq/rootcause.git
cd rootcause
cp deploy/.env.example deploy/.env    # set a strong POSTGRES_PASSWORD
docker compose -f deploy/docker-compose.yml up -d --build
```

Open <http://localhost:8080>, create the first admin account, then create a project to get its DSN.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `ROOTCAUSE_DATABASE_URL` | — (required) | Postgres connection URL |
| `ROOTCAUSE_PUBLIC_URL` | `http://localhost:8080` | External URL; used in DSNs. Use `https://` in production (enables Secure cookies) |
| `ROOTCAUSE_LISTEN_ADDR` | `:8080` | Address to listen on |
| `ROOTCAUSE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `ROOTCAUSE_SESSION_TTL` | `336h` | Login session lifetime |

Commands: `rootcause serve` (applies migrations on start), `rootcause migrate`, `rootcause version`.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
make dev-db   # Postgres on :5434
make test
make run
```

## Architecture

A single Go binary serves the API, the embedded React UI and a Postgres-backed job queue.
Requests touching tenant data run as a restricted database role with the organization set on
the transaction, and Row-Level Security policies filter every query. Details in [PLAN.md](PLAN.md).

## License

[AGPL-3.0](LICENSE). Not affiliated with Sentry; "compatible with Sentry SDKs" refers only to
the public event protocol.
