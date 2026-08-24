import { requireAdmin } from "@/lib/data";

export default async function AdminLayout({ children }: { children: React.ReactNode }) {
  await requireAdmin();
  return children;
}
