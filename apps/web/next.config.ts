import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";
import { legacyPageRedirects } from "./lib/legacy-page-redirects";

export default function nextConfig(phase: string): NextConfig {
  if (phase !== PHASE_DEVELOPMENT_SERVER) return { output: "export", trailingSlash: true };
  const origin = new URL(process.env.GO_API_ORIGIN ?? "http://127.0.0.1:8080");
  if (!["http:", "https:"].includes(origin.protocol) || origin.username || origin.password || origin.pathname !== "/" || origin.search || origin.hash) {
    throw new Error("GO_API_ORIGIN must be an HTTP(S) origin");
  }
  return {
    allowedDevOrigins: [new URL(process.env.PUBLIC_ORIGIN ?? "http://localhost:3000").hostname],
    skipTrailingSlashRedirect: true,
    async redirects() { return legacyPageRedirects(); },
    async rewrites() {
      return { beforeFiles: [
        { source: "/api/:path*", destination: `${origin.origin}/api/:path*` },
        { source: "/algorithm-assets/:path*", destination: `${origin.origin}/algorithm-assets/:path*` }
      ], afterFiles: [], fallback: [] };
    }
  };
}
