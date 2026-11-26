import type { Metadata } from "next";
import Link from "next/link";
import { allDocs, getDoc } from "@/lib/docs";

export const dynamicParams = false;

export function generateStaticParams() {
  return allDocs().map((d) => ({ slug: d.slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params;
  const d = getDoc(slug);
  return { title: d.title, description: d.description, alternates: { canonical: `/docs/${slug}/` } };
}

export default async function DocPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const doc = getDoc(slug);
  const list = allDocs();
  const i = list.findIndex((d) => d.slug === slug);
  const prev = list[i - 1];
  const next = list[i + 1];
  return (
    <>
      <article className="prose">
        <p className="muted" style={{ margin: "0 0 6px", fontSize: "0.9rem" }}>{doc.section}</p>
        <h1>{doc.title}</h1>
        <p className="desc">{doc.description}</p>
        <div dangerouslySetInnerHTML={{ __html: doc.html }} />
        <nav className="pager" aria-label="Pages">
          {prev ? <Link href={`/docs/${prev.slug}/`}><small>Previous</small>{prev.title}</Link> : <span />}
          {next ? <Link href={`/docs/${next.slug}/`} style={{ textAlign: "right" }}><small>Next</small>{next.title}</Link> : <span />}
        </nav>
      </article>
      <aside className="toc" aria-label="On this page">
        {doc.headings.length > 1 && (
          <>
            <h5>On this page</h5>
            {doc.headings.map((h) => <a key={h.id} href={`#${h.id}`}>{h.text}</a>)}
          </>
        )}
      </aside>
    </>
  );
}
