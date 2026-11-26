import type { Metadata, Viewport } from "next";
import Link from "next/link";
import { JsonLd } from "@/components/JsonLd";
import { FRAMEWORKS, PRODUCT_PAGES } from "@/lib/landing";
import { NAME, SITE, organization, website } from "@/lib/seo";
import "./globals.css";

export const metadata: Metadata = {
  metadataBase: new URL(SITE),
  title: { default: "ErrorLens — Open-Source Error Tracking & Monitoring", template: "%s | ErrorLens" },
  description:
    "Open-source error tracking, performance monitoring, logs, cron and uptime monitoring. Secrets scrubbed before storage. Free hosted plan or self-host on Postgres.",
  applicationName: NAME,
  creator: NAME,
  publisher: NAME,
  category: "technology",
  icons: { icon: "/favicon.svg", apple: "/apple-touch-icon.png" },
  openGraph: { siteName: NAME, type: "website", locale: "en_US", images: [{ url: "/og.png", width: 1280, height: 440, alt: NAME }] },
  twitter: { card: "summary_large_image", images: ["/og.png"] },
  robots: { index: true, follow: true, googleBot: { index: true, follow: true, "max-image-preview": "large", "max-snippet": -1, "max-video-preview": -1 } },
  formatDetection: { telephone: false, email: false, address: false },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: dark)", color: "#0b0f19" },
    { media: "(prefers-color-scheme: light)", color: "#ffffff" },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <JsonLd graph={[organization(), website()]} />
        <a href="#main" className="skip">Skip to content</a>
        <header className="site-header">
          <div className="wrap header-row">
            <Link href="/" className="logo" aria-label="ErrorLens home">
              <img src="/favicon.svg" alt="" width={26} height={26} />
              <span>Error<span className="brand-lens">Lens</span></span>
            </Link>
            <nav className="main-nav" aria-label="Main">
              <Link href="/error-tracking/">Error tracking</Link>
              <Link href="/apm/">Performance</Link>
              <Link href="/docs/">Docs</Link>
              <Link href="/pricing/">Pricing</Link>
              <a href="https://github.com/errorlenshq/errorlens" rel="noopener">GitHub</a>
            </nav>
            <div className="auth-links">
              <a href="/login" className="btn ghost" rel="nofollow">Sign in</a>
              <a href="/signup" className="btn" rel="nofollow">Sign up</a>
            </div>
          </div>
        </header>
        <div id="main">{children}</div>
        <footer className="site-footer">
          <div className="wrap footer-grid">
            <div>
              <div className="logo small">
                <img src="/favicon.svg" alt="" width={20} height={20} />
                <span>Error<span className="brand-lens">Lens</span></span>
              </div>
              <p className="muted">Open-source error tracking, performance monitoring and logs. Your errors, your servers, your eyes only.</p>
            </div>
            <div>
              <h4>Product</h4>
              <Link href="/error-tracking/">Error tracking</Link>
              {PRODUCT_PAGES.map((p) => <Link key={p.path} href={p.path}>{p.nav}</Link>)}
              <Link href="/pricing/">Pricing</Link>
            </div>
            <div>
              <h4>Error tracking for</h4>
              {FRAMEWORKS.map((f) => <Link key={f.path} href={f.path}>{f.nav}</Link>)}
            </div>
            <div>
              <h4>Docs</h4>
              <Link href="/docs/quickstart/">Quick start</Link>
              <Link href="/docs/python/">Python SDK</Link>
              <Link href="/docs/javascript-node/">Node.js SDK</Link>
              <Link href="/docs/javascript-browser/">Browser SDK</Link>
              <Link href="/docs/opentelemetry/">OpenTelemetry</Link>
              <Link href="/docs/self-hosting/">Self-hosting</Link>
              <Link href="/docs/data-scrubbing/">Data scrubbing</Link>
            </div>
            <div>
              <h4>Company</h4>
              <a href="https://github.com/errorlenshq" rel="noopener">GitHub</a>
              <a href="mailto:hello@errorlens.io">Contact</a>
              <Link href="/terms/">Terms of Service</Link>
              <Link href="/privacy/">Privacy Policy</Link>
              <a href="mailto:security@errorlens.io">Report a vulnerability</a>
            </div>
          </div>
          <div className="wrap legal-line muted">© {new Date().getFullYear()} ErrorLens. Server licensed AGPL-3.0, SDKs MIT.</div>
        </footer>
      </body>
    </html>
  );
}
