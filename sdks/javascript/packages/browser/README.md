# @rootcausehq/browser

Report errors from browser apps to your own [RootCause](https://rootcause.sh) server.

```bash
npm install @rootcausehq/browser
```

```js
import * as RootCause from "@rootcausehq/browser";

RootCause.init({
  dsn: "https://<key>@errors.example.com/1", // from your RootCause project page
  release: "my-app@1.0.0",
  environment: "production",
});
```

Uncaught errors and unhandled promise rejections are reported automatically.
Use `RootCause.captureException(err)` / `RootCause.captureMessage(msg)` to report manually and
`RootCause.setUser({ id })` to attach the signed-in user.

License: MIT.
