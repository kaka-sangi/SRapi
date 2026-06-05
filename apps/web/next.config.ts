import path from "node:path";
import type { NextConfig } from "next";
import bundleAnalyzer from "@next/bundle-analyzer";

const SECURITY_HEADERS = [
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
];

const isProd = process.env.NODE_ENV === "production";

// Human-verification widget CDNs (Cloudflare Turnstile / hCaptcha / reCAPTCHA).
// Allowed so the operator-toggled captcha can load its script + challenge iframe.
// Dormant — nothing is fetched from them — until the server reports captcha
// enabled and the widget actually mounts.
const CAPTCHA_SCRIPT_SRC =
  "https://challenges.cloudflare.com https://js.hcaptcha.com https://www.google.com https://www.gstatic.com";
const CAPTCHA_FRAME_SRC =
  "https://challenges.cloudflare.com https://newassets.hcaptcha.com https://www.google.com";
const CAPTCHA_CONNECT_SRC =
  "https://challenges.cloudflare.com https://*.hcaptcha.com https://www.google.com";

const cspDirectives = [
  "default-src 'self'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "object-src 'none'",
  isProd
    ? `script-src 'self' ${CAPTCHA_SCRIPT_SRC}`
    : `script-src 'self' 'unsafe-eval' 'unsafe-inline' ${CAPTCHA_SCRIPT_SRC}`,
  isProd ? "style-src 'self'" : "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL
    ? `connect-src 'self' ${CAPTCHA_CONNECT_SRC} ${new URL(process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL).origin}`
    : `connect-src 'self' ${CAPTCHA_CONNECT_SRC}`,
  `frame-src ${CAPTCHA_FRAME_SRC}`,
].join("; ");

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
