import { useEffect, useState } from "react";
import { api, EventDetail, Issue, IssueStatus, Payload, TagSummary } from "../api";
import { linkProps } from "../router";
import { Bars, formatCount, Level, TimeAgo } from "../ui";

export function IssueDetailPage({ org, id, role }: { org: string; id: number; role: string }) {
  const [issue, setIssue] = useState<Issue | null>(null);
  const [tags, setTags] = useState<TagSummary[]>([]);
  const [eventId, setEventId] = useState("latest");
  const [event, setEvent] = useState<EventDetail | null>(null);
  const [error, setError] = useState("");
  const canEdit = role !== "viewer";

  useEffect(() => {
    setIssue(null);
    setEventId("latest");
    api.issue(org, id).then((d) => { setIssue(d.issue); setTags(d.tags); }).catch((e) => setError(e.message));
  }, [org, id]);

  useEffect(() => {
    setEvent(null);
    api.event(org, id, eventId).then(setEvent).catch((e) => setError(e.message));
  }, [org, id, eventId]);

  const setStatus = async (status: IssueStatus) => {
    try {
      await api.setIssueStatus(org, id, status);
      setIssue((i) => (i ? { ...i, status } : i));
    } catch (e) {
      setError((e as Error).message);
    }
  };

  if (error && !issue) return <p className="error">{error}</p>;
  if (!issue) return <p className="muted">Loading…</p>;

  return (
    <>
      <a className="back muted" {...linkProps(`/${org}/issues`)}>← Issues</a>
      <div className="issue-header">
        <div className="issue-header-main">
          <h1 className="issue-h1"><Level level={issue.level} /> {issue.title}</h1>
          <div className="muted mono">{issue.culprit}</div>
        </div>
        {canEdit && (
          <div className="actions">
            {issue.status === "unresolved" ? (
              <>
                <button onClick={() => setStatus("resolved")}>Resolve</button>
                <button className="ghost" onClick={() => setStatus("ignored")}>Ignore</button>
              </>
            ) : (
              <button className="ghost" onClick={() => setStatus("unresolved")}>
                {issue.status === "resolved" ? "Unresolve" : "Stop ignoring"}
              </button>
            )}
          </div>
        )}
      </div>
      {error && <p className="error">{error}</p>}

      <div className="stats">
        <Stat label="Events" value={formatCount(issue.count)} />
        <Stat label="Status" value={issue.status} cap />
        <Stat label="First seen" value={<TimeAgo iso={issue.firstSeen} />} sub={issue.firstRelease} />
        <Stat label="Last seen" value={<TimeAgo iso={issue.lastSeen} />} sub={issue.lastRelease} />
        <div className="stat chart">
          <span className="muted">Last 14 days</span>
          <Bars data={issue.trend} height={40} label="Events per day, last 14 days" />
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-main">
          <div className="event-nav card">
            {event ? (
              <>
                <div>
                  <strong>Event</strong> <span className="mono muted">{event.id.slice(0, 12)}</span>
                  <span className="muted"> · <TimeAgo iso={event.timestamp} /></span>
                  {event.tags.trace_id && <> · <a className="link" {...linkProps(`/${org}/traces/${event.tags.trace_id}`)}>View trace</a></>}
                  {event.scrubbed > 0 && (
                    <span className="badge ok" title="Values removed before storage">🔒 {event.scrubbed} values filtered</span>
                  )}
                </div>
                <div className="actions">
                  <button className="ghost" disabled={!event.prev} onClick={() => event.prev && setEventId(event.prev)}>← Older</button>
                  <button className="ghost" disabled={!event.next} onClick={() => event.next && setEventId(event.next)}>Newer →</button>
                  <button className="ghost" disabled={!event.next} onClick={() => setEventId("latest")}>Latest</button>
                </div>
              </>
            ) : (
              <span className="muted">Loading event…</span>
            )}
          </div>
          {event && <EventBody payload={event.payload} />}
        </div>

        <aside className="detail-side">
          <section className="card">
            <h2>Tags</h2>
            {tags.length === 0 && <p className="muted">No tags.</p>}
            {tags.map((t) => (
              <div key={t.key} className="tag-summary">
                <div className="tag-key">{t.key}</div>
                {t.values.map((v) => (
                  <div key={v.value} className="tag-bar" title={`${v.value}: ${v.count}`}>
                    <span className="fill" style={{ width: `${(v.count / t.total) * 100}%` }} />
                    <span className="tag-val">{v.value}</span>
                    <span className="tag-pct">{Math.round((v.count / t.total) * 100)}%</span>
                  </div>
                ))}
              </div>
            ))}
          </section>
        </aside>
      </div>
    </>
  );
}

function Stat({ label, value, sub, cap }: { label: string; value: React.ReactNode; sub?: string; cap?: boolean }) {
  return (
    <div className="stat">
      <span className="muted">{label}</span>
      <strong className={cap ? "cap" : undefined}>{value}</strong>
      {sub && <span className="muted mono small">{sub}</span>}
    </div>
  );
}

// --- event body -------------------------------------------------------------

type Frame = Payload;
type ExceptionValue = { type?: string; value?: string; module?: string; stacktrace?: { frames?: Frame[] }; mechanism?: Payload };

function exceptionsOf(p: Payload): ExceptionValue[] {
  const ex = p.exception;
  const vals: ExceptionValue[] = Array.isArray(ex) ? ex : ex?.values ?? [];
  if (vals.length) return [...vals].reverse(); // outermost first
  const st = p.stacktrace ?? p.threads?.values?.find((t: Payload) => t.stacktrace)?.stacktrace;
  return st ? [{ stacktrace: st }] : [];
}

function EventBody({ payload }: { payload: Payload }) {
  const exceptions = exceptionsOf(payload);
  const message = typeof payload.message === "string" ? payload.message : payload.logentry?.formatted ?? payload.message?.formatted ?? payload.logentry?.message;
  const crumbs: Payload[] = payload.breadcrumbs?.values ?? [];
  return (
    <>
      {message && (
        <section className="card">
          <h2>Message</h2>
          <pre className="message">{message}</pre>
        </section>
      )}
      {exceptions.map((ex, i) => (
        <section className="card" key={i}>
          <h2>{i === 0 ? "Exception" : "Caused by"}</h2>
          {(ex.type || ex.value) && (
            <p className="exc-title"><strong>{ex.type}</strong>{ex.value ? `: ${ex.value}` : ""}</p>
          )}
          {ex.mechanism && ex.mechanism.handled === false && <span className="badge warn">Unhandled</span>}
          <StackTrace frames={ex.stacktrace?.frames ?? []} />
        </section>
      ))}
      {crumbs.length > 0 && <Breadcrumbs crumbs={crumbs} />}
      {payload.request && <RequestSection req={payload.request} />}
      <KeyValueSection title="User" data={payload.user} />
      <KeyValueSection title="Tags" data={payload.tags && !Array.isArray(payload.tags) ? payload.tags : Object.fromEntries(payload.tags ?? [])} />
      {payload.contexts && Object.entries(payload.contexts as Record<string, Payload>).map(([k, v]) => (
        <KeyValueSection key={k} title={`Context: ${k}`} data={v} />
      ))}
      <KeyValueSection title="Additional data" data={payload.extra} />
      <KeyValueSection title="SDK" data={payload.sdk ? { name: payload.sdk.name, version: payload.sdk.version } : undefined} />
    </>
  );
}

function StackTrace({ frames }: { frames: Frame[] }) {
  const ordered = [...frames].reverse(); // most recent call first
  const hasInApp = ordered.some((f) => f.in_app);
  const [showAll, setShowAll] = useState(!hasInApp);
  const visible = showAll ? ordered : ordered.filter((f) => f.in_app);
  if (!frames.length) return <p className="muted">No stack trace.</p>;
  return (
    <div className="stack">
      {hasInApp && (
        <label className="toggle">
          <input type="checkbox" checked={showAll} onChange={(e) => setShowAll(e.target.checked)} /> Show library frames ({frames.length})
        </label>
      )}
      {visible.map((f, i) => <FrameRow key={i} frame={f} open={i === 0} />)}
    </div>
  );
}

function FrameRow({ frame: f, open }: { frame: Frame; open: boolean }) {
  const [expanded, setExpanded] = useState(open);
  const hasContext = Boolean(f.context_line);
  const file = f.filename || f.abs_path || f.module || "<unknown>";
  return (
    <div className={`frame ${f.in_app ? "in-app" : ""}`}>
      <button className="frame-head" onClick={() => hasContext && setExpanded(!expanded)} aria-expanded={expanded} disabled={!hasContext}>
        <span className="mono">
          <strong>{file}</strong>
          {f.function && <> in <strong>{f.function}</strong></>}
          {f.lineno != null && <span className="muted"> at line {f.lineno}{f.colno != null ? `:${f.colno}` : ""}</span>}
        </span>
        {f.in_app && <span className="badge">In app</span>}
      </button>
      {expanded && hasContext && (
        <div className="frame-body">
          <ol className="context mono" start={(f.lineno ?? 1) - (f.pre_context?.length ?? 0)}>
            {(f.pre_context ?? []).map((l: string, i: number) => <li key={`a${i}`}>{l || " "}</li>)}
            <li className="current">{f.context_line}</li>
            {(f.post_context ?? []).map((l: string, i: number) => <li key={`b${i}`}>{l || " "}</li>)}
          </ol>
          {f.vars && Object.keys(f.vars).length > 0 && (
            <table className="kv">
              <tbody>
                {Object.entries(f.vars).map(([k, v]) => (
                  <tr key={k}><th className="mono">{k}</th><td className="mono">{fmt(v)}</td></tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}

function Breadcrumbs({ crumbs }: { crumbs: Payload[] }) {
  const [all, setAll] = useState(false);
  const shown = all ? crumbs : crumbs.slice(-10);
  return (
    <section className="card">
      <h2>Breadcrumbs</h2>
      <table className="crumbs">
        <thead><tr><th>Type</th><th>Category</th><th>Description</th><th>Level</th><th>Time</th></tr></thead>
        <tbody>
          {[...shown].reverse().map((c, i) => (
            <tr key={i}>
              <td>{c.type ?? "default"}</td>
              <td className="mono">{c.category ?? ""}</td>
              <td className="mono crumb-msg">{c.message ?? (c.data ? fmt(c.data) : "")}</td>
              <td>{c.level ?? "info"}</td>
              <td className="muted">{crumbTime(c.timestamp)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {crumbs.length > 10 && (
        <button className="ghost small-btn" onClick={() => setAll(!all)}>{all ? "Show last 10" : `Show all ${crumbs.length}`}</button>
      )}
    </section>
  );
}

function crumbTime(ts: unknown): string {
  const d = typeof ts === "number" ? new Date(ts * 1000) : typeof ts === "string" ? new Date(ts) : null;
  return d && !isNaN(d.getTime()) ? d.toLocaleTimeString() : "";
}

function RequestSection({ req }: { req: Payload }) {
  const headers: [string, string][] = Array.isArray(req.headers) ? req.headers : Object.entries(req.headers ?? {});
  return (
    <section className="card">
      <h2>Request</h2>
      <p className="mono"><strong>{req.method ?? "GET"}</strong> {req.url}</p>
      <table className="kv">
        <tbody>
          {req.query_string && <tr><th>Query</th><td className="mono">{fmt(req.query_string)}</td></tr>}
          {req.data && <tr><th>Body</th><td className="mono">{fmt(req.data)}</td></tr>}
          {req.cookies && <tr><th>Cookies</th><td className="mono">{fmt(req.cookies)}</td></tr>}
          {headers.map(([k, v]) => <tr key={k}><th className="mono">{k}</th><td className="mono">{fmt(v)}</td></tr>)}
        </tbody>
      </table>
    </section>
  );
}

function KeyValueSection({ title, data }: { title: string; data?: Payload }) {
  if (!data || typeof data !== "object" || Object.keys(data).length === 0) return null;
  return (
    <section className="card">
      <h2>{title}</h2>
      <table className="kv">
        <tbody>
          {Object.entries(data).map(([k, v]) => (
            <tr key={k}><th className="mono">{k}</th><td className="mono">{fmt(v)}</td></tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function fmt(v: unknown): string {
  if (v === null || v === undefined) return "null";
  if (typeof v === "string") return v;
  return JSON.stringify(v, null, 2);
}
