---
title: Browser JavaScript
description: Set up @errorlenshq/browser for websites and single-page apps: uncaught errors, breadcrumbs, page-load tracing, web vitals, React error boundaries.
section: Sending data
order: 3
seo_title: Browser JavaScript SDK Setup
---

## Install

```bash
npm install @errorlenshq/browser
```

Or without a bundler, with a script tag (12 KB gzipped):

```html
<script src="https://cdn.jsdelivr.net/npm/@errorlenshq/browser@1/dist/errorlens.min.js" crossorigin="anonymous"></script>
<script>ErrorLens.init({ dsn: "https://<key>@errorlens.io/<project-id>" });</script>
```

## Configure

Initialize as early as possible in your app's entry point:

```js
import * as ErrorLens from "@errorlenshq/browser";

ErrorLens.init({
  dsn: "https://<key>@errorlens.io/<project-id>",
  release: "web@1.8.0",
  environment: "production",
});
```

The DSN's key can only *send* data to the project, never read it, so it is safe to include in code that runs in the browser.

Uncaught errors and unhandled promise rejections are reported automatically, with the clicks, navigation, console output and network requests that led up to them as breadcrumbs. Errors from browser extensions and other unfixable noise are dropped; use `allowUrls` to report only errors from your own scripts.

## Reliable delivery

Reports are sent with `keepalive`, so errors right before the user leaves the page still arrive. On flaky networks — a dropped mobile connection, being briefly offline — a report that can't be sent is retried once, then kept in `localStorage` (at most 30 reports, for up to 24 hours) and sent again 30 seconds later, when the browser comes back online, or on the next page load. Answers from the server, such as a rate limit, are never retried.

To keep nothing in the browser, turn it off:

```js
ErrorLens.init({ dsn: "…", offlineQueue: false });
```

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

## Performance

```js
ErrorLens.init({ dsn: "…", tracesSampleRate: 0.1 });
```

This records each page load (with network timing, resources and web vitals such as LCP and CLS), each route change in single-page apps, and the `fetch`/XHR calls made along the way. Requests to your own origin carry a `traceparent` header, so they join the same trace as your backend. To include API calls to other origins, list them in `tracePropagationTargets` (the API must allow the `traceparent` and `tracestate` headers in CORS).

## Logs

```js
ErrorLens.init({ dsn: "…", enableLogs: true });
ErrorLens.logger.warn("Cart {cartId} is slow", { cartId: 7 });
```

## Readable stack traces

Production JavaScript is minified, so stack traces point at `main.4f2a.js:1:120554`. Upload your source maps for each release and ErrorLens shows the original file, line and code instead. See [Releases and source maps](/docs/releases-source-maps/).

Use the same `release` value in `init` and in the upload command.

## React, Vue, Svelte and other frameworks

`@errorlenshq/browser` works with any framework. For React, wrap risky parts of the tree in an error boundary and report from it:

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
