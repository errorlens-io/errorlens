# Contributing to RootCause

Thanks for helping! A few ground rules keep the project healthy.

## Clean-room rule

RootCause is compatible with Sentry SDKs, but **must not contain code copied or translated
from Sentry's server** (which is under the Functional Source License). Implement behavior from
public protocol documentation and SDK behavior only. If you have worked on Sentry's server
code, say so in your PR so reviewers can take extra care.

## Developer Certificate of Origin

We use the [DCO](https://developercertificate.org/) instead of a CLA. Sign off every commit:

```bash
git commit -s -m "your message"
```

## Development setup

Requirements: Go (see `go.mod`), Node 24+, Docker.

```bash
make dev-db      # Postgres on localhost:5434
make test        # all tests, including database tests
make run         # build UI + binary, serve on http://localhost:8080
```

For UI work, run `make run` in one terminal and `cd web && npm run dev` in another
(Vite on :5173 proxies `/api` to :8080).

## Pull requests

- Keep changes focused; include tests for new behavior.
- Every tenant-owned table needs `org_id` and a Row-Level Security policy, plus a test that
  another organization cannot read it.
- Never log or persist raw event payloads before scrubbing.
