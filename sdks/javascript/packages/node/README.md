# @rootcausehq/node

Report errors from Node.js apps to your own [RootCause](https://rootcause.sh) server.

```bash
npm install @rootcausehq/node
```

```js
// Load this before anything else in your app.
import * as RootCause from "@rootcausehq/node";

RootCause.init({
  dsn: "https://<key>@errors.example.com/1", // from your RootCause project page
  release: "my-app@1.0.0",
  environment: "production",
});
```

Unhandled exceptions and promise rejections are reported automatically. Manual reporting:

```js
try {
  risky();
} catch (err) {
  RootCause.captureException(err);
}
RootCause.captureMessage("Payment provider slow", "warning");
RootCause.setUser({ id: "42" });
```

Express: call `RootCause.setupExpressErrorHandler(app)` after your routes (Fastify, Koa and Hapi
helpers are also exported). Configuration can come from `ROOTCAUSE_DSN`, `ROOTCAUSE_RELEASE` and
`ROOTCAUSE_ENVIRONMENT`.

License: MIT.
