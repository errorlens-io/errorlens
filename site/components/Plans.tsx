import Link from "next/link";

/** The pricing cards, shared by the home page and /pricing/. */
export function Plans() {
  return (
    <div className="plans">
      <div className="card plan">
        <h3>Free</h3>
        <div className="price">$0</div>
        <p>Hosted at errorlens.io</p>
        <ul className="checks">
          <li>5,000 error events per month</li>
          <li>Performance traces, logs and monitors included</li>
          <li>Unlimited projects and team members</li>
          <li>30 days of history by default</li>
        </ul>
        <a href="/signup" className="btn">Create your account</a>
      </div>
      <div className="card plan">
        <h3>Self-hosted</h3>
        <div className="price">$0</div>
        <p>On your own servers</p>
        <ul className="checks">
          <li>Unlimited events — limited only by your hardware</li>
          <li>One container plus Postgres</li>
          <li>Data never leaves your network</li>
          <li>AGPL-3.0 open source</li>
        </ul>
        <Link href="/self-hosted-error-tracking/" className="btn ghost">Self-hosting guide</Link>
      </div>
      <div className="card plan">
        <h3>Team</h3>
        <div className="price">Soon</div>
        <p>Hosted, for larger volumes</p>
        <ul className="checks">
          <li>Higher monthly event quotas</li>
          <li>Longer retention</li>
          <li>Priority support</li>
        </ul>
        <a href="mailto:hello@errorlens.io" className="btn ghost">Tell me when it is ready</a>
      </div>
    </div>
  );
}
