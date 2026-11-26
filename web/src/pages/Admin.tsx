import { useCallback, useEffect, useState } from "react";
import { AdminOrg, AdminPlan, api } from "../api";
import { goToOrg } from "../router";
import { formatCount, timeAgo } from "../ui";

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1).replace(/-/g, " ");
const fullDate = (iso: string) => new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });

export function AdminPage() {
  const [orgs, setOrgs] = useState<AdminOrg[] | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<AdminOrg | null>(null);
  const [catalog, setCatalog] = useState<{ hosted: boolean; plans: AdminPlan[] } | null>(null);
  const load = useCallback(() => {
    api.adminOrgs().then(setOrgs).catch((e) => setError(e.message));
  }, []);
  useEffect(() => { api.adminPlans().then(setCatalog).catch((e) => setError(e.message)); }, []);
  const planLabel = (id: string) => catalog?.plans.find((p) => p.id === id)?.label ?? capitalize(id);
  // On a self-hosted server there is one plan (no limit): nothing to change unless an org is on something else.
  const canChange = (o: AdminOrg) => !!catalog && (catalog.hosted || o.plan !== "self-hosted" || o.quota !== null);
  useEffect(load, [load]);

  const totals = (orgs ?? []).reduce(
    (t, o) => ({ events: t.events + o.usedThisMonth, members: t.members + o.members, projects: t.projects + o.projects }),
    { events: 0, members: 0, projects: 0 },
  );

  return (
    <>
      <div className="page-head"><h1>Instance admin</h1></div>
      <p className="muted admin-intro">
        Every organization on this server, with its plan and how many error events it sent this month.{" "}
        {catalog && (catalog.hosted
          ? "Each plan comes with its own monthly limit."
          : "This is a self-hosted server: organizations have no limit.")}{" "}
        Only instance administrators can see this page.
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
                  <td className="row-actions">{canChange(o) && <button className="ghost small" onClick={() => setEditing(o)}>Change plan</button>}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {editing && catalog && (
        <PlanDialog
          org={editing}
          plans={catalog.plans}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); load(); }}
        />
      )}
    </>
  );
}

function PlanDialog({ org, plans, onClose, onSaved }: { org: AdminOrg; plans: AdminPlan[]; onClose: () => void; onSaved: () => void }) {
  const current = plans.find((p) => p.id === org.plan);
  // an org on a plan this server no longer offers starts on the closest one: no limit stays no limit
  const closest = plans.find((p) => (p.quota === null) === (org.quota === null)) ?? plans[0];
  const [plan, setPlan] = useState(current?.id ?? closest?.id ?? "");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [onClose]);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await api.adminUpdateOrg(org.id, plan);
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
        {!current && <p className="muted small">Currently on “{org.plan}”, which this server doesn't offer any more.</p>}
        <div className="plan-options" role="radiogroup" aria-label="Plan">
          {plans.map((p) => (
            <label key={p.id} className={`plan-option${plan === p.id ? " selected" : ""}`}>
              <input type="radio" name="plan" value={p.id} checked={plan === p.id} onChange={() => setPlan(p.id)} />
              <span>
                <strong>{p.label}</strong>
                <span className="plan-limit">{p.quota === null ? "No limit" : `${p.quota.toLocaleString()} error events a month`}</span>
                <span className="muted small">{p.note}</span>
              </span>
            </label>
          ))}
        </div>
        {plans.some((p) => p.quota !== null) && (
          <p className="muted small">
            Only error events count. At the limit, new errors pause until the 1st of next month; performance data and
            logs keep arriving.
          </p>
        )}
        {error && <p className="error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" className="ghost" onClick={onClose}>Cancel</button>
          <button type="submit" disabled={saving || plan === org.plan}>{saving ? "Saving…" : "Save"}</button>
        </div>
      </form>
    </div>
  );
}
