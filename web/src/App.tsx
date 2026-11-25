import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, ApiError, Organization, User } from "./api";
import { IssueDetailPage } from "./pages/IssueDetail";
import { IssuesPage } from "./pages/Issues";
import { ProjectsPage } from "./pages/Projects";
import { AlertsPage } from "./pages/Alerts";
import { LogsPage } from "./pages/Logs";
import { ReleasesPage } from "./pages/Releases";
import { InvitePage } from "./pages/Invite";
import { SettingsPage } from "./pages/Settings";
import { LoginPage, ResetPasswordPage, SignupPage, VerifyEmailPage } from "./pages/Auth";
import { AccountPage } from "./pages/Account";
import { AdminPage } from "./pages/Admin";
import { PerformancePage, TracePage } from "./pages/Performance";
import { MonitorDetailPage, MonitorsPage } from "./pages/Monitors";
import { linkProps, navigate, useLocation } from "./router";

type Session = { user: User; organizations: Organization[]; signupEnabled: boolean };
type View = { kind: "loading" } | { kind: "setup" } | { kind: "login" } | { kind: "app"; session: Session };

export function App() {
  const [view, setView] = useState<View>({ kind: "loading" });

  const refresh = useCallback(async () => {
    try {
      const [me, cfg] = await Promise.all([api.me(), api.authConfig()]);
      setView({ kind: "app", session: { ...me, signupEnabled: cfg.signupEnabled } });
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 401) throw e;
      const { needsSetup } = await api.setupStatus();
      setView({ kind: needsSetup ? "setup" : "login" });
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Token links work whether or not someone is signed in.
  const tokenRoute = location.pathname.match(/^\/(invite|verify-email|reset-password)\/([A-Za-z0-9_-]+)$/);
  if (tokenRoute && view.kind !== "loading") {
    const [, kind, token] = tokenRoute;
    if (kind === "invite") return <InvitePage token={token} onDone={refresh} Brand={Brand} />;
    if (kind === "verify-email") return <VerifyEmailPage token={token} Brand={Brand} />;
    return <ResetPasswordPage token={token} Brand={Brand} />;
  }
  if (view.kind === "login" && location.pathname === "/signup") {
    return <SignupPage onDone={refresh} Brand={Brand} />;
  }

  switch (view.kind) {
    case "loading":
      return <div className="center muted">Loading…</div>;
    case "setup":
      return <SetupPage onDone={refresh} />;
    case "login":
      return <LoginPage onDone={refresh} Brand={Brand} />;
    case "app":
      return <Shell session={view.session} onLogout={refresh} />;
  }
}

function Brand() {
  return (
    <div className="brand">
      <img src="/favicon.svg" alt="" width={28} height={28} />
      <span>ErrorLens</span>
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
  } else if (section === "account") {
    page = <AccountPage user={session.user} canCreateOrg={session.user.isSuperuser || session.signupEnabled} onChanged={onLogout} />;
  } else if (section === "admin" && session.user.isSuperuser) {
    page = <AdminPage />;
  } else if (section === "performance") {
    page = <PerformancePage org={org.slug} search={search} />;
  } else if (section === "traces" && parts[2]) {
    page = <TracePage org={org.slug} traceId={parts[2]} />;
  } else if (section === "monitors") {
    page = parts[2] ? <MonitorDetailPage org={org.slug} id={parts[2]} role={org.role} /> : <MonitorsPage org={org.slug} role={org.role} />;
  } else if (section === "logs") {
    page = <LogsPage org={org.slug} search={search} />;
  } else if (section === "releases") {
    page = <ReleasesPage org={org.slug} />;
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
          <a className={section === "performance" || section === "traces" ? "active" : ""} {...linkProps(`/${org.slug}/performance`)}>Performance</a>
          <a className={section === "logs" ? "active" : ""} {...linkProps(`/${org.slug}/logs`)}>Logs</a>
          <a className={section === "monitors" ? "active" : ""} {...linkProps(`/${org.slug}/monitors`)}>Monitors</a>
          <a className={section === "releases" ? "active" : ""} {...linkProps(`/${org.slug}/releases`)}>Releases</a>
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
        {session.user.isSuperuser && <a className={`nav-extra ${section === "admin" ? "active" : ""}`} {...linkProps(`/${org.slug}/admin`)}>Admin</a>}
        <a className="muted user-email" {...linkProps(`/${org.slug}/account`)}>{session.user.email}</a>
        <button className="ghost" onClick={logout}>Sign out</button>
      </header>
      {!session.user.emailVerified && (
        <div className="banner">Please confirm your email address — check your inbox for the link.{" "}
          <button className="linklike" onClick={() => api.resendVerification().then(() => alert("Sent.")).catch((e) => alert(e.message))}>Resend</button>
        </div>
      )}
      <main>{page}</main>
    </div>
  );
}
