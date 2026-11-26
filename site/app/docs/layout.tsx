import { DocsNav } from "@/components/DocsNav";
import { allDocs, SECTIONS } from "@/lib/docs";

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const items = allDocs().map(({ slug, title, section }) => ({ slug, title, section }));
  return (
    <div className="docs">
      <DocsNav sections={SECTIONS} items={items} />
      {children}
    </div>
  );
}
