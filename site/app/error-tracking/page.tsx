import type { Metadata } from "next";
import Link from "next/link";
import { LandingPage } from "@/components/LandingPage";
import { FRAMEWORKS, landingByPath } from "@/lib/landing";
import { pageMeta } from "@/lib/seo";

const l = landingByPath("/error-tracking/")!;
export const metadata: Metadata = pageMeta({ title: l.title, description: l.description, path: l.path });

export default function Page() {
  return (
    <>
      <LandingPage landing={l} />
      <section className="band frameworks">
        <div className="wrap narrow">
          <h2>Guides by language and framework</h2>
          <div className="grid">
            {FRAMEWORKS.map((f) => (
              <Link key={f.path} href={f.path} className="card related">
                <h3>{f.nav} error tracking</h3>
                <p>{f.description}</p>
              </Link>
            ))}
          </div>
        </div>
      </section>
    </>
  );
}
