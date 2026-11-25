# @errorlens/browser

Report errors from browser apps to your own [ErrorLens](https://errorlens.io) server.

```bash
npm install @errorlens/browser
```

```js
import * as ErrorLens from "@errorlens/browser";

ErrorLens.init({
  dsn: "https://<key>@errors.example.com/1", // from your ErrorLens project page
  release: "my-app@1.0.0",
  environment: "production",
});
```

Uncaught errors and unhandled promise rejections are reported automatically.
Use `ErrorLens.captureException(err)` / `ErrorLens.captureMessage(msg)` to report manually and
`ErrorLens.setUser({ id })` to attach the signed-in user.

License: MIT.
