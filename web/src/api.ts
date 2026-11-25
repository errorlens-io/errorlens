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


export type ProjectKey = { id: string; label: string; dsn: string; active: boolean; createdAt: string };
export type ProjectSettings = {
  id: string; name: string; slug: string; platform: string; retentionDays: number;
  scrubDefaults: boolean; scrubIp: "keep" | "truncate" | "drop"; sensitiveFields: string[]; safeFields: string[];
  keys: ProjectKey[];
};
export type AlertChannel = { id: string; kind: "email" | "slack" | "discord" | "webhook"; name: string; target: string; hasSecret: boolean; enabled: boolean; createdAt: string };
export type AlertRule = {
  id: string; name: string; project: string | null; trigger: "new_issue" | "regression" | "frequency";
  threshold: number; windowMinutes: number; minLevel: string; environment: string; channelIds: string[];
  cooldownMinutes: number; enabled: boolean;
};
export type AlertHistory = { id: number; rule: string | null; issueId: number | null; issueTitle: string | null; channel: string | null; status: string; error: string; createdAt: string };
export type Member = { userId: string; email: string; name: string; role: string; joinedAt: string };
export type Invitation = { email: string; role: string; createdAt: string; expiresAt: string };
export type AuditEntry = { id: number; actorEmail: string | null; action: string; target: string; ip: string; createdAt: string };

export type LogLine = { id: number; timestamp: string; project: string; severity: string; body: string; traceId: string; release: string; environment: string; attributes: Record<string, string> };
export type Release = { version: string; project: string; firstEvent: string | null; lastEvent: string | null; createdAt: string; newIssues: number; files: number };
export type APIToken = { id: string; name: string; prefix: string; role: string; createdAt: string; lastUsedAt: string | null };

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

  projectSettings: (org: string, p: string) => request<ProjectSettings>("GET", `/organizations/${org}/projects/${p}/`),
  saveProjectSettings: (org: string, p: string, b: Omit<ProjectSettings, "id" | "slug" | "keys">) =>
    request<{ ok: boolean }>("PUT", `/organizations/${org}/projects/${p}/`, b),
  deleteProject: (org: string, p: string) => request<void>("DELETE", `/organizations/${org}/projects/${p}/`),
  createKey: (org: string, p: string, label: string) => request<{ ok: boolean }>("POST", `/organizations/${org}/projects/${p}/keys`, { label }),
  setKeyActive: (org: string, p: string, id: string, active: boolean) =>
    request<{ ok: boolean }>("PUT", `/organizations/${org}/projects/${p}/keys/${id}`, { active }),

  channels: (org: string) => request<AlertChannel[]>("GET", `/organizations/${org}/alerts/channels`),
  createChannel: (org: string, b: { kind: string; name: string; url?: string; secret?: string; emails?: string[] }) =>
    request<{ id: string }>("POST", `/organizations/${org}/alerts/channels`, b),
  deleteChannel: (org: string, id: string) => request<void>("DELETE", `/organizations/${org}/alerts/channels/${id}`),
  testChannel: (org: string, id: string) => request<{ ok: boolean }>("POST", `/organizations/${org}/alerts/channels/${id}/test`, {}),
  rules: (org: string) => request<AlertRule[]>("GET", `/organizations/${org}/alerts/rules`),
  saveRule: (org: string, b: Partial<AlertRule> & { project?: string }, id?: string) =>
    request<{ id: string }>(id ? "PUT" : "POST", `/organizations/${org}/alerts/rules${id ? `/${id}` : ""}`, b),
  deleteRule: (org: string, id: string) => request<void>("DELETE", `/organizations/${org}/alerts/rules/${id}`),
  alertHistory: (org: string) => request<AlertHistory[]>("GET", `/organizations/${org}/alerts/history`),

  members: (org: string) => request<{ members: Member[]; invitations: Invitation[] }>("GET", `/organizations/${org}/members`),
  invite: (org: string, email: string, role: string) =>
    request<{ added?: boolean; invited?: boolean; emailed?: boolean; link?: string }>("POST", `/organizations/${org}/members`, { email, role }),
  revokeInvite: (org: string, email: string) => request<void>("DELETE", `/organizations/${org}/invitations/${encodeURIComponent(email)}`),
  setMemberRole: (org: string, userId: string, role: string) => request<void>("PUT", `/organizations/${org}/members/${userId}`, { role }),
  removeMember: (org: string, userId: string) => request<void>("DELETE", `/organizations/${org}/members/${userId}`),
  auditLog: (org: string) => request<AuditEntry[]>("GET", `/organizations/${org}/audit-log`),

  logs: (org: string, p: { project?: string; severity?: string; query?: string; before?: string; after?: string; trace?: string }) =>
    request<LogLine[]>("GET", `/organizations/${org}/logs${qs(p)}`),
  releases: (org: string, project?: string) => request<Release[]>("GET", `/organizations/${org}/releases${qs({ project })}`),
  tokens: (org: string) => request<APIToken[]>("GET", `/organizations/${org}/tokens`),
  createToken: (org: string, name: string, role: string) => request<{ id: string; token: string }>("POST", `/organizations/${org}/tokens`, { name, role }),
  deleteToken: (org: string, id: string) => request<void>("DELETE", `/organizations/${org}/tokens/${id}`),

  inviteInfo: (token: string) =>
    request<{ organization: string; email: string; role: string; accountExists: boolean }>("GET", `/invitations/${token}`),
  acceptInvite: (token: string, name: string, password: string) =>
    request<{ organization: string }>("POST", `/invitations/${token}/accept`, { name, password }),
};
