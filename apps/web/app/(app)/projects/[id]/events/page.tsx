import { permanentRedirect } from "next/navigation";
import { legacyProjectEventListHref } from "@/lib/project-navigation";

export default async function EventsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  permanentRedirect(legacyProjectEventListHref(Number(id)));
}
