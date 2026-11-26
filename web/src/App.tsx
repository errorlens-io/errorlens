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
import { base, configureTenant, goToOrg, hostKind, linkProps, mainUrl, navigate, orgUrl, safeNext, setProjectRoute, tenantOrg, useLocation } from "./router";

type Session = { user: User; organizations: Organization[]; signupEnabled: boolean };
type View = { kind: "loading" } | { kind: "setup" } | { kind: "login" } | { kind: "app"; session: Session };

export function App() {
  const [view, setView] = useState<View>({ kind: "loading" });

  const refresh = useCallback(async () => {
    const cfg = await api.authConfig();
    configureTenant(cfg.tenantDomain ?? "", cfg.publicUrl ?? "");
    try {
      const me = await api.me();
      setView({ kind: "app", session: { ...me, signupEnabled: cfg.signupEnabled } });
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 401) throw e;
      // Organization subdomains have no sign-in page of their own.
      if (hostKind() === "org") {
        location.href = mainUrl(`/login?next=${encodeURIComponent(location.href)}`);
        return;
      }
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
      if (hostKind() === "apex") return <ToDashboard session={view.session} />;
      return <Shell session={view.session} onLogout={refresh} />;
  }
}

/** On the main domain a signed-in user is sent to their organization's subdomain. */
function ToDashboard({ session }: { session: Session }) {
  const first = session.organizations[0]?.slug;
  useEffect(() => {
    const next = safeNext(new URLSearchParams(location.search).get("next"));
    if (next) location.replace(next);
    else if (first) location.replace(orgUrl(first, "/issues"));
  }, [first]);
  if (!first) return <div className="center muted">You are not a member of any organization yet. Ask for an invitation, or sign up to create one.</div>;
  return <div className="center muted">Opening your dashboard…</div>;
}

// First path segments that are dashboard sections, not project slugs.
const SECTIONS = new Set(["issues", "performance", "traces", "logs", "monitors", "releases", "projects", "alerts", "settings", "account", "admin"]);
// Sections that can be scoped to one project: /<project>/<section>.
const PROJECT_SECTIONS = new Set(["issues", "performance", "logs", "monitors", "releases"]);

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
  const all = path.split("/").filter(Boolean);
  const fromHost = tenantOrg();
  const slug = fromHost || all[0];
  let rest = fromHost ? all : all.slice(1);
  const fallback = session.organizations[0]?.slug ?? "";
  const org = session.organizations.find((o) => o.slug === slug);

  // /<project>/<section>: scope the section to one project.
  let project = "";
  if (rest[0] && !SECTIONS.has(rest[0])) {
    project = rest[0];
    rest = rest.slice(1);
  }
  const section = rest[0] ?? "issues";
  const scoped = project && PROJECT_SECTIONS.has(section) ? project : "";
  const prefix = org ? base(org.slug) : "";
  const projectPrefix = scoped ? `/${scoped}` : "";

  // "/" or an unknown org: go to the first org's issues (self-hosted mode).
  useEffect(() => {
    if (!org && fallback && !fromHost) navigate(`/${fallback}/issues`);
  }, [org, fallback, fromHost]);

  // ?project=x links and project pickers become /<x>/<section> paths.
  useEffect(() => {
    if (!org || !PROJECT_SECTIONS.has(section)) {
      setProjectRoute(null);
      return;
    }
    const go = (p: string, extra: Record<string, string>, replace = false) => {
      const q = new URLSearchParams(location.search);
      q.delete("project");
      q.delete("offset");
      for (const [k, v] of Object.entries(extra)) (v ? q.set(k, v) : q.delete(k));
      const qs = q.toString();
      const to = `${prefix}${p ? `/${p}` : ""}/${section}${qs ? `?${qs}` : ""}`;
      if (replace) {
        history.replaceState(null, "", to);
        window.dispatchEvent(new PopStateEvent("popstate"));
      } else navigate(to);
    };
    setProjectRoute((p, extra) => go(p, extra));
    const qp = new URLSearchParams(search).get("project");
    if (qp !== null && rest.length <= 1) go(qp, {}, true);
    return () => setProjectRoute(null);
  }, [org, section, search, prefix, rest.length]);

  if (!org) {
    if (fromHost) {
      return (
        <div className="center">
          <div className="card auth">
            <h1>No access</h1>
            <p className="muted">You are not a member of <b>{fromHost}</b>.</p>
            {session.organizations.map((o) => <p key={o.id}><a className="link" href={orgUrl(o.slug, "/issues")}>Go to {o.name}</a></p>)}
          </div>
        </div>
      );
    }
    return fallback ? null : <div className="center muted">You are not a member of any organization.</div>;
  }

  // Pages read the project filter from the query string.
  const q = new URLSearchParams(search);
  if (scoped) q.set("project", scoped);
  const pageSearch = q.toString() ? `?${q.toString()}` : "";

  const isAdmin = org.role === "owner" || org.role === "admin";
  let page: React.ReactNode;
  if (section === "issues" && rest[1] && /^\d+$/.test(rest[1])) {
    page = <IssueDetailPage key={rest[1]} org={org.slug} id={Number(rest[1])} role={org.role} />;
  } else if (section === "projects") {
    page = <ProjectsPage org={org} />;
  } else if (section === "account") {
    page = <AccountPage user={session.user} canCreateOrg={session.user.isSuperuser || session.signupEnabled} onChanged={onLogout} />;
  } else if (section === "admin" && session.user.isSuperuser) {
    page = <AdminPage />;
  } else if (section === "performance") {
    page = <PerformancePage org={org.slug} search={pageSearch} />;
  } else if (section === "traces" && rest[1]) {
    page = <TracePage org={org.slug} traceId={rest[1]} />;
  } else if (section === "monitors") {
    page = rest[1] ? <MonitorDetailPage org={org.slug} id={rest[1]} role={org.role} /> : <MonitorsPage org={org.slug} role={org.role} project={scoped} />;
  } else if (section === "logs") {
    page = <LogsPage org={org.slug} search={pageSearch} />;
  } else if (section === "releases") {
    page = <ReleasesPage org={org.slug} project={scoped} />;
  } else if (section === "alerts") {
    page = <AlertsPage org={org.slug} />;
  } else if (section === "settings") {
    page = <SettingsPage org={org} tab={rest[1] ?? "members"} project={rest[2]} />;
  } else {
    page = <IssuesPage org={org.slug} search={pageSearch} />;
  }

  const logout = async () => {
    await api.logout();
    if (hostKind()) {
      location.href = mainUrl("/");
      return;
    }
    navigate("/");
    onLogout();
  };

  return (
    <div className="layout">
      <header className="topbar">
        <a className="brand-link" {...linkProps(`${prefix}${projectPrefix}/issues`)}><Brand /></a>
        <nav className="nav">
          <a className={section === "issues" ? "active" : ""} {...linkProps(`${prefix}${projectPrefix}/issues`)}>Issues</a>
          <a className={section === "performance" || section === "traces" ? "active" : ""} {...linkProps(`${prefix}${projectPrefix}/performance`)}>Performance</a>
          <a className={section === "logs" ? "active" : ""} {...linkProps(`${prefix}${projectPrefix}/logs`)}>Logs</a>
          <a className={section === "monitors" ? "active" : ""} {...linkProps(`${prefix}${projectPrefix}/monitors`)}>Monitors</a>
          <a className={section === "releases" ? "active" : ""} {...linkProps(`${prefix}${projectPrefix}/releases`)}>Releases</a>
          <a className={section === "projects" ? "active" : ""} {...linkProps(`${prefix}/projects`)}>Projects</a>
          {isAdmin && <a className={section === "alerts" ? "active" : ""} {...linkProps(`${prefix}/alerts`)}>Alerts</a>}
          <a className={section === "settings" ? "active" : ""} {...linkProps(`${prefix}/settings/members`)}>Settings</a>
        </nav>
        {session.organizations.length > 1 && (
          <select aria-label="Organization" value={org.slug} onChange={(e) => goToOrg(e.target.value, "issues")}>
            {session.organizations.map((o) => <option key={o.id} value={o.slug}>{o.name}</option>)}
          </select>
        )}
        {scoped && (
          <span className="tag project-scope">
            {scoped} <a aria-label="Show all projects" {...linkProps(`${prefix}/${section}`)}>×</a>
          </span>
        )}
        <div className="spacer" />
        {session.user.isSuperuser && <a className={`nav-extra ${section === "admin" ? "active" : ""}`} {...linkProps(`${prefix}/admin`)}>Admin</a>}
        <a className="muted user-email" {...linkProps(`${prefix}/account`)}>{session.user.email}</a>
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
