---
title: Quick start
description: Create an account, make a project and see your first error in about five minutes.
section: Getting started
order: 2
---

## 1. Create your account

Go to [errorlens.io/signup](https://errorlens.io/signup) and enter your name, email, a password of at least 10 characters, and an **organization name**. The organization is your team's workspace; its name becomes your dashboard address — "Acme Shop" becomes `acme-shop.errorlens.io`.

![The sign-up form](/screenshots/signup.png "Sign up with your work email")

We send a confirmation link to your email. You can start using ErrorLens straight away; confirm your address when the email arrives.

> Running your own server? The first visit to a new self-hosted instance shows a one-time setup form instead. See [Self-hosting](/docs/self-hosting/).

## 2. Create a project

A project is one app or service — "Checkout API", "Web storefront". Open **Projects**, type a name, pick the platform and click **Create project**. Every project gets a **DSN**: the address your SDK sends data to.

![The projects page with a DSN and its setup guide](/screenshots/projects.png "Each project has a DSN and a setup guide for its platform")

Click **Setup guide** for copy-paste instructions for your platform.

## 3. Install an SDK

Pick your language:

**Python**

```bash
pip install errorlens
```

```python
import errorlens

errorlens.init(
    dsn="https://<key>@errorlens.io/<project-id>",
    release="my-app@1.0.0",
    environment="production",
)
```

**Node.js**

```bash
npm install @errorlens/node
```

```js
import * as ErrorLens from "@errorlens/node";

ErrorLens.init({ dsn: "https://<key>@errorlens.io/<project-id>", environment: "production" });
```

**Browser**

```bash
npm install @errorlens/browser
```

```js
import * as ErrorLens from "@errorlens/browser";

ErrorLens.init({ dsn: "https://<key>@errorlens.io/<project-id>" });
```

Other languages work too — see [Other SDKs](/docs/other-sdks/) and [OpenTelemetry](/docs/opentelemetry/).

## 4. Send a test error

Trigger an error your app doesn't handle, or report one on purpose:

```python
errorlens.capture_message("Hello from ErrorLens")
```

```js
ErrorLens.captureException(new Error("Hello from ErrorLens"));
```

Within a few seconds it appears on the **Issues** page of your dashboard.

## 5. Get notified

Open **Alerts**, add a channel (email, Slack, Discord or a webhook), then add a rule such as "a new issue appears in production". See [Alerts](/docs/alerts/).

## What's next

- [Issues and grouping](/docs/issues/) — how errors become issues
- [Performance](/docs/performance/) — turn on tracing with one option
- [Data scrubbing](/docs/data-scrubbing/) — check what is removed before storage
- [Members and roles](/docs/members-roles/) — invite your team
