import type { ReactNode } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";

export function DataTable<T extends Record<string, unknown>>({
  items,
  columns
}: {
  items: T[];
  columns: { key: keyof T; label: string; render?: (item: T) => ReactNode }[];
}) {
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((column) => <TableHead key={String(column.key)}>{column.label}</TableHead>)}
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.length ? items.map((item, index) => (
            <TableRow key={String(item.id ?? index)}>
              {columns.map((column) => (
                <TableCell key={String(column.key)}>
                  {column.render ? column.render(item) : String(item[column.key] ?? "")}
                </TableCell>
              ))}
            </TableRow>
          )) : (
            <TableRow>
              <TableCell className="h-24 text-center" colSpan={columns.length}>暂无数据</TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  );
}
