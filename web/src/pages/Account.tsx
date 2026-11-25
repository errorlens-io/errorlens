import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, User } from "../api";
import { navigate } from "../router";

export function AccountPage({ user, canCreateOrg, onChanged }: { user: User; canCreateOrg: boolean; onChanged: () => void }) {
  return (
    <>
      <div className="page-head"><h1>Your account</h1></div>
      <p className="muted">{user.email}</p>
      <PasswordSection />
      <MFASection />
      {canCreateOrg && <NewOrgSection onChanged={onChanged} />}
    </>
  );
}

function PasswordSection() {
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setMsg(""); setError("");
    try { await api.changePassword(cur, next); setMsg("Password changed."); setCur(""); setNext(""); } catch (err) { setError((err as Error).message); }
  };
  return (
    <form className="card form-grid" onSubmit={submit}>
      <h2>Password</h2>
      <label>Current password<input type="password" required value={cur} onChange={(e) => setCur(e.target.value)} autoComplete="current-password" /></label>
      <label>New password<input type="password" required minLength={10} value={next} onChange={(e) => setNext(e.target.value)} autoComplete="new-password" /></label>
      <div className="row"><button>Change password</button>{msg && <span className="ok-text">{msg}</span>}{error && <span className="error">{error}</span>}</div>
    </form>
  );
}

function MFASection() {
  const [status, setStatus] = useState<{ enabled: boolean; recoveryCodesLeft: number } | null>(null);
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[]>([]);
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const load = useCallback(() => { api.mfaStatus().then(setStatus).catch((e) => setError(e.message)); }, []);
  useEffect(load, [load]);

  const act = (fn: () => Promise<void>) => async (e?: FormEvent) => {
    e?.preventDefault();
    setError("");
    try { await fn(); } catch (err) { setError((err as Error).message); }
  };

  return (
    <section className="card form-grid">
      <h2>Two-factor authentication</h2>
      {status?.enabled ? (
        <>
          <p>✅ Enabled. {status.recoveryCodesLeft} recovery codes left.</p>
          {codes.length > 0 && <RecoveryCodes codes={codes} />}
          <form className="row" onSubmit={act(async () => { await api.mfaDisable(password); setPassword(""); setCodes([]); load(); })}>
            <input type="password" aria-label="Password" placeholder="Password to turn off 2FA" required value={password} onChange={(e) => setPassword(e.target.value)} />
            <button className="ghost danger">Turn off</button>
          </form>
        </>
      ) : setup ? (
        <form className="form-grid" onSubmit={act(async () => { const r = await api.mfaConfirm(code); setCodes(r.recoveryCodes); setSetup(null); setCode(""); load(); })}>
          <p className="muted small">Add this account to an authenticator app (1Password, Google Authenticator, Authy…) using the setup key or link, then enter the 6-digit code it shows.</p>
          <label>Setup key<code className="mono setup-key">{setup.secret.match(/.{1,4}/g)?.join(" ")}</code></label>
          <a className="link small" href={setup.uri}>Open in authenticator app</a>
          <label>Code<input inputMode="numeric" autoComplete="one-time-code" required value={code} onChange={(e) => setCode(e.target.value)} /></label>
          <div className="row"><button>Turn on</button><button type="button" className="ghost" onClick={() => setSetup(null)}>Cancel</button></div>
        </form>
      ) : (
        <>
          <p className="muted">Protect your account with a code from your phone in addition to your password.</p>
          <div><button onClick={act(async () => setSetup(await api.mfaSetup()))}>Set up two-factor authentication</button></div>
        </>
      )}
      {error && <p className="error">{error}</p>}
    </section>
  );
}

function RecoveryCodes({ codes }: { codes: string[] }) {
  return (
    <div className="recovery">
      <p className="small"><strong>Save these recovery codes now.</strong> Each works once if you lose your phone. They won't be shown again.</p>
      <pre className="mono">{codes.join("\n")}</pre>
    </div>
  );
}

function NewOrgSection({ onChanged }: { onChanged: () => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const org = await api.createOrganization(name);
      onChanged();
      navigate(`/${org.slug}/projects`);
    } catch (err) { setError((err as Error).message); }
  };
  return (
    <form className="card row" onSubmit={submit}>
      <input aria-label="Organization name" placeholder="New organization name" required value={name} onChange={(e) => setName(e.target.value)} />
      <button>Create organization</button>
      {error && <p className="error">{error}</p>}
    </form>
  );
}
