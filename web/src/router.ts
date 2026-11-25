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
