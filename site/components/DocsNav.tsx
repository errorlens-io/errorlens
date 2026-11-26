"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

type Item = { slug: string; title: string; section: string };

export function DocsNav({ sections, items }: { sections: string[]; items: Item[] }) {
  const path = usePathname();
  return (
    <nav className="docs-nav" aria-label="Documentation">
      <Link href="/docs/" className={path === "/docs/" ? "active" : ""}>Overview</Link>
      {sections.map((s) => (
        <div key={s}>
          <h5>{s}</h5>
          {items.filter((i) => i.section === s).map((i) => {
            const href = `/docs/${i.slug}/`;
            return <Link key={i.slug} href={href} className={path === href ? "active" : ""}>{i.title}</Link>;
          })}
        </div>
      ))}
    </nav>
  );
}
