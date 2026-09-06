import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";

export default function nextConfig(phase: string): NextConfig {
  if (phase !== PHASE_DEVELOPMENT_SERVER) return {};
  const origin = new URL(process.env.GO_API_ORIGIN ?? "http://127.0.0.1:8080");
  if (!["http:", "https:"].includes(origin.protocol) || origin.username || origin.password || origin.pathname !== "/" || origin.search || origin.hash) {
    throw new Error("GO_API_ORIGIN must be an HTTP(S) origin");
  }
  return {
    async rewrites() {
      return { beforeFiles: [
        { source: "/api/:path*", destination: `${origin.origin}/api/:path*` },
        { source: "/algorithm-assets/:path*", destination: `${origin.origin}/algorithm-assets/:path*` }
      ], afterFiles: [], fallback: [] };
    }
  };
}
