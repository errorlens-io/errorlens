import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, APIToken } from "../api";
import { TimeAgo } from "../ui";

export function TokensSection({ org }: { org: string }) {
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [name, setName] = useState("");
  const [role, setRole] = useState("member");
  const [created, setCreated] = useState("");
  const [error, setError] = useState("");
  const load = useCallback(() => { api.tokens(org).then(setTokens).catch((e) => setError(e.message)); }, [org]);
  useEffect(load, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const r = await api.createToken(org, name, role);
      setCreated(r.token);
      setName("");
      load();
    } catch (err) { setError((err as Error).message); }
  };

  return (
    <section className="card">
      <h2>API tokens</h2>
      <p className="muted small">For CI and scripts, e.g. uploading source maps. Tokens act only within this organization.</p>
      <form className="row" onSubmit={submit}>
        <input aria-label="Token name" placeholder="Token name, e.g. GitHub Actions" required value={name} onChange={(e) => setName(e.target.value)} />
        <select aria-label="Token role" value={role} onChange={(e) => setRole(e.target.value)}>
          <option value="member">Member (upload, read)</option>
          <option value="viewer">Viewer (read only)</option>
          <option value="admin">Admin</option>
        </select>
        <button>Create token</button>
      </form>
      {created && (
        <p className="small">Copy this token now — it won't be shown again:<br /><code className="mono">{created}</code></p>
      )}
      {error && <p className="error">{error}</p>}
      <ul className="rows">
        {tokens.map((t) => (
          <li key={t.id} className="row-item">
            <strong>{t.name}</strong>
            <code className="mono muted">{t.prefix}…</code>
            <span className="badge">{t.role}</span>
            <span className="muted small">{t.lastUsedAt ? <>used <TimeAgo iso={t.lastUsedAt} /></> : "never used"}</span>
            <span className="spacer" />
            <button className="ghost danger" onClick={async () => { await api.deleteToken(org, t.id); load(); }}>Revoke</button>
          </li>
        ))}
      </ul>
    </section>
  );
}
