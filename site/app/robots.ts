import type { MetadataRoute } from "next";
import { SITE } from "@/lib/seo";

export const dynamic = "force-static";

// Everything public is crawlable, including by AI search engines. Account and
// API paths are served by the app, not the website, and have nothing to index.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{ userAgent: "*", allow: "/", disallow: ["/api/", "/login", "/signup", "/invite", "/verify-email", "/reset-password", "/webhooks/"] }],
    sitemap: `${SITE}/sitemap.xml`,
    host: SITE,
  };
}
