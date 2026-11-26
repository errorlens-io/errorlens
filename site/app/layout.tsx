import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  metadataBase: new URL("https://errorlens.io"),
  title: { default: "ErrorLens — private error tracking, performance and logs", template: "%s · ErrorLens" },
  description:
    "Open-source error tracking, performance monitoring, logs and uptime checks. Secrets are scrubbed before anything is stored. Use our hosted service or run it on your own servers.",
  icons: { icon: "/favicon.svg" },
  openGraph: { siteName: "ErrorLens", type: "website" },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <header className="site-header">
          <div className="wrap header-row">
            <Link href="/" className="logo">
              <img src="/favicon.svg" alt="" width={26} height={26} />
              <span>ErrorLens</span>
            </Link>
            <nav className="main-nav" aria-label="Main">
              <Link href="/docs/">Docs</Link>
              <Link href="/#features">Features</Link>
              <Link href="/#pricing">Pricing</Link>
              <a href="https://github.com/errorlenshq/errorlens" rel="noopener">GitHub</a>
            </nav>
            <div className="auth-links">
              <a href="/login" className="btn ghost">Sign in</a>
              <a href="/signup" className="btn">Sign up</a>
            </div>
          </div>
        </header>
        {children}
        <footer className="site-footer">
          <div className="wrap footer-grid">
            <div>
              <div className="logo small">
                <img src="/favicon.svg" alt="" width={20} height={20} />
                <span>ErrorLens</span>
              </div>
              <p className="muted">Your errors, your servers, your eyes only.</p>
            </div>
            <div>
              <h4>Product</h4>
              <Link href="/#features">Features</Link>
              <Link href="/#pricing">Pricing</Link>
              <Link href="/docs/self-hosting/">Self-hosting</Link>
            </div>
            <div>
              <h4>Docs</h4>
              <Link href="/docs/quickstart/">Quick start</Link>
              <Link href="/docs/python/">Python</Link>
              <Link href="/docs/javascript-node/">Node.js</Link>
              <Link href="/docs/opentelemetry/">OpenTelemetry</Link>
            </div>
            <div>
              <h4>Legal</h4>
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
