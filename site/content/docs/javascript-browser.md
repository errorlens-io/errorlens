---
title: Browser JavaScript
description: Report errors from websites and single-page apps with @errorlens/browser.
section: Sending data
order: 3
---

## Install

```bash
npm install @errorlens/browser
```

## Configure

Initialize as early as possible in your app's entry point:

```js
import * as ErrorLens from "@errorlens/browser";

ErrorLens.init({
  dsn: "https://<key>@errorlens.io/<project-id>",
  release: "web@1.8.0",
  environment: "production",
});
```

The DSN's key can only *send* data to the project, never read it, so it is safe to include in code that runs in the browser.

Uncaught errors and unhandled promise rejections are reported automatically, with the clicks, navigation and network requests that led up to them as breadcrumbs.

## Report errors

```js
try {
  renderCart(cart);
} catch (err) {
  ErrorLens.captureException(err);
}

ErrorLens.captureMessage("Checkout button clicked twice");
```

## Identify the user

```js
ErrorLens.setUser({ id: account.id });
```

Avoid putting names or email addresses here unless your privacy policy allows it — an opaque ID is enough to count affected users.

## Readable stack traces

Production JavaScript is minified, so stack traces point at `main.4f2a.js:1:120554`. Upload your source maps for each release and ErrorLens shows the original file, line and code instead. See [Releases and source maps](/docs/releases-source-maps/).

Use the same `release` value in `init` and in the upload command.

## React, Vue, Svelte and other frameworks

`@errorlens/browser` works with any framework. For React, wrap risky parts of the tree in an error boundary and report from it:

```jsx
class ErrorBoundary extends React.Component {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  componentDidCatch(error, info) {
    ErrorLens.withScope((scope) => {
      scope.setContext("react", { componentStack: info.componentStack });
      ErrorLens.captureException(error);
    });
  }
  render() { return this.state.failed ? <p>Something went wrong.</p> : this.props.children; }
}
```

## Ad blockers and content security policy

If your site has a Content-Security-Policy, allow requests to the DSN's host:

```text
connect-src 'self' https://errorlens.io;
```
