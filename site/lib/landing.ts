// Landing pages for search: one page per topic people search for, each with
// real setup code, a short FAQ and links to the docs. Body strings are trusted
// HTML written here (links, <code>), never user input.

export type Code = { label?: string; lang: string; code: string };
export type Section = { h2: string; body?: string[]; list?: string[]; code?: Code[] };
export type Faq = { q: string; a: string };
export type Landing = {
  path: string; // with leading and trailing slash
  nav: string; // short name for menus and breadcrumbs
  title: string; // <title>, under ~60 characters with the brand suffix
  description: string; // meta description, ~140–160 characters
  eyebrow: string;
  h1: string;
  lead: string;
  sections: Section[];
  faq: Faq[];
  related: string[];
  group: "error-tracking" | "framework" | "product";
};

const DSN = "https://&lt;key&gt;@errorlens.io/&lt;project-id&gt;";
const dsn = "https://<key>@errorlens.io/<project-id>";

const PY_INSTALL: Code = { lang: "bash", code: "pip install errorlens" };
const NODE_INSTALL: Code = { lang: "bash", code: "npm install @errorlenshq/node" };
const BROWSER_INSTALL: Code = { lang: "bash", code: "npm install @errorlenshq/browser" };

const PRIVACY_LINE =
  'Passwords, tokens, API keys, cookies and card numbers are removed in your app <em>and</em> again on the server before anything is stored — see <a href="/docs/data-scrubbing/">data scrubbing</a>.';

export const LANDINGS: Landing[] = [
  // ---------------------------------------------------------------- pillar
  {
    path: "/error-tracking/",
    nav: "Error tracking",
    group: "error-tracking",
    title: "Open-Source Error Tracking Software",
    description:
      "ErrorLens is open-source error tracking and monitoring: grouped issues, stack traces with source context, alerts and releases. Free to self-host or hosted.",
    eyebrow: "Error tracking",
    h1: "Open-source error tracking that finds the bug, not just the log line",
    lead:
      "ErrorLens catches every unhandled exception in your app, groups repeats into one issue, and shows the stack trace, the request, the user and the steps that led to it — then tells you the moment something new breaks.",
    sections: [
      {
        h2: "What error tracking gives you that logs don't",
        body: [
          "A log file tells you an exception happened. An error tracker tells you which exception, how often, since which release, for how many users, and exactly which line of your code raised it. Instead of grepping through thousands of lines, you get a short list of issues sorted by impact.",
        ],
        list: [
          "<strong>Grouping</strong> — the same bug seen 10,000 times is one issue with a counter and a graph, not 10,000 emails",
          "<strong>Stack traces with source context</strong> — the lines around every frame of your own code, plus local variables in Python",
          "<strong>Breadcrumbs</strong> — the clicks, requests, queries and log lines leading up to the error",
          "<strong>Releases and regressions</strong> — see which deploy introduced a bug; resolved issues reopen if it comes back",
          "<strong>Alerts</strong> — new issues, regressions and spikes go to email, Slack, Discord or webhooks",
          "<strong>Readable JavaScript</strong> — upload source maps and minified stack traces show your original code",
        ],
      },
      {
        h2: "Set up in two minutes",
        body: ["Install the SDK for your language, add one line with your project's DSN, and deploy. Unhandled errors are reported automatically from then on."],
        code: [
          { label: "Python", lang: "python", code: `import errorlens\n\nerrorlens.init(dsn="${dsn}")` },
          { label: "Node.js", lang: "js", code: `import * as ErrorLens from "@errorlenshq/node";\n\nErrorLens.init({ dsn: process.env.ERRORLENS_DSN });` },
          { label: "Browser", lang: "js", code: `import * as ErrorLens from "@errorlenshq/browser";\n\nErrorLens.init({ dsn: "${dsn}" });` },
        ],
      },
      {
        h2: "Why teams choose ErrorLens",
        list: [
          "<strong>Open source</strong> — the server is AGPL-3.0 and the SDKs are MIT; read every line that touches your data",
          "<strong>Private by design</strong> — secrets are scrubbed before storage, IPs are truncated, every event view is audited",
          "<strong>Simple to run</strong> — one container and Postgres; no Kafka, Redis or ClickHouse",
          "<strong>More than errors</strong> — performance tracing, logs, cron and uptime monitors in the same place, linked by trace",
          "<strong>Light SDKs</strong> — zero dependencies; the browser bundle is 12 KB gzipped",
        ],
      },
    ],
    faq: [
      { q: "What is error tracking?", a: "Error tracking automatically captures exceptions from running applications, groups identical errors into issues, and records the context needed to fix them: stack trace, request, user, release and the events leading up to the error. It alerts you when new problems appear." },
      { q: "Is ErrorLens free?", a: "Yes. Self-hosting is free with no event limits, and the hosted service has a free plan with 5,000 events a month. The server is open source under AGPL-3.0 and the SDKs under MIT." },
      { q: "Which languages are supported?", a: "ErrorLens has its own SDKs for Python, Node.js and browser JavaScript. Other languages can send errors with any SDK that speaks the standard error-envelope protocol, and traces and logs with OpenTelemetry." },
      { q: "How is error tracking different from logging?", a: "Logs record everything as text. Error tracking extracts exceptions, groups repeats, counts affected users, links each error to a release and a trace, and alerts on new or returning issues — so you see the few problems that matter instead of every line." },
    ],
    related: ["/self-hosted-error-tracking/", "/error-tracking/python/", "/error-tracking/javascript/", "/apm/"],
  },

  // ---------------------------------------------------------------- frameworks
  {
    path: "/error-tracking/python/",
    nav: "Python",
    group: "framework",
    title: "Python Error Tracking and Monitoring",
    description:
      "Track Python exceptions with full tracebacks, local variables and request context. One line to set up for Django, Flask, FastAPI and Celery. Open source.",
    eyebrow: "Python error tracking",
    h1: "Python error tracking with tracebacks you can actually debug",
    lead:
      "The errorlens package reports every unhandled exception in your Python app with the full traceback, the source around each frame, the local variables at the moment it failed, and the request that triggered it.",
    sections: [
      {
        h2: "Install and initialize",
        body: ["The package has no dependencies and supports Python 3.9 and newer. Call <code>init</code> once, as early as possible:"],
        code: [PY_INSTALL, { lang: "python", code: `import errorlens\n\nerrorlens.init(\n    dsn="${dsn}",\n    release="my-app@1.4.2",\n    traces_sample_rate=0.2,   # optional: performance tracing\n    enable_logs=True,         # optional: send logs too\n)` }],
      },
      {
        h2: "Detected automatically",
        body: ["There is no extra setup for the libraries you already use. When they're installed, ErrorLens hooks into them:"],
        list: [
          '<a href="/error-tracking/django/">Django</a>, <a href="/error-tracking/flask/">Flask</a>, <a href="/error-tracking/fastapi/">FastAPI</a> and Starlette — errors, request data and route-named transactions',
          '<a href="/error-tracking/celery/">Celery</a> — task failures, with traces that continue from the code that queued the task',
          "SQLAlchemy and Django ORM — query spans; httpx, <code>requests</code>/<code>urllib</code> — outgoing request spans and breadcrumbs",
          "The standard <code>logging</code> module — ERROR records become events, lower levels become breadcrumbs",
          "Threads and uncaught exceptions in scripts",
        ],
      },
      {
        h2: "Report handled errors and add context",
        code: [{ lang: "python", code: `try:\n    charge(order)\nexcept PaymentError:\n    errorlens.capture_exception()   # the current exception\n\nerrorlens.set_user({"id": user.id})\nerrorlens.set_tag("plan", "pro")\nerrorlens.add_breadcrumb(category="payment", message="Charging card")` }],
      },
      {
        h2: "Python logging vs. error tracking",
        body: [
          "<code>logging.exception()</code> writes a traceback to a file and stops there. ErrorLens keeps your logging setup and adds what logs can't: identical exceptions grouped into one issue, a count of affected users, the release that introduced it, and an alert the first time it happens.",
          PRIVACY_LINE,
        ],
      },
    ],
    faq: [
      { q: "Does the SDK slow my Python app down?", a: "Events are serialized and sent on a background thread with a bounded queue, so request handling never waits on the network. Tracing is off unless you set traces_sample_rate." },
      { q: "Can I see local variables in tracebacks?", a: "Yes. Local variables are captured for frames in your own code (include_local_variables, on by default), with secrets such as passwords and tokens filtered out." },
      { q: "Does it work with async code?", a: "Yes. Context is stored per request and per task using contextvars, so asyncio apps such as FastAPI keep users and tags separate between concurrent requests." },
    ],
    related: ["/error-tracking/django/", "/error-tracking/fastapi/", "/error-tracking/celery/", "/apm/"],
  },
  {
    path: "/error-tracking/django/",
    nav: "Django",
    group: "framework",
    title: "Django Error Tracking and Monitoring",
    description:
      "Catch Django 500 errors with full tracebacks, request data, users and slow queries. Grouped issues and instant alerts instead of ADMINS emails. Open source.",
    eyebrow: "Django error tracking",
    h1: "Django error tracking without the 500-error email flood",
    lead:
      "Django can email ADMINS a traceback for every 500 — one email per occurrence, no grouping, no way to mark it fixed. ErrorLens groups those exceptions into issues, adds the request, user and SQL queries, and alerts you once.",
    sections: [
      {
        h2: "Add it to settings.py",
        body: ["Install the package and initialize it in <code>settings.py</code>. The Django integration turns on by itself — no middleware or <code>INSTALLED_APPS</code> entry needed."],
        code: [PY_INSTALL, { label: "settings.py", lang: "python", code: `import errorlens\n\nerrorlens.init(\n    dsn="${dsn}",\n    release="shop@2.3.0",\n    traces_sample_rate=0.2,\n)` }],
      },
      {
        h2: "What you get for every Django error",
        list: [
          "The full traceback with source context and local variables for your own frames",
          "The request: method, URL, route, headers and query string (cookies and bodies only if you opt in)",
          "The transaction named after the URL pattern, e.g. <code>/orders/&lt;int:pk&gt;/</code>, not the raw path",
          "Every SQL query run by the request as a span, so slow views and N+1 queries stand out",
          "Users and tags you set during the request, kept separate between concurrent requests",
          "Both WSGI and ASGI deployments, including async views",
        ],
      },
      {
        h2: "Attach the logged-in user",
        body: ["Add the user once per request, for example in a small middleware, and every error shows who was affected:"],
        code: [{ lang: "python", code: `def errorlens_user(get_response):\n    def middleware(request):\n        if request.user.is_authenticated:\n            errorlens.set_user({"id": request.user.pk})\n        return get_response(request)\n    return middleware` }],
      },
      { h2: "Privacy", body: [PRIVACY_LINE] },
    ],
    faq: [
      { q: "Do I still need ADMINS emails?", a: "No. ErrorLens alerts you once per new issue (and again on regressions or spikes), with a link to the grouped issue, so the ADMINS email handler can be removed." },
      { q: "Does it capture errors from management commands and Celery tasks?", a: "Yes. Uncaught exceptions in management commands are reported, and Celery task failures are captured by the Celery integration." },
      { q: "Which Django versions are supported?", a: "Current Django releases on Python 3.9 and newer, under WSGI or ASGI." },
    ],
    related: ["/error-tracking/python/", "/error-tracking/celery/", "/apm/", "/cron-job-monitoring/"],
  },
  {
    path: "/error-tracking/flask/",
    nav: "Flask",
    group: "framework",
    title: "Flask Error Tracking and Monitoring",
    description:
      "Report Flask exceptions with tracebacks, request data and route names, and trace slow endpoints. Set up with one line, no extension needed. Open source.",
    eyebrow: "Flask error tracking",
    h1: "Flask error tracking in one line",
    lead:
      "Initialize ErrorLens before you create your Flask app and every unhandled exception is reported with its traceback, the request and the route — no extension or error handler to write.",
    sections: [
      {
        h2: "Set up",
        code: [PY_INSTALL, { label: "app.py", lang: "python", code: `import errorlens\nfrom flask import Flask\n\nerrorlens.init(dsn="${dsn}", traces_sample_rate=0.2)\n\napp = Flask(__name__)\n\n@app.get("/orders/<int:order_id>")\ndef order(order_id):\n    return load_order(order_id)` }],
      },
      {
        h2: "What is captured",
        list: [
          "Exceptions raised in views, including those Flask turns into 500 responses",
          "Request method, URL, headers and route (<code>/orders/&lt;int:order_id&gt;</code>)",
          "Per-request context: users and tags set in one request never leak into another",
          "Transactions per route with SQLAlchemy query spans and outgoing HTTP calls",
          "Streaming responses, which are timed until the last chunk is sent",
        ],
      },
      {
        h2: "Flask error logging, upgraded",
        body: [
          "<code>app.logger.exception()</code> still works — ERROR log records become ErrorLens events and lower levels become breadcrumbs. What you gain is grouping, alerts, releases and the request context next to each traceback.",
          PRIVACY_LINE,
        ],
      },
    ],
    faq: [
      { q: "Do I need a Flask extension?", a: "No. Calling errorlens.init() before creating the app is enough; the Flask integration is detected automatically." },
      { q: "Does it work with Gunicorn and uWSGI?", a: "Yes. The SDK is fork-safe: its background sender restarts in each worker process." },
    ],
    related: ["/error-tracking/python/", "/error-tracking/django/", "/apm/", "/log-management/"],
  },
  {
    path: "/error-tracking/fastapi/",
    nav: "FastAPI",
    group: "framework",
    title: "FastAPI Error Tracking and Monitoring",
    description:
      "Track FastAPI and Starlette exceptions with route-named traces, async-safe request context and query spans. One-line setup, open source, self-hostable.",
    eyebrow: "FastAPI error tracking",
    h1: "FastAPI error tracking built for async",
    lead:
      "ErrorLens reports unhandled exceptions from FastAPI and Starlette with the request and route, keeps context separate between concurrent async requests, and traces each endpoint's latency.",
    sections: [
      {
        h2: "Set up",
        code: [PY_INSTALL, { label: "main.py", lang: "python", code: `import errorlens\nfrom fastapi import FastAPI\n\nerrorlens.init(dsn="${dsn}", traces_sample_rate=0.2)\n\napp = FastAPI()\n\n@app.get("/items/{item_id}")\nasync def read_item(item_id: int):\n    return await fetch_item(item_id)` }],
      },
      {
        h2: "Built for async Python",
        list: [
          "Context lives in <code>contextvars</code>, so tags and users stay with their own request across <code>await</code>s",
          "Transactions are named after the route template — <code>GET /items/{item_id}</code> — so every item ID groups together",
          "httpx calls become spans and carry a <code>traceparent</code> header to the next service",
          "Validation errors (4xx) are not reported as errors; server errors are",
        ],
      },
      { h2: "Privacy", body: [PRIVACY_LINE] },
    ],
    faq: [
      { q: "Does it work with Starlette apps without FastAPI?", a: "Yes. The integration hooks Starlette itself, so plain Starlette apps and other frameworks built on it are covered." },
      { q: "How do I trace background work?", a: "Wrap it in errorlens.start_transaction(...) and errorlens.start_span(...), or move it to Celery, whose tasks are traced automatically." },
    ],
    related: ["/error-tracking/python/", "/apm/", "/opentelemetry-backend/", "/error-tracking/celery/"],
  },
  {
    path: "/error-tracking/celery/",
    nav: "Celery",
    group: "framework",
    title: "Celery Task Monitoring and Error Tracking",
    description:
      "Monitor Celery tasks: failures with tracebacks, task durations, traces that follow a job from the web request that queued it, and alerts for missed beat jobs.",
    eyebrow: "Celery monitoring",
    h1: "Celery task monitoring that connects jobs to the request that queued them",
    lead:
      "A failed Celery task is usually discovered hours later. ErrorLens reports task failures the moment they happen, times every task, and links each one to the web request that enqueued it.",
    sections: [
      {
        h2: "Set up in your Celery app",
        body: ["Initialize ErrorLens wherever your Celery app is created, so it runs in every worker:"],
        code: [PY_INSTALL, { label: "celery_app.py", lang: "python", code: `import errorlens\nfrom celery import Celery\n\nerrorlens.init(dsn="${dsn}", traces_sample_rate=0.2)\n\napp = Celery("shop", broker="redis://localhost:6379/0")` }],
      },
      {
        h2: "What you get",
        list: [
          "Task exceptions with traceback, task name and task ID",
          "A transaction per task run with its duration and the queries and HTTP calls inside it",
          "Distributed traces: the trace continues from the web request that called <code>.delay()</code>",
          "Failures and durations grouped per task name, so a flaky or slow task stands out",
        ],
      },
      {
        h2: "Catch beat jobs that never ran",
        body: ['A scheduled task that silently stops running raises no exception at all. Wrap it in a cron monitor and ErrorLens alerts you when a run is missed, fails or runs too long — see <a href="/cron-job-monitoring/">cron job monitoring</a>.'],
        code: [{ lang: "python", code: `@app.task\n@errorlens.monitor("nightly-invoices", schedule="0 2 * * *", max_runtime=30)\ndef send_invoices():\n    ...` }],
      },
    ],
    faq: [
      { q: "Is this a replacement for Flower?", a: "They answer different questions. Flower shows live worker and queue state; ErrorLens records what failed and why, how long tasks take over time, and alerts you — with each task linked to the request that queued it." },
      { q: "Are task arguments sent?", a: "No. Arguments often contain personal data, so only the task name and ID are attached to events." },
    ],
    related: ["/cron-job-monitoring/", "/error-tracking/django/", "/error-tracking/python/", "/apm/"],
  },
  {
    path: "/error-tracking/nodejs/",
    nav: "Node.js",
    group: "framework",
    title: "Node.js Error Tracking and Monitoring",
    description:
      "Track Node.js errors, unhandled rejections and slow requests with per-request context and distributed tracing. Zero-dependency SDK, ESM and CommonJS.",
    eyebrow: "Node.js error tracking",
    h1: "Node.js error tracking with per-request context",
    lead:
      "@errorlenshq/node reports uncaught exceptions and unhandled promise rejections, isolates context per request in any HTTP server, and traces outgoing calls — with no dependencies.",
    sections: [
      {
        h2: "Install and initialize",
        body: ["Node.js 18 or newer. Initialize before anything else in your entry file. ES modules and CommonJS both work."],
        code: [NODE_INSTALL, { lang: "js", code: `import * as ErrorLens from "@errorlenshq/node";\n\nErrorLens.init({\n  dsn: process.env.ERRORLENS_DSN,\n  tracesSampleRate: 0.2,\n  enableLogs: true,\n});` }],
      },
      {
        h2: "Automatic, for any Node.js server",
        list: [
          "Uncaught exceptions and unhandled rejections — reported, then the process exits exactly as Node.js would",
          "Every incoming request gets its own scope (AsyncLocalStorage): users and breadcrumbs never mix between requests",
          "Transactions named after the route, continuing incoming W3C <code>traceparent</code> headers",
          "Outgoing <code>http</code>, <code>https</code> and <code>fetch</code> calls traced and propagated",
          "Console output as breadcrumbs, source context for your own frames, secrets scrubbed",
          '<a href="/error-tracking/express/">Express</a>, Fastify, Koa and Hapi helpers for one-line error reporting',
        ],
      },
      {
        h2: "Serverless and scripts",
        code: [{ lang: "js", code: `// AWS Lambda, Google Cloud Functions…\nexport const handler = ErrorLens.wrapHandler(async (event) => {\n  // events are sent before the function returns\n});\n\n// short scripts\nawait ErrorLens.flush();` }],
      },
    ],
    faq: [
      { q: "Will it change how my process crashes?", a: "No. After reporting an uncaught error the SDK flushes and exits with code 1, as Node.js does by default, and it respects --unhandled-rejections=warn or none." },
      { q: "Does it support TypeScript?", a: "Yes. Types are included, and the package ships both ES module and CommonJS builds." },
      { q: "How big is the SDK?", a: "It has no third-party dependencies; the Node.js package and its core are about 35 KB packed." },
    ],
    related: ["/error-tracking/express/", "/error-tracking/nextjs/", "/apm/", "/log-management/"],
  },
  {
    path: "/error-tracking/express/",
    nav: "Express",
    group: "framework",
    title: "Express Error Tracking and Monitoring",
    description:
      "Report Express errors with route-named transactions, request context and traces. Works with Express 4 and 5 in one line. Open-source error monitoring.",
    eyebrow: "Express error tracking",
    h1: "Express error tracking in one line",
    lead:
      "Add one error handler after your routes and every server error in your Express app is reported with the route, the request and the trace — while your own error handlers keep working.",
    sections: [
      {
        h2: "Set up",
        code: [NODE_INSTALL, { label: "server.js", lang: "js", code: `import * as ErrorLens from "@errorlenshq/node";\nErrorLens.init({ dsn: process.env.ERRORLENS_DSN, tracesSampleRate: 0.2 });\n\nimport express from "express";\nconst app = express();\n\napp.get("/orders/:id", getOrder);\n\n// after all routes, before your own error handlers\nErrorLens.setupExpressErrorHandler(app);\n\napp.listen(3000);` }],
      },
      {
        h2: "What you get",
        list: [
          "Only server errors are reported — a 404 or validation error passed to <code>next(err)</code> with a 4xx status is skipped",
          "Transactions named after the full route, including routers: <code>GET /api/orders/:id</code>",
          "Request method, URL, headers and query string; cookies and bodies only with <code>sendDefaultPii</code>",
          "Each request isolated: <code>ErrorLens.setUser()</code> in one request never shows up on another",
        ],
      },
      {
        h2: "Fastify, Koa and Hapi",
        code: [{ lang: "js", code: `ErrorLens.setupFastifyErrorHandler(fastify);\nErrorLens.setupKoaErrorHandler(app);\nawait ErrorLens.setupHapiErrorHandler(server);` }],
      },
    ],
    faq: [
      { q: "Do I still need my own error middleware?", a: "Keep it. The ErrorLens handler reports the error and passes it on with next(err), so your handler still decides the response." },
      { q: "Express 4 or 5?", a: "Both are supported." },
    ],
    related: ["/error-tracking/nodejs/", "/apm/", "/opentelemetry-backend/", "/uptime-monitoring/"],
  },
  {
    path: "/error-tracking/javascript/",
    nav: "JavaScript",
    group: "framework",
    title: "JavaScript Error Tracking for the Browser",
    description:
      "Track JavaScript errors in the browser with breadcrumbs, source maps, page-load timing and web vitals. 12 KB gzipped, works with any frontend. Open source.",
    eyebrow: "JavaScript error tracking",
    h1: "JavaScript error tracking for real users' browsers",
    lead:
      "Most frontend errors happen on devices you'll never see. @errorlenshq/browser reports them with the clicks, requests and navigation that led there — and source maps turn minified traces back into your code.",
    sections: [
      {
        h2: "Install with npm or a script tag",
        code: [BROWSER_INSTALL, { lang: "js", code: `import * as ErrorLens from "@errorlenshq/browser";\n\nErrorLens.init({\n  dsn: "${dsn}",\n  release: "web@1.8.0",\n  tracesSampleRate: 0.1,\n});` }, { label: "Without a bundler", lang: "html", code: `<script src="https://cdn.jsdelivr.net/npm/@errorlenshq/browser@1/dist/errorlens.min.js" crossorigin="anonymous"></script>\n<script>ErrorLens.init({ dsn: "${dsn}" });</script>` }],
      },
      {
        h2: "What is captured",
        list: [
          "Uncaught errors and unhandled promise rejections",
          "Breadcrumbs: clicks with a readable element path, <code>fetch</code>/XHR calls, navigation and console output",
          "Page-load and route-change transactions with network timing and web vitals (LCP, FCP, CLS)",
          "A <code>traceparent</code> header on same-origin requests, so frontend and backend share one trace",
          "Noise dropped: browser-extension errors, cross-origin “Script error.” and ResizeObserver warnings",
        ],
      },
      {
        h2: "Readable stack traces with source maps",
        body: ['Upload your source maps for each release from CI and ErrorLens shows the original file, line and code instead of <code>main.4f2a.js:1:120554</code>. See <a href="/docs/releases-source-maps/">releases and source maps</a>.'],
      },
      {
        h2: "Reliable delivery",
        body: ["Reports are sent with <code>fetch</code> keepalive and flushed when the tab is hidden, so an error right before the user navigates away still arrives. Requests are CORS “simple” requests: no preflight round trip.", "On flaky mobile networks a report that can't be sent is retried, then kept in the browser and sent again when the connection is back or on the next page load — so errors from users with bad connections aren't lost."],
      },
    ],
    faq: [
      { q: "Is the DSN safe to put in frontend code?", a: "Yes. The DSN's key can only send data to its project; it can't read anything." },
      { q: "Does it work with React, Vue, Angular and Svelte?", a: "Yes, it works with any framework. See the React and Next.js guides for error boundaries and server-side setup." },
      { q: "Will ad blockers block it?", a: "Self-hosting on your own domain avoids most blocklists. On the hosted service, allow errorlens.io in your Content-Security-Policy connect-src." },
    ],
    related: ["/error-tracking/react/", "/error-tracking/nextjs/", "/docs/releases-source-maps/", "/apm/"],
  },
  {
    path: "/error-tracking/react/",
    nav: "React",
    group: "framework",
    title: "React Error Tracking and Monitoring",
    description:
      "Track React errors caught by error boundaries and uncaught render errors, with component stacks, breadcrumbs and source maps. Works with React 18 and 19.",
    eyebrow: "React error tracking",
    h1: "React error tracking, including what your error boundaries catch",
    lead:
      "Error boundaries keep your UI alive but swallow the error. ErrorLens records every render error — caught or not — with the component stack, the user's clicks and readable source.",
    sections: [
      {
        h2: "Initialize before rendering",
        code: [BROWSER_INSTALL, { label: "main.tsx (React 19)", lang: "tsx", code: `import * as ErrorLens from "@errorlenshq/browser";\nimport { createRoot } from "react-dom/client";\n\nErrorLens.init({ dsn: "${dsn}", tracesSampleRate: 0.1 });\n\ncreateRoot(document.getElementById("root")!, {\n  // errors caught by your error boundaries\n  onCaughtError: (error, info) =>\n    ErrorLens.withScope((scope) => {\n      scope.setContext("react", { componentStack: info.componentStack });\n      ErrorLens.captureException(error);\n    }),\n}).render(<App />);` }],
      },
      {
        h2: "React 18: report from your error boundary",
        code: [{ lang: "tsx", code: `class ErrorBoundary extends React.Component {\n  state = { failed: false };\n  static getDerivedStateFromError() { return { failed: true }; }\n  componentDidCatch(error, info) {\n    ErrorLens.withScope((scope) => {\n      scope.setContext("react", { componentStack: info.componentStack });\n      ErrorLens.captureException(error);\n    });\n  }\n  render() { return this.state.failed ? <p>Something went wrong.</p> : this.props.children; }\n}` }],
      },
      {
        h2: "Also captured",
        list: [
          "Uncaught render errors and errors in event handlers and effects",
          "Clicks, route changes and API calls as breadcrumbs",
          "A transaction per route change in single-page apps",
          'Original source lines once you <a href="/docs/releases-source-maps/">upload source maps</a>',
        ],
      },
    ],
    faq: [
      { q: "Does it work with React Router?", a: "Yes. Route changes through the History API are recorded as navigation breadcrumbs and, with tracing on, as navigation transactions." },
      { q: "What about React Native?", a: "This SDK is for browsers. React Native apps can use any SDK that speaks the standard error-envelope protocol." },
    ],
    related: ["/error-tracking/javascript/", "/error-tracking/nextjs/", "/docs/releases-source-maps/", "/apm/"],
  },
  {
    path: "/error-tracking/nextjs/",
    nav: "Next.js",
    group: "framework",
    title: "Next.js Error Tracking and Monitoring",
    description:
      "Track Next.js errors on the server and in the browser: route handlers, server components and client errors in one trace. Setup with instrumentation.ts.",
    eyebrow: "Next.js error tracking",
    h1: "Next.js error tracking on the server and in the browser",
    lead:
      "Next.js errors happen in two places — on the server rendering your routes and in your users' browsers. ErrorLens catches both and links them with one trace.",
    sections: [
      {
        h2: "Server: instrumentation.ts",
        body: ["Initialize the Node.js SDK in Next.js's instrumentation hook, and report request errors from <code>onRequestError</code>:"],
        code: [{ lang: "bash", code: "npm install @errorlenshq/node @errorlenshq/browser" }, { label: "instrumentation.ts", lang: "ts", code: `export async function register() {\n  if (process.env.NEXT_RUNTIME === "nodejs") {\n    const ErrorLens = await import("@errorlenshq/node");\n    ErrorLens.init({\n      dsn: process.env.ERRORLENS_DSN,\n      tracesSampleRate: 0.2,\n      // Next.js caches server fetches by their headers: don't add trace headers to them\n      tracePropagationTargets: [],\n    });\n  }\n}\n\nexport async function onRequestError(error: unknown) {\n  if (process.env.NEXT_RUNTIME === "nodejs") {\n    const ErrorLens = await import("@errorlenshq/node");\n    ErrorLens.captureException(error);\n  }\n}` }],
      },
      {
        h2: "Browser: instrumentation-client.ts",
        code: [{ label: "instrumentation-client.ts", lang: "ts", code: `import * as ErrorLens from "@errorlenshq/browser";\n\nErrorLens.init({\n  dsn: process.env.NEXT_PUBLIC_ERRORLENS_DSN,\n  tracesSampleRate: 0.1,\n});` }, { label: "app/global-error.tsx", lang: "tsx", code: `"use client";\nimport * as ErrorLens from "@errorlenshq/browser";\nimport { useEffect } from "react";\n\nexport default function GlobalError({ error }: { error: Error }) {\n  useEffect(() => { ErrorLens.captureException(error); }, [error]);\n  return <html><body><h2>Something went wrong.</h2></body></html>;\n}` }],
      },
      {
        h2: "What you get",
        list: [
          "Server errors from route handlers, server components and server actions",
          "Client errors with clicks, navigation and fetch breadcrumbs",
          "Page loads and client-side navigations as transactions, with web vitals",
          "Same-origin API calls carry <code>traceparent</code>, so a slow page links to the slow server request",
        ],
        body: ["Dynamic pages are traced by their path. To group them by route instead — one row for <code>/articles/[slug]</code> rather than one per article — rename transactions in <code>beforeSendTransaction</code>, for example <code>event.transaction = event.transaction.replace(/^(GET \\/articles)\\/[^/]+$/, \"$1/[slug]\")</code>."],
      },
    ],
    faq: [
      { q: "Does it support the Edge runtime?", a: "The Node.js SDK runs in the Node.js runtime. For Edge routes, report errors from the browser SDK or send traces with OpenTelemetry." },
      { q: "Pages Router or App Router?", a: "Both. instrumentation.ts works with either; with the Pages Router, report client errors from pages/_error or an error boundary." },
    ],
    related: ["/error-tracking/react/", "/error-tracking/nodejs/", "/error-tracking/javascript/", "/apm/"],
  },

  // ---------------------------------------------------------------- product topics
  {
    path: "/self-hosted-error-tracking/",
    nav: "Self-hosted",
    group: "product",
    title: "Self-Hosted Error Tracking and Monitoring",
    description:
      "Run error tracking, performance monitoring and logs on your own servers: one container plus Postgres, no Kafka or ClickHouse. Free, open source, AGPL-3.0.",
    eyebrow: "Self-hosted",
    h1: "Self-hosted error tracking that runs on one container and Postgres",
    lead:
      "Keep every stack trace, request and log line inside your own network. ErrorLens is a single Go binary with Postgres as its only dependency — small enough for a 1 GB VPS, built to scale out when you need it.",
    sections: [
      {
        h2: "Running in five minutes",
        code: [{ lang: "bash", code: `git clone https://github.com/errorlenshq/errorlens.git && cd errorlens\ncp deploy/.env.example deploy/.env      # set a strong POSTGRES_PASSWORD\ndocker compose -f deploy/docker-compose.yml up -d\n# open http://localhost:8080 and create the first account` }, { label: "Or just the container (with your own Postgres)", lang: "bash", code: `docker run -d -p 8080:8080 \\\n  -e ERRORLENS_DATABASE_URL="postgres://user:pass@db:5432/errorlens?sslmode=disable" \\\n  errorlens/errorlens` }],
        body: ['The image <a href="https://hub.docker.com/r/errorlens/errorlens">errorlens/errorlens</a> is about 40 MB, runs on x86-64 and ARM, has no shell and runs as a non-root user. Kubernetes? A Helm chart is included — see <a href="/docs/kubernetes/">Kubernetes</a>. Full guide: <a href="/docs/self-hosting/">self-hosting</a>.'],
      },
      {
        h2: "Why it's easy to operate",
        list: [
          "<strong>One dependency</strong> — Postgres stores events, issues, traces and logs; no Kafka, Redis, ClickHouse or Zookeeper",
          "<strong>One binary</strong> — the API, dashboard and background workers; run web and worker roles separately to scale",
          "<strong>Cheap retention</strong> — data lives in daily partitions, so expiring old data is an instant drop, not a slow delete",
          "<strong>Upgrades</strong> — migrations run on start; back up Postgres and you've backed up everything",
        ],
      },
      {
        h2: "Built for data you can't send elsewhere",
        list: [
          "Secrets scrubbed before anything is written to disk, with per-project rules for your own fields",
          "Optional AES-256-GCM encryption of stored events with your own key",
          "IP addresses truncated by default; an audit log of every time someone opens an event",
          "Single sign-on with any OpenID Connect provider, 2FA and role-based access",
          "No telemetry: a self-hosted install never contacts us",
        ],
      },
    ],
    faq: [
      { q: "What hardware does it need?", a: "A small server with 1 CPU and 1 GB of memory handles a typical team. Give Postgres more memory and disk as your volume and retention grow." },
      { q: "Is self-hosting really free?", a: "Yes. The server is open source under AGPL-3.0, with no event limits or paid features held back." },
      { q: "Can I move between hosted and self-hosted?", a: "Yes. Both run the same software; point your SDKs at a different DSN to switch." },
    ],
    related: ["/error-tracking/", "/log-management/", "/apm/", "/pricing/"],
  },
  {
    path: "/apm/",
    nav: "Performance (APM)",
    group: "product",
    title: "Open-Source APM and Performance Monitoring",
    description:
      "Open-source application performance monitoring: p50/p95 latency, throughput, failure rate and distributed traces linked to errors and logs. Self-host free.",
    eyebrow: "Application performance monitoring",
    h1: "Open-source APM that links slow requests to their errors and logs",
    lead:
      "See which endpoints are slow, how slow, and why. ErrorLens records transactions and spans from your apps and shows p50 and p95 latency, throughput and failure rate — with every trace joined to the errors and log lines it produced.",
    sections: [
      {
        h2: "What you can see",
        list: [
          "Latency percentiles (p50, p95), requests per minute and failure rate for every endpoint and job",
          "A trace waterfall: database queries, outgoing HTTP calls and your own spans on one timeline",
          "Distributed traces across services via W3C <code>traceparent</code> — browser, Node.js, Python and OpenTelemetry",
          "Errors and logs from the same request shown on the trace",
          "Page loads with network timing and web vitals for frontend performance",
        ],
      },
      {
        h2: "Turn it on with one option",
        code: [{ label: "Python", lang: "python", code: `errorlens.init(dsn="${dsn}", traces_sample_rate=0.2)` }, { label: "Node.js", lang: "js", code: `ErrorLens.init({ dsn: process.env.ERRORLENS_DSN, tracesSampleRate: 0.2 });` }],
        body: ['Already instrumented with OpenTelemetry? Point your exporter at ErrorLens instead — see <a href="/opentelemetry-backend/">OpenTelemetry backend</a>.'],
      },
      {
        h2: "Fast queries on plain Postgres",
        body: ["Hourly and daily rollups answer 7- and 30-day performance views in milliseconds, with log-scale latency histograms for accurate percentiles — without a separate analytics database to run."],
      },
    ],
    faq: [
      { q: "What is APM?", a: "Application performance monitoring measures how fast and reliably your application serves requests — latency, throughput and errors per endpoint — and records traces showing where time is spent." },
      { q: "How much overhead does tracing add?", a: "You choose the share of requests to trace with a sample rate; spans are recorded in memory and sent in the background. Start with 0.1–0.2 in production." },
      { q: "Are metrics supported?", a: "ErrorLens focuses on traces, errors and logs. OTLP metrics are not ingested; percentiles and rates are computed from traces." },
    ],
    related: ["/opentelemetry-backend/", "/error-tracking/", "/log-management/", "/docs/performance/"],
  },
  {
    path: "/opentelemetry-backend/",
    nav: "OpenTelemetry",
    group: "product",
    title: "Open-Source OpenTelemetry Backend for Traces and Logs",
    description:
      "An open-source OpenTelemetry backend: send traces and logs over OTLP/HTTP and get trace waterfalls, latency percentiles, log search and alerts. Self-host free.",
    eyebrow: "OpenTelemetry backend",
    h1: "An open-source backend for your OpenTelemetry traces and logs",
    lead:
      "OpenTelemetry collects and exports telemetry, but it doesn't store or show it — you need a backend. ErrorLens accepts OTLP traces and logs directly and gives you trace views, performance dashboards, log search and alerts.",
    sections: [
      {
        h2: "Point your exporter at ErrorLens",
        body: ["Every OpenTelemetry SDK reads the standard environment variables, so usually no code changes are needed:"],
        code: [{ lang: "bash", code: `export OTEL_EXPORTER_OTLP_ENDPOINT="https://errorlens.io"\nexport OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"\nexport OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer <dsn-public-key>"\nexport OTEL_SERVICE_NAME="checkout-api"` }, { label: "Collector", lang: "yaml", code: `exporters:\n  otlphttp/errorlens:\n    endpoint: https://errorlens.io\n    headers:\n      Authorization: "Bearer <dsn-public-key>"\nservice:\n  pipelines:\n    traces: { exporters: [otlphttp/errorlens] }\n    logs:   { exporters: [otlphttp/errorlens] }` }],
      },
      {
        h2: "What it supports",
        list: [
          "OTLP over HTTP, protobuf or JSON, optionally gzip-compressed",
          "Traces at <code>/v1/traces</code> and logs at <code>/v1/logs</code>",
          "Exceptions recorded on spans become ErrorLens issues with grouping and alerts",
          "Logs linked to their trace; traces from OpenTelemetry and ErrorLens SDKs join into one",
        ],
        body: ['Metrics and OTLP/gRPC are not supported. Details: <a href="/docs/opentelemetry/">OpenTelemetry docs</a>.'],
      },
      {
        h2: "Why use it as your backend",
        list: [
          "Errors, traces and logs in one place instead of three tools",
          "Runs on Postgres — no ClickHouse or Elasticsearch cluster to operate",
          "Secrets scrubbed from span attributes and log bodies before storage",
          "Open source and free to self-host, or hosted at errorlens.io",
        ],
      },
    ],
    faq: [
      { q: "Is OpenTelemetry a backend?", a: "No. OpenTelemetry is a standard and set of SDKs for producing and exporting telemetry. A backend such as ErrorLens receives, stores and visualizes it." },
      { q: "Which languages can send data?", a: "Any language with an OpenTelemetry SDK — Go, Java, .NET, Ruby, PHP, Rust and more — plus the OpenTelemetry Collector." },
      { q: "Do I need the Collector?", a: "No. SDKs can export straight to ErrorLens. A Collector is useful for batching, filtering or fanning out to several backends." },
    ],
    related: ["/apm/", "/log-management/", "/docs/opentelemetry/", "/self-hosted-error-tracking/"],
  },
  {
    path: "/log-management/",
    nav: "Log management",
    group: "product",
    title: "Open-Source Log Management and Search",
    description:
      "Open-source log management: structured logs from your SDK or OpenTelemetry with search, severity filters, live tail and links to traces. Self-host on Postgres.",
    eyebrow: "Log management",
    h1: "Open-source log management linked to your errors and traces",
    lead:
      "Collect structured logs from every service, search them by text or attribute, tail them live, and jump from any line to the request and errors around it.",
    sections: [
      {
        h2: "Send logs",
        code: [{ label: "Python", lang: "python", code: `errorlens.init(dsn="${dsn}", enable_logs=True)\nerrorlens.logger.info("Order {order_id} paid", order_id=42, amount=19.99)` }, { label: "Node.js", lang: "js", code: `ErrorLens.init({ dsn: process.env.ERRORLENS_DSN, enableLogs: true });\nErrorLens.logger.info("Order {orderId} paid", { orderId: 42, amount: 19.99 });` }],
        body: ['Python\'s standard <code>logging</code> records are sent too, and any language can ship logs with <a href="/opentelemetry-backend/">OpenTelemetry</a>.'],
      },
      {
        h2: "Find the line that matters",
        list: [
          "Search message text and attributes; filter by severity, project, environment and release",
          "Placeholders like <code>{order_id}</code> become attributes you can filter on",
          "Live tail while you deploy or reproduce a bug",
          "Every line from a traced request links to its trace, errors and spans",
        ],
      },
      {
        h2: "Safe to log",
        body: ["Attributes named like secrets — passwords, tokens, API keys — are filtered in the SDK, and the server scrubs log bodies again before storing them. Old logs expire by day, so retention costs stay predictable."],
      },
    ],
    faq: [
      { q: "Is this a replacement for a full log pipeline?", a: "For application logs, often yes: collection, search, live tail and trace links in the same tool as your errors. It isn't designed for infrastructure-wide log shipping from every host." },
      { q: "How long are logs kept?", a: "30 days by default on the hosted service; on self-hosted installs you choose the retention." },
    ],
    related: ["/opentelemetry-backend/", "/apm/", "/error-tracking/", "/docs/logs/"],
  },
  {
    path: "/cron-job-monitoring/",
    nav: "Cron monitoring",
    group: "product",
    title: "Cron Job Monitoring — Open Source and Self-Hosted",
    description:
      "Get alerted when a cron job or scheduled task fails, runs too long or doesn't run at all. One HTTP call or one decorator. Open source and self-hostable.",
    eyebrow: "Cron job monitoring",
    h1: "Cron job monitoring that notices when a job doesn't run",
    lead:
      "A crashed job throws an error. A job that never starts throws nothing. ErrorLens expects each run on schedule and alerts you when one is missed, fails or takes too long.",
    sections: [
      {
        h2: "From any script: one HTTP call",
        code: [{ lang: "bash", code: `# at the end of your job\ncurl -fsS "https://errorlens.io/api/<project-id>/monitors/nightly-backup/checkin?key=<dsn-key>&status=ok"` }],
        body: ["Report <code>in_progress</code> at the start and <code>ok</code> or <code>error</code> at the end to measure duration and catch jobs that hang."],
      },
      {
        h2: "From your code: one decorator",
        code: [{ label: "Python", lang: "python", code: `@errorlens.monitor("nightly-backup", schedule="0 2 * * *", max_runtime=30)\ndef backup():\n    ...` }, { label: "Node.js", lang: "js", code: `await ErrorLens.withMonitor("nightly-backup", () => runBackup(), {\n  schedule: { type: "crontab", value: "0 2 * * *" },\n  maxRuntime: 30,\n});` }],
        body: ["The monitor is created from your code the first time it checks in, and its schedule stays in sync with every deploy."],
      },
      {
        h2: "What raises an alert",
        list: [
          "<strong>Missed</strong> — no check-in within the schedule plus a grace margin",
          "<strong>Failed</strong> — the job reported an error, or the process crashed",
          "<strong>Timed out</strong> — started but didn't finish within the maximum runtime",
          "Alerts go to email, Slack, Discord or webhooks, once per state change",
        ],
      },
    ],
    faq: [
      { q: "Which schedules are supported?", a: "Crontab expressions (with a timezone) and simple intervals such as every 10 minutes." },
      { q: "Does it work with Kubernetes CronJobs, Celery beat and GitHub Actions?", a: "Yes. Anything that can make an HTTP request can check in; Python and Node.js jobs can use the SDK helpers." },
    ],
    related: ["/uptime-monitoring/", "/error-tracking/celery/", "/docs/cron-monitors/", "/self-hosted-error-tracking/"],
  },
  {
    path: "/uptime-monitoring/",
    nav: "Uptime monitoring",
    group: "product",
    title: "Uptime Monitoring — Open Source Website Checks",
    description:
      "Check your websites and APIs every 30 seconds to every hour and get alerted when they go down or come back. Open-source uptime monitoring, self-hostable.",
    eyebrow: "Uptime monitoring",
    h1: "Uptime monitoring next to the errors that explain the outage",
    lead:
      "ErrorLens checks your URLs on a schedule and tells you when one stops answering — and because your errors and traces live in the same place, the cause is one click away.",
    sections: [
      {
        h2: "How checks work",
        list: [
          "Check every 30 seconds, 1, 5 or 15 minutes, or every hour",
          "A check passes on any 2xx or 3xx response within the timeout (10 seconds by default)",
          "Timeouts, connection errors, TLS errors and 4xx/5xx responses count as failures",
          "One alert when a monitor goes down and one when it recovers — not one per minute",
          "Response times and up/down history on every monitor",
        ],
      },
      {
        h2: "Set up in the dashboard",
        body: ['Open <strong>Monitors</strong>, add a URL, choose an interval and an alert channel. See <a href="/docs/uptime-monitors/">uptime monitors</a> for details.'],
      },
    ],
    faq: [
      { q: "Can it check internal services?", a: "On a self-hosted install, yes — checks run from your server, so private addresses are reachable when you allow them." },
      { q: "Is uptime monitoring included in the free plan?", a: "Yes. Uptime and cron monitors are part of every plan, including self-hosted." },
    ],
    related: ["/cron-job-monitoring/", "/error-tracking/", "/apm/", "/docs/alerts/"],
  },
  {
    path: "/pricing/",
    nav: "Pricing",
    group: "product",
    title: "Pricing — Free Error Tracking, Hosted or Self-Hosted",
    description:
      "ErrorLens pricing: a free hosted plan with 5,000 events a month and unlimited projects, or self-host for free with no limits. No credit card required.",
    eyebrow: "Pricing",
    h1: "Free error tracking, hosted or on your own servers",
    lead:
      "Start on the hosted free plan in a minute, or run ErrorLens yourself at no cost. Every plan includes errors, performance, logs, cron and uptime monitors.",
    sections: [],
    faq: [
      { q: "Is there really a free plan?", a: "Yes: 5,000 events a month on the hosted service, with unlimited projects and team members and no credit card. Self-hosting is free with no limits." },
      { q: "What counts as an event?", a: "Only error events — exceptions and messages — count toward the monthly quota. Performance traces, log lines, cron check-ins and uptime checks don't." },
      { q: "What happens when I reach the limit?", a: "New error events are paused until the month ends; performance data and logs keep arriving, nothing stored is deleted, and your apps keep running normally." },
      { q: "Are paid plans available?", a: "Plans with higher quotas, longer retention and priority support are coming. Email hello@errorlens.io to be told when they're ready." },
    ],
    related: ["/self-hosted-error-tracking/", "/error-tracking/", "/docs/usage-quotas/", "/docs/quickstart/"],
  },
];

export const landingByPath = (p: string) => LANDINGS.find((l) => l.path === p);
export const FRAMEWORKS = LANDINGS.filter((l) => l.group === "framework");
export const PRODUCT_PAGES = LANDINGS.filter((l) => l.group === "product" && l.path !== "/pricing/");
export const DSN_HTML = DSN;
