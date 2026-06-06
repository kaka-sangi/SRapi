import path from "node:path";
import type { NextConfig } from "next";
import bundleAnalyzer from "@next/bundle-analyzer";

// The Content-Security-Policy is NOT set here. It needs a per-request nonce so
// the App Router's inline bootstrap/RSC scripts are allowed under a strict
// script-src; a static header cannot carry a nonce. It is built per request in
// src/middleware.ts instead. These remaining headers are nonce-independent.
const SECURITY_HEADERS = [
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
];

// rewrites proxy /api and /v1 to the backend (Next same-origin proxy)
const proxyTarget = process.env.SRAPI_API_PROXY_TARGET ?? "http://127.0.0.1:8080";

const withBundleAnalyzer = bundleAnalyzer({ enabled: process.env.ANALYZE === "1" });

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  // Pin Turbopack's workspace root explicitly to silence the multi-lockfile
  // inference warning (lockfiles at repo root + apps/web). It must be the repo
  // root, NOT apps/web: this app imports the generated SDK from
  // ../../packages/sdk, which lives outside apps/web, and Turbopack refuses to
  // resolve modules above the configured root (next build fails otherwise).
  turbopack: { root: path.join(__dirname, "..", "..") },
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: SECURITY_HEADERS,
      },
    ];
  },
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${proxyTarget}/api/:path*` },
      { source: "/v1/:path*", destination: `${proxyTarget}/v1/:path*` },
    ];
  },
};

export default withBundleAnalyzer(nextConfig);
