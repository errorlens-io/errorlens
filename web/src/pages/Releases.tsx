import { useRef } from "react";
import { usePaged } from "../paged";
import { api, Release } from "../api";
import { linkProps, base, PAGE_SIZE } from "../router";
import { Empty, TimeAgo } from "../ui";

export function ReleasesPage({ org, project }: { org: string; project?: string }) {
  const offset = useRef(0);
  const { items: releases, error, pager } = usePaged<Release>(async (after) => {
    offset.current = after ? offset.current + PAGE_SIZE : 0;
    return api.releases(org, project || undefined, offset.current || undefined);
  }, [org, project]);
  return (
    <>
      <div className="page-head"><h1>Releases</h1></div>
      <p className="muted">Releases appear automatically when your SDK sets <code className="mono">release</code>. Upload source maps per release with <code className="mono">errorlens sourcemaps upload</code>.</p>
      {error && <p className="error">{error}</p>}
      {releases === null ? <p className="muted">Loading…</p> : releases.length === 0 ? <Empty title="No releases yet" /> : (
        <div className="card flush">
          <table className="kv release-table">
            <thead><tr><th>Version</th><th>Project</th><th>New issues</th><th>Source files</th><th>Last event</th></tr></thead>
            <tbody>
              {releases.map((r) => (
                <tr key={r.project + r.version}>
                  <td className="mono"><strong>{r.version}</strong></td>
                  <td>{r.project}</td>
                  <td>{r.newIssues > 0 ? <a className="link" {...linkProps(`${base(org)}/issues?project=${r.project}&status=all&query=`)}>{r.newIssues}</a> : 0}</td>
                  <td>{r.files}</td>
                  <td>{r.lastEvent ? <TimeAgo iso={r.lastEvent} /> : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {pager}
    </>
  );
}
