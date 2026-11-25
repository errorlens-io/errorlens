import { useCallback, useEffect, useState } from "react";
import { AdminOrg, api } from "../api";
import { formatCount, TimeAgo } from "../ui";

export function AdminPage() {
  const [orgs, setOrgs] = useState<AdminOrg[] | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(() => { api.adminOrgs().then(setOrgs).catch((e) => setError(e.message)); }, []);
  useEffect(load, [load]);

  const edit = async (o: AdminOrg) => {
    const plan = prompt(`Plan for ${o.name}`, o.plan);
    if (plan === null) return;
    const q = prompt("Monthly event quota (empty = unlimited)", o.quota === null ? "" : String(o.quota));
    if (q === null) return;
    try {
      await api.adminUpdateOrg(o.id, plan, q.trim() === "" ? null : Number(q));
      load();
    } catch (e) { setError((e as Error).message); }
  };

  return (
    <>
      <div className="page-head"><h1>Instance admin</h1></div>
      <p className="muted">All organizations on this server. Only instance administrators see this page.</p>
      {error && <p className="error">{error}</p>}
      <div className="card flush">
        <table className="kv admin-table">
          <thead><tr><th>Organization</th><th>Plan</th><th>Usage (month)</th><th>Members</th><th>Projects</th><th>Created</th><th /></tr></thead>
          <tbody>
            {orgs?.map((o) => (
              <tr key={o.id}>
                <td><strong>{o.name}</strong> <span className="muted mono">{o.slug}</span></td>
                <td className="cap">{o.plan}{o.billingStatus && <span className="muted"> · {o.billingStatus}</span>}</td>
                <td>{formatCount(o.usedThisMonth)} / {o.quota === null ? "∞" : formatCount(o.quota)}</td>
                <td>{o.members}</td>
                <td>{o.projects}</td>
                <td><TimeAgo iso={o.createdAt} /></td>
                <td><button className="ghost small-btn" onClick={() => edit(o)}>Edit plan</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
