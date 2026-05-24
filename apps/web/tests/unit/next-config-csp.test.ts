import { describe, expect, it } from "vitest";
import { buildProdCSP, telemetryConnectOrigin } from "../../next.config";

describe("production CSP", () => {
  it("keeps connect-src self-only when telemetry is not configured", () => {
    expect(buildProdCSP("")).toContain("connect-src 'self'");
  });

  it("allows only the telemetry collector origin in connect-src", () => {
    const csp = buildProdCSP("https://telemetry.example/collect/path");

    expect(telemetryConnectOrigin("https://telemetry.example/collect/path")).toBe(
      "https://telemetry.example",
    );
    expect(csp).toContain("connect-src 'self' https://telemetry.example");
    expect(csp).not.toContain("/collect/path");
  });

  it("does not allow unsupported telemetry URL schemes", () => {
    expect(telemetryConnectOrigin("javascript:alert(1)")).toBeNull();
    expect(buildProdCSP("javascript:alert(1)")).toContain("connect-src 'self'");
  });
});
