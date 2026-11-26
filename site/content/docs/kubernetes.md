---
title: Kubernetes
description: Deploy ErrorLens on Kubernetes with the Helm chart: values, external Postgres, ingress and TLS, scaling web and worker roles, and upgrades.
section: Self-hosting
order: 3
seo_title: Self-Hosting on Kubernetes (Helm)
---

The repository includes a Helm chart in `deploy/helm/errorlens`. It runs database migrations as a job before each install or upgrade, then separate **web** and **worker** deployments.

## Install

You need a PostgreSQL database — a managed service or an operator such as CloudNativePG.

```bash
git clone https://github.com/errorlenshq/errorlens.git
helm install errorlens errorlens/deploy/helm/errorlens \
  --set publicURL=https://errors.example.com \
  --set database.url='postgres://user:pass@db:5432/errorlens?sslmode=require'
```

Better, keep secrets in Kubernetes Secrets and reference them from a values file:

```yaml
# values.prod.yaml
publicURL: https://errors.example.com
database:
  existingSecret: errorlens-db        # Secret with key DATABASE_URL
encryption:
  existingSecret: errorlens-crypto    # Secret with key ENCRYPTION_KEY
env:
  ERRORLENS_SMTP_HOST: smtp.example.com
  ERRORLENS_TRUSTED_PROXIES: 10.0.0.0/8
ingress:
  enabled: true
  host: errors.example.com
```

```bash
helm upgrade --install errorlens errorlens/deploy/helm/errorlens -f values.prod.yaml
```

`deploy/helm/errorlens/values.yaml` lists every option: image, replica counts and resources for web and worker pods, the service, ingress, and `env` for any setting from the [configuration reference](/docs/configuration/).

## Scaling

- **web** pods serve the dashboard, the API and ingestion. Scale them with traffic.
- **worker** pods process events, send alerts, run monitors and delete expired data. One or two are enough for most installations.

Both are stateless; everything lives in Postgres.

## Upgrading

```bash
cd errorlens && git pull
helm upgrade errorlens deploy/helm/errorlens -f values.prod.yaml
```

The migration job runs first; the new pods start once it succeeds.
