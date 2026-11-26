---
title: Node.js
description: Set up @errorlenshq/node: report errors and unhandled rejections, Express, Fastify, Koa and Hapi helpers, tracing, logs, cron monitors and serverless.
section: Sending data
order: 2
seo_title: Node.js SDK: Error Tracking Setup
---

## Install

Node.js 18 or newer. The package has no dependencies besides its own core.

```bash
npm install @errorlenshq/node
```

## Configure

Initialize ErrorLens **before anything else** in your app, so it can instrument the modules you load afterwards. A common pattern is a separate file loaded first:

```js
// instrument.js
import * as ErrorLens from "@errorlenshq/node";

ErrorLens.init({
  dsn: "https://<key>@errorlens.io/<project-id>",
  release: "api@2.3.0",
  environment: "production",
});
```

```bash
node --import ./instrument.js server.js
```

With CommonJS, `require("./instrument")` on the first line of your entry file instead.

`dsn`, `release` and `environment` default to the `ERRORLENS_DSN`, `ERRORLENS_RELEASE` and `ERRORLENS_ENVIRONMENT` environment variables. Without a DSN the SDK does nothing.

## Frameworks

Unhandled exceptions and unhandled promise rejections are reported automatically. Web frameworks catch errors themselves, so register the error handler after your routes:

```js
import express from "express";
import * as ErrorLens from "@errorlenshq/node";

const app = express();
// ... your routes ...
ErrorLens.setupExpressErrorHandler(app);
app.listen(3000);
```

The equivalents for other frameworks are `setupFastifyErrorHandler(app)`, `setupKoaErrorHandler(app)` and `await setupHapiErrorHandler(server)`. Only server errors are reported; 4xx responses like "not found" are not.

Every incoming request — in any framework, or a plain `http` server — gets its own context automatically: a user or tag set while handling one request never appears on another's errors. Unhandled errors are reported and then the process exits the way Node.js normally would.

For serverless functions, wrap the handler so events are sent before the function is frozen:

```js
export const handler = ErrorLens.wrapHandler(async (event) => {
  // ...
});
```

## Report errors

```js
try {
  await charge(order);
} catch (err) {
  ErrorLens.captureException(err);
}

ErrorLens.captureMessage("Payment provider is slow", "warning");
```

## Add context

```js
ErrorLens.setUser({ id: "42" });
ErrorLens.setTag("region", "eu-west");
ErrorLens.setContext("order", { id: 9817, items: 3 });
ErrorLens.addBreadcrumb({ category: "payment", message: "Charging card" });

// Context for one block of code only:
ErrorLens.withScope((scope) => {
  scope.setTag("job", "nightly-sync");
  ErrorLens.captureException(err);
});
```

## Performance

Set a sample rate to record a share of incoming requests together with their database queries and outgoing HTTP calls:

```js
ErrorLens.init({ dsn: "…", tracesSampleRate: 0.2 });
```

They appear on the [Performance](/docs/performance/) page, named after the route (`GET /orders/:id`). Outgoing `http`, `https` and `fetch` calls carry a W3C `traceparent` header, so traces continue into other services.

Time your own code with `startSpan`; promises are awaited:

```js
const rows = await ErrorLens.startSpan({ name: "load orders", op: "db.query" }, () => db.orders.find());
```

## Logs

```js
ErrorLens.init({ dsn: "…", enableLogs: true });
ErrorLens.logger.info("Order {orderId} paid", { orderId: 42, amount: 19.99 });
```

Placeholders and extra fields become searchable attributes on the [Logs](/docs/logs/) page. Add `consoleLogs: true` to also send `console.*` output.

## Cron monitors

```js
await ErrorLens.withMonitor("nightly-backup", () => runBackup(), {
  schedule: { type: "crontab", value: "0 2 * * *" },
  maxRuntime: 30,
});
```

See [Cron monitors](/docs/cron-monitors/).

## Before exit

In scripts, CLIs and serverless functions, wait for queued events before the process ends:

```js
await ErrorLens.flush(2000);
```

## Filtering events

```js
ErrorLens.init({
  dsn: "…",
  beforeSend(event) {
    if (event.request?.url?.endsWith("/health")) return null; // drop
    return event;
  },
});
```

Passwords, tokens and API keys are removed in your app before anything is sent (add names with `scrubFields`), and the ErrorLens server [removes secrets again](/docs/data-scrubbing/) before storage. IP addresses, cookies and request bodies are only sent with `sendDefaultPii: true`.

Source: [github.com/errorlenshq/errorlens-javascript](https://github.com/errorlenshq/errorlens-javascript).
