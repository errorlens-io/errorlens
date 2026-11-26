---
title: Hosted mode
description: Run ErrorLens as a multi-tenant hosted service: organization subdomains, public sign-up, quotas, usage metering and billing hooks on Postgres RLS.
section: Self-hosting
order: 4
seo_title: Hosted Mode: Multi-Tenant Setup
---

The same ErrorLens server that runs on one team's machine can also run a service for many organizations — this is how errorlens.io works. These settings are for people who want to offer ErrorLens to others; a single team doesn't need them.

## Tenant isolation

Every organization's data lives in the same database, separated by **PostgreSQL row-level security**. Requests that touch organization data run as a restricted database role with the organization set on the transaction, so even a bug in a query can't return another organization's rows.

## Sign-up and quotas

```bash
ERRORLENS_SIGNUP_ENABLED=true
ERRORLENS_FREE_EVENT_QUOTA=5000
```

Anyone can then create an account and an organization. New organizations get the free monthly quota; the instance admin can change an organization's plan and quota in the **Admin** page. Configure [email](/docs/configuration/#email) too, so sign-ups can confirm their address and reset passwords.

## Organization subdomains

```bash
ERRORLENS_PUBLIC_URL=https://errors.example.com
ERRORLENS_TENANT_DOMAIN=errors.example.com
```

With a tenant domain set:

- Sign-in, sign-up, invitations, email links, the API and SDK ingestion stay on `ERRORLENS_PUBLIC_URL`.
- Each organization's dashboard is served at `https://<org-slug>.errors.example.com/`, with projects at `/<project>/`.
- The session cookie is shared across the domain, so one sign-in works for every organization a person belongs to.
- Links in alerts and emails point at the organization's subdomain.
- Organization slugs that would clash with service names (`www`, `api`, `docs`, `admin`…) get `-org` added.

You need:

1. **Wildcard DNS**: an `A` or `CNAME` record for `*.errors.example.com` pointing at your proxy.
2. **A wildcard certificate** for `*.errors.example.com` and `errors.example.com`. Wildcards need a DNS challenge with Let's Encrypt, or use a certificate from your CDN — for example a Cloudflare origin certificate behind Cloudflare's proxy.
3. **A proxy** that sends both the main domain and every subdomain to ErrorLens.

## Billing

ErrorLens can sell plans with Stripe. Create products and monthly prices in Stripe, then:

```bash
ERRORLENS_STRIPE_SECRET_KEY=sk_live_…
ERRORLENS_STRIPE_WEBHOOK_SECRET=whsec_…
ERRORLENS_STRIPE_PLANS=team:price_123:100000,business:price_456:1000000
```

Point a Stripe webhook at `https://errors.example.com/webhooks/stripe`. Organization owners can then subscribe and manage billing under **Settings → Usage & billing**, and quotas follow the plan.
