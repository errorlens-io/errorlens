import { FormEvent, useCallback, useEffect, useState } from "react";
import { AlertChannel, AlertHistory, AlertRule, api, Project } from "../api";
import { linkProps, base } from "../router";
import { usePaged } from "../paged";
import { Empty, TimeAgo } from "../ui";

const TRIGGERS: Record<AlertRule["trigger"], string> = {
  new_issue: "A new issue is created",
  regression: "A resolved issue happens again",
  frequency: "An issue spikes",
};

export function AlertsPage({ org }: { org: string }) {
  const [channels, setChannels] = useState<AlertChannel[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    Promise.all([api.channels(org), api.rules(org), api.projects(org)])
      .then(([c, r, p]) => { setChannels(c); setRules(r); setProjects(p); })
      .catch((e) => setError(e.message));
  }, [org]);
  const { items: history, pager: historyPager } = usePaged<AlertHistory>((after) => api.alertHistory(org, after?.id), [org]);
  useEffect(load, [load]);

  const run = (fn: () => Promise<unknown>) => async () => {
    setError("");
    try { await fn(); load(); } catch (e) { setError((e as Error).message); }
  };

  return (
    <>
      <div className="page-head"><h1>Alerts</h1></div>
      <p className="muted">Get notified when something breaks. Add where alerts should go, then decide when they fire.</p>
      {error && <p className="error">{error}</p>}

      <section className="card">
        <h2>Destinations</h2>
        {channels.length === 0 && <p className="muted">No destinations yet.</p>}
        <ul className="rows">
          {channels.map((c) => (
            <li key={c.id} className="row-item">
              <span className="badge">{c.kind}</span>
              <strong>{c.name}</strong>
              <span className="muted mono ellipsis">{c.target}</span>
              <span className="spacer" />
              <button className="ghost" onClick={async () => {
                setError("");
                try { await api.testChannel(org, c.id); alert("Test notification sent."); } catch (e) { setError((e as Error).message); }
              }}>Send test</button>
              <button className="ghost danger" onClick={run(() => api.deleteChannel(org, c.id))}>Delete</button>
            </li>
          ))}
        </ul>
        <ChannelForm org={org} onSaved={load} />
      </section>

      <section className="card">
        <h2>Rules</h2>
        {rules.length === 0 && <p className="muted">No rules yet.</p>}
        <ul className="rows">
          {rules.map((r) => (
            <li key={r.id} className="row-item">
              <strong>{r.name}</strong>
              <span className="muted">
                {TRIGGERS[r.trigger]}
                {r.trigger === "frequency" && ` (≥ ${r.threshold} events in ${r.windowMinutes} min)`}
                {" · "}{r.project ?? "all projects"} · {r.minLevel}+ · {r.channelIds.length} destination{r.channelIds.length === 1 ? "" : "s"}
              </span>
              <span className="spacer" />
              <label className="inline">
                <input type="checkbox" checked={r.enabled} onChange={run(() => api.saveRule(org, { ...r, project: r.project ?? "", enabled: !r.enabled }, r.id))} /> Enabled
              </label>
              <button className="ghost danger" onClick={run(() => api.deleteRule(org, r.id))}>Delete</button>
            </li>
          ))}
        </ul>
        {channels.length > 0 ? <RuleForm org={org} channels={channels} projects={projects} onSaved={load} /> : <p className="muted small">Add a destination first.</p>}
      </section>

      <section className="card">
        <h2>Recent notifications</h2>
        {history === null ? <p className="muted">Loading…</p> : history.length === 0 ? <Empty title="Nothing sent yet" /> : (
          <table className="kv">
            <tbody>
              {history.map((h) => (
                <tr key={h.id}>
                  <td className="nowrap"><TimeAgo iso={h.createdAt} /></td>
                  <td>{h.issueId ? <a className="link" {...linkProps(`${base(org)}/issues/${h.issueId}`)}>{h.issueTitle}</a> : "—"}</td>
                  <td>{h.rule} → {h.channel ?? "deleted"}</td>
                  <td><span className={`badge ${h.status === "sent" ? "ok" : h.status === "failed" ? "warn" : ""}`} title={h.error}>{h.status}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {historyPager}
      </section>
    </>
  );
}

function ChannelForm({ org, onSaved }: { org: string; onSaved: () => void }) {
  const [kind, setKind] = useState("slack");
  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await api.createChannel(org, kind === "email"
        ? { kind, name, emails: target.split(/[,\s]+/).filter(Boolean) }
        : { kind, name, url: target, secret: kind === "webhook" ? secret : undefined });
      setName(""); setTarget(""); setSecret("");
      onSaved();
    } catch (err) { setError((err as Error).message); }
  };
  return (
    <form className="row subform" onSubmit={submit}>
      <select aria-label="Destination type" value={kind} onChange={(e) => setKind(e.target.value)}>
        <option value="slack">Slack</option>
        <option value="discord">Discord</option>
        <option value="webhook">Webhook</option>
        <option value="email">Email</option>
      </select>
      <input aria-label="Name" placeholder="Name, e.g. #oncall" required value={name} onChange={(e) => setName(e.target.value)} />
      <input aria-label={kind === "email" ? "Email addresses" : "URL"} required value={target} onChange={(e) => setTarget(e.target.value)}
        placeholder={kind === "email" ? "ops@example.com, dev@example.com" : kind === "slack" ? "Slack incoming webhook URL" : kind === "discord" ? "Discord webhook URL" : "https://example.com/hooks/errorlens"} />
      {kind === "webhook" && <input aria-label="Signing secret" placeholder="Signing secret (optional)" value={secret} onChange={(e) => setSecret(e.target.value)} />}
      <button>Add destination</button>
      {error && <p className="error">{error}</p>}
    </form>
  );
}

function RuleForm({ org, channels, projects, onSaved }: { org: string; channels: AlertChannel[]; projects: Project[]; onSaved: () => void }) {
  const [f, setF] = useState({ name: "", trigger: "new_issue" as AlertRule["trigger"], project: "", minLevel: "error", threshold: 100, windowMinutes: 60, environment: "" });
  const [chosen, setChosen] = useState<string[]>([channels[0].id]);
  const [error, setError] = useState("");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await api.saveRule(org, { ...f, channelIds: chosen, cooldownMinutes: 60 });
      setF({ ...f, name: "" });
      onSaved();
    } catch (err) { setError((err as Error).message); }
  };
  return (
    <form className="subform rule-form" onSubmit={submit}>
      <div className="row">
        <input aria-label="Rule name" placeholder="Rule name" required value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
        <select aria-label="When" value={f.trigger} onChange={(e) => setF({ ...f, trigger: e.target.value as AlertRule["trigger"] })}>
          {Object.entries(TRIGGERS).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
        {f.trigger === "frequency" && (
          <>
            <input aria-label="Event threshold" type="number" min={1} value={f.threshold} onChange={(e) => setF({ ...f, threshold: Number(e.target.value) })} className="num" />
            <span className="muted">events in</span>
            <input aria-label="Window minutes" type="number" min={1} max={1440} value={f.windowMinutes} onChange={(e) => setF({ ...f, windowMinutes: Number(e.target.value) })} className="num" />
            <span className="muted">min</span>
          </>
        )}
      </div>
      <div className="row">
        <select aria-label="Project" value={f.project} onChange={(e) => setF({ ...f, project: e.target.value })}>
          <option value="">All projects</option>
          {projects.map((p) => <option key={p.id} value={p.slug}>{p.name}</option>)}
        </select>
        <select aria-label="Minimum level" value={f.minLevel} onChange={(e) => setF({ ...f, minLevel: e.target.value })}>
          {["debug", "info", "warning", "error", "fatal"].map((l) => <option key={l} value={l}>{l} and above</option>)}
        </select>
        <input aria-label="Environment" placeholder="Environment (any)" value={f.environment} onChange={(e) => setF({ ...f, environment: e.target.value })} />
      </div>
      <div className="row">
        <span className="muted">Send to:</span>
        {channels.map((c) => (
          <label key={c.id} className="inline">
            <input type="checkbox" checked={chosen.includes(c.id)} onChange={(e) => setChosen(e.target.checked ? [...chosen, c.id] : chosen.filter((x) => x !== c.id))} /> {c.name}
          </label>
        ))}
        <span className="spacer" />
        <button disabled={chosen.length === 0}>Add rule</button>
      </div>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
