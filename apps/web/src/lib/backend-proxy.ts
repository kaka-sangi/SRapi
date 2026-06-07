import type { NextRequest } from "next/server";

const PROXY_TARGET = (process.env.SRAPI_API_PROXY_TARGET || "http://127.0.0.1:8080").replace(
  /\/+$/,
  "",
);

// Request headers we must NOT forward verbatim: hop-by-hop headers, plus any
// fetch must recompute. accept-encoding is forced to identity so the upstream
// never gzips — gzip buffers internally and would collapse token-by-token SSE
// into a single all-at-once delivery (the streaming bug this proxy fixes).
const STRIP_REQUEST_HEADERS = new Set([
  "host",
  "connection",
  "content-length",
  "accept-encoding",
  "transfer-encoding",
]);

// Response headers we must drop: the body is already decoded and re-streamed by
// us, so a stale content-encoding/length/transfer-encoding would corrupt it.
const STRIP_RESPONSE_HEADERS = new Set([
  "content-encoding",
  "content-length",
  "transfer-encoding",
  "connection",
]);

/**
 * Streaming same-origin reverse proxy to the Go backend, used by the
 * app/api/[...path] and app/v1/[...path] catch-all route handlers.
 *
 * It replaces next.config `rewrites()`, which BUFFER `text/event-stream`
 * responses (vercel/next.js #45048, #66263) so SSE arrived all-at-once. An App
 * Router route handler instead returns the upstream `ReadableStream` body
 * directly, which Next streams chunk-by-chunk. Same-origin, so the session
 * cookie + CSP/nonce handling are preserved exactly as the rewrites did
 * (`src/proxy.ts`'s matcher already excludes /api and /v1).
 *
 * Forwards method, query, cookies, CSRF, and the (possibly streaming) request
 * body upstream, and streams the response — including Set-Cookie — back.
 */
export async function proxyToBackend(req: NextRequest): Promise<Response> {
  // Forward the path verbatim (it already carries the /api or /v1 prefix the
  // backend serves), preserving any encoding the parsed segments would lose.
  const url = `${PROXY_TARGET}${req.nextUrl.pathname}${req.nextUrl.search}`;

  const headers = new Headers();
  req.headers.forEach((value, key) => {
    if (!STRIP_REQUEST_HEADERS.has(key.toLowerCase())) headers.set(key, value);
  });
  headers.set("accept-encoding", "identity");

  const method = req.method.toUpperCase();
  const hasBody = method !== "GET" && method !== "HEAD";

  const init: RequestInit & { duplex?: "half" } = {
    method,
    headers,
    redirect: "manual",
    cache: "no-store",
    signal: req.signal,
  };
  if (hasBody) {
    init.body = req.body;
    // Required by Node/undici whenever a streaming request body is passed.
    init.duplex = "half";
  }

  let upstream: Response;
  try {
    upstream = await fetch(url, init);
  } catch (err) {
    if ((err as Error)?.name === "AbortError") {
      // Client disconnected; nothing to return.
      return new Response(null, { status: 499 });
    }
    return new Response(JSON.stringify({ error: { message: "upstream unreachable" } }), {
      status: 502,
      headers: { "content-type": "application/json" },
    });
  }

  const respHeaders = new Headers();
  upstream.headers.forEach((value, key) => {
    if (!STRIP_RESPONSE_HEADERS.has(key.toLowerCase())) respHeaders.set(key, value);
  });
  // Re-append Set-Cookie individually: forEach above collapses multiple cookies
  // into one comma-joined header, which breaks login (it sets several cookies).
  const setCookies =
    (upstream.headers as Headers & { getSetCookie?: () => string[] }).getSetCookie?.() ?? [];
  if (setCookies.length > 0) {
    respHeaders.delete("set-cookie");
    for (const cookie of setCookies) respHeaders.append("set-cookie", cookie);
  }

  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: respHeaders,
  });
}
