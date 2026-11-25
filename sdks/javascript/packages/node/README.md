# @errorlens/node

Report errors from Node.js apps to your own [ErrorLens](https://errorlens.io) server.

```bash
npm install @errorlens/node
```

```js
// Load this before anything else in your app.
import * as ErrorLens from "@errorlens/node";

ErrorLens.init({
  dsn: "https://<key>@errors.example.com/1", // from your ErrorLens project page
  release: "my-app@1.0.0",
  environment: "production",
});
```

Unhandled exceptions and promise rejections are reported automatically. Manual reporting:

```js
try {
  risky();
} catch (err) {
  ErrorLens.captureException(err);
}
ErrorLens.captureMessage("Payment provider slow", "warning");
ErrorLens.setUser({ id: "42" });
```

Express: call `ErrorLens.setupExpressErrorHandler(app)` after your routes (Fastify, Koa and Hapi
helpers are also exported). Configuration can come from `ERRORLENS_DSN`, `ERRORLENS_RELEASE` and
`ERRORLENS_ENVIRONMENT`.

License: MIT.
