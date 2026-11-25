import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, ApiError, Organization, Project, User } from "./api";

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

  switch (view.kind) {
    case "loading":
      return <div className="center muted">Loading…</div>;
    case "setup":
      return <SetupPage onDone={refresh} />;
    case "login":
      return <LoginPage onDone={refresh} />;
    case "app":
      return <Dashboard session={view.session} onLogout={refresh} />;
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

const PLATFORMS = ["javascript", "node", "python", "go", "java", "php", "ruby", "dotnet", "rust", "react-native", "flutter", "android", "apple", "other"];

function Dashboard({ session, onLogout }: { session: Session; onLogout: () => void }) {
  const [orgSlug, setOrgSlug] = useState(session.organizations[0]?.slug ?? "");
  const org = session.organizations.find((o) => o.slug === orgSlug);
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [name, setName] = useState("");
  const [platform, setPlatform] = useState("javascript");
  const canManage = org?.role === "owner" || org?.role === "admin";

  useEffect(() => {
    if (!orgSlug) return;
    setProjects(null);
    api.projects(orgSlug).then(setProjects);
  }, [orgSlug]);

  const { error, busy, submit } = useSubmit(async () => {
    const p = await api.createProject(orgSlug, name, platform);
    setProjects((ps) => [...(ps ?? []), p].sort((a, b) => a.name.localeCompare(b.name)));
    setName("");
  });

  const logout = async () => {
    await api.logout();
    onLogout();
  };

  return (
    <div className="layout">
      <header className="topbar">
        <Brand />
        {session.organizations.length > 1 && (
          <select value={orgSlug} onChange={(e) => setOrgSlug(e.target.value)}>
            {session.organizations.map((o) => (
              <option key={o.id} value={o.slug}>{o.name}</option>
            ))}
          </select>
        )}
        <div className="spacer" />
        <span className="muted">{session.user.email}</span>
        <button className="ghost" onClick={logout}>Sign out</button>
      </header>

      <main>
        <h1>{org?.name ?? "No organization"}</h1>
        <p className="muted">Projects receive errors from your apps. Point any Sentry SDK at a project's DSN.</p>

        {canManage && (
          <form className="card row" onSubmit={submit}>
            <input placeholder="New project name" required value={name} onChange={(e) => setName(e.target.value)} />
            <select value={platform} onChange={(e) => setPlatform(e.target.value)}>
              {PLATFORMS.map((p) => <option key={p}>{p}</option>)}
            </select>
            <button disabled={busy}>Create project</button>
            {error && <p className="error">{error}</p>}
          </form>
        )}

        {projects === null ? (
          <p className="muted">Loading projects…</p>
        ) : projects.length === 0 ? (
          <div className="card empty">No projects yet. Create one to get a DSN.</div>
        ) : (
          <ul className="projects">
            {projects.map((p) => <ProjectCard key={p.id} project={p} />)}
          </ul>
        )}
      </main>
    </div>
  );
}

function ProjectCard({ project }: { project: Project }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(project.dsn);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <li className="card">
      <div className="project-head">
        <strong>{project.name}</strong>
        <span className="tag">{project.platform}</span>
      </div>
      <div className="dsn">
        <code>{project.dsn || "No active key"}</code>
        {project.dsn && <button className="ghost" onClick={copy}>{copied ? "Copied" : "Copy DSN"}</button>}
      </div>
    </li>
  );
}
