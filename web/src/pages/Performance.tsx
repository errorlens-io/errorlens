import { useEffect, useRef, useState } from "react";
import { usePaged } from "../paged";
import { api, Project, TraceView, TxRow, TxSummary } from "../api";
import { linkProps, setQuery, base, PAGE_SIZE } from "../router";
import { Empty, formatCount, TimeAgo } from "../ui";

const offset = (d: number) => (d < 0 ? `−${ms(-d)}` : `+${ms(d)}`);
export const ms = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(2)}s` : `${n.toFixed(n < 10 ? 1 : 0)}ms`);
const PERIODS = [["1h", "1 hour"], ["24h", "24 hours"], ["7d", "7 days"], ["30d", "30 days"]] as const;

export function PerformancePage({ org, search }: { org: string; search: string }) {
  const p = new URLSearchParams(search);
  const period = p.get("period") || "24h";
  const project = p.get("project") || "";
  const name = p.get("name") || "";
  const [projects, setProjects] = useState<Project[]>([]);
  const [rows, setRows] = useState<TxSummary[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { api.projects(org).then(setProjects).catch(() => {}); }, [org]);
  useEffect(() => {
    if (name) return;
    let current = true; // a newer period/project makes this response stale
    setRows(null);
    setError("");
    api.performance(org, { period, project }).then((r) => current && setRows(r)).catch((e) => current && setError(e.message));
    return () => { current = false; };
  }, [org, period, project, name]);

  const filters = (
    <div className="filters">
      <div className="segmented">
        {PERIODS.map(([v, l]) => <button key={v} className={period === v ? "on" : ""} onClick={() => setQuery({ period: v === "24h" ? "" : v })}>{l}</button>)}
      </div>
      <select aria-label="Project" value={project} onChange={(e) => setQuery({ project: e.target.value })}>
        <option value="">All projects</option>
        {projects.map((x) => <option key={x.id} value={x.slug}>{x.name}</option>)}
      </select>
    </div>
  );

  if (name) return <TransactionList org={org} name={name} period={period} project={project} filters={filters} />;

  return (
    <>
      <div className="page-head"><h1>Performance</h1></div>
      {filters}
      {error && <p className="error">{error}</p>}
      {rows === null ? <p className="muted">Loading…</p> : rows.length === 0 ? (
        <Empty title="No transactions yet">Enable tracing in your SDK (e.g. a traces sample rate), or send OpenTelemetry traces to <code className="mono">/v1/traces</code>.</Empty>
      ) : (
        <div className="card flush">
          <table className="kv perf-table">
            <thead><tr><th>Transaction</th><th>Project</th><th className="num-col">Throughput</th><th className="num-col">p50</th><th className="num-col">p95</th><th className="num-col">Failures</th></tr></thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.project + r.name}>
                  <td><a className="link mono" {...linkProps(`${base(org)}/performance?name=${encodeURIComponent(r.name)}&project=${r.project}${period !== "24h" ? `&period=${period}` : ""}`)}>{r.name}</a> <span className="muted small">{r.op}</span></td>
                  <td>{r.project}</td>
                  <td className="num-col">{formatCount(r.count)} <span className="muted small">({r.perMinute >= 1 ? `${r.perMinute.toFixed(1)}/min` : `${(r.perMinute * 60).toFixed(1)}/hr`})</span></td>
                  <td className="num-col">{ms(r.p50)}</td>
                  <td className={`num-col ${r.p95 > 1000 ? "warn-text" : ""}`}>{ms(r.p95)}</td>
                  <td className={`num-col ${r.failureRate > 0.05 ? "warn-text" : ""}`}>{(r.failureRate * 100).toFixed(1)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function TransactionList({ org, name, period, project, filters }: { org: string; name: string; period: string; project: string; filters: React.ReactNode }) {
  const offset = useRef(0);
  const { items: rows, error, pager } = usePaged<TxRow>(async (after) => {
    offset.current = after ? offset.current + PAGE_SIZE : 0;
    return api.transactions(org, { name, period, project, offset: offset.current || undefined });
  }, [org, name, period, project]);
  const max = Math.max(1, ...(rows ?? []).map((r) => r.durationMs));
  return (
    <>
      <a className="back muted" {...linkProps(`${base(org)}/performance`)}>← Performance</a>
      <div className="page-head"><h1 className="mono">{name}</h1></div>
      {filters}
      <p className="muted small">Slowest first. Open one to see where the time went.</p>
      {error && <p className="error">{error}</p>}
      <div className="card flush">
        <table className="kv">
          <tbody>
            {rows?.map((r) => (
              <tr key={r.id}>
                <td className="nowrap"><TimeAgo iso={r.start} /></td>
                <td style={{ width: "50%" }}><div className="dur-bar"><span style={{ width: `${(r.durationMs / max) * 100}%` }} className={r.status !== "ok" ? "bad" : ""} /></div></td>
                <td className="num-col">{ms(r.durationMs)}</td>
                <td>{r.status !== "ok" && <span className="badge warn">{r.status}</span>}</td>
                <td><a className="link" {...linkProps(`${base(org)}/traces/${r.traceId}`)}>View trace</a></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {pager}
    </>
  );
}

export function TracePage({ org, traceId }: { org: string; traceId: string }) {
  const [t, setT] = useState<TraceView | null>(null);
  const [error, setError] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  useEffect(() => { api.trace(org, traceId).then(setT).catch((e) => setError(e.message)); }, [org, traceId]);
  if (error) return <p className="error">{error}</p>;
  if (!t) return <p className="muted">Loading…</p>;

  const starts = t.transactions.map((x) => new Date(x.start).getTime());
  // Time the trace from its transactions; errors/logs are shown relative to that.
  const t0 = starts.length ? Math.min(...starts) : Math.min(...t.errors.map((e) => new Date(e.timestamp).getTime()), Date.now());
  const end = Math.max(...t.transactions.map((x) => new Date(x.start).getTime() + x.durationMs), t0 + 1);
  const total = end - t0;
  const pos = (startMs: number, dur: number) => ({ left: `${(startMs / total) * 100}%`, width: `${Math.max(0.3, (dur / total) * 100)}%` });

  return (
    <>
      <div className="page-head"><h1>Trace <span className="mono muted">{traceId.slice(0, 16)}…</span></h1></div>
      <p className="muted">{t.transactions.length} transactions · {t.errors.length} errors · {t.logs.length} logs · {ms(total)} total</p>
      {t.errors.length > 0 && (
        <section className="card">
          <h2>Errors in this trace</h2>
          <ul className="rows">
            {t.errors.map((e) => (
              <li key={e.eventId} className="row-item">
                <span className={`level level-${e.level}`} />
                <a className="link" {...linkProps(`${base(org)}/issues/${e.issueId}`)}>{e.title}</a>
                <span className="muted small">{offset(new Date(e.timestamp).getTime() - t0)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      <section className="card">
        <h2>Waterfall</h2>
        <div className="waterfall">
          {t.transactions.map((x) => {
            const off = new Date(x.start).getTime() - t0;
            return (
              <div key={x.id}>
                <button className="wf-row tx" onClick={() => setOpen(open === x.id ? null : x.id)}>
                  <span className="wf-label mono"><strong>{x.name}</strong> <span className="muted">{x.project}</span></span>
                  <span className="wf-track"><span className={`wf-bar ${x.status !== "ok" ? "bad" : ""}`} style={pos(off, x.durationMs)} /></span>
                  <span className="wf-dur">{ms(x.durationMs)}</span>
                </button>
                {x.spans.map((s) => (
                  <div key={x.id + s.spanId} className="wf-row span" title={s.description}>
                    <span className="wf-label mono"><span className="muted">{s.op}</span> {s.description}</span>
                    <span className="wf-track"><span className={`wf-bar span-bar ${s.status && s.status !== "ok" ? "bad" : ""}`} style={pos(off + s.startMs, s.durationMs)} /></span>
                    <span className="wf-dur">{ms(s.durationMs)}</span>
                  </div>
                ))}
                {open === x.id && Object.keys(x.tags).length > 0 && (
                  <table className="kv"><tbody>{Object.entries(x.tags).map(([k, v]) => <tr key={k}><th className="mono">{k}</th><td className="mono">{v}</td></tr>)}</tbody></table>
                )}
              </div>
            );
          })}
        </div>
      </section>
      {t.logs.length > 0 && (
        <section className="card">
          <h2>Logs</h2>
          <table className="log-table">
            <tbody>
              {t.logs.map((l) => (
                <tr key={l.id} className="log-row">
                  <td className="muted nowrap">{offset(new Date(l.timestamp).getTime() - t0)}</td>
                  <td><span className={`sev sev-${l.severity}`}>{l.severity}</span></td>
                  <td className="mono log-body">{l.body}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </>
  );
}
