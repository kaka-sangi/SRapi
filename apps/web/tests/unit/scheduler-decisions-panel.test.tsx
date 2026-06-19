import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SchedulerDecisionsPanel } from "@/components/features/scheduler-decisions-panel";
import { LanguageProvider } from "@/context/LanguageContext";

const mocks = vi.hoisted(() => ({
  useSchedulerDecisions: vi.fn(),
}));

vi.mock("@/hooks/queries", () => ({
  useSchedulerDecisions: mocks.useSchedulerDecisions,
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(window.location.search),
}));

Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: () => null,
    setItem: vi.fn(),
    removeItem: vi.fn(),
    clear: vi.fn(),
  },
});

describe("SchedulerDecisionsPanel", () => {
  beforeEach(() => {
    window.history.replaceState(
      null,
      "",
      "/admin/ops?tab=scheduler-decisions&f_request_id=req-filter",
    );
    mocks.useSchedulerDecisions.mockReturnValue({
      data: [
        {
          created_at: "2026-06-18T10:00:00Z",
          request_id: "req-filter",
          model: "gpt-4o-mini",
          source_endpoint: "/v1/chat/completions",
          candidate_count: 3,
          selected_account_id: "12",
          selected_account_name: "account-12",
          rejected_count: 2,
          rejected_reasons: [{ account: "account-13", reason: "cooldown_active" }],
          scores: [{ account: "account-12", score: 0.9, latency: 0.7, cost: 0.8, quota: 0.6 }],
          warnings: [],
          logs: ["selected account-12"],
        },
      ],
      isFetching: false,
      refetch: vi.fn(),
    });
  });

  it("passes the request id URL filter to scheduler decisions query", () => {
    render(<SchedulerDecisionsPanel />, { wrapper: LanguageProvider });

    expect(mocks.useSchedulerDecisions).toHaveBeenCalledWith({ request_id: "req-filter" });
    expect(screen.getAllByText("req-filter").length).toBeGreaterThan(0);
    expect(screen.getByText("gpt-4o-mini")).toBeInTheDocument();
    expect(screen.getAllByText("account-12").length).toBeGreaterThan(0);
  });
});
