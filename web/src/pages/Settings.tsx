import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, AuditEntry, Invitation, Member, Organization, Project, ProjectSettings } from "../api";
import { linkProps, navigate } from "../router";
import { TimeAgo } from "../ui";
import { TokensSection } from "./Tokens";
import { BillingSection } from "./Billing";

const PLATFORMS = ["javascript", "node", "python", "go", "java", "php", "ruby", "dotnet", "rust", "react-native", "flutter", "android", "apple", "other"];

export function SettingsPage({ org, tab, project }: { org: Organization; tab: string; project?: string }) {
  const isAdmin = org.role === "owner" || org.role === "admin";
  const tabs = [["members", "Members"], ["projects", "Projects"], ...(isAdmin ? [["tokens", "API tokens"]] as const : []), ...(org.role === "owner" ? [["billing", "Usage & billing"]] as const : []), ["audit", "Audit log"]] as const;
  return (
    <>
      <div className="page-head"><h1>Settings</h1></div>
      <div className="segmented" role="tablist">
        {tabs.map(([t, label]) => (
          <a key={t} role="tab" aria-selected={tab === t} className={`seg ${tab === t ? "on" : ""}`} {...linkProps(`/${org.slug}/settings/${t}`)}>{label}</a>
        ))}
      </div>
      {tab === "projects" ? (project ? <ProjectSettingsForm org={org.slug} slug={project} /> : <ProjectPicker org={org.slug} />) :
        tab === "audit" ? <AuditLog org={org.slug} /> : tab === "tokens" ? <TokensSection org={org.slug} /> : tab === "billing" ? <BillingSection org={org.slug} /> : <Members org={org} />}
    </>
  );
}

function ProjectPicker({ org }: { org: string }) {
  const [projects, setProjects] = useState<Project[]>([]);
  useEffect(() => { api.projects(org).then(setProjects).catch(() => {}); }, [org]);
  return (
    <section className="card">
      <h2>Choose a project</h2>
      {projects.length === 0 && <p className="muted">No projects.</p>}
      <ul className="rows">
        {projects.map((p) => (
          <li key={p.id} className="row-item">
            <a className="link" {...linkProps(`/${org}/settings/projects/${p.slug}`)}><strong>{p.name}</strong></a>
            <span className="badge">{p.platform}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function ProjectSettingsForm({ org, slug }: { org: string; slug: string }) {
  const [ps, setPs] = useState<ProjectSettings | null>(null);
  const [sensitive, setSensitive] = useState("");
  const [safe, setSafe] = useState("");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(() => {
    api.projectSettings(org, slug).then((p) => {
      setPs(p);
      setSensitive(p.sensitiveFields.join(", "));
      setSafe(p.safeFields.join(", "));
    }).catch((e) => setError(e.message));
  }, [org, slug]);
  useEffect(load, [load]);

  if (!ps) return error ? <p className="error">{error}</p> : <p className="muted">Loading…</p>;
  const list = (s: string) => s.split(",").map((x) => x.trim()).filter(Boolean);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setMsg(""); setError("");
    try {
      await api.saveProjectSettings(org, slug, {
        name: ps.name, platform: ps.platform, retentionDays: ps.retentionDays, scrubDefaults: ps.scrubDefaults,
        scrubIp: ps.scrubIp, sensitiveFields: list(sensitive), safeFields: list(safe),
      });
      setMsg("Saved.");
    } catch (err) { setError((err as Error).message); }
  };

  return (
    <>
      <a className="back muted" {...linkProps(`/${org}/settings/projects`)}>← All projects</a>
      <form className="card form-grid" onSubmit={save}>
        <h2>General</h2>
        <label>Name<input value={ps.name} onChange={(e) => setPs({ ...ps, name: e.target.value })} required /></label>
        <label>Platform
          <select value={ps.platform} onChange={(e) => setPs({ ...ps, platform: e.target.value })}>
            {PLATFORMS.map((p) => <option key={p}>{p}</option>)}
          </select>
        </label>
        <label>Keep events for (days)
          <input type="number" min={1} max={3650} value={ps.retentionDays} onChange={(e) => setPs({ ...ps, retentionDays: Number(e.target.value) })} />
        </label>

        <h2>Privacy</h2>
        <p className="muted small">These rules run on the server before anything is stored. Filtered values never reach the database.</p>
        <label className="inline">
          <input type="checkbox" checked={ps.scrubDefaults} onChange={(e) => setPs({ ...ps, scrubDefaults: e.target.checked })} />
          Remove passwords, tokens, API keys, cookies, card numbers and other secrets (recommended)
        </label>
        <label>User IP addresses
          <select value={ps.scrubIp} onChange={(e) => setPs({ ...ps, scrubIp: e.target.value as ProjectSettings["scrubIp"] })}>
            <option value="truncate">Truncate (203.0.113.0) — recommended</option>
            <option value="drop">Don't store</option>
            <option value="keep">Store in full</option>
          </select>
        </label>
        <label>Also remove these fields
          <input placeholder="e.g. national_id, date_of_birth" value={sensitive} onChange={(e) => setSensitive(e.target.value)} />
        </label>
        <label>Never remove these fields
          <input placeholder="e.g. session_count" value={safe} onChange={(e) => setSafe(e.target.value)} />
        </label>
        <div className="row">
          <button>Save changes</button>
          {msg && <span className="ok-text">{msg}</span>}
          {error && <span className="error">{error}</span>}
        </div>
      </form>

      <section className="card">
        <h2>Client keys (DSN)</h2>
        <ul className="rows">
          {ps.keys.map((k) => (
            <li key={k.id} className="row-item wrap">
              <strong>{k.label}</strong>
              {!k.active && <span className="badge warn">disabled</span>}
              <code className="mono ellipsis grow">{k.dsn}</code>
              <button className="ghost" onClick={async () => { await api.setKeyActive(org, slug, k.id, !k.active); load(); }}>
                {k.active ? "Disable" : "Enable"}
              </button>
            </li>
          ))}
        </ul>
        <button className="ghost" onClick={async () => { await api.createKey(org, slug, ""); load(); }}>Generate new key</button>
      </section>

      <section className="card danger-zone">
        <h2>Danger zone</h2>
        <p className="muted small">Deleting a project removes all its issues and events permanently.</p>
        <button className="danger-btn" onClick={async () => {
          if (!confirm(`Delete ${ps.name} and all its data? This cannot be undone.`)) return;
          await api.deleteProject(org, slug);
          navigate(`/${org}/settings/projects`);
        }}>Delete project</button>
      </section>
    </>
  );
}

function Members({ org }: { org: Organization }) {
  const [data, setData] = useState<{ members: Member[]; invitations: Invitation[] } | null>(null);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [link, setLink] = useState("");
  const [error, setError] = useState("");
  const isAdmin = org.role === "owner" || org.role === "admin";
  const load = useCallback(() => { api.members(org.slug).then(setData).catch((e) => setError(e.message)); }, [org.slug]);
  useEffect(load, [load]);

  const act = async (fn: () => Promise<unknown>) => {
    setError("");
    try { await fn(); load(); } catch (e) { setError((e as Error).message); }
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(""); setLink("");
    try {
      const r = await api.invite(org.slug, email, role);
      if (r.link && !r.emailed) setLink(r.link);
      setEmail("");
      load();
    } catch (err) { setError((err as Error).message); }
  };

  return (
    <>
      {isAdmin && (
        <form className="card row" onSubmit={submit}>
          <input type="email" aria-label="Email" placeholder="teammate@example.com" required value={email} onChange={(e) => setEmail(e.target.value)} />
          <select aria-label="Role" value={role} onChange={(e) => setRole(e.target.value)}>
            {org.role === "owner" && <option value="owner">Owner</option>}
            <option value="admin">Admin</option>
            <option value="member">Member</option>
            <option value="viewer">Viewer</option>
          </select>
          <button>Invite</button>
          {link && (
            <p className="small grow">Email isn't configured, so share this invitation link (valid 7 days):<br /><code className="mono">{link}</code></p>
          )}
          {error && <p className="error">{error}</p>}
        </form>
      )}
      <section className="card">
        <h2>Members</h2>
        <ul className="rows">
          {data?.members.map((m) => (
            <li key={m.userId} className="row-item">
              <strong>{m.name || m.email}</strong>
              <span className="muted">{m.email}</span>
              <span className="spacer" />
              {isAdmin ? (
                <>
                  <select aria-label={`Role for ${m.email}`} value={m.role} onChange={(e) => act(() => api.setMemberRole(org.slug, m.userId, e.target.value))}>
                    {["owner", "admin", "member", "viewer"].map((r) => <option key={r} value={r}>{r}</option>)}
                  </select>
                  <button className="ghost danger" onClick={() => confirm(`Remove ${m.email}?`) && act(() => api.removeMember(org.slug, m.userId))}>Remove</button>
                </>
              ) : <span className="badge">{m.role}</span>}
            </li>
          ))}
          {data?.invitations.map((i) => (
            <li key={i.email} className="row-item">
              <span className="muted">{i.email}</span>
              <span className="badge">invited · {i.role}</span>
              <span className="spacer" />
              {isAdmin && <button className="ghost" onClick={() => act(() => api.revokeInvite(org.slug, i.email))}>Revoke</button>}
            </li>
          ))}
        </ul>
        {!isAdmin && error && <p className="error">{error}</p>}
      </section>
    </>
  );
}

function AuditLog({ org }: { org: string }) {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { api.auditLog(org).then(setEntries).catch((e) => setError(e.message)); }, [org]);
  if (error) return <p className="error">{error}</p>;
  return (
    <section className="card">
      <h2>Audit log</h2>
      <p className="muted small">Every sign-in, settings change and every time someone opens a full event is recorded here. Entries can't be edited or deleted.</p>
      <table className="kv">
        <tbody>
          {entries?.map((e) => (
            <tr key={e.id}>
              <td className="nowrap"><TimeAgo iso={e.createdAt} /></td>
              <td>{e.actorEmail ?? "system"}</td>
              <td className="mono">{e.action}</td>
              <td className="mono ellipsis">{e.target}</td>
              <td className="muted">{e.ip}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
