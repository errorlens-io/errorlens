---
title: Configuration reference
description: Every environment variable a self-hosted ErrorLens server reads.
section: Self-hosting
order: 2
---

ErrorLens is configured with environment variables. With Docker Compose, put them in `deploy/.env`.

## Core

| Variable | Default | Description |
|---|---|---|
| `ERRORLENS_DATABASE_URL` | *(required)* | PostgreSQL connection URL, e.g. `postgres://user:pass@db:5432/errorlens?sslmode=require` |
| `ERRORLENS_PUBLIC_URL` | `http://localhost:8080` | The address people and SDKs use. Appears in DSNs and email links. Use `https://` in production — it also turns on secure cookies |
| `ERRORLENS_LISTEN_ADDR` | `:8080` | Address and port to listen on |
| `ERRORLENS_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `ERRORLENS_SESSION_TTL` | `336h` | How long a sign-in lasts (14 days) |
| `ERRORLENS_TRUSTED_PROXIES` | | Comma-separated IPs or CIDR ranges of reverse proxies whose `X-Forwarded-For` header is believed |

## Privacy and security

| Variable | Default | Description |
|---|---|---|
| `ERRORLENS_ENCRYPTION_KEY` | | Base64-encoded 32-byte key that turns on [encryption of stored events](/docs/encryption/). Generate with `openssl rand -base64 32` and back it up |
| `ERRORLENS_ALLOW_PRIVATE_WEBHOOKS` | `false` | Let alert webhooks and uptime checks reach private and loopback addresses. Keep it off on servers shared by several teams |

## Email

| Variable | Default | Description |
|---|---|---|
| `ERRORLENS_SMTP_HOST` | | SMTP server. Without it, email alerts, invitation emails and password resets are disabled (invitation links are still shown in the dashboard) |
| `ERRORLENS_SMTP_PORT` | `587` | SMTP port; STARTTLS is used when the server offers it |
| `ERRORLENS_SMTP_USERNAME` | | SMTP user |
| `ERRORLENS_SMTP_PASSWORD` | | SMTP password |
| `ERRORLENS_SMTP_FROM` | | Sender, e.g. `ErrorLens <alerts@example.com>` |

## Single sign-on (OpenID Connect)

| Variable | Description |
|---|---|
| `ERRORLENS_OIDC_ISSUER` | Your provider's issuer URL, e.g. `https://accounts.google.com` |
| `ERRORLENS_OIDC_CLIENT_ID` | Client ID of the app you registered with the provider |
| `ERRORLENS_OIDC_CLIENT_SECRET` | Its client secret |
| `ERRORLENS_OIDC_NAME` | Button label on the sign-in page, e.g. `Google` |
| `ERRORLENS_OIDC_AUTO_JOIN_ORG` | Organization slug that new SSO users join automatically |
| `ERRORLENS_OIDC_DEFAULT_ROLE` | Role they get there, e.g. `member` |
| `ERRORLENS_OIDC_ALLOWED_DOMAINS` | Comma-separated email domains allowed to auto-join, e.g. `example.com` |

Register `<ERRORLENS_PUBLIC_URL>/api/0/auth/oidc/callback` as the redirect URL with your provider.

## Hosted mode

For running ErrorLens as a service for other people. See [Hosted mode](/docs/hosted-mode/).

| Variable | Default | Description |
|---|---|---|
| `ERRORLENS_SIGNUP_ENABLED` | `false` | Let anyone create an account and organization |
| `ERRORLENS_FREE_EVENT_QUOTA` | `5000` | Monthly error events for organizations created by sign-up |
| `ERRORLENS_TENANT_DOMAIN` | | Serve each organization at `<org>.<domain>` |
| `ERRORLENS_STRIPE_SECRET_KEY` | | Stripe API key for paid plans |
| `ERRORLENS_STRIPE_WEBHOOK_SECRET` | | Signing secret of the Stripe webhook pointed at `/webhooks/stripe` |
| `ERRORLENS_STRIPE_PLANS` | | Plans as `id:price_id:monthly_events`, comma-separated |

## Commands

| Command | Purpose |
|---|---|
| `errorlens serve [--role=all\|web\|worker] [--no-migrate]` | Run the server |
| `errorlens migrate` | Apply database migrations and exit |
| `errorlens sourcemaps upload …` | Upload source maps — see [Releases and source maps](/docs/releases-source-maps/) |
| `errorlens demo --dsn … [--platform python\|javascript]` | Send sample data |
| `errorlens version` | Print the version |
