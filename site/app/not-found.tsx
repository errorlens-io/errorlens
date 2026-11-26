import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = { title: "Page not found", robots: { index: false, follow: true } };

export default function NotFound() {
  return (
    <main className="legal" style={{ textAlign: "center" }}>
      <h1>Page not found</h1>
      <p className="muted">The page you're looking for doesn't exist or has moved.</p>
      <p>
        <Link href="/">Home</Link> · <Link href="/docs/">Documentation</Link> · <Link href="/error-tracking/">Error tracking</Link> ·{" "}
        <Link href="/pricing/">Pricing</Link>
      </p>
    </main>
  );
}
