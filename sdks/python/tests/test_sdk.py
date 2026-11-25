"""Tests run the SDK against a tiny in-process HTTP server that records envelopes."""

import gzip
import http.server
import json
import threading
import unittest

import errorlens


class _Recorder(http.server.BaseHTTPRequestHandler):
    bodies: list = []

    def do_POST(self):  # noqa: N802
        raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.headers.get("Content-Encoding") == "gzip":
            raw = gzip.decompress(raw)
        _Recorder.bodies.append(raw)
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b"{}")

    def log_message(self, *args):
        pass


def _events():
    out = []
    for body in _Recorder.bodies:
        lines = body.split(b"\n")
        for i, line in enumerate(lines[1:], start=1):
            try:
                hdr = json.loads(line)
            except ValueError:
                continue
            if isinstance(hdr, dict) and hdr.get("type") == "event" and i + 1 < len(lines):
                out.append(json.loads(lines[i + 1]))
    return out


class SDKTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = http.server.HTTPServer(("127.0.0.1", 0), _Recorder)
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()
        port = cls.server.server_address[1]
        cls.dsn = f"http://{'a' * 32}@127.0.0.1:{port}/7"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def setUp(self):
        _Recorder.bodies.clear()

    def test_no_dsn_is_noop(self):
        errorlens.init(dsn=None)
        self.assertFalse(errorlens.is_enabled())
        errorlens.capture_message("ignored")

    def test_reports_branded_event_with_privacy_defaults(self):
        seen = []

        def my_hook(event, hint):
            seen.append(event["sdk"]["name"])
            return event

        errorlens.init(self.dsn, release="app@1.2.3", environment="test", before_send=my_hook, unknown_option=1)
        self.assertTrue(errorlens.is_enabled())
        errorlens.set_user({"id": "7"})
        errorlens.set_tag("region", "eu")
        try:
            1 / 0
        except ZeroDivisionError:
            errorlens.capture_exception()
        errorlens.flush(5)

        events = _events()
        self.assertEqual(len(events), 1)
        ev = events[0]
        self.assertEqual(ev["sdk"]["name"], "errorlens.python")
        self.assertEqual(ev["sdk"]["version"], errorlens.__version__)
        self.assertEqual(seen, ["errorlens.python"])  # user hook still runs
        self.assertEqual(ev["release"], "app@1.2.3")
        self.assertEqual(ev["environment"], "test")
        self.assertEqual(ev["tags"]["region"], "eu")
        self.assertEqual(ev["exception"]["values"][-1]["type"], "ZeroDivisionError")
        self.assertNotIn("ip_address", ev.get("user", {}))
        self.assertNotIn("sentry", json.dumps(ev["sdk"]).lower())

    def test_env_vars(self):
        import os

        os.environ.update(ERRORLENS_DSN=self.dsn, ERRORLENS_RELEASE="env@9", ERRORLENS_ENVIRONMENT="ci")
        try:
            errorlens.init()
            errorlens.capture_message("hello from env")
            errorlens.flush(5)
        finally:
            for k in ("ERRORLENS_DSN", "ERRORLENS_RELEASE", "ERRORLENS_ENVIRONMENT"):
                os.environ.pop(k, None)
        ev = _events()[-1]
        self.assertEqual((ev["release"], ev["environment"]), ("env@9", "ci"))


if __name__ == "__main__":
    unittest.main()
