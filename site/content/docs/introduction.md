---
title: What is ErrorLens?
description: Open-source error tracking, performance monitoring, logs and uptime checks that keep sensitive data out of your reports.
section: Getting started
order: 1
---

ErrorLens collects the errors, slow requests, logs and failed jobs from your applications and turns them into a short list of problems you can act on. It is open source, and you can use it two ways:

- **Hosted at errorlens.io** — sign up, create a project and start sending data in minutes. Each organization gets its own address, like `acme.errorlens.io`.
- **Self-hosted** — run one container and a Postgres database on your own servers. Your data never leaves your network. See [Self-hosting](/docs/self-hosting/).

![The issues list: errors grouped into issues with counts, first and last seen, and a 24-hour graph](/screenshots/issues.png "Errors from every project, grouped into issues")

## What it does

| Feature | What you get |
|---|---|
| [Issues](/docs/issues/) | Errors grouped by cause, with stack traces, breadcrumbs, tags, releases and regressions |
| [Performance](/docs/performance/) | Latency percentiles, throughput and failure rate per endpoint, and full traces |
| [Logs](/docs/logs/) | Structured logs with search, severity filters, live tail and links to traces |
| [Uptime monitors](/docs/uptime-monitors/) | Your URLs checked on a schedule, with alerts when they fail |
| [Cron monitors](/docs/cron-monitors/) | Alerts when scheduled jobs fail, run too long, or don't run at all |
| [Alerts](/docs/alerts/) | Email, Slack, Discord and signed webhooks for new issues, regressions and spikes |
| [Releases & source maps](/docs/releases-source-maps/) | Which version introduced an error, and readable JavaScript stack traces |

## Why ErrorLens

Error reports are full of things that should never be stored: passwords in form posts, tokens in headers, card numbers in request bodies, database URLs in exception messages. ErrorLens [scrubs them on the server before anything is written to disk](/docs/data-scrubbing/), even if your SDK sends them by mistake. Stored events can also be [encrypted](/docs/encryption/), and every time someone opens an event it is recorded in the [audit log](/docs/security/).

ErrorLens works with the error-reporting SDKs you may already use — it speaks the same envelope protocol — and with OpenTelemetry. Switching usually means changing one line: the DSN.

## Next steps

1. Follow the [quick start](/docs/quickstart/) to send your first error.
2. Learn how [organizations, projects and DSNs](/docs/concepts/) fit together.
3. Set up [alerts](/docs/alerts/) so problems find you.
