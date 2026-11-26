---
title: Performance and traces
description: Monitor latency, throughput and failure rate per endpoint with ErrorLens, and follow distributed traces across services with spans, errors and logs.
section: Features
order: 3
seo_title: Performance Monitoring and Tracing
---

The **Performance** page shows how fast your endpoints and background jobs are, how much traffic they get and how often they fail. Every number comes from *transactions* — recordings of one request or job — sent by an SDK or OpenTelemetry.

![The performance overview](/screenshots/performance.webp "Every endpoint with its throughput, median (p50) and 95th percentile (p95) duration, and failure rate")

## Turning it on

Tracing is off until you choose a sample rate — the share of requests to record:

```python
errorlens.init(dsn="…", traces_sample_rate=0.2)   # Python: 20% of requests
```

```js
ErrorLens.init({ dsn: "…", tracesSampleRate: 0.2 }); // Node.js
```

Or send spans from any [OpenTelemetry](/docs/opentelemetry/) SDK. Start with a low rate on busy services and raise it if you need more detail.

## The overview

Each row is one transaction name — usually an endpoint such as `POST /api/checkout` or a job name — with:

| Column | Meaning |
|---|---|
| Throughput | Transactions per minute over the selected period |
| p50 | Median duration: half the requests were faster |
| p95 | 95th percentile: only 1 in 20 requests was slower — what your slowest users feel |
| Failures | Share of transactions that ended with an error status |

Choose the period — **1 hour**, **24 hours**, **7 days** or **30 days** — and filter by project, or open a project's path such as `/checkout-api/performance`.

The 1-hour and 24-hour views are computed from every individual request. The 7-day and 30-day views read hourly and daily summaries instead, so they load instantly however much traffic you have: counts, throughput and failure rates are exact, and p50 and p95 are within 4% of the exact value.

## One endpoint

Click a transaction name to list its individual requests, slowest first, with their duration, status, release and environment. Open one to see where the time went.

![The transactions of one endpoint](/screenshots/performance-endpoint.webp "Individual POST /api/checkout requests, slowest first")

## Traces

A trace follows one request through your system. The trace view joins everything that shares its trace ID:

- **Errors in this trace** — issues raised during the request, with how far into the request they happened
- **Waterfall** — the transaction and its spans (database queries, outgoing HTTP calls, cache lookups…) on a timeline, so the slow step stands out
- **Logs** — log lines written during the request

![A trace with its error, spans and logs](/screenshots/trace.webp "One checkout request: the error it raised, its database query and payment call, and its logs")

You can reach a trace from an endpoint's request list, from **View trace** on an issue's event, or from a log line's trace link.
