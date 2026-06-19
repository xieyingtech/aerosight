import Link from "next/link";

const items = [
  ["管理总览", "/admin"],
  ["用户管理", "/admin/users"],
  ["团队管理", "/admin/teams"],
  ["项目管理", "/admin/projects"]
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid gap-6 md:grid-cols-[220px_1fr]">
      <aside>
        <nav className="space-y-1">
          {items.map(([label, href]) => (
            <Link className="block rounded-md px-3 py-2 text-sm hover:bg-white" href={href} key={href}>
              {label}
            </Link>
          ))}
        </nav>
      </aside>
      <section>{children}</section>
    </div>
  );
}
