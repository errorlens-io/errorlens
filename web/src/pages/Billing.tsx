import { useEffect, useState } from "react";
import { api, BillingInfo } from "../api";
import { Bars, formatCount } from "../ui";

export function BillingSection({ org }: { org: string }) {
  const [b, setB] = useState<BillingInfo | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { api.billing(org).then(setB).catch((e) => setError(e.message)); }, [org]);
  if (error) return <p className="error">{error}</p>;
  if (!b) return <p className="muted">Loading…</p>;
  const pct = b.quota ? Math.min(100, (b.used / b.quota) * 100) : 0;
  const go = async (fn: () => Promise<{ url: string }>) => {
    try { location.href = (await fn()).url; } catch (e) { setError((e as Error).message); }
  };
  return (
    <>
      <section className="card form-grid">
        <h2>Plan & usage this month</h2>
        <p><strong className="cap">{b.plan}</strong>{b.status && <span className="muted"> · {b.status}</span>}</p>
        <div>
          <div className="quota-bar"><span style={{ width: `${pct}%` }} className={pct >= 90 ? "hot" : ""} /></div>
          <p className="small muted">
            {formatCount(b.used)} events {b.quota !== null ? `of ${formatCount(b.quota)}` : "(unlimited)"}
            {b.dropped > 0 && ` · ${formatCount(b.dropped)} dropped`}
          </p>
        </div>
        {b.days.length > 0 && <Bars data={b.days.map((d) => d.accepted)} height={48} label="Events per day this month" />}
      </section>
      {b.billingEnabled && (
        <section className="card form-grid">
          <h2>Upgrade</h2>
          <div className="plans">
            {b.plans?.map((p) => (
              <div key={p.id} className="plan">
                <strong>{p.label}</strong>
                <span className="muted">{formatCount(p.quota)} events / month</span>
                <button disabled={p.id === b.plan} onClick={() => go(() => api.billingCheckout(org, p.id))}>{p.id === b.plan ? "Current plan" : "Choose"}</button>
              </div>
            ))}
          </div>
          {b.hasCustomer && <div><button className="ghost" onClick={() => go(() => api.billingPortal(org))}>Manage subscription & invoices</button></div>}
        </section>
      )}
    </>
  );
}
