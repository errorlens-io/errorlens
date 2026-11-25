import { FormEvent, useCallback, useEffect, useState } from "react";
import { AlertChannel, api, CheckIn, Monitor, Project } from "../api";
import { linkProps, navigate } from "../router";
import { Empty, TimeAgo } from "../ui";
import { ms } from "./Performance";

const statusLabel: Record<string, string> = { ok: "Up", error: "Failing", missed: "Missed", timeout: "Timed out", pending: "Waiting", in_progress: "Running" };

function Dots({ recent }: { recent: string[] }) {
  return <span className="dots">{[...recent].reverse().map((s, i) => <span key={i} className={`dot st-${s}`} title={s} />)}</span>;
}

export function MonitorsPage({ org, role }: { org: string; role: string }) {
  const [items, setItems] = useState<Monitor[] | null>(null);
  const [adding, setAdding] = useState(false);
  const load = useCallback(() => { api.monitors(org).then(setItems).catch(() => setItems([])); }, [org]);
  useEffect(load, [load]);
  const isAdmin = role === "owner" || role === "admin";
  return (
    <>
      <div className="page-head"><h1>Monitors</h1>{isAdmin && <button onClick={() => setAdding(!adding)}>{adding ? "Cancel" : "New monitor"}</button>}</div>
      <p className="muted">Uptime checks for your URLs, and cron monitors that alert when a scheduled job doesn't run or fails.</p>
      {adding && <MonitorForm org={org} onSaved={() => { setAdding(false); load(); }} />}
      {items === null ? <p className="muted">Loading…</p> : items.length === 0 ? <Empty title="No monitors yet" /> : (
        <div className="card flush">
          <table className="kv">
            <tbody>
              {items.map((m) => (
                <tr key={m.id}>
                  <td><span className={`pill st-${m.status}`}>{statusLabel[m.status] ?? m.status}</span></td>
                  <td><a className="link" {...linkProps(`/${org}/monitors/${m.id}`)}><strong>{m.name}</strong></a><div className="muted small mono">{m.kind === "uptime" ? m.url : m.schedule}</div></td>
                  <td className="hide-sm">{m.project}</td>
                  <td><Dots recent={m.recent} /></td>
                  <td className="nowrap muted small">{m.lastCheckIn ? <TimeAgo iso={m.lastCheckIn} /> : "never"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function MonitorForm({ org, onSaved }: { org: string; onSaved: () => void }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [channels, setChannels] = useState<AlertChannel[]>([]);
  const [f, setF] = useState({ kind: "uptime" as "uptime" | "cron", name: "", project: "", url: "https://", intervalSeconds: 60, schedule: "0 * * * *", timezone: "UTC", checkinMarginMinutes: 5, maxRuntimeMinutes: 30 });
  const [chosen, setChosen] = useState<string[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    api.projects(org).then((p) => { setProjects(p); setF((x) => ({ ...x, project: x.project || p[0]?.slug || "" })); });
    api.channels(org).then(setChannels).catch(() => {});
  }, [org]);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try { await api.saveMonitor(org, { ...f, channelIds: chosen }); onSaved(); } catch (err) { setError((err as Error).message); }
  };
  return (
    <form className="card form-grid" onSubmit={submit}>
      <div className="segmented">
        <button type="button" className={f.kind === "uptime" ? "on" : ""} onClick={() => setF({ ...f, kind: "uptime" })}>Uptime check</button>
        <button type="button" className={f.kind === "cron" ? "on" : ""} onClick={() => setF({ ...f, kind: "cron" })}>Cron job</button>
      </div>
      <div className="row">
        <input aria-label="Name" placeholder="Name" required value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
        <select aria-label="Project" value={f.project} onChange={(e) => setF({ ...f, project: e.target.value })}>
          {projects.map((p) => <option key={p.id} value={p.slug}>{p.name}</option>)}
        </select>
      </div>
      {f.kind === "uptime" ? (
        <div className="row">
          <input aria-label="URL" type="url" required value={f.url} onChange={(e) => setF({ ...f, url: e.target.value })} />
          <select aria-label="Interval" value={f.intervalSeconds} onChange={(e) => setF({ ...f, intervalSeconds: Number(e.target.value) })}>
            {[30, 60, 300, 900, 3600].map((s) => <option key={s} value={s}>every {s < 60 ? `${s}s` : `${s / 60} min`}</option>)}
          </select>
        </div>
      ) : (
        <div className="row">
          <input aria-label="Schedule" className="mono" placeholder="0 * * * *" required value={f.schedule} onChange={(e) => setF({ ...f, schedule: e.target.value })} />
          <input aria-label="Timezone" value={f.timezone} onChange={(e) => setF({ ...f, timezone: e.target.value })} />
          <span className="muted small">grace</span>
          <input aria-label="Margin minutes" className="num" type="number" min={1} value={f.checkinMarginMinutes} onChange={(e) => setF({ ...f, checkinMarginMinutes: Number(e.target.value) })} />
          <span className="muted small">min, max runtime</span>
          <input aria-label="Max runtime minutes" className="num" type="number" min={1} value={f.maxRuntimeMinutes} onChange={(e) => setF({ ...f, maxRuntimeMinutes: Number(e.target.value) })} />
          <span className="muted small">min</span>
        </div>
      )}
      {channels.length > 0 && (
        <div className="row">
          <span className="muted">Alert:</span>
          {channels.map((c) => (
            <label key={c.id} className="inline"><input type="checkbox" checked={chosen.includes(c.id)} onChange={(e) => setChosen(e.target.checked ? [...chosen, c.id] : chosen.filter((x) => x !== c.id))} /> {c.name}</label>
          ))}
        </div>
      )}
      <div className="row"><button>Create monitor</button>{error && <span className="error">{error}</span>}</div>
    </form>
  );
}

export function MonitorDetailPage({ org, id, role }: { org: string; id: string; role: string }) {
  const [d, setD] = useState<{ monitor: Monitor; checkins: CheckIn[] } | null>(null);
  const [error, setError] = useState("");
  const [dsn, setDsn] = useState("");
  useEffect(() => { api.monitor(org, id).then(setD).catch((e) => setError(e.message)); }, [org, id]);
  useEffect(() => {
    if (d?.monitor.kind === "cron") api.projects(org).then((ps) => setDsn(ps.find((p) => p.slug === d.monitor.project)?.dsn ?? ""));
  }, [org, d]);
  if (error) return <p className="error">{error}</p>;
  if (!d) return <p className="muted">Loading…</p>;
  const m = d.monitor;
  let curl = "";
  if (dsn) {
    const u = new URL(dsn);
    const projectId = u.pathname.split("/").pop();
    curl = `curl -fsS "${u.protocol}//${u.host}/api/${projectId}/monitors/${m.slug}/checkin?key=${u.username}&status=ok"`;
  }
  return (
    <>
      <a className="back muted" {...linkProps(`/${org}/monitors`)}>← Monitors</a>
      <div className="page-head">
        <h1>{m.name} <span className={`pill st-${m.status}`}>{statusLabel[m.status] ?? m.status}</span></h1>
        {(role === "owner" || role === "admin") && (
          <button className="ghost danger" onClick={async () => { if (confirm("Delete this monitor?")) { await api.deleteMonitor(org, id); navigate(`/${org}/monitors`); } }}>Delete</button>
        )}
      </div>
      <p className="muted mono">{m.kind === "uptime" ? `${m.url} · every ${m.intervalSeconds}s` : `${m.schedule} (${m.timezone}) · next ${m.nextExpected ? new Date(m.nextExpected).toLocaleString() : "—"}`}</p>
      {m.kind === "cron" && curl && (
        <section className="card">
          <h2>Report runs</h2>
          <p className="muted small">Call this at the end of your job (or use your SDK's check-in API with slug <code className="mono">{m.slug}</code>):</p>
          <pre className="mono code-block-plain">{curl}</pre>
        </section>
      )}
      <section className="card">
        <h2>Recent {m.kind === "uptime" ? "checks" : "check-ins"}</h2>
        {d.checkins.length === 0 ? <p className="muted">Nothing yet.</p> : (
          <table className="kv">
            <tbody>
              {d.checkins.map((c) => (
                <tr key={c.id}>
                  <td><span className={`pill st-${c.status}`}>{statusLabel[c.status] ?? c.status}</span></td>
                  <td className="nowrap"><TimeAgo iso={c.startedAt} /></td>
                  <td className="num-col">{c.durationMs != null ? ms(c.durationMs) : "—"}</td>
                  <td className="muted small">{c.details || c.environment}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
