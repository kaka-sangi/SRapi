import type { NextConfig } from "next";
import bundleAnalyzer from "@next/bundle-analyzer";

const SECURITY_HEADERS = [
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
];

const isProd = process.env.NODE_ENV === "production";

const cspDirectives = [
  "default-src 'self'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "object-src 'none'",
  isProd ? "script-src 'self'" : "script-src 'self' 'unsafe-eval' 'unsafe-inline'",
  isProd ? "style-src 'self'" : "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL
    ? `connect-src 'self' ${new URL(process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL).origin}`
    : "connect-src 'self'",
  "frame-src 'none'",
].join("; ");

// rewrites proxy /api and /v1 to the backend (Next same-origin proxy)
const proxyTarget = process.env.SRAPI_API_PROXY_TARGET ?? "http://127.0.0.1:8080";

const withBundleAnalyzer = bundleAnalyzer({ enabled: process.env.ANALYZE === "1" });

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [...SECURITY_HEADERS, { key: "Content-Security-Policy", value: cspDirectives }],
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
