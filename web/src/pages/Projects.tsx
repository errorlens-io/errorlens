import { FormEvent, useEffect, useState } from "react";
import { api, Organization, Project } from "../api";
import { Empty } from "../ui";
import { base, linkProps } from "../router";
import { SetupGuide } from "./Setup";

const PLATFORMS = ["javascript", "node", "python", "go", "java", "php", "ruby", "dotnet", "rust", "react-native", "flutter", "android", "apple", "other"];

export function ProjectsPage({ org }: { org: Organization }) {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [name, setName] = useState("");
  const [platform, setPlatform] = useState("javascript");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const canManage = org.role === "owner" || org.role === "admin";

  useEffect(() => {
    setProjects(null);
    api.projects(org.slug).then(setProjects).catch((e) => setError(e.message));
  }, [org.slug]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const p = await api.createProject(org.slug, name, platform);
      setProjects((ps) => [...(ps ?? []), p].sort((a, b) => a.name.localeCompare(b.name)));
      setName("");
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="page-head">
        <h1>Projects</h1>
      </div>
      <p className="muted">Each project has a DSN. Configure your platform's SDK with it and errors start arriving here.</p>

      {canManage && (
        <form className="card row" onSubmit={submit}>
          <input placeholder="New project name" aria-label="Project name" required value={name} onChange={(e) => setName(e.target.value)} />
          <select aria-label="Platform" value={platform} onChange={(e) => setPlatform(e.target.value)}>
            {PLATFORMS.map((p) => <option key={p}>{p}</option>)}
          </select>
          <button disabled={busy}>Create project</button>
          {error && <p className="error">{error}</p>}
        </form>
      )}

      {projects === null ? (
        <p className="muted">Loading projects…</p>
      ) : projects.length === 0 ? (
        <Empty title="No projects yet">Create one to get a DSN.</Empty>
      ) : (
        <ul className="projects">
          {projects.map((p) => <ProjectCard key={p.id} org={org.slug} project={p} />)}
        </ul>
      )}
    </>
  );
}

function ProjectCard({ org, project }: { org: string; project: Project }) {
  const [copied, setCopied] = useState(false);
  const [open, setOpen] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(project.dsn);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <li className="card">
      <div className="project-head">
        <a className="link" {...linkProps(`${base(org)}/${project.slug}/issues`)}><strong>{project.name}</strong></a>
        <span className="tag">{project.platform}</span>
      </div>
      <div className="dsn">
        <code>{project.dsn || "No active key"}</code>
        {project.dsn && <button className="ghost" onClick={copy}>{copied ? "Copied" : "Copy DSN"}</button>}
        {project.dsn && <button className="ghost" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? "Hide setup" : "Setup guide"}</button>}
      </div>
      {open && project.dsn && <SetupGuide platform={project.platform} dsn={project.dsn} />}
    </li>
  );
}
