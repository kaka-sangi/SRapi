import { afterEach, describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import RouteError from "@/app/error";
import GlobalError from "@/app/global-error";
import { captureException } from "@/lib/telemetry";

vi.mock("@/lib/telemetry", () => ({
  captureException: vi.fn(),
}));

const captureExceptionMock = vi.mocked(captureException);
const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

describe("Next error boundaries", () => {
  afterEach(() => {
    captureExceptionMock.mockClear();
    consoleError.mockClear();
  });

  it("captures route errors with low-cardinality context", async () => {
    const error = Object.assign(new Error("route boom"), { digest: "digest-route" });

    render(<RouteError error={error} reset={vi.fn()} />);

    await waitFor(() => {
      expect(captureExceptionMock).toHaveBeenCalledWith(error, {
        boundary: "route",
        digest: "digest-route",
      });
    });
  });

  it("captures global shell errors with low-cardinality context", async () => {
    const error = Object.assign(new Error("global boom"), { digest: "digest-global" });

    render(<GlobalError error={error} reset={vi.fn()} />);

    await waitFor(() => {
      expect(captureExceptionMock).toHaveBeenCalledWith(error, {
        boundary: "global",
        digest: "digest-global",
      });
    });
  });
});
