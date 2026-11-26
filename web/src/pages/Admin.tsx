import { useCallback, useEffect, useState } from "react";
import { AdminOrg, api } from "../api";
import { goToOrg } from "../router";
import { formatCount, timeAgo } from "../ui";

/** Plans an organization can be on. Paid plans come from billing; any other name can still be typed. */
const PLAN_LABEL: Record<string, string> = { "self-hosted": "Self-hosted", free: "Free" };
const planLabel = (plan: string) => PLAN_LABEL[plan] ?? plan.charAt(0).toUpperCase() + plan.slice(1);
const fullDate = (iso: string) => new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });

export function AdminPage() {
  const [orgs, setOrgs] = useState<AdminOrg[] | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<AdminOrg | null>(null);
  const load = useCallback(() => {
    api.adminOrgs().then(setOrgs).catch((e) => setError(e.message));
  }, []);
  useEffect(load, [load]);

  const totals = (orgs ?? []).reduce(
    (t, o) => ({ events: t.events + o.usedThisMonth, members: t.members + o.members, projects: t.projects + o.projects }),
    { events: 0, members: 0, projects: 0 },
  );

  return (
    <>
      <div className="page-head"><h1>Instance admin</h1></div>
      <p className="muted admin-intro">
        Every organization on this server, with its plan and how many error events it sent this month. Only instance
        administrators can see this page.
      </p>
      {error && <p className="error">{error}</p>}

      {orgs && (
        <div className="admin-stats">
          <div className="stat"><span className="muted">Organizations</span><strong>{formatCount(orgs.length)}</strong></div>
          <div className="stat"><span className="muted">Error events this month</span><strong>{formatCount(totals.events)}</strong></div>
          <div className="stat"><span className="muted">Members</span><strong>{formatCount(totals.members)}</strong></div>
          <div className="stat"><span className="muted">Projects</span><strong>{formatCount(totals.projects)}</strong></div>
        </div>
      )}

      <div className="card flush admin-orgs-wrap">
        <table className="admin-orgs">
          <thead>
            <tr>
              <th>Organization</th>
              <th>Plan</th>
              <th title="Error events accepted since the 1st of this month (UTC). Traces and logs don't count.">Error events this month</th>
              <th className="num">Members</th>
              <th className="num">Projects</th>
              <th>Created</th>
              <th aria-label="Actions" />
            </tr>
          </thead>
          <tbody>
            {orgs === null && !error && <tr><td colSpan={7} className="muted">Loading…</td></tr>}
            {orgs?.length === 0 && <tr><td colSpan={7} className="muted">No organizations yet.</td></tr>}
            {orgs?.map((o) => {
              const share = o.quota ? Math.min(1, o.usedThisMonth / o.quota) : 0;
              return (
                <tr key={o.id}>
                  <td data-label="Organization">
                    <button className="link-btn org-name" onClick={() => goToOrg(o.slug)} title="Open this organization's dashboard">{o.name}</button>
                    <span className="muted mono small">{o.slug}</span>
                  </td>
                  <td data-label="Plan">
                    <span className="badge plan-badge">{planLabel(o.plan)}</span>
                    {o.billingStatus && <span className="muted small"> {o.billingStatus}</span>}
                  </td>
                  <td data-label="Error events this month">
                    <div>
                      <strong>{formatCount(o.usedThisMonth)}</strong>
                      <span className="muted"> {o.quota === null ? "· no limit" : `of ${formatCount(o.quota)}`}</span>
                    </div>
                    {o.quota !== null && (
                      <div className={`quota-bar${share >= 1 ? " full" : share >= 0.8 ? " high" : ""}`} title={`${Math.round(share * 100)}% of the monthly quota`}>
                        <span style={{ width: `${Math.max(2, share * 100)}%` }} />
                      </div>
                    )}
                  </td>
                  <td className="num" data-label="Members">{o.members}</td>
                  <td className="num" data-label="Projects">{o.projects}</td>
                  <td data-label="Created"><span title={new Date(o.createdAt).toLocaleString()}>{fullDate(o.createdAt)}</span><div className="muted small">{timeAgo(o.createdAt)}</div></td>
                  <td className="row-actions"><button className="ghost small" onClick={() => setEditing(o)}>Change plan</button></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {editing && (
        <PlanDialog
          org={editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </>
  );
}

function PlanDialog({ org, onClose, onSaved }: { org: AdminOrg; onClose: () => void; onSaved: () => void }) {
  const known = Object.keys(PLAN_LABEL);
  const [plan, setPlan] = useState(org.plan);
  const [custom, setCustom] = useState(known.includes(org.plan) ? "" : org.plan);
  const [limited, setLimited] = useState(org.quota !== null);
  const [quota, setQuota] = useState(org.quota === null ? "5000" : String(org.quota));
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [onClose]);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = (known.includes(plan) ? plan : custom).trim();
    const q = limited ? Number(quota) : null;
    if (!name) return setError("Choose a plan.");
    if (q !== null && (!Number.isInteger(q) || q < 0)) return setError("The quota must be a whole number of events.");
    setSaving(true);
    try {
      await api.adminUpdateOrg(org.id, name, q);
      onSaved();
    } catch (err) {
      setError((err as Error).message);
      setSaving(false);
    }
  };

  return (
    <div className="dialog-backdrop" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <form className="dialog card" role="dialog" aria-modal="true" aria-labelledby="plan-title" onSubmit={save}>
        <h2 id="plan-title">Change plan for {org.name}</h2>
        <label>
          Plan
          <select value={known.includes(plan) ? plan : "other"} onChange={(e) => setPlan(e.target.value)}>
            {known.map((p) => <option key={p} value={p}>{PLAN_LABEL[p]}</option>)}
            <option value="other">Other (paid plan name)…</option>
          </select>
        </label>
        {!known.includes(plan) && (
          <label>
            Plan name
            <input value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="e.g. team" autoFocus />
          </label>
        )}
        <fieldset className="quota-choice">
          <legend>Monthly error events</legend>
          <label className="inline"><input type="radio" checked={!limited} onChange={() => setLimited(false)} /> No limit</label>
          <label className="inline"><input type="radio" checked={limited} onChange={() => setLimited(true)} /> Limit to</label>
          {limited && <input type="number" min={0} step={1} value={quota} onChange={(e) => setQuota(e.target.value)} aria-label="Monthly error event quota" />}
          <p className="muted small">
            Only error events count. When an organization reaches its limit, new errors are paused until the 1st of next
            month; performance data and logs keep arriving.
          </p>
        </fieldset>
        {error && <p className="error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" className="ghost" onClick={onClose}>Cancel</button>
          <button type="submit" disabled={saving}>{saving ? "Saving…" : "Save"}</button>
        </div>
      </form>
    </div>
  );
}
