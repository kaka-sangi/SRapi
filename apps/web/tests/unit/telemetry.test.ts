import { afterEach, describe, expect, it, vi } from "vitest";

describe("telemetry captureException", () => {
  const originalTelemetryUrl = process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL;
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

  afterEach(() => {
    vi.resetModules();
    vi.unstubAllGlobals();
    process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL = originalTelemetryUrl;
    consoleError.mockClear();
  });

  it("does not send exception telemetry when no collector is configured", async () => {
    delete process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL;
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetchMock);

    const { captureException } = await import("@/lib/telemetry");
    captureException(new Error("boom"));

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("sends redacted exception summaries to the configured collector", async () => {
    process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL = "https://telemetry.example/collect";
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(new Response(null, { status: 202 })),
    );
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("navigator", {});

    const { captureException } = await import("@/lib/telemetry");
    captureException(new Error("failed with sk-secret1234567890 and sess_abcdef1234567890"), {
      apiKey: "sk-secret1234567890",
      requestId: "req_123",
      nested: { raw: "not uploaded" },
      authorization: "Bearer secret-token",
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(init).toBeDefined();
    expect(url).toBe("https://telemetry.example/collect");
    expect(init).toMatchObject({
      method: "POST",
      keepalive: true,
      headers: { "Content-Type": "application/json" },
    });
    const body = JSON.parse(String(init?.body));
    expect(body.kind).toBe("exception");
    expect(body.error.message).toContain("sk-[redacted]");
    expect(body.error.message).toContain("[redacted]");
    expect(body.context).toEqual({
      apiKey: "[redacted]",
      requestId: "req_123",
      nested: "[object]",
      authorization: "[redacted]",
    });
    expect(JSON.stringify(body)).not.toContain("secret1234567890");
    expect(JSON.stringify(body)).not.toContain("secret-token");
    expect(JSON.stringify(body)).not.toContain("not uploaded");
  });
});
