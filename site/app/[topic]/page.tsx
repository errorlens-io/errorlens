import type { Metadata } from "next";
import { LandingPage } from "@/components/LandingPage";
import { LANDINGS, landingByPath } from "@/lib/landing";
import { pageMeta } from "@/lib/seo";

const TOPICS = LANDINGS.filter((l) => l.group === "product");

export const dynamicParams = false;

export function generateStaticParams() {
  return TOPICS.map((l) => ({ topic: l.path.slice(1, -1) }));
}

export async function generateMetadata({ params }: { params: Promise<{ topic: string }> }): Promise<Metadata> {
  const l = landingByPath(`/${(await params).topic}/`)!;
  return pageMeta({ title: l.title, description: l.description, path: l.path });
}

export default async function Page({ params }: { params: Promise<{ topic: string }> }) {
  return <LandingPage landing={landingByPath(`/${(await params).topic}/`)!} />;
}
