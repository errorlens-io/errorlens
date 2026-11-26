import type { Metadata } from "next";
import { LandingPage } from "@/components/LandingPage";
import { FRAMEWORKS, landingByPath } from "@/lib/landing";
import { pageMeta } from "@/lib/seo";

export const dynamicParams = false;

export function generateStaticParams() {
  return FRAMEWORKS.map((f) => ({ framework: f.path.split("/")[2] }));
}

export async function generateMetadata({ params }: { params: Promise<{ framework: string }> }): Promise<Metadata> {
  const l = landingByPath(`/error-tracking/${(await params).framework}/`)!;
  return pageMeta({ title: l.title, description: l.description, path: l.path });
}

export default async function Page({ params }: { params: Promise<{ framework: string }> }) {
  return <LandingPage landing={landingByPath(`/error-tracking/${(await params).framework}/`)!} />;
}
