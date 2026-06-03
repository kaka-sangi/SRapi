import { NextResponse, type NextRequest } from "next/server";
import { SESSION_PRESENT_COOKIE, SESSION_ROLE_COOKIE } from "@/lib/session-cookie";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE } from "@/lib/routes";

/**
 * SRapi edge proxy (Next.js 16 successor to middleware.ts).
 *
 * Server-side gate for protected console routes so users never see a flash of
 * unauthenticated content. Inspects two non-credential cookies set by the
 * client after `apiService.login` succeeds (see `src/lib/session-cookie.ts`).
 * Real auth + CSRF still live in the backend's HttpOnly cookies and are never
 * read here — treat this as a UX optimization on top of the client guards.
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
    url.pathname = ADMIN_HOME_ROUTE;
    return NextResponse.redirect(url);
  }
  if (role !== "admin" && matchPath(pathname, ADMIN_ONLY_PATHS)) {
    const url = request.nextUrl.clone();
    url.pathname = USER_HOME_ROUTE;
    return NextResponse.redirect(url);
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!api/|_next/static/|_next/image|favicon.ico|srapi-health).*)"],
};
