"""ErrorLens Python SDK.

Reports errors to a self-hosted ErrorLens server. Event capture and transport
are provided by the MIT-licensed open-source error SDK this package builds on;
this module adds ErrorLens defaults (privacy-first, quiet console) and a
ErrorLens-branded API.
"""

from __future__ import annotations

import os
from typing import Any, Callable, Optional

# The underlying library prints setup hints mentioning its own product to the
# console. ErrorLens users should never see those.
os.environ.setdefault("SENTRY_AGENT_HINTS", "0")

import sentry_sdk as _impl  # noqa: E402
from sentry_sdk import consts as _consts  # noqa: E402

from ._version import __version__  # noqa: E402

SDK_NAME = "errorlens.python"

__all__ = [
    "__version__",
    "init",
    "is_enabled",
    "capture_exception",
    "capture_message",
    "capture_event",
    "add_breadcrumb",
    "set_user",
    "set_tag",
    "set_tags",
    "set_context",
    "set_extra",
    "flush",
    "last_event_id",
    "new_scope",
    "isolation_scope",
]

Event = dict
Hint = dict
BeforeSend = Callable[[Event, Hint], Optional[Event]]


def _supported(option: str) -> bool:
    defaults = getattr(_consts, "DEFAULT_OPTIONS", None)
    return defaults is None or option in defaults


def _brand(user_hook: Optional[BeforeSend]) -> BeforeSend:
    """Wraps before_send so stored events identify this SDK as ErrorLens."""

    def hook(event: Event, hint: Hint) -> Optional[Event]:
        sdk = event.get("sdk") or {}
        sdk["name"] = SDK_NAME
        sdk["version"] = __version__
        # Report our package plus the core library version under a neutral name.
        sdk["packages"] = [
            {"name": "pypi:errorlens", "version": __version__},
            {"name": "pypi:core", "version": getattr(_impl, "VERSION", "")},
        ]
        event["sdk"] = sdk
        return user_hook(event, hint) if user_hook else event

    return hook


def init(
    dsn: Optional[str] = None,
    *,
    release: Optional[str] = None,
    environment: Optional[str] = None,
    before_send: Optional[BeforeSend] = None,
    **options: Any,
) -> None:
    """Start reporting errors to ErrorLens.

    dsn, release and environment default to the ERRORLENS_DSN,
    ERRORLENS_RELEASE and ERRORLENS_ENVIRONMENT environment variables. With no
    DSN the SDK is a no-op, so it is safe to call in tests and local dev.

    Any other keyword argument is passed to the underlying client; options it
    does not support are ignored rather than raising.
    """
    opts: dict = {
        "dsn": dsn or os.environ.get("ERRORLENS_DSN") or None,
        "release": release or os.environ.get("ERRORLENS_RELEASE") or None,
        "environment": environment or os.environ.get("ERRORLENS_ENVIRONMENT") or None,
        "before_send": _brand(before_send),
        # Privacy-first defaults (callers may override).
        "send_default_pii": False,
        # ErrorLens does not ingest performance data yet.
        "traces_sample_rate": None,
    }
    opts.update(options)
    opts = {k: v for k, v in opts.items() if _supported(k)}
    _impl.init(**opts)


def is_enabled() -> bool:
    """True if init() was called with a DSN."""
    client = _impl.get_client()
    return bool(client and client.is_active() and client.dsn)


def _export(name: str):
    fn = getattr(_impl, name, None)
    if fn is None:

        def missing(*_a: Any, **_k: Any) -> None:
            raise NotImplementedError(f"errorlens.{name} is not available with the installed core library")

        return missing
    return fn


capture_exception = _export("capture_exception")
capture_message = _export("capture_message")
capture_event = _export("capture_event")
add_breadcrumb = _export("add_breadcrumb")
set_user = _export("set_user")
set_tag = _export("set_tag")
set_tags = _export("set_tags")
set_context = _export("set_context")
set_extra = _export("set_extra")
flush = _export("flush")
last_event_id = _export("last_event_id")
new_scope = _export("new_scope")
isolation_scope = _export("isolation_scope")
