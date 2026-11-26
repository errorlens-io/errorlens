// Site-wide SEO constants and schema.org (JSON-LD) builders.
import type { Metadata } from "next";

export const SITE = "https://errorlens.io";
export const NAME = "ErrorLens";
export const TAGLINE = "Open-source error tracking, performance monitoring and logs";
export const SAME_AS = [
  "https://github.com/errorlenshq",
  "https://github.com/errorlenshq/errorlens",
  "https://pypi.org/project/errorlens/",
  "https://www.npmjs.com/package/@errorlenshq/node",
  "https://www.npmjs.com/package/@errorlenshq/browser",
];

export const abs = (path: string) => (path.startsWith("http") ? path : `${SITE}${path}`);

/** Title, description, canonical URL and social cards for one page. */
export function pageMeta(o: { title: string; description: string; path: string; absoluteTitle?: boolean; type?: "website" | "article" }): Metadata {
  return {
    title: o.absoluteTitle ? { absolute: o.title } : o.title,
    description: o.description,
    alternates: { canonical: o.path },
    openGraph: { title: o.title, description: o.description, url: o.path, type: o.type || "website", siteName: NAME, images: [{ url: "/og.png", width: 1280, height: 440, alt: NAME }] },
    twitter: { card: "summary_large_image", title: o.title, description: o.description, images: ["/og.png"] },
  };
}

type Json = Record<string, unknown>;

export const organization = (): Json => ({
  "@type": "Organization",
  "@id": `${SITE}/#organization`,
  name: NAME,
  url: SITE,
  logo: { "@type": "ImageObject", url: abs("/apple-touch-icon.png"), width: 180, height: 180 },
  sameAs: SAME_AS,
  email: "hello@errorlens.io",
});

export const website = (): Json => ({
  "@type": "WebSite",
  "@id": `${SITE}/#website`,
  url: SITE,
  name: NAME,
  description: TAGLINE,
  publisher: { "@id": `${SITE}/#organization` },
  inLanguage: "en",
});

export const software = (): Json => ({
  "@type": "SoftwareApplication",
  "@id": `${SITE}/#software`,
  name: NAME,
  url: SITE,
  description:
    "Open-source error tracking, performance monitoring (APM), log management, cron and uptime monitoring. Self-hosted on Postgres or hosted at errorlens.io.",
  applicationCategory: "DeveloperApplication",
  applicationSubCategory: "Error tracking and application monitoring",
  operatingSystem: "Linux, macOS, Windows (Docker)",
  softwareHelp: abs("/docs/"),
  license: "https://www.gnu.org/licenses/agpl-3.0.html",
  isAccessibleForFree: true,
  image: abs("/og.png"),
  screenshot: abs("/screenshots/issues.webp"),
  featureList: [
    "Error tracking with issue grouping and stack traces",
    "Performance monitoring and distributed tracing",
    "Log management with search and live tail",
    "Cron job and uptime monitoring",
    "Alerts to email, Slack, Discord and webhooks",
    "OpenTelemetry (OTLP) ingestion",
    "Server-side scrubbing of secrets and personal data",
  ],
  offers: [
    { "@type": "Offer", name: "Free (hosted)", price: "0", priceCurrency: "USD", description: "5,000 error events per month" },
    { "@type": "Offer", name: "Self-hosted", price: "0", priceCurrency: "USD", description: "Unlimited, on your own servers" },
  ],
  publisher: { "@id": `${SITE}/#organization` },
});

export const breadcrumbs = (items: { name: string; path: string }[]): Json => ({
  "@type": "BreadcrumbList",
  itemListElement: items.map((it, i) => ({ "@type": "ListItem", position: i + 1, name: it.name, item: abs(it.path) })),
});

const strip = (html: string) => html.replace(/<[^>]+>/g, "").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&amp;/g, "&");

export const faqPage = (faq: { q: string; a: string }[]): Json => ({
  "@type": "FAQPage",
  mainEntity: faq.map((f) => ({ "@type": "Question", name: f.q, acceptedAnswer: { "@type": "Answer", text: strip(f.a) } })),
});

export const article = (o: { title: string; description: string; path: string; section?: string }): Json => ({
  "@type": "TechArticle",
  headline: o.title,
  description: o.description,
  url: abs(o.path),
  mainEntityOfPage: abs(o.path),
  articleSection: o.section,
  inLanguage: "en",
  author: { "@id": `${SITE}/#organization` },
  publisher: { "@id": `${SITE}/#organization` },
  image: abs("/og.png"),
});

export const webPage = (o: { title: string; description: string; path: string }): Json => ({
  "@type": "WebPage",
  "@id": `${abs(o.path)}#webpage`,
  url: abs(o.path),
  name: o.title,
  description: o.description,
  isPartOf: { "@id": `${SITE}/#website` },
  about: { "@id": `${SITE}/#software` },
  inLanguage: "en",
});
