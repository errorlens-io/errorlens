import type { Metadata } from "next";
import Link from "next/link";
import { allDocs, SECTIONS } from "@/lib/docs";

export const metadata: Metadata = {
  title: "Documentation",
  description: "Guides for every ErrorLens feature: sending errors, performance data and logs, alerts, monitors, privacy settings and self-hosting.",
};

export default function DocsIndex() {
  const docs = allDocs();
  return (
    <article className="prose docs-index">
      <h1>ErrorLens documentation</h1>
      <p className="desc">
        Everything you need to send errors, performance data and logs to ErrorLens, and to make sense of them once they
        arrive. New here? Start with the <Link href="/docs/quickstart/">quick start</Link>.
      </p>
      {SECTIONS.map((s) => (
        <section key={s}>
          <h2>{s}</h2>
          <div className="grid">
            {docs.filter((d) => d.section === s).map((d) => (
              <Link key={d.slug} href={`/docs/${d.slug}/`} className="card" style={{ color: "inherit" }}>
                <h3 style={{ color: "var(--accent)" }}>{d.title}</h3>
                <p>{d.description}</p>
              </Link>
            ))}
          </div>
        </section>
      ))}
    </article>
  );
}
