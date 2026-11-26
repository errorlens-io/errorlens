import { useEffect, useState } from "react";

// Minimal pushState router: routes are matched in App.

export function navigate(to: string) {
  if (to === location.pathname + location.search) return;
  history.pushState(null, "", to);
  window.dispatchEvent(new PopStateEvent("popstate"));
}

export function useLocation() {
  const [loc, setLoc] = useState({ path: location.pathname, search: location.search });
  useEffect(() => {
    const on = () => setLoc({ path: location.pathname, search: location.search });
    window.addEventListener("popstate", on);
    return () => window.removeEventListener("popstate", on);
  }, []);
  return loc;
}

/** Updates query parameters without adding a history entry. */
export function setQuery(params: Record<string, string>) {
  if ("project" in params && projectRoute) {
    const { project, ...rest } = params;
    projectRoute(project, rest);
    return;
  }
  const p = new URLSearchParams(location.search);
  for (const [k, v] of Object.entries(params)) (v ? p.set(k, v) : p.delete(k));
  const s = p.toString();
  history.replaceState(null, "", location.pathname + (s ? `?${s}` : ""));
  window.dispatchEvent(new PopStateEvent("popstate"));
}

/** Intercepts plain left-clicks on internal links. */
export function linkProps(to: string) {
  return {
    href: to,
    onClick: (e: React.MouseEvent) => {
      if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      e.preventDefault();
      navigate(to);
    },
  };
}

// --- tenant mode -------------------------------------------------------------
// In hosted mode every organization has its own subdomain (<org>.<domain>) and
// paths inside it start at the section: /issues, /<project>/logs. Self-hosted
// installs keep /<org>/issues on one host.

let tenantDomain = "";
let publicUrl = "";

export function configureTenant(domain: string, url: string) {
  tenantDomain = domain;
  publicUrl = url.replace(/\/$/, "");
}

export function tenantDomainName() {
  return tenantDomain;
}

/** "apex" on the main domain, "org" on an organization subdomain, "" when not in tenant mode. */
export function hostKind(): "" | "apex" | "org" {
  if (!tenantDomain) return "";
  const h = location.hostname;
  if (h === tenantDomain) return "apex";
  return h.endsWith("." + tenantDomain) ? "org" : "apex";
}

/** The organization slug taken from the subdomain, or "". */
export function tenantOrg(): string {
  return hostKind() === "org" ? location.hostname.slice(0, -(tenantDomain.length + 1)) : "";
}

/** Path prefix for an organization's pages: "" in tenant mode, "/<org>" otherwise. */
export function base(org: string): string {
  return tenantDomain ? "" : `/${org}`;
}

/** Absolute URL of a page in another organization (tenant mode). */
export function orgUrl(org: string, path = "/") {
  return `${location.protocol}//${org}.${tenantDomain}${location.port ? ":" + location.port : ""}${path}`;
}

/** Opens an organization's page, switching subdomain in tenant mode. */
export function goToOrg(org: string, path = "issues") {
  if (tenantDomain && tenantOrg() !== org) {
    location.href = orgUrl(org, `/${path}`);
    return;
  }
  navigate(`${base(org)}/${path}`);
}

/** The main site (sign-in, sign-up). */
export function mainUrl(path = "/") {
  return (publicUrl || location.origin) + path;
}

/** A safe post-login destination: only organization subdomains of our domain. */
export function safeNext(next: string | null): string {
  if (!next || !tenantDomain) return "";
  try {
    const u = new URL(next);
    if (u.protocol === location.protocol && u.hostname.endsWith("." + tenantDomain)) return u.toString();
  } catch {
    /* not a URL */
  }
  return "";
}

// Pages change the project filter through setQuery({project}); on project-scoped
// URLs (/<project>/issues) the shell turns that into a path change instead.
let projectRoute: ((slug: string, rest: Record<string, string>) => void) | null = null;
export function setProjectRoute(fn: typeof projectRoute) {
  projectRoute = fn;
}

/** Page length of lists the server pages (?before= / ?offset=). */
export const PAGE_SIZE = 100;
