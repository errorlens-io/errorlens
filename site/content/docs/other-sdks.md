---
title: Other languages
description: Use ErrorLens from Go, Java, PHP, Ruby, .NET, Rust, mobile apps and more.
section: Sending data
order: 4
---

ErrorLens accepts the standard **error-envelope protocol** used by the most widely deployed open-source error-reporting SDKs. If your language has an SDK that is configured with a DSN like `https://<key>@<host>/<project-id>`, it almost certainly works with ErrorLens: install it as its own documentation describes and give it your ErrorLens DSN.

```text
https://<key>@errorlens.io/<project-id>
```

That covers Go, Java and Kotlin, PHP and Laravel, Ruby and Rails, .NET, Rust, Elixir, Dart and Flutter, React Native, Android and iOS, among others. Use the DSN shown on your project's page in ErrorLens.

> ErrorLens' own SDKs are [Python](/docs/python/), [Node.js](/docs/javascript-node/) and [browser JavaScript](/docs/javascript-browser/). They add privacy-first defaults; for other languages, set the equivalent options yourself — most SDKs have a "send default PII" option you should leave off.

## What is stored

| Data the SDK sends | What ErrorLens does |
|---|---|
| Errors and messages | Grouped into [issues](/docs/issues/) |
| Transactions and spans | Shown on [Performance](/docs/performance/) |
| Log records | Shown on [Logs](/docs/logs/) |
| Cron check-ins | Update [cron monitors](/docs/cron-monitors/) |
| Sessions, attachments, replays, profiles, client reports | Accepted and discarded, so SDKs don't retry them |

Everything is [scrubbed](/docs/data-scrubbing/) before it is stored.

## Sending data without an SDK

The ingestion endpoints are plain HTTP:

| Endpoint | Body |
|---|---|
| `POST /api/<project-id>/envelope/` | An envelope: a JSON header line followed by item headers and payloads |
| `POST /api/<project-id>/store/` | A single JSON event |

Authenticate with the DSN key in an `X-ErrorLens-Key` header or a `?key=` query parameter (SDKs use their own authentication header, which also works). Bodies may be gzip-compressed (`Content-Encoding: gzip`). A minimal error with `curl`:

```bash
curl -X POST "https://errorlens.io/api/<project-id>/store/" \
  -H "X-ErrorLens-Key: <key>" \
  -H "Content-Type: application/json" \
  -d '{"message": "Disk almost full", "level": "warning", "platform": "other"}'
```

## Rate limits

Each key accepts bursts of up to 1,000 events, refilling at 100 per second. Above that the server answers `429 Too Many Requests` with a `Retry-After` header, and SDKs pause automatically. When an organization's monthly quota is used up, the response is also `429`, with a one-hour `Retry-After`.
