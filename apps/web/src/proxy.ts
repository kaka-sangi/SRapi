import { NextResponse, type NextRequest } from "next/server";
import { SESSION_PRESENT_COOKIE, SESSION_ROLE_COOKIE } from "@/lib/session-cookie";

/**
 * SRapi v0.1.0 edge proxy (formerly Next.js middleware).
 *
 * Server-side gate for the protected console routes so users never see a
 * flash of unauthenticated content. Renamed from `middleware.ts` per
 * Next.js 16 conventions; same edge runtime.
 *
 * Inspects two non-credential cookies set by the client after
 * `apiService.login` succeeds (see `src/lib/session-cookie.ts`). Real auth
 * and CSRF tokens still live in the backend's HttpOnly cookies and are not
 * read here. Treat this as a UX optimization on top of the existing
 * client-side guards.
 */
const PROTECTED_PATHS = [
  "/dashboard",
  "/admin",
  "/api-keys",
  "/usage",
  "/provider-accounts",
  "/scheduler-decisions",
];

const ADMIN_ONLY_PATHS = ["/admin", "/provider-accounts", "/scheduler-decisions"];
const USER_ONLY_PATHS = ["/dashboard"];

function matchPath(pathname: string, prefixes: string[]): boolean {
  return prefixes.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  if (!matchPath(pathname, PROTECTED_PATHS)) {
    return NextResponse.next();
  }

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
    url.pathname = "/admin";
    return NextResponse.redirect(url);
  }
  if (role !== "admin" && matchPath(pathname, ADMIN_ONLY_PATHS)) {
    const url = request.nextUrl.clone();
    url.pathname = "/dashboard";
    return NextResponse.redirect(url);
  }

  return NextResponse.next();
}

// `api/` (the backend reverse-proxy rewrite root) and Next.js framework
// paths are excluded so the auth gate never sits in front of static assets
// or the JSON API. Anchored with a trailing `/` so route names that start
// with the literal characters "api" — e.g. `/api-keys` — still pass through.
export const config = {
  matcher: ["/((?!api/|_next/static/|_next/image|favicon.ico|srapi-health).*)"],
};
