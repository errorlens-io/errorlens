import type { MetadataRoute } from "next";
import { allDocs } from "@/lib/docs";
import { LANDINGS } from "@/lib/landing";
import { SITE } from "@/lib/seo";

export const dynamic = "force-static";

export default function sitemap(): MetadataRoute.Sitemap {
  const now = new Date();
  const entry = (path: string, priority: number, changeFrequency: "weekly" | "monthly" | "yearly" = "monthly") =>
    ({ url: `${SITE}${path}`, lastModified: now, changeFrequency, priority });
  return [
    entry("/", 1, "weekly"),
    ...LANDINGS.map((l) => entry(l.path, l.group === "framework" ? 0.8 : 0.9)),
    entry("/docs/", 0.8, "weekly"),
    ...allDocs().map((d) => entry(`/docs/${d.slug}/`, 0.7)),
    entry("/terms/", 0.2, "yearly"),
    entry("/privacy/", 0.2, "yearly"),
  ];
}
