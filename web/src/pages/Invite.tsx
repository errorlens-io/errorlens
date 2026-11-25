import { FormEvent, useEffect, useState } from "react";
import { api } from "../api";
import { navigate } from "../router";

export function InvitePage({ token, onDone, Brand }: { token: string; onDone: () => void; Brand: () => React.ReactElement }) {
  const [info, setInfo] = useState<{ organization: string; email: string; role: string; accountExists: boolean } | null>(null);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => { api.inviteInfo(token).then(setInfo).catch((e) => setError(e.message)); }, [token]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(""); setBusy(true);
    try {
      const r = await api.acceptInvite(token, name, password);
      navigate(`/${r.organization}/issues`);
      onDone();
    } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  };

  return (
    <div className="center">
      <form className="card auth" onSubmit={submit}>
        <Brand />
        {!info ? (
          error ? <p className="error">{error}</p> : <p className="muted">Loading invitation…</p>
        ) : (
          <>
            <h1>Join {info.organization}</h1>
            <p className="muted">You've been invited as <strong>{info.role}</strong> ({info.email}).</p>
            {info.accountExists ? (
              <p className="muted small">You already have an account. Sign in as {info.email}, then open this link again.</p>
            ) : (
              <>
                <label>Your name<input value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" /></label>
                <label>Choose a password
                  <input type="password" required minLength={10} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
                </label>
              </>
            )}
            {error && <p className="error">{error}</p>}
            <button disabled={busy}>{info.accountExists ? "Accept invitation" : busy ? "Joining…" : "Create account and join"}</button>
          </>
        )}
      </form>
    </div>
  );
}
