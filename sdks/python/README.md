# errorlens — Python SDK for ErrorLens

Report errors from Python apps to your own [ErrorLens](https://errorlens.io) server.

```bash
pip install errorlens
```

```python
import errorlens

errorlens.init(
    dsn="https://<key>@errors.example.com/1",  # from your ErrorLens project page
    release="my-app@1.0.0",
    environment="production",
)
```

Unhandled exceptions are reported automatically. You can also report manually:

```python
try:
    risky()
except Exception:
    errorlens.capture_exception()

errorlens.capture_message("Payment provider slow", level="warning")
errorlens.set_user({"id": "42"})
errorlens.set_tag("region", "eu-west")
```

Configuration can also come from environment variables: `ERRORLENS_DSN`,
`ERRORLENS_RELEASE`, `ERRORLENS_ENVIRONMENT`.

Privacy defaults: personal data (IP addresses, cookies, request bodies) is **not** sent unless
you pass `send_default_pii=True`, and your ErrorLens server scrubs secrets again before storage.

Django, Flask, FastAPI, Celery and other frameworks are detected automatically.

License: MIT.
