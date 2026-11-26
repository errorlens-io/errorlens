import fs from "node:fs";
import path from "node:path";
import { Marked } from "marked";

// Docs are Markdown files in content/docs with a small front matter block:
//
//   ---
//   title: Issues and grouping
//   description: One line for search engines and the docs index.
//   section: Features
//   order: 10
//   ---

const DIR = path.join(process.cwd(), "content", "docs");

export const SECTIONS = ["Getting started", "Sending data", "Features", "Privacy & security", "Team & account", "Self-hosting"];

export type DocMeta = { slug: string; title: string; description: string; section: string; order: number };
export type Doc = DocMeta & { html: string; headings: { id: string; text: string }[] };

function parse(file: string): { meta: Record<string, string>; body: string } {
  const raw = fs.readFileSync(path.join(DIR, file), "utf8");
  const m = raw.match(/^---\n([\s\S]*?)\n---\n?/);
  if (!m) throw new Error(`${file}: missing front matter`);
  const meta: Record<string, string> = {};
  for (const line of m[1].split("\n")) {
    const i = line.indexOf(":");
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return { meta, body: raw.slice(m[0].length) };
}

export function allDocs(): DocMeta[] {
  return fs
    .readdirSync(DIR)
    .filter((f) => f.endsWith(".md"))
    .map((f) => {
      const { meta } = parse(f);
      for (const k of ["title", "description", "section", "order"]) if (!meta[k]) throw new Error(`${f}: missing ${k}`);
      if (!SECTIONS.includes(meta.section)) throw new Error(`${f}: unknown section ${meta.section}`);
      return { slug: f.replace(/\.md$/, ""), title: meta.title, description: meta.description, section: meta.section, order: Number(meta.order) };
    })
    .sort((a, b) => SECTIONS.indexOf(a.section) - SECTIONS.indexOf(b.section) || a.order - b.order);
}

const slugify = (s: string) => s.toLowerCase().replace(/<[^>]+>/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
const escape = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

export function getDoc(slug: string): Doc {
  const meta = allDocs().find((d) => d.slug === slug);
  if (!meta) throw new Error(`no doc ${slug}`);
  const { body } = parse(`${slug}.md`);
  const headings: Doc["headings"] = [];
  const md = new Marked({
    gfm: true,
    renderer: {
      heading({ tokens, depth }) {
        const text = this.parser.parseInline(tokens);
        const id = slugify(text);
        if (depth === 2) headings.push({ id, text: text.replace(/<[^>]+>/g, "") });
        return `<h${depth} id="${id}"><a class="anchor" href="#${id}">${text}</a></h${depth}>\n`;
      },
      image({ href, title, text }) {
        const cap = title ? `<figcaption>${escape(title)}</figcaption>` : "";
        return `<figure class="shot"><img src="${escape(href)}" alt="${escape(text)}" loading="lazy" />${cap}</figure>`;
      },
      link({ href, tokens }) {
        const text = this.parser.parseInline(tokens);
        const external = /^https?:\/\//.test(href) && !href.startsWith("https://errorlens.io");
        return `<a href="${escape(href)}"${external ? ' rel="noopener" target="_blank"' : ""}>${text}</a>`;
      },
      code({ text, lang }) {
        return `<pre class="code"${lang ? ` data-lang="${escape(lang)}"` : ""}><code>${escape(text)}</code></pre>`;
      },
    },
  });
  const html = md.parse(body, { async: false }) as string;
  return { ...meta, html, headings };
}
