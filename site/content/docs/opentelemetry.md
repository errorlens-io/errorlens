---
title: OpenTelemetry
description: Send traces and logs to ErrorLens from any OpenTelemetry SDK or Collector over OTLP/HTTP, with environment variables, Collector config and data mapping.
section: Sending data
order: 5
seo_title: OpenTelemetry (OTLP) Setup
---

ErrorLens accepts OpenTelemetry data over **OTLP/HTTP**, in both protobuf and JSON encoding, optionally gzip-compressed. Use it for languages without an ErrorLens SDK, or if your services are already instrumented with OpenTelemetry.

| Signal | Endpoint |
|---|---|
| Traces | `https://errorlens.io/v1/traces` |
| Logs | `https://errorlens.io/v1/logs` |

Metrics and OTLP over gRPC are not supported.

## Authentication

There is no project ID in the OTLP paths; ErrorLens finds the project from the DSN's public key. Send it in either header:

```text
Authorization: Bearer <dsn-public-key>
X-ErrorLens-Key: <dsn-public-key>
```

The public key is the part of the DSN before the `@`: in `https://4f1c…d9e2@errorlens.io/12` it is `4f1c…d9e2`.

## Using environment variables

Every OpenTelemetry SDK reads the standard variables, so you can usually configure it without code changes:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="https://errorlens.io"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer <dsn-public-key>"
export OTEL_SERVICE_NAME="checkout-api"
```

The SDK appends `/v1/traces` and `/v1/logs` to the endpoint itself.

## OpenTelemetry Collector

To send from a Collector, add an `otlphttp` exporter:

```yaml
exporters:
  otlphttp/errorlens:
    endpoint: https://errorlens.io
    headers:
      Authorization: "Bearer <dsn-public-key>"

service:
  pipelines:
    traces:
      exporters: [otlphttp/errorlens]
    logs:
      exporters: [otlphttp/errorlens]
```

## How data maps

- **Spans**: every server or consumer span, and every span whose parent isn't in the same batch, becomes a transaction on the [Performance](/docs/performance/) page, with its child spans in the waterfall. An error status on that span marks the transaction as failed.
- **Logs**: each log record appears on the [Logs](/docs/logs/) page with its severity, body and attributes. Records that carry a trace ID are linked to that trace.
- The `service.version` resource attribute becomes the release, and `deployment.environment.name` (or the older `deployment.environment`) becomes the environment.

Log bodies and attributes are [scrubbed](/docs/data-scrubbing/) like everything else before storage.
