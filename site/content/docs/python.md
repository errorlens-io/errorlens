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

Python 3.9 or newer is supported. The package has no dependencies.

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

Django, Flask, FastAPI, Starlette, Celery, SQLAlchemy, httpx, `urllib`/`http.client` and the standard `logging` module are detected automatically — there is nothing else to set up. Each web request and Celery task gets its own context, so a user or tag set while handling one request never appears on another's errors.

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

Requests in supported frameworks, plus their database queries and outgoing HTTP calls, then appear on the [Performance](/docs/performance/) page. Outgoing requests carry a W3C `traceparent` header, so a request that passes through several services — Python, Node.js or the browser — shows up as one trace.

To time work no integration covers, such as a script or a job:

```python
with errorlens.start_transaction("nightly-export", op="task"):
    with errorlens.start_span("db.query", "load rows"):
        rows = load_rows()
```

## Logs

Turn logs on with `enable_logs=True`. Then use the structured logger, whose `{placeholders}` become searchable attributes:

```python
errorlens.init(dsn="…", enable_logs=True)
errorlens.logger.info("Order {order_id} paid", order_id=42, amount=19.99)
```

Records from Python's standard `logging` module are sent too:

```python
import logging
logging.getLogger("checkout").warning("Inventory low for SKU %s", sku)
```

They appear on the [Logs](/docs/logs/) page, linked to the trace they belong to when tracing is on. Logs are batched and sent in the background.

## Cron monitors

Report each run of a scheduled job; you're alerted when a run fails, runs too long, or doesn't happen. The monitor is created from your code:

```python
@errorlens.monitor("nightly-backup", schedule="0 2 * * *", max_runtime=30)
def backup():
    ...
```

`monitor` also works as a context manager (`with errorlens.monitor("nightly-backup"):`) and on `async` functions. See [Cron monitors](/docs/cron-monitors/).

## Privacy

The SDK does **not** send personal data — IP addresses, cookies or request bodies — unless you opt in with `send_default_pii=True`. Passwords, tokens, API keys, cookies and similar values are removed in your app before anything is sent (add your own field names with `scrub_fields`), and the ErrorLens server [scrubs secrets again](/docs/data-scrubbing/) before storing anything.

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
| `init(dsn, **options)` | Start the SDK |
| `capture_exception(error=None)` | Report an exception (the current one if omitted) |
| `capture_message(message, level="info")` | Report a message |
| `capture_event(event)` | Send a raw event |
| `set_user`, `set_tag`, `set_tags`, `set_context`, `set_extra` | Attach context |
| `add_breadcrumb(category=, message=, level=, data=)` | Record a step that led up to an error |
| `start_transaction(name, op)`, `start_span(op, description)` | Time work |
| `get_traceparent()` | Header value for an outgoing request |
| `logger.info(...)` etc. | Structured logs |
| `monitor(slug, schedule=...)`, `capture_checkin(...)` | Cron monitors |
| `new_scope()`, `isolation_scope()` | Context managers for temporary context |
| `flush(timeout)` | Wait until queued events are sent |
| `last_event_id()` | The ID of the last event sent |

Useful `init` options: `sample_rate`, `traces_sample_rate` or `traces_sampler`, `enable_logs`, `send_default_pii`, `scrub_fields`, `ignore_errors`, `before_send`, `before_send_transaction`, `before_breadcrumb`, `before_send_log`, `include_local_variables` (default on), `max_breadcrumbs`, `trace_propagation_targets`, `disabled_integrations`, `http_proxy` and `debug`. Unknown options are reported with a warning.

Source: [github.com/errorlenshq/errorlens-python](https://github.com/errorlenshq/errorlens-python).
