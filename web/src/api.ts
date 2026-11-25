// Thin client for RootCause's JSON API. Cookies carry the session.

export type User = { id: string; email: string; name: string; isSuperuser: boolean };
export type Organization = { id: string; name: string; slug: string; role: string };
export type Project = {
  id: string;
  name: string;
  slug: string;
  platform: string;
  dsn: string;
  createdAt: string;
};

export type IssueStatus = "unresolved" | "resolved" | "ignored";
export type Issue = {
  id: number;
  project: string;
  title: string;
  culprit: string;
  level: string;
  platform: string;
  status: IssueStatus;
  firstSeen: string;
  lastSeen: string;
  count: number;
  firstRelease: string;
  lastRelease: string;
  regressedAt: string | null;
  trend: number[];
};
export type TagSummary = { key: string; total: number; values: { value: string; count: number }[] };

export type EventSummary = {
  id: string;
  timestamp: string;
  receivedAt: string;
  level: string;
  title: string;
  release: string;
  environment: string;
  tags: Record<string, string>;
  scrubbed: number;
};
// Raw SDK event payload; shape varies by platform, so access defensively.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Payload = Record<string, any>;
export type EventDetail = EventSummary & { payload: Payload; prev: string | null; next: string | null };

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/0${path}`, {
    method,
    credentials: "same-origin",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, data.error ?? `Request failed (${res.status})`);
  return data as T;
}

const qs = (params: Record<string, string | number | undefined>) => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : "";
};

export const api = {
  setupStatus: () => request<{ needsSetup: boolean }>("GET", "/setup/status"),
  setup: (b: { name: string; email: string; password: string; organizationName: string }) =>
    request<{ user: User; organization: Organization }>("POST", "/setup", b),
  login: (email: string, password: string) => request<void>("POST", "/auth/login", { email, password }),
  logout: () => request<void>("POST", "/auth/logout", {}),
  me: () => request<{ user: User; organizations: Organization[] }>("GET", "/auth/me"),
  projects: (org: string) => request<Project[]>("GET", `/organizations/${org}/projects`),
  createProject: (org: string, name: string, platform: string) =>
    request<Project>("POST", `/organizations/${org}/projects`, { name, platform }),

  issues: (org: string, p: { status?: string; project?: string; query?: string; sort?: string; offset?: number }) =>
    request<{ issues: Issue[]; total: number }>("GET", `/organizations/${org}/issues${qs(p)}`),
  issue: (org: string, id: number) =>
    request<{ issue: Issue; tags: TagSummary[] }>("GET", `/organizations/${org}/issues/${id}`),
  setIssueStatus: (org: string, id: number, status: IssueStatus) =>
    request<{ id: number; status: IssueStatus }>("PUT", `/organizations/${org}/issues/${id}`, { status }),
  issueEvents: (org: string, id: number, before?: string) =>
    request<EventSummary[]>("GET", `/organizations/${org}/issues/${id}/events${qs({ before })}`),
  event: (org: string, issueId: number, eventId: string) =>
    request<EventDetail>("GET", `/organizations/${org}/issues/${issueId}/events/${eventId}`),
};
