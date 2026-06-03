import { NextResponse } from "next/server";

const PROXY_TARGET = (process.env.SRAPI_API_PROXY_TARGET || "http://127.0.0.1:8080").replace(
  /\/+$/,
  "",
);

/**
 * Same-origin health probe used by `apiService.isBackendConnected()`.
 * Forwards to the backend's `/api/v1/health` so the browser never needs a
 * cross-origin request to check connectivity.
 */
export async function GET() {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 2500);
    const res = await fetch(`${PROXY_TARGET}/api/v1/health`, {
      cache: "no-store",
      signal: controller.signal,
    });
    clearTimeout(timeout);
    return NextResponse.json({ ok: res.ok }, { status: res.ok ? 200 : 503 });
  } catch {
    return NextResponse.json({ ok: false }, { status: 503 });
  }
}
