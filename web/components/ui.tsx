import Link from "next/link";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Button({
  children,
  className,
  href,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  href?: string;
  children: ReactNode;
}) {
  const classes = cn(
    "inline-flex h-9 items-center justify-center gap-2 rounded-md bg-sky-700 px-3 text-sm font-medium text-white shadow-sm hover:bg-sky-800 disabled:opacity-50",
    className
  );
  if (href) {
    return (
      <Link className={classes} href={href}>
        {children}
      </Link>
    );
  }
  return (
    <button className={classes} {...props}>
      {children}
    </button>
  );
}

export function SecondaryButton({
  children,
  href,
  className
}: {
  children: ReactNode;
  href: string;
  className?: string;
}) {
  return (
    <Link
      className={cn(
        "inline-flex h-9 items-center justify-center rounded-md border border-slate-300 bg-white px-3 text-sm font-medium text-slate-700 hover:bg-slate-50",
        className
      )}
      href={href}
    >
      {children}
    </Link>
  );
}

export function Page({
  title,
  description,
  actions,
  children
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-slate-200 pb-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-950">{title}</h1>
          {description ? <p className="mt-1 text-sm text-slate-500">{description}</p> : null}
        </div>
        {actions}
      </header>
      {children}
    </section>
  );
}

export function DataTable<T extends Record<string, unknown>>({
  items,
  columns
}: {
  items: T[];
  columns: { key: keyof T; label: string; render?: (item: T) => ReactNode }[];
}) {
  return (
    <div className="overflow-hidden rounded-md border border-slate-200 bg-white">
      <table className="w-full border-collapse text-left text-sm">
        <thead className="bg-slate-50 text-slate-500">
          <tr>
            {columns.map((column) => (
              <th className="px-4 py-3 font-medium" key={String(column.key)}>
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <tr className="border-t border-slate-100" key={String(item.id ?? index)}>
              {columns.map((column) => (
                <td className="px-4 py-3" key={String(column.key)}>
                  {column.render ? column.render(item) : String(item[column.key] ?? "")}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Badge({ children }: { children: ReactNode }) {
  return <span className="rounded bg-sky-50 px-2 py-1 text-xs font-medium text-sky-700">{children}</span>;
}
