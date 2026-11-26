import { useCallback, useEffect, useRef, useState } from "react";
import { PAGE_SIZE } from "./router";

/**
 * Loads a list one page at a time. `load(after)` fetches the page following
 * the item `after` (undefined for the first page). Responses that arrive
 * after `deps` changed are ignored, and "more" turns false once a page comes
 * back shorter than a full page.
 */
export function usePaged<T>(load: (after: T | undefined) => Promise<T[]>, deps: unknown[], pageSize = PAGE_SIZE) {
  const [items, setItems] = useState<T[] | null>(null);
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const loadRef = useRef(load);
  loadRef.current = load;

  const reload = useCallback(() => {
    const gen = ++generation.current;
    setItems(null);
    setError("");
    loadRef.current(undefined)
      .then((page) => { if (gen === generation.current) { setItems(page); setMore(page.length >= pageSize); } })
      .catch((e) => gen === generation.current && setError((e as Error).message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(reload, [reload]);

  const loadMore = async () => {
    if (!items?.length || loading) return;
    const gen = generation.current;
    setLoading(true);
    try {
      const page = await loadRef.current(items[items.length - 1]);
      if (gen !== generation.current) return;
      setItems([...items, ...page]);
      setMore(page.length >= pageSize);
    } catch (e) {
      if (gen === generation.current) setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const pager = more ? (
    <div className="pager">
      <button className="ghost" disabled={loading} onClick={loadMore}>{loading ? "Loading…" : "Load more"}</button>
    </div>
  ) : null;

  return { items, error, reload, pager };
}
