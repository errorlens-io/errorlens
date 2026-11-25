import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, ApiError, Organization, User } from "./api";
import { IssueDetailPage } from "./pages/IssueDetail";
import { IssuesPage } from "./pages/Issues";
import { ProjectsPage } from "./pages/Projects";
import { AlertsPage } from "./pages/Alerts";
import { InvitePage } from "./pages/Invite";
import { SettingsPage } from "./pages/Settings";
import { linkProps, navigate, useLocation } from "./router";

type Session = { user: User; organizations: Organization[] };
type View = { kind: "loading" } | { kind: "setup" } | { kind: "login" } | { kind: "app"; session: Session };

export function App() {
  const [view, setView] = useState<View>({ kind: "loading" });

  const refresh = useCallback(async () => {
    try {
      setView({ kind: "app", session: await api.me() });
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 401) throw e;
      const { needsSetup } = await api.setupStatus();
      setView({ kind: needsSetup ? "setup" : "login" });
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Invitation links work whether or not someone is signed in.
  const invite = location.pathname.match(/^\/invite\/([A-Za-z0-9_-]+)$/);
  if (invite && view.kind !== "loading") {
    return <InvitePage token={invite[1]} onDone={refresh} Brand={Brand} />;
  }

  switch (view.kind) {
    case "loading":
      return <div className="center muted">Loading…</div>;
    case "setup":
      return <SetupPage onDone={refresh} />;
    case "login":
      return <LoginPage onDone={refresh} />;
    case "app":
      return <Shell session={view.session} onLogout={refresh} />;
  }
}

function Brand() {
  return (
    <div className="brand">
      <img src="/favicon.svg" alt="" width={28} height={28} />
      <span>RootCause</span>
    </div>
  );
}

function useSubmit(fn: () => Promise<void>) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };
  return { error, busy, submit };
}

function SetupPage({ onDone }: { onDone: () => void }) {
  const [f, setF] = useState({ name: "", email: "", password: "", organizationName: "" });
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });
  const { error, busy, submit } = useSubmit(async () => {
    await api.setup(f);
    onDone();
  });
  return (
    <div className="center">
      <form className="card auth" onSubmit={submit}>
        <Brand />
        <h1>Welcome</h1>
        <p className="muted">Create the first admin account. Everything stays on this server.</p>
        <label>Your name<input value={f.name} onChange={set("name")} autoComplete="name" /></label>
        <label>Email<input type="email" required value={f.email} onChange={set("email")} autoComplete="email" /></label>
        <label>
          Password
          <input type="password" required minLength={10} value={f.password} onChange={set("password")} autoComplete="new-password" />
        </label>
        <label>Organization name<input required value={f.organizationName} onChange={set("organizationName")} /></label>
        {error && <p className="error">{error}</p>}
        <button disabled={busy}>{busy ? "Creating…" : "Create account"}</button>
      </form>
    </div>
  );
}

function LoginPage({ onDone }: { onDone: () => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const { error, busy, submit } = useSubmit(async () => {
    await api.login(email, password);
    onDone();
  });
  return (
    <div className="center">
      <form className="card auth" onSubmit={submit}>
        <Brand />
        <h1>Sign in</h1>
        <label>Email<input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" /></label>
        <label>
          Password
          <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        {error && <p className="error">{error}</p>}
        <button disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
      </form>
    </div>
  );
}

function Shell({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const { path, search } = useLocation();
  const parts = path.split("/").filter(Boolean);
  const fallback = session.organizations[0]?.slug ?? "";
  const org = session.organizations.find((o) => o.slug === parts[0]);

  // "/" or an unknown org: go to the first org's issues.
  useEffect(() => {
    if (!org && fallback) navigate(`/${fallback}/issues`);
  }, [org, fallback]);

  if (!org) {
    return fallback ? null : <div className="center muted">You are not a member of any organization.</div>;
  }

  const section = parts[1] ?? "issues";
  const isAdmin = org.role === "owner" || org.role === "admin";
  let page: React.ReactNode;
  if (section === "issues" && parts[2] && /^\d+$/.test(parts[2])) {
    page = <IssueDetailPage key={parts[2]} org={org.slug} id={Number(parts[2])} role={org.role} />;
  } else if (section === "projects") {
    page = <ProjectsPage org={org} />;
  } else if (section === "alerts") {
    page = <AlertsPage org={org.slug} />;
  } else if (section === "settings") {
    page = <SettingsPage org={org} tab={parts[2] ?? "members"} project={parts[3]} />;
  } else {
    page = <IssuesPage org={org.slug} search={search} />;
  }

  const logout = async () => {
    await api.logout();
    navigate("/");
    onLogout();
  };

  return (
    <div className="layout">
      <header className="topbar">
        <a className="brand-link" {...linkProps(`/${org.slug}/issues`)}><Brand /></a>
        <nav className="nav">
          <a className={section === "issues" ? "active" : ""} {...linkProps(`/${org.slug}/issues`)}>Issues</a>
          <a className={section === "projects" ? "active" : ""} {...linkProps(`/${org.slug}/projects`)}>Projects</a>
          {isAdmin && <a className={section === "alerts" ? "active" : ""} {...linkProps(`/${org.slug}/alerts`)}>Alerts</a>}
          <a className={section === "settings" ? "active" : ""} {...linkProps(`/${org.slug}/settings/members`)}>Settings</a>
        </nav>
        {session.organizations.length > 1 && (
          <select aria-label="Organization" value={org.slug} onChange={(e) => navigate(`/${e.target.value}/issues`)}>
            {session.organizations.map((o) => <option key={o.id} value={o.slug}>{o.name}</option>)}
          </select>
        )}
        <div className="spacer" />
        <span className="muted user-email">{session.user.email}</span>
        <button className="ghost" onClick={logout}>Sign out</button>
      </header>
      <main>{page}</main>
    </div>
  );
}
