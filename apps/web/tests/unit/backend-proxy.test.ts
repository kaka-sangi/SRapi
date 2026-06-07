// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";
import { proxyToBackend } from "@/lib/backend-proxy";

function fakeReq(opts: {
  method?: string;
  pathname?: string;
  search?: string;
  headers?: Record<string, string>;
  body?: BodyInit | null;
}): NextRequest {
  return {
    method: opts.method ?? "GET",
    nextUrl: { pathname: opts.pathname ?? "/api/v1/test", search: opts.search ?? "" },
    headers: new Headers(opts.headers ?? {}),
    body: opts.body ?? null,
    signal: new AbortController().signal,
  } as unknown as NextRequest;
}

afterEach(() => vi.restoreAllMocks());

describe("proxyToBackend", () => {
  it("streams chunks through without buffering (the SSE-streaming fix)", async () => {
    // An upstream body we drive chunk-by-chunk. If the proxy buffered (e.g. by
    // doing `await upstream.text()`), the first read below would hang until the
    // stream closes and the test would time out — so this deterministically
    // proves the proxy forwards the live stream.
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const upstreamBody = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
      },
    });
    const enc = new TextEncoder();
    const dec = new TextDecoder();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(upstreamBody, {
            status: 200,
            headers: { "content-type": "text/event-stream" },
          }),
      ),
    );

    const res = await proxyToBackend(fakeReq({ pathname: "/api/v1/admin/copilot/chat" }));
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("text/event-stream");
    expect(res.body).toBeTruthy();

    const reader = res.body!.getReader();

    controller.enqueue(enc.encode("event: a\ndata: 1\n\n"));
    const first = await reader.read();
    expect(dec.decode(first.value)).toContain("data: 1"); // arrived before close

    controller.enqueue(enc.encode("event: b\ndata: 2\n\n"));
    const second = await reader.read();
    expect(dec.decode(second.value)).toContain("data: 2");

    controller.close();
    expect((await reader.read()).done).toBe(true);
  });

  it("forwards method/query/cookies/csrf + identity encoding, and preserves multiple Set-Cookie", async () => {
    const seen: { url?: string; init?: RequestInit & { duplex?: string } } = {};
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init: RequestInit & { duplex?: string }) => {
        seen.url = url;
        seen.init = init;
        const h = new Headers({ "content-type": "application/json" });
        h.append("set-cookie", "a=1; Path=/");
        h.append("set-cookie", "b=2; Path=/");
        h.set("content-encoding", "gzip"); // must be stripped (body already decoded)
        return new Response('{"ok":true}', { status: 200, headers: h });
      }),
    );

    const res = await proxyToBackend(
      fakeReq({
        method: "POST",
        pathname: "/api/v1/admin/copilot/chat",
        search: "?x=1",
        headers: {
          "content-type": "application/json",
          "x-csrf-token": "tok",
          cookie: "s=1",
          "accept-encoding": "gzip",
        },
        body: '{"a":1}',
      }),
    );

    expect(seen.url).toBe("http://127.0.0.1:8080/api/v1/admin/copilot/chat?x=1");
    const sentHeaders = seen.init!.headers as Headers;
    expect(sentHeaders.get("accept-encoding")).toBe("identity"); // no upstream gzip buffering
    expect(sentHeaders.get("x-csrf-token")).toBe("tok");
    expect(sentHeaders.get("cookie")).toBe("s=1");
    expect(seen.init!.duplex).toBe("half"); // streaming request body
    expect(res.headers.get("content-encoding")).toBeNull(); // stripped
    expect(res.headers.getSetCookie()).toEqual(["a=1; Path=/", "b=2; Path=/"]);
  });

  it("returns 502 when the upstream is unreachable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("ECONNREFUSED");
      }),
    );
    const res = await proxyToBackend(fakeReq({ pathname: "/api/v1/health" }));
    expect(res.status).toBe(502);
  });
});
