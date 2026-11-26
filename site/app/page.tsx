import type { Metadata } from "next";
import Link from "next/link";
import { JsonLd } from "@/components/JsonLd";
import { Plans } from "@/components/Plans";
import { FRAMEWORKS } from "@/lib/landing";
import { faqPage, pageMeta, software, webPage } from "@/lib/seo";

const TITLE = "ErrorLens — Open-Source Error Tracking & Monitoring";
const DESCRIPTION =
  "Open-source error tracking, performance monitoring, logs, cron and uptime monitoring. Secrets scrubbed before storage. Free hosted plan or self-host on Postgres.";
export const metadata: Metadata = pageMeta({ title: TITLE, description: DESCRIPTION, path: "/", absoluteTitle: true });

const FAQ = [
  { q: "What is ErrorLens?", a: "ErrorLens is an open-source platform for error tracking, performance monitoring (APM), logs, and cron and uptime monitoring. It captures exceptions from your apps, groups them into issues with full context, and alerts your team." },
  { q: "Is ErrorLens free?", a: "Yes. The hosted free plan includes 5,000 error events a month with unlimited projects and team members, and self-hosting is free with no limits." },
  { q: "Can I self-host ErrorLens?", a: "Yes. ErrorLens runs as a single container with Postgres as its only dependency — no Kafka, Redis or ClickHouse. Docker Compose and a Helm chart are included." },
  { q: "Which languages and frameworks are supported?", a: "Python (Django, Flask, FastAPI, Celery), Node.js (Express, Fastify, Koa, Hapi), browser JavaScript (React, Next.js and any framework) with our own SDKs, any language through OpenTelemetry, and any SDK that speaks the standard error-envelope protocol." },
  { q: "How does ErrorLens protect sensitive data?", a: "Passwords, tokens, API keys, cookies, card numbers and private keys are scrubbed in the SDK and again on the server before anything is stored. IP addresses are truncated, stored events can be encrypted with your own key, and every event view is audited." },
  { q: "Does ErrorLens support OpenTelemetry?", a: "Yes. Send traces and logs over OTLP/HTTP (protobuf or JSON) and ErrorLens shows trace waterfalls, latency percentiles, log search and errors from spans." },
];

const FEATURES = [
  { title: "Error tracking", href: "/error-tracking/", text: "Errors are grouped into issues with full stack traces, source context, breadcrumbs, tags and the release they started in. Resolved issues reopen if they come back." },
  { title: "Performance & traces", href: "/apm/", text: "p50 and p95 latency, throughput and failure rate for every endpoint, and a trace view that joins requests, errors and logs." },
  { title: "Logs", href: "/log-management/", text: "Structured logs from your SDK or OpenTelemetry, with search, severity filters and live tail, linked to the trace they belong to." },
  { title: "Uptime monitors", href: "/uptime-monitoring/", text: "Check your URLs every minute from the server and get alerted the moment one stops answering or returns an error." },
  { title: "Cron monitors", href: "/cron-job-monitoring/", text: "Scheduled jobs report each run with one HTTP call. Missed runs, failures and jobs that run too long raise an alert." },
  { title: "Alerts", href: "/docs/alerts/", text: "New issues, regressions and spikes go to email, Slack, Discord or signed webhooks, with cooldowns so you are not flooded." },
  { title: "Releases & source maps", href: "/docs/releases-source-maps/", text: "See which release introduced an error. Upload JavaScript source maps from CI and get readable stack traces." },
  { title: "Teams", href: "/docs/members-roles/", text: "Organizations, projects, owner / admin / member / viewer roles, invitations, API tokens, two-factor login and single sign-on." },
];

const PRIVACY = [
  "Passwords, tokens, API keys, cookies, card numbers, JWTs and private keys are removed before an event is written to disk",
  "IP addresses are truncated by default, or dropped entirely",
  "Per-project rules for your own sensitive fields",
  "Optional encryption of every stored event with your own key",
  "An audit log of every time someone opens an event",
  "No tracking, no telemetry, no selling or sharing of your data",
];

export default function Home() {
  return (
    <main>
      <JsonLd graph={[webPage({ title: TITLE, description: DESCRIPTION, path: "/" }), software(), faqPage(FAQ)]} />
      <section className="hero wrap">
        <p className="eyebrow">Open-source error tracking · APM · logs</p>
        <h1>
          See every error in your app.<br />
          <em>Keep every secret out of it.</em>
        </h1>
        <p className="lead">
          ErrorLens is open-source error tracking, performance monitoring, logs and uptime checks. Sensitive data is
          scrubbed before anything is stored — on our hosted service or on your own servers.
        </p>
        <div className="ctas">
          <a href="/signup" className="btn big">Start free</a>
          <Link href="/docs/quickstart/" className="btn ghost big">Read the quick start</Link>
        </div>
        <p className="note">Free plan: 5,000 events a month, no card required. Self-hosting is free forever.</p>
        <div className="hero-shot">
          <img src="/screenshots/issues.webp" width={1360} height={860} fetchPriority="high" alt="The ErrorLens issues list: errors grouped into issues with event counts, affected users and sparkline graphs" />
        </div>
      </section>

      <section className="band soft" id="features">
        <div className="wrap">
          <h2>Everything you need to find the root cause</h2>
          <p className="sub">One dashboard for what broke, how slow it was, what was logged, and whether your jobs and URLs are healthy.</p>
          <div className="grid">
            {FEATURES.map((f) => (
              <div className="card" key={f.title}>
                <h3>{f.title}</h3>
                <p>{f.text}</p>
                <Link className="more" href={f.href}>{f.title} →</Link>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="band" id="privacy">
        <div className="wrap split">
          <div>
            <h2>Private by design</h2>
            <p className="sub">Error reports are full of things nobody should read: request headers, form fields, connection strings. ErrorLens removes them on the way in.</p>
            <ul className="checks">
              {PRIVACY.map((p) => <li key={p}>{p}</li>)}
            </ul>
            <p><Link href="/docs/data-scrubbing/">How data scrubbing works →</Link></p>
          </div>
          <img src="/screenshots/issue-detail.webp" width={1360} height={1500} loading="lazy" alt="An issue's stack trace in ErrorLens where an API key variable shows as [Filtered]" />
        </div>
      </section>

      <section className="band soft">
        <div className="wrap split">
          <img src="/screenshots/trace.webp" width={1360} height={860} loading="lazy" alt="A distributed trace in ErrorLens showing a request's spans, the error it raised and its log lines on one timeline" />
          <div>
            <h2>Works with the SDKs you already use</h2>
            <p className="sub">Install our SDK, or point any SDK that speaks the standard error-envelope protocol at your DSN. OpenTelemetry traces and logs work too.</p>
            <pre className="term">{`pip install errorlens

import errorlens
errorlens.init(dsn="https://<key>@errorlens.io/1")`}</pre>
            <p><Link href="/error-tracking/python/">Python</Link> · <Link href="/error-tracking/nodejs/">Node.js</Link> · <Link href="/error-tracking/javascript/">Browser JavaScript</Link> · <Link href="/docs/other-sdks/">Other languages</Link> · <Link href="/opentelemetry-backend/">OpenTelemetry</Link></p>
          </div>
        </div>
      </section>

      <section className="band" id="pricing">
        <div className="wrap">
          <h2>Pricing</h2>
          <p className="sub">Start on the hosted free plan, or run ErrorLens yourself at no cost.</p>
          <Plans />
        </div>
      </section>

      <section className="band soft" id="frameworks">
        <div className="wrap">
          <h2>Error tracking for your stack</h2>
          <p className="sub">Step-by-step setup guides for the frameworks you use, each in a few lines of code.</p>
          <div className="pills">
            {FRAMEWORKS.map((f) => <Link key={f.path} href={f.path} className="pill">{f.nav} error tracking</Link>)}
            <Link href="/opentelemetry-backend/" className="pill">OpenTelemetry</Link>
            <Link href="/docs/other-sdks/" className="pill">Other languages</Link>
          </div>
        </div>
      </section>

      <section className="band" id="faq">
        <div className="wrap narrow">
          <h2>Frequently asked questions</h2>
          <div className="faq">
            {FAQ.map((f) => (
              <details key={f.q}>
                <summary><h3>{f.q}</h3></summary>
                <p>{f.a}</p>
              </details>
            ))}
          </div>
        </div>
      </section>

      <section className="band soft">
        <div className="wrap" style={{ textAlign: "center" }}>
          <h2>Your dashboard, at your own address</h2>
          <p className="sub" style={{ margin: "0 auto 28px" }}>
            Every organization gets its own subdomain — <code>acme.errorlens.io</code> — and every project its own
            path: <code>acme.errorlens.io/checkout-api/</code>.
          </p>
          <a href="/signup" className="btn big">Sign up free</a>
        </div>
      </section>
    </main>
  );
}
