import type { NextConfig } from "next";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import bundleAnalyzer from "@next/bundle-analyzer";

const apiProxyTarget = (process.env.SRAPI_API_PROXY_TARGET || "http://127.0.0.1:8080").replace(
  /\/+$/,
  "",
);
const appDir = dirname(fileURLToPath(import.meta.url));

const isProd = process.env.NODE_ENV === "production";

// SRapi v0.1.0 production security headers.
// Dev keeps loose CSP so HMR + react-refresh inline scripts still work.
const securityHeaders = [
  { key: "X-DNS-Prefetch-Control", value: "off" },
  { key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
];

const prodCSP = [
  "default-src 'self'",
  "script-src 'self' 'unsafe-inline'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "frame-ancestors 'none'",
  "base-uri 'self'",
  "form-action 'self'",
].join("; ");

const baseConfig: NextConfig = {
  turbopack: {
    root: resolve(appDir, "../.."),
  },
  poweredByHeader: false,
  reactStrictMode: true,
  async headers() {
    const headers = [...securityHeaders];
    if (isProd) {
      headers.push({ key: "Content-Security-Policy", value: prodCSP });
    }
    return [{ source: "/(.*)", headers }];
  },
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiProxyTarget}/api/:path*` },
      { source: "/v1/:path*", destination: `${apiProxyTarget}/v1/:path*` },
    ];
  },
};

const withBundleAnalyzer = bundleAnalyzer({
  enabled: process.env.ANALYZE === "1" || process.env.ANALYZE === "true",
});

export default withBundleAnalyzer(baseConfig);
