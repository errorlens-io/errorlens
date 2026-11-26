import Link from "next/link";
import { JsonLd } from "@/components/JsonLd";
import { Plans } from "@/components/Plans";
import { LANDINGS, landingByPath, type Landing } from "@/lib/landing";
import { breadcrumbs, faqPage, software, webPage } from "@/lib/seo";

const DOC_TITLES: Record<string, string> = {
  "/docs/releases-source-maps/": "Releases and source maps",
  "/docs/performance/": "Performance docs",
  "/docs/opentelemetry/": "OpenTelemetry docs",
  "/docs/logs/": "Logs docs",
  "/docs/cron-monitors/": "Cron monitors docs",
  "/docs/alerts/": "Alerts docs",
  "/docs/usage-quotas/": "Usage and quotas",
  "/docs/quickstart/": "Quick start",
};

// A real screenshot per page: richer results in image search and a clearer page.
const SHOTS: Record<string, [string, number, number, string]> = {
  "/error-tracking/": ["issue-detail.webp", 1360, 1500, "An ErrorLens issue with its stack trace, source context, tags and breadcrumbs"],
  "/self-hosted-error-tracking/": ["issues.webp", 1360, 860, "The ErrorLens issues list running on a self-hosted server"],
  "/apm/": ["performance-endpoint.webp", 1360, 1100, "ErrorLens performance view of one endpoint with p50 and p95 latency, throughput and failure rate"],
  "/opentelemetry-backend/": ["trace.webp", 1360, 860, "A trace from OpenTelemetry in ErrorLens with spans, errors and logs on one timeline"],
  "/log-management/": ["logs.webp", 1360, 860, "ErrorLens log search with severity filters and links to traces"],
  "/cron-job-monitoring/": ["monitor-cron.webp", 1360, 860, "An ErrorLens cron monitor showing check-ins, durations and missed runs"],
  "/uptime-monitoring/": ["monitor-uptime.webp", 1360, 935, "An ErrorLens uptime monitor with response times and up/down history"],
};

function trail(l: Landing) {
  const t = [{ name: "Home", path: "/" }];
  if (l.group === "framework") t.push({ name: "Error tracking", path: "/error-tracking/" });
  t.push({ name: l.nav, path: l.path });
  return t;
}

export function LandingPage({ landing: l }: { landing: Landing }) {
  const crumbs = trail(l);
  const graph = [webPage({ title: l.title, description: l.description, path: l.path }), breadcrumbs(crumbs), software()];
  if (l.faq.length) graph.push(faqPage(l.faq));
  return (
    <main className="landing">
      <JsonLd graph={graph} />
      <div className="wrap">
        <nav className="crumbs" aria-label="Breadcrumb">
          {crumbs.map((c, i) => (
            <span key={c.path}>{i < crumbs.length - 1 ? <><Link href={c.path}>{c.name}</Link> / </> : <span aria-current="page">{c.name}</span>}</span>
          ))}
        </nav>
      </div>
      <header className="landing-hero wrap">
        <p className="eyebrow">{l.eyebrow}</p>
        <h1>{l.h1}</h1>
        <p className="lead">{l.lead}</p>
        <div className="ctas">
          <a href="/signup" className="btn big">Start free</a>
          <Link href="/self-hosted-error-tracking/" className="btn ghost big">Self-host it</Link>
        </div>
      </header>

      {SHOTS[l.path] && (
        <div className="wrap landing-shot">
          <img src={`/screenshots/${SHOTS[l.path][0]}`} width={SHOTS[l.path][1]} height={SHOTS[l.path][2]} alt={SHOTS[l.path][3]} fetchPriority="high" />
        </div>
      )}

      {l.path === "/pricing/" && (
        <section className="band"><div className="wrap"><Plans /></div></section>
      )}

      {l.sections.map((s, i) => (
        <section className={`band${i % 2 ? "" : " soft"}`} key={s.h2}>
          <div className="wrap narrow">
            <h2>{s.h2}</h2>
            {s.body?.map((p, j) => <p key={j} className="body" dangerouslySetInnerHTML={{ __html: p }} />)}
            {s.list && <ul className="checks">{s.list.map((li, j) => <li key={j} dangerouslySetInnerHTML={{ __html: li }} />)}</ul>}
            {s.code?.map((c, j) => (
              <figure className="codeblock" key={j}>
                {c.label && <figcaption>{c.label}</figcaption>}
                <pre className="term"><code className={`language-${c.lang}`}>{c.code}</code></pre>
              </figure>
            ))}
          </div>
        </section>
      ))}

      {l.faq.length > 0 && (
        <section className="band soft" id="faq">
          <div className="wrap narrow">
            <h2>Frequently asked questions</h2>
            <div className="faq">
              {l.faq.map((f) => (
                <details key={f.q}>
                  <summary><h3>{f.q}</h3></summary>
                  <p>{f.a}</p>
                </details>
              ))}
            </div>
          </div>
        </section>
      )}

      <section className="band">
        <div className="wrap narrow">
          <h2>Keep reading</h2>
          <div className="grid">
            {l.related.map((href) => {
              const other = landingByPath(href);
              return (
                <Link key={href} href={href} className="card related">
                  <h3>{other ? other.h1.split(" — ")[0] : DOC_TITLES[href] || href}</h3>
                  {other && <p>{other.description}</p>}
                </Link>
              );
            })}
          </div>
        </div>
      </section>

      <section className="band soft cta-band">
        <div className="wrap" style={{ textAlign: "center" }}>
          <h2>Start tracking errors in minutes</h2>
          <p className="sub" style={{ margin: "0 auto 28px" }}>Free hosted plan with 5,000 errors a month, or self-host with no limits. No credit card.</p>
          <div className="ctas" style={{ justifyContent: "center" }}>
            <a href="/signup" className="btn big">Create a free account</a>
            <Link href="/docs/quickstart/" className="btn ghost big">Read the quick start</Link>
          </div>
        </div>
      </section>
    </main>
  );
}

export const ALL_LANDINGS = LANDINGS;
