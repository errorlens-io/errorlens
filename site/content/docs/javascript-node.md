---
title: Node.js
description: Report errors and performance data from Node.js servers with @errorlens/node.
section: Sending data
order: 2
---

## Install

```bash
npm install @errorlens/node
```

## Configure

Initialize ErrorLens **before anything else** in your app, so it can instrument the modules you load afterwards. A common pattern is a separate file loaded first:

```js
// instrument.js
import * as ErrorLens from "@errorlens/node";

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
import * as ErrorLens from "@errorlens/node";

const app = express();
// ... your routes ...
ErrorLens.setupExpressErrorHandler(app);
app.listen(3000);
```

The equivalents for other frameworks are `setupFastifyErrorHandler(app)`, `setupKoaErrorHandler(app)` and `setupHapiErrorHandler(server)`.

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

They appear on the [Performance](/docs/performance/) page. For logs from Node.js, use [OpenTelemetry](/docs/opentelemetry/).

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

The ErrorLens server also [removes secrets](/docs/data-scrubbing/) from every event before storage, whatever the SDK sends.
