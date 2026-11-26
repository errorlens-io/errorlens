# ErrorLens

**Open-source error tracking, performance monitoring, logs, cron and uptime monitoring.** One small container plus Postgres — no Kafka, Redis or ClickHouse.

- Website and docs: [errorlens.io](https://errorlens.io) · [Self-hosting guide](https://errorlens.io/docs/self-hosting/)
- Source: [github.com/errorlenshq/errorlens](https://github.com/errorlenshq/errorlens) (AGPL-3.0)
- SDKs: `pip install errorlens` · `npm install @errorlenshq/node` · `npm install @errorlenshq/browser`, or OpenTelemetry

## Tags

| Tag | |
|---|---|
| `latest` | Newest release |
| `1`, `1.0`, `1.0.0` | Releases (semver) |
| `edge` | Latest commit on `main` |

Images are built for **linux/amd64** and **linux/arm64**, on a distroless base with no shell, running as a non-root user, with SBOM and provenance attestations.

## Quick start

```bash
docker network create errorlens

docker run -d --name errorlens-db --network errorlens \
  -e POSTGRES_USER=errorlens -e POSTGRES_PASSWORD=change-me -e POSTGRES_DB=errorlens \
  -v errorlens-db:/var/lib/postgresql/data postgres:17-alpine

docker run -d --name errorlens --network errorlens -p 8080:8080 \
  -e ERRORLENS_DATABASE_URL="postgres://errorlens:change-me@errorlens-db:5432/errorlens?sslmode=disable" \
  -e ERRORLENS_PUBLIC_URL="http://localhost:8080" \
  errorlens/errorlens
```

Open <http://localhost:8080>, create the first account and a project, and point an SDK at its DSN. Migrations run automatically on start.

For production, use the Docker Compose file or Helm chart in the repository, put HTTPS in front, and set `ERRORLENS_ENCRYPTION_KEY` to encrypt stored events.

## Configuration

| Variable | Default | |
|---|---|---|
| `ERRORLENS_DATABASE_URL` | (required) | Postgres connection URL |
| `ERRORLENS_PUBLIC_URL` | `http://localhost:8080` | External URL, used in DSNs and links |
| `ERRORLENS_LISTEN_ADDR` | `:8080` | Listen address |
| `ERRORLENS_ENCRYPTION_KEY` | | Base64 32-byte key to encrypt stored events (`openssl rand -base64 32`) |
| `ERRORLENS_SMTP_HOST` / `_PORT` / `_USERNAME` / `_PASSWORD` / `_FROM` | | Email for alerts and invitations |
| `ERRORLENS_TRUSTED_PROXIES` | | Proxy IPs/CIDRs whose `X-Forwarded-For` is trusted |
| `ERRORLENS_OIDC_ISSUER` / `_CLIENT_ID` / `_CLIENT_SECRET` | | Single sign-on |

All options: [configuration reference](https://errorlens.io/docs/configuration/).

## Commands

The image runs `serve` by default. Others: `migrate`, `version`, `healthcheck` (used by the built-in `HEALTHCHECK`), `demo --dsn DSN` (sample data), `sourcemaps upload` (JavaScript source maps from CI). Run `serve --role web` and `serve --role worker` to scale the HTTP and background parts separately (disable the health check on worker containers).
