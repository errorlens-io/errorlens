---
title: Logs
description: Search, filter and live-tail structured logs, linked to the traces and errors they belong to.
section: Features
order: 4
---

The **Logs** page collects log lines from your services in one place, newest first. Logs that belong to a traced request link to that trace, so you can jump from a suspicious line to the request, its spans and its errors.

![The logs page](/screenshots/logs.png "Log lines with time, severity, project and message; a trace link appears on lines that belong to a trace")

## Sending logs

- **Python**: pass `enable_logs=True` to `errorlens.init` and records from the standard `logging` module are sent. See [Python](/docs/python/#logs).
- **Any language**: export logs with [OpenTelemetry](/docs/opentelemetry/) to `https://errorlens.io/v1/logs`.
- SDKs in other languages that support structured logs over the envelope protocol work too.

## Finding lines

- **Search** matches text in the log message and its attributes.
- **Severity** shows a level and everything above it: *trace*, *debug*, *info*, *warn*, *error*, *fatal*. Choose "error and above" to see only problems.
- **Project** narrows the list to one service, or open a project's path such as `/checkout-api/logs`.
- **Trace** — following a trace link shows only the lines of that trace.

The page shows the newest 100 lines; click **Load older** at the bottom to page back in time.

## Live tail

Tick **Live tail** to watch new lines arrive as they are written, with your filters applied. It is useful while deploying or reproducing a bug.

## Attributes

Click a line to see its attributes — the structured fields sent with it, such as `order_id` or `service` — along with its release, environment and trace ID.

## Privacy

Log messages and attributes go through the same [scrubbing](/docs/data-scrubbing/) as errors, so a token printed in a log line is removed before it is stored. Logs are kept for the project's retention period.
