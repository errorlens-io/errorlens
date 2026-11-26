import { Fragment, useEffect, useRef, useState } from "react";
import { api, LogLine, Project } from "../api";
import { linkProps, setQuery, base } from "../router";

const SEVERITIES = ["trace", "debug", "info", "warn", "error", "fatal"];

export function LogsPage({ org, search }: { org: string; search: string }) {
  const params = new URLSearchParams(search);
  const project = params.get("project") || "";
  const severity = params.get("severity") || "";
  const query = params.get("query") || "";
  const trace = params.get("trace") || "";
  const [projects, setProjects] = useState<Project[]>([]);
  const [lines, setLines] = useState<LogLine[] | null>(null);
  const [text, setText] = useState(query);
  const [live, setLive] = useState(false);
  const [open, setOpen] = useState<number | null>(null);
  const [error, setError] = useState("");
  const newest = useRef<string>("");

  useEffect(() => { api.projects(org).then(setProjects).catch(() => {}); }, [org]);

  useEffect(() => {
    let current = true; // a newer filter makes this response stale
    setLines(null);
    setError("");
    api.logs(org, { project, severity, query, trace })
      .then((l) => { if (current) { setLines(l); newest.current = l[0]?.timestamp ?? ""; } })
      .catch((e) => current && setError(e.message));
    return () => { current = false; };
  }, [org, project, severity, query, trace]);

  // Live tail: poll for lines newer than the newest one shown. The next poll
  // starts only after the previous one finished, and polling pauses while the
  // tab is in the background.
  useEffect(() => {
    if (!live) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (!document.hidden) {
        try {
          const fresh = await api.logs(org, { project, severity, query, trace, after: newest.current || undefined });
          if (!stopped && fresh.length) {
            newest.current = fresh[0].timestamp;
            setLines((old) => {
              const seen = new Set((old ?? []).map((l) => l.id));
              return [...fresh.filter((l) => !seen.has(l.id)), ...(old ?? [])].slice(0, 1000);
            });
          }
        } catch { /* keep polling */ }
      }
      if (!stopped) timer = setTimeout(poll, 3000);
    };
    timer = setTimeout(poll, 3000);
    const onVisible = () => { if (!document.hidden) { clearTimeout(timer); poll(); } };
    document.addEventListener("visibilitychange", onVisible);
    return () => { stopped = true; clearTimeout(timer); document.removeEventListener("visibilitychange", onVisible); };
  }, [live, org, project, severity, query, trace]);

  useEffect(() => {
    const t = setTimeout(() => text !== query && setQuery({ query: text }), 300);
    return () => clearTimeout(t);
  }, [text, query]);

  const [loadingOlder, setLoadingOlder] = useState(false);
  const [noMore, setNoMore] = useState(false);
  useEffect(() => setNoMore(false), [org, project, severity, query, trace]);
  const older = async () => {
    if (!lines?.length || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const more = await api.logs(org, { project, severity, query, trace, before: lines[lines.length - 1].timestamp });
      const seen = new Set(lines.map((l) => l.id));
      setLines([...lines, ...more.filter((l) => !seen.has(l.id))]);
      if (more.length < 100) setNoMore(true);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoadingOlder(false);
    }
  };

  return (
    <>
      <div className="page-head">
        <h1>Logs</h1>
        <label className="inline"><input type="checkbox" checked={live} onChange={(e) => setLive(e.target.checked)} /> Live tail</label>
      </div>
      <div className="filters">
        <select aria-label="Project" value={project} onChange={(e) => setQuery({ project: e.target.value })}>
          <option value="">All projects</option>
          {projects.map((p) => <option key={p.id} value={p.slug}>{p.name}</option>)}
        </select>
        <select aria-label="Minimum severity" value={severity} onChange={(e) => setQuery({ severity: e.target.value })}>
          <option value="">All severities</option>
          {SEVERITIES.map((s) => <option key={s} value={s}>{s} and above</option>)}
        </select>
        <input className="search" type="search" placeholder="Search message or attributes" value={text} onChange={(e) => setText(e.target.value)} />
        {trace && <button className="ghost" onClick={() => setQuery({ trace: "" })}>Trace {trace.slice(0, 8)}… ✕</button>}
      </div>
      {error && <p className="error">{error}</p>}
      {lines === null ? <p className="muted">Loading…</p> : lines.length === 0 ? (
        <div className="card empty">
          <strong>No logs yet</strong>
          <span className="muted">Send logs with your SDK's logger, or point an OpenTelemetry exporter at <code className="mono">/v1/logs</code> with your DSN key as a Bearer token.</span>
        </div>
      ) : (
        <div className="card flush logs">
          <table className="log-table">
            <tbody>
              {lines.map((l) => (
                <Fragment key={l.id}>
                  <tr className={`log-row sev-${l.severity}`} onClick={() => setOpen(open === l.id ? null : l.id)}>
                    <td className="mono nowrap muted">{new Date(l.timestamp).toLocaleString()}</td>
                    <td><span className={`sev sev-${l.severity}`}>{l.severity}</span></td>
                    <td className="mono muted nowrap hide-sm">{l.project}</td>
                    <td className="mono log-body">{l.body}</td>
                  </tr>
                  {open === l.id && (
                    <tr className="log-detail">
                      <td colSpan={4}>
                        <table className="kv">
                          <tbody>
                            {l.release && <tr><th>release</th><td className="mono">{l.release}</td></tr>}
                            {l.environment && <tr><th>environment</th><td className="mono">{l.environment}</td></tr>}
                            {l.traceId && <tr><th>trace</th><td className="mono"><a className="link" {...linkProps(`${base(org)}/traces/${l.traceId}`)}>{l.traceId}</a> <button className="ghost small-btn" onClick={() => setQuery({ trace: l.traceId })}>Filter logs</button></td></tr>}
                            {Object.entries(l.attributes).map(([k, v]) => <tr key={k}><th className="mono">{k}</th><td className="mono">{v}</td></tr>)}
                          </tbody>
                        </table>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
          {lines.length >= 100 && !noMore && <div className="pager"><button className="ghost" disabled={loadingOlder} onClick={older}>{loadingOlder ? "Loading…" : "Load older"}</button></div>}
        </div>
      )}
    </>
  );
}
