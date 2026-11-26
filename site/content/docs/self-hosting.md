---
title: Self-hosting with Docker
description: Run ErrorLens on your own server with Docker Compose — one container and Postgres.
section: Self-hosting
order: 1
---

ErrorLens is a single Go program with the dashboard built in, plus a PostgreSQL database. There is no Kafka, Redis or ClickHouse to run. A small server — 1 CPU and 1 GB of memory — handles a typical team; give Postgres more memory and disk as your volume grows.

## Requirements

- Docker with the Compose plugin
- A domain name pointing at the server, if you want HTTPS (recommended)

## Install

```bash
git clone https://github.com/errorlens-io/errorlens.git
cd errorlens
cp deploy/.env.example deploy/.env
```

Edit `deploy/.env`. At minimum, set a long random database password and the address people will use:

```bash
POSTGRES_PASSWORD=<openssl rand -hex 24>
ERRORLENS_PUBLIC_URL=https://errors.example.com
```

Start it:

```bash
docker compose -f deploy/docker-compose.yml up -d
```

ErrorLens creates its database tables on first start, then listens on port 8080. Open the address in a browser: the first visit shows a one-time form to create the **first admin account** and organization. Do this straight away — until it is done, anyone who can reach the server could claim it.

## HTTPS

Put ErrorLens behind a reverse proxy that terminates TLS. With [Caddy](https://caddyserver.com), which gets certificates automatically, the whole configuration is:

```text
errors.example.com {
	reverse_proxy localhost:8080
}
```

Then tell ErrorLens to trust the proxy's forwarded client addresses, so rate limits and the audit log see real IPs:

```bash
ERRORLENS_TRUSTED_PROXIES=127.0.0.1
```

## Try it with sample data

To see ErrorLens with realistic errors, traces, logs and cron check-ins before connecting your apps, create a project and run:

```bash
docker compose -f deploy/docker-compose.yml exec errorlens errorlens demo --dsn "<the project's DSN>"
```

## Email

Alerts, invitations and password resets need an SMTP server. Any provider works:

```bash
ERRORLENS_SMTP_HOST=smtp.example.com
ERRORLENS_SMTP_PORT=587
ERRORLENS_SMTP_USERNAME=…
ERRORLENS_SMTP_PASSWORD=…
ERRORLENS_SMTP_FROM=ErrorLens <alerts@example.com>
```

## Updating

```bash
git pull
docker compose -f deploy/docker-compose.yml up -d --build
```

Database migrations run automatically when the new version starts. Back up the database before upgrading across major versions.

## Backups

All state is in Postgres. Back it up with `pg_dump`:

```bash
docker compose -f deploy/docker-compose.yml exec postgres pg_dump -U errorlens errorlens | gzip > errorlens-$(date +%F).sql.gz
```

If you set `ERRORLENS_ENCRYPTION_KEY`, back up the key separately — encrypted events can't be read without it.

## Scaling

The same image can run in three roles:

```bash
errorlens serve --role=web      # dashboard, API and ingestion
errorlens serve --role=worker   # processing, alerts, monitors, retention
errorlens serve --role=all      # both (the default)
```

Run several web replicas behind a load balancer and one or more workers; they coordinate through Postgres. Events, transactions and logs are stored in daily partitions, so deleting old data is cheap. For Kubernetes, see [Kubernetes](/docs/kubernetes/).

## Telemetry

ErrorLens sends nothing to us or anyone else. The only outgoing connections are the alerts, uptime checks and emails you configure.
