// Small shared UI pieces.

export function timeAgo(iso: string): string {
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  const units: [number, string][] = [[60, "min"], [3600, "hr"], [86400, "day"], [2592000, "mo"], [31536000, "yr"]];
  for (let i = units.length - 1; i >= 0; i--) {
    const [size, name] = units[i];
    if (s >= size) {
      const n = Math.floor(s / size);
      return `${n} ${name}${n > 1 && name !== "min" && name !== "hr" ? "s" : ""} ago`;
    }
  }
  return "just now";
}

export function TimeAgo({ iso }: { iso: string }) {
  return <time dateTime={iso} title={new Date(iso).toLocaleString()}>{timeAgo(iso)}</time>;
}

export function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${Math.round(n / 1000)}k`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export function Level({ level }: { level: string }) {
  return <span className={`level level-${level}`} title={level} aria-label={level} />;
}

/** Bar chart of counts; used as a sparkline in lists and a chart on detail pages. */
export function Bars({ data, height = 28, label }: { data: number[]; height?: number; label: string }) {
  const max = Math.max(1, ...data);
  const w = 100 / Math.max(1, data.length);
  return (
    <svg className="bars" viewBox={`0 0 100 ${height}`} preserveAspectRatio="none" height={height} role="img" aria-label={label}>
      {data.map((n, i) => {
        const h = n === 0 ? 0 : Math.max(1.5, (n / max) * height);
        return (
          <rect key={i} x={i * w + w * 0.12} width={w * 0.76} y={height - h} height={h}>
            <title>{n}</title>
          </rect>
        );
      })}
    </svg>
  );
}

export function Empty({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <div className="card empty">
      <strong>{title}</strong>
      {children && <div className="muted">{children}</div>}
    </div>
  );
}
