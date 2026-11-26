import Link from "next/link";

const FEATURES = [
  { title: "Error tracking", href: "/docs/issues/", text: "Errors are grouped into issues with full stack traces, source context, breadcrumbs, tags and the release they started in. Resolved issues reopen if they come back." },
  { title: "Performance & traces", href: "/docs/performance/", text: "p50 and p95 latency, throughput and failure rate for every endpoint, and a trace view that joins requests, errors and logs." },
  { title: "Logs", href: "/docs/logs/", text: "Structured logs from your SDK or OpenTelemetry, with search, severity filters and live tail, linked to the trace they belong to." },
  { title: "Uptime monitors", href: "/docs/uptime-monitors/", text: "Check your URLs every minute from the server and get alerted the moment one stops answering or returns an error." },
  { title: "Cron monitors", href: "/docs/cron-monitors/", text: "Scheduled jobs report each run with one HTTP call. Missed runs, failures and jobs that run too long raise an alert." },
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
      <section className="hero wrap">
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
          <img src="/screenshots/issues.png" alt="The ErrorLens issues list grouped by error, with event counts and sparkline graphs" />
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
                <Link className="more" href={f.href}>Learn more →</Link>
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
          <img src="/screenshots/issue-detail.png" alt="An issue's stack trace where an API key variable shows as [Filtered]" />
        </div>
      </section>

      <section className="band soft">
        <div className="wrap split">
          <img src="/screenshots/trace.png" alt="A trace showing a request's spans, the error it raised and its log lines on one timeline" />
          <div>
            <h2>Works with the SDKs you already use</h2>
            <p className="sub">Install our SDK, or point any SDK that speaks the standard error-envelope protocol at your DSN. OpenTelemetry traces and logs work too.</p>
            <pre className="term">{`pip install errorlens

import errorlens
errorlens.init(dsn="https://<key>@errorlens.io/1")`}</pre>
            <p><Link href="/docs/python/">Python</Link> · <Link href="/docs/javascript-node/">Node.js</Link> · <Link href="/docs/javascript-browser/">Browser</Link> · <Link href="/docs/other-sdks/">Other languages</Link> · <Link href="/docs/opentelemetry/">OpenTelemetry</Link></p>
          </div>
        </div>
      </section>

      <section className="band" id="pricing">
        <div className="wrap">
          <h2>Pricing</h2>
          <p className="sub">Start on the hosted free plan, or run ErrorLens yourself at no cost.</p>
          <div className="plans">
            <div className="card plan">
              <h3>Free</h3>
              <div className="price">$0</div>
              <p>Hosted at errorlens.io</p>
              <ul className="checks">
                <li>5,000 events per month</li>
                <li>Errors, performance, logs and monitors</li>
                <li>Unlimited projects and team members</li>
                <li>30 days of history by default</li>
              </ul>
              <a href="/signup" className="btn">Create your account</a>
            </div>
            <div className="card plan">
              <h3>Self-hosted</h3>
              <div className="price">$0</div>
              <p>On your own servers</p>
              <ul className="checks">
                <li>Unlimited events — limited only by your hardware</li>
                <li>One container plus Postgres</li>
                <li>Data never leaves your network</li>
                <li>AGPL-3.0 open source</li>
              </ul>
              <Link href="/docs/self-hosting/" className="btn ghost">Self-hosting guide</Link>
            </div>
            <div className="card plan">
              <h3>Team</h3>
              <div className="price">Soon</div>
              <p>Hosted, for larger volumes</p>
              <ul className="checks">
                <li>Higher monthly event quotas</li>
                <li>Longer retention</li>
                <li>Priority support</li>
              </ul>
              <a href="mailto:hello@errorlens.io" className="btn ghost">Tell me when it is ready</a>
            </div>
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
