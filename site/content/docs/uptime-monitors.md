---
title: Uptime monitors
description: Check websites and APIs every 30 seconds to every hour with ErrorLens uptime monitors, see response times and get alerted when they go down or recover.
section: Features
order: 5
seo_title: Uptime Monitors for URLs and APIs
---

An uptime monitor requests a URL on a schedule from the ErrorLens server. If the request fails, times out or returns an error status, the monitor turns red and your chosen alert channels are notified. When it answers again, you get a "recovered" notification.

![The monitors list](/screenshots/monitors.webp "Uptime checks and cron jobs together, with their status and last check")

## Create one

1. Open **Monitors** and click **New monitor**.
2. Choose **Uptime check**.
3. Enter a name, the project it belongs to and the URL — for example your health-check endpoint, `https://shop.example.com/healthz`.
4. Pick how often to check: every 30 seconds, 1, 5 or 15 minutes, or every hour.
5. Tick the [alert channels](/docs/alerts/) to notify, then click **Create monitor**.

Admins and owners can create and delete monitors.

## What counts as down

A check passes when the URL answers with a status below 400 — any 2xx or 3xx — within the timeout (10 seconds by default). Anything else is a failure:

- a 4xx or 5xx status
- no answer within the timeout
- a connection, DNS or TLS error

Each check is recorded with its result and how long the request took.

![An uptime monitor's history](/screenshots/monitor-uptime.webp "Recent checks with their result and response time")

## Alerts

Notifications are sent when the status *changes*: once when the monitor starts failing, and once when it recovers. You won't get a message every minute while a site stays down.

## Network safety

For your protection, uptime checks on the hosted service can't reach private or internal network addresses (such as `10.x.x.x`, `192.168.x.x` or `localhost`). On a self-hosted server you can allow them with `ERRORLENS_ALLOW_PRIVATE_WEBHOOKS=true` — see [Configuration](/docs/configuration/).
