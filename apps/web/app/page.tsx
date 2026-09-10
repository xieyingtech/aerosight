"use client";
import { useEffect } from "react";
import { useRouter } from "next/navigation";

export default function HomePage() {
  const router = useRouter();
  useEffect(() => { router.replace("/projects/"); }, [router]);
  return <p className="p-6 text-sm text-muted-foreground">正在打开项目列表…</p>;
}
