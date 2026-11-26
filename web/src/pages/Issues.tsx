import { useEffect, useState } from "react";
import { api, Issue, Project } from "../api";
import { linkProps, setQuery, base } from "../router";
import { Bars, Empty, formatCount, Level, TimeAgo } from "../ui";

const STATUSES = [
  ["unresolved", "Unresolved"],
  ["resolved", "Resolved"],
  ["ignored", "Ignored"],
  ["all", "All"],
] as const;

export function IssuesPage({ org, search }: { org: string; search: string }) {
  const params = new URLSearchParams(search);
  const status = params.get("status") || "unresolved";
  const project = params.get("project") || "";
  const sort = params.get("sort") || "last_seen";
  const query = params.get("query") || "";
  const offset = Number(params.get("offset") || 0);

  const [projects, setProjects] = useState<Project[]>([]);
  const [data, setData] = useState<{ issues: Issue[]; total: number; totalCapped?: boolean } | null>(null);
  const [error, setError] = useState("");
  const [text, setText] = useState(query);

  useEffect(() => {
    api.projects(org).then(setProjects).catch(() => {});
  }, [org]);

  useEffect(() => {
    let live = true;
    setError("");
    api
      .issues(org, { status, project, sort, query, offset })
      .then((d) => live && setData(d))
      .catch((e) => live && setError(e.message));
    return () => {
      live = false;
    };
  }, [org, status, project, sort, query, offset]);

  // Debounced search box.
  useEffect(() => {
    const t = setTimeout(() => text !== query && setQuery({ query: text, offset: "" }), 300);
    return () => clearTimeout(t);
  }, [text, query]);

  const pageSize = 50;
  return (
    <>
      <div className="page-head">
        <h1>Issues</h1>
      </div>

      <div className="filters">
        <div className="segmented" role="tablist">
          {STATUSES.map(([v, label]) => (
            <button key={v} role="tab" aria-selected={status === v} className={status === v ? "on" : ""}
              onClick={() => setQuery({ status: v === "unresolved" ? "" : v, offset: "" })}>
              {label}
            </button>
          ))}
        </div>
        <select aria-label="Project" value={project} onChange={(e) => setQuery({ project: e.target.value, offset: "" })}>
          <option value="">All projects</option>
          {projects.map((p) => <option key={p.id} value={p.slug}>{p.name}</option>)}
        </select>
        <select aria-label="Sort" value={sort} onChange={(e) => setQuery({ sort: e.target.value === "last_seen" ? "" : e.target.value })}>
          <option value="last_seen">Last seen</option>
          <option value="first_seen">First seen</option>
          <option value="count">Events</option>
        </select>
        <input className="search" type="search" placeholder="Search title or location" value={text} onChange={(e) => setText(e.target.value)} />
      </div>

      {error && <p className="error">{error}</p>}
      {data === null ? (
        <p className="muted">Loading issues…</p>
      ) : data.issues.length === 0 ? (
        projects.length === 0 ? (
          <Empty title="No projects yet">Create a project to get a DSN, then send your first error.</Empty>
        ) : (
          <Empty title={status === "unresolved" && !query ? "No unresolved issues 🎉" : "No matching issues"}>
            {status === "unresolved" && !query ? "Errors your apps send will show up here." : "Try a different filter."}
          </Empty>
        )
      ) : (
        <>
          <div className="issue-list card flush">
            <div className="issue-row head">
              <span>Issue</span>
              <span className="col-trend">Last 24h</span>
              <span className="col-num">Events</span>
            </div>
            {data.issues.map((is) => (
              <a key={is.id} className="issue-row" {...linkProps(`${base(org)}/issues/${is.id}`)}>
                <div className="issue-main">
                  <div className="issue-title">
                    <Level level={is.level} />
                    <strong>{is.title}</strong>
                    {is.regressedAt && is.status === "unresolved" && <span className="badge warn">Regressed</span>}
                    {is.status !== "unresolved" && <span className="badge">{is.status}</span>}
                  </div>
                  <div className="issue-meta muted">
                    <span className="mono">{is.culprit || "—"}</span>
                    <span>{is.project}</span>
                    <span>Last <TimeAgo iso={is.lastSeen} /></span>
                    <span>First <TimeAgo iso={is.firstSeen} /></span>
                  </div>
                </div>
                <div className="col-trend"><Bars data={is.trend} label="Events per hour, last 24 hours" /></div>
                <div className="col-num">{formatCount(is.count)}</div>
              </a>
            ))}
          </div>
          {data.total > pageSize && (
            <div className="pager">
              <span className="muted">{offset + 1}–{Math.min(offset + pageSize, data.total)} of {data.totalCapped ? `${data.total.toLocaleString()}+` : data.total.toLocaleString()}</span>
              <button className="ghost" disabled={offset === 0} onClick={() => setQuery({ offset: String(Math.max(0, offset - pageSize)) })}>Previous</button>
              <button className="ghost" disabled={offset + pageSize >= data.total} onClick={() => setQuery({ offset: String(offset + pageSize) })}>Next</button>
            </div>
          )}
        </>
      )}
    </>
  );
}
