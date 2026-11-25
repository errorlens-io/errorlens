# rootcausehq — Python SDK for RootCause

Report errors from Python apps to your own [RootCause](https://rootcause.sh) server.

```bash
pip install rootcausehq
```

```python
import rootcausehq

rootcausehq.init(
    dsn="https://<key>@errors.example.com/1",  # from your RootCause project page
    release="my-app@1.0.0",
    environment="production",
)
```

Unhandled exceptions are reported automatically. You can also report manually:

```python
try:
    risky()
except Exception:
    rootcausehq.capture_exception()

rootcausehq.capture_message("Payment provider slow", level="warning")
rootcausehq.set_user({"id": "42"})
rootcausehq.set_tag("region", "eu-west")
```

Configuration can also come from environment variables: `ROOTCAUSE_DSN`,
`ROOTCAUSE_RELEASE`, `ROOTCAUSE_ENVIRONMENT`.

Privacy defaults: personal data (IP addresses, cookies, request bodies) is **not** sent unless
you pass `send_default_pii=True`, and your RootCause server scrubs secrets again before storage.

Django, Flask, FastAPI, Celery and other frameworks are detected automatically.

License: MIT.
