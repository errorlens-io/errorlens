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
};
