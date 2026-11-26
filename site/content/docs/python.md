---
title: Python
description: Report errors, performance data and logs from Python apps with the errorlens package.
section: Sending data
order: 1
---

## Install

```bash
pip install errorlens
```

Python 3.9 or newer is supported.

## Configure

Call `init` as early as possible — before your web framework or task queue starts:

```python
import errorlens

errorlens.init(
    dsn="https://<key>@errorlens.io/<project-id>",
    release="my-app@1.4.2",       # the version you deployed
    environment="production",     # production, staging, ...
)
```

`dsn`, `release` and `environment` can also come from environment variables, which keeps them out of your code:

```bash
export ERRORLENS_DSN="https://<key>@errorlens.io/<project-id>"
export ERRORLENS_RELEASE="my-app@1.4.2"
export ERRORLENS_ENVIRONMENT="production"
```

```python
errorlens.init()  # reads the variables above
```

Without a DSN the SDK does nothing, so it is safe to call `init()` in tests and local development.

Django, Flask, FastAPI, Starlette, Celery, RQ, SQLAlchemy, Redis and the standard `logging` module are detected automatically — there is nothing else to set up.

## Report errors

Unhandled exceptions are reported automatically. To report one you caught:

```python
try:
    charge(order)
except PaymentError:
    errorlens.capture_exception()
```

To report something that isn't an exception:

```python
errorlens.capture_message("Payment provider is slow", level="warning")
```

## Add context

Context makes issues easier to search and understand. It is attached to every event sent afterwards:

```python
errorlens.set_user({"id": "42"})                 # who was affected
errorlens.set_tag("region", "eu-west")            # searchable, shown as a tag breakdown
errorlens.set_context("order", {"id": 9817, "items": 3})
errorlens.add_breadcrumb(category="payment", message="Charging card")
```

Tags appear in the tag breakdown on each issue — for example, which regions or releases an error affects.

## Performance

Tracing is off by default. Turn it on with a sample rate between `0` and `1` (the share of requests to record):

```python
errorlens.init(dsn="…", traces_sample_rate=0.2)
```

Requests in supported frameworks, plus their database queries and outgoing HTTP calls, then appear on the [Performance](/docs/performance/) page.

## Logs

Send records from Python's standard `logging` module as ErrorLens logs:

```python
import logging

errorlens.init(dsn="…", enable_logs=True)
logging.getLogger("checkout").warning("Inventory low for SKU %s", sku)
```

They appear on the [Logs](/docs/logs/) page, linked to the trace they belong to when tracing is on.

Logs need version 3 or newer of the SDK's core dependency, which a fresh `pip install errorlens` installs. If you pinned an older one, upgrade it or send logs with [OpenTelemetry](/docs/opentelemetry/).

## Privacy

The SDK does **not** send personal data — IP addresses, cookies or request bodies — unless you opt in with `send_default_pii=True`. Whatever it does send, the ErrorLens server [scrubs secrets again](/docs/data-scrubbing/) before storing anything.

To change or drop an event before it leaves your app, use `before_send`:

```python
def before_send(event, hint):
    if "healthcheck" in event.get("transaction", ""):
        return None          # drop it
    return event

errorlens.init(dsn="…", before_send=before_send)
```

## Make sure events are sent before exit

Short scripts and serverless functions can exit before events are delivered. Call `flush` at the end:

```python
errorlens.flush(timeout=5)
```

## All functions

| Function | Purpose |
|---|---|
| `init(dsn, release, environment, before_send, **options)` | Start the SDK |
| `capture_exception(error=None)` | Report an exception (the current one if omitted) |
| `capture_message(message, level="info")` | Report a message |
| `capture_event(event)` | Send a raw event |
| `set_user`, `set_tag`, `set_tags`, `set_context`, `set_extra` | Attach context |
| `add_breadcrumb(...)` | Record a step that led up to an error |
| `new_scope()`, `isolation_scope()` | Context managers for temporary context |
| `flush(timeout)` | Wait until queued events are sent |
| `last_event_id()` | The ID of the last event sent |

Other keyword arguments to `init` are passed to the underlying client; options it doesn't support are ignored.
