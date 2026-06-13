import { NextResponse, type NextRequest } from "next/server";
import { SESSION_PRESENT_COOKIE, SESSION_ROLE_COOKIE } from "@/lib/session-cookie";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE } from "@/lib/routes";

/**
 * SRapi edge proxy (Next.js 16 successor to middleware.ts).
 *
 * Two jobs:
 *  1. Per-request Content-Security-Policy with a fresh nonce (see buildCsp).
 *  2. Server-side gate for protected console routes so users never see a flash of
 *     unauthenticated content. Inspects two non-credential cookies set by the
 *     client after `apiService.login` succeeds (see `src/lib/session-cookie.ts`).
 *     Real auth + CSRF still live in the backend's HttpOnly cookies and are never
 *     read here — treat this as a UX optimization on top of the client guards.
 */
const PROTECTED_PATHS = [
  "/dashboard",
  "/admin",
  "/api-keys",
  "/usage",
];

const ADMIN_ONLY_PATHS = ["/admin"];
const USER_ONLY_PATHS = ["/dashboard"];

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

const isProd = process.env.NODE_ENV === "production";

// The CSP is built per request so each document carries a fresh nonce. A STATIC
// header cannot do this: the App Router emits inline bootstrap/RSC scripts
// (`self.__next_f.push(...)`), and without 'nonce-…' / 'unsafe-inline' a strict
// `script-src 'self'` blocks them — React never hydrates and the whole console
// renders but stays non-interactive. Next reads the nonce from the request-side
// CSP header and stamps it onto every inline script it generates; the matching
// response header lets the browser run them while still rejecting injected ones.
function buildCsp(nonce: string): string {
  const telemetry = process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL;
  const connectExtra = telemetry ? ` ${new URL(telemetry).origin}` : "";
  return [
    "default-src 'self'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "object-src 'none'",
    // Dev (Turbopack + React Refresh) needs eval + inline; prod locks scripts to
    // 'self' + this request's nonce (plus the captcha CDNs).
    isProd
      ? `script-src 'self' 'nonce-${nonce}' ${CAPTCHA_SCRIPT_SRC}`
      : `script-src 'self' 'unsafe-eval' 'unsafe-inline' ${CAPTCHA_SCRIPT_SRC}`,
    // React and Next inject inline `style` attributes (stagger delays, image and
    // font sizing, …). A nonce/hash cannot cover style ATTRIBUTES, so inline
    // styles must be allowed outright — a far smaller risk than inline scripts.
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    `connect-src 'self' ${CAPTCHA_CONNECT_SRC}${connectExtra}`,
    `frame-src ${CAPTCHA_FRAME_SRC}`,
  ].join("; ");
}

// Pass the request through with the CSP applied. The nonce + CSP go on the
// REQUEST headers so the App Router stamps the nonce onto its inline scripts; the
// CSP is mirrored on the RESPONSE so the browser enforces it.
function documentResponse(request: NextRequest): NextResponse {
  const nonce = btoa(crypto.randomUUID());
  const csp = buildCsp(nonce);
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("x-nonce", nonce);
  requestHeaders.set("Content-Security-Policy", csp);
  const response = NextResponse.next({ request: { headers: requestHeaders } });
  response.headers.set("Content-Security-Policy", csp);
  return response;
}

function matchPath(pathname: string, prefixes: string[]): boolean {
  return prefixes.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (matchPath(pathname, PROTECTED_PATHS)) {
    const present = request.cookies.get(SESSION_PRESENT_COOKIE)?.value;
    if (present !== "1") {
      const url = request.nextUrl.clone();
      url.pathname = "/";
      url.searchParams.set("from", pathname);
      return NextResponse.redirect(url);
    }

    const role = request.cookies.get(SESSION_ROLE_COOKIE)?.value;
    if (role === "admin" && matchPath(pathname, USER_ONLY_PATHS)) {
      const url = request.nextUrl.clone();
      url.pathname = ADMIN_HOME_ROUTE;
      return NextResponse.redirect(url);
    }
    if (role !== "admin" && matchPath(pathname, ADMIN_ONLY_PATHS)) {
      const url = request.nextUrl.clone();
      url.pathname = USER_HOME_ROUTE;
      return NextResponse.redirect(url);
    }
  }

  return documentResponse(request);
}

export const config = {
  // Document/page requests only. Skip the /api and /v1 proxy (the backend sets
  // its own CSP) and Next's static assets (chunks load fine under 'self').
  matcher: ["/((?!api/|v1/|_next/static/|_next/image|favicon.ico|srapi-health).*)"],
};
