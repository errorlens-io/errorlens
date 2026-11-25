import { FormEvent, useEffect, useState } from "react";
import { api, AuthConfig } from "../api";
import { navigate } from "../router";

type Props = { onDone: () => void; Brand: () => React.ReactElement };

function useForm(fn: () => Promise<void>) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try { await fn(); } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  };
  return { error, busy, submit, setError };
}

export function LoginPage({ onDone, Brand }: Props) {
  const [cfg, setCfg] = useState<AuthConfig | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [mfaToken, setMfaToken] = useState("");
  const [code, setCode] = useState("");
  const [forgot, setForgot] = useState(false);
  const [sent, setSent] = useState(false);
  const ssoError = new URLSearchParams(location.search).get("sso_error");

  useEffect(() => { api.authConfig().then(setCfg).catch(() => {}); }, []);

  const login = useForm(async () => {
    const r = await api.login(email, password);
    if (r?.mfaRequired) setMfaToken(r.mfaToken);
    else onDone();
  });
  const mfa = useForm(async () => { await api.loginMFA(mfaToken, code); onDone(); });
  const reset = useForm(async () => { await api.forgotPassword(email); setSent(true); });

  if (mfaToken) {
    return (
      <div className="center">
        <form className="card auth" onSubmit={mfa.submit}>
          <Brand />
          <h1>Two-factor authentication</h1>
          <p className="muted">Enter the 6-digit code from your authenticator app, or one of your recovery codes.</p>
          <label>Code<input autoFocus inputMode="numeric" autoComplete="one-time-code" required value={code} onChange={(e) => setCode(e.target.value)} /></label>
          {mfa.error && <p className="error">{mfa.error}</p>}
          <button disabled={mfa.busy}>Verify</button>
          <button type="button" className="ghost" onClick={() => { setMfaToken(""); setCode(""); }}>Back</button>
        </form>
      </div>
    );
  }

  if (forgot) {
    return (
      <div className="center">
        <form className="card auth" onSubmit={reset.submit}>
          <Brand />
          <h1>Reset password</h1>
          {sent ? <p className="muted">If an account exists for {email}, a reset link is on its way.</p> : (
            <>
              <label>Email<input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" /></label>
              {reset.error && <p className="error">{reset.error}</p>}
              <button disabled={reset.busy}>Send reset link</button>
            </>
          )}
          <button type="button" className="ghost" onClick={() => { setForgot(false); setSent(false); }}>Back to sign in</button>
        </form>
      </div>
    );
  }

  return (
    <div className="center">
      <form className="card auth" onSubmit={login.submit}>
        <Brand />
        <h1>Sign in</h1>
        {ssoError && <p className="error">{ssoError}</p>}
        {cfg?.sso && (
          <>
            <a className="button-link" href="/api/0/auth/oidc/start">Continue with {cfg.sso.name}</a>
            <div className="divider"><span>or</span></div>
          </>
        )}
        <label>Email<input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" /></label>
        <label>Password<input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" /></label>
        {login.error && <p className="error">{login.error}</p>}
        <button disabled={login.busy}>{login.busy ? "Signing in…" : "Sign in"}</button>
        <div className="auth-links">
          {cfg?.emailEnabled && <button type="button" className="linklike" onClick={() => setForgot(true)}>Forgot password?</button>}
          {cfg?.signupEnabled && <button type="button" className="linklike" onClick={() => navigate("/signup")}>Create an account</button>}
        </div>
      </form>
    </div>
  );
}

export function SignupPage({ onDone, Brand }: Props) {
  const [f, setF] = useState({ name: "", email: "", password: "", organizationName: "" });
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });
  const form = useForm(async () => {
    const r = await api.signup(f);
    navigate(`/${r.organization.slug}/projects`);
    onDone();
  });
  return (
    <div className="center">
      <form className="card auth" onSubmit={form.submit}>
        <Brand />
        <h1>Create your account</h1>
        <p className="muted">Error tracking that keeps your data private.</p>
        <label>Your name<input value={f.name} onChange={set("name")} autoComplete="name" /></label>
        <label>Work email<input type="email" required value={f.email} onChange={set("email")} autoComplete="email" /></label>
        <label>Password<input type="password" required minLength={10} value={f.password} onChange={set("password")} autoComplete="new-password" /></label>
        <label>Organization name<input required value={f.organizationName} onChange={set("organizationName")} /></label>
        {form.error && <p className="error">{form.error}</p>}
        <button disabled={form.busy}>{form.busy ? "Creating…" : "Create account"}</button>
        <button type="button" className="ghost" onClick={() => navigate("/")}>I already have an account</button>
      </form>
    </div>
  );
}

export function VerifyEmailPage({ token, Brand }: { token: string; Brand: () => React.ReactElement }) {
  const [state, setState] = useState<"working" | "ok" | string>("working");
  useEffect(() => { api.verifyEmail(token).then(() => setState("ok")).catch((e) => setState(e.message)); }, [token]);
  return (
    <div className="center">
      <div className="card auth">
        <Brand />
        <h1>Email verification</h1>
        {state === "working" ? <p className="muted">Verifying…</p> : state === "ok" ? <p>Your email is confirmed. 🎉</p> : <p className="error">{state}</p>}
        <a className="button-link" href="/">Continue</a>
      </div>
    </div>
  );
}

export function ResetPasswordPage({ token, Brand }: { token: string; Brand: () => React.ReactElement }) {
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);
  const form = useForm(async () => { await api.resetPassword(token, password); setDone(true); });
  return (
    <div className="center">
      <form className="card auth" onSubmit={form.submit}>
        <Brand />
        <h1>Choose a new password</h1>
        {done ? (
          <>
            <p>Password updated. You've been signed out of all other sessions.</p>
            <a className="button-link" href="/">Sign in</a>
          </>
        ) : (
          <>
            <label>New password<input type="password" required minLength={10} value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" /></label>
            {form.error && <p className="error">{form.error}</p>}
            <button disabled={form.busy}>Set password</button>
          </>
        )}
      </form>
    </div>
  );
}
