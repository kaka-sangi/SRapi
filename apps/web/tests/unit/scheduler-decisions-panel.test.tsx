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

vi.mock("@/hooks/use-account-name-lookup", () => ({
  useAccountNameLookup: () => ({
    get: (id?: string | null) => (id ? `account-${id}` : "—"),
    map: new Map(),
    query: {},
  }),
}));

vi.mock("@/hooks/use-provider-name-lookup", () => ({
  useProviderNameLookup: () => ({
    get: (id?: string | null) => (id ? `provider-${id}` : "—"),
    map: new Map(),
    query: {},
  }),
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
      "/admin/ops?tab=scheduler-decisions&f_request_id=req-filter&f_account_id=12&f_provider_id=3&f_model=gpt-4o-mini",
    );
    mocks.useSchedulerDecisions.mockReturnValue({
      data: [
        {
          id: "dec-77",
          created_at: "2026-06-18T10:00:00Z",
          request_id: "req-filter",
          attempt_no: 2,
          model: "gpt-4o-mini",
          source_protocol: "openai-compatible",
          source_endpoint: "/v1/chat/completions",
          target_protocol: "anthropic-compatible",
          strategy: "cost_saver",
          strategy_version: "v4",
          fallback_from_decision_id: "dec-76",
          candidate_count: 3,
          selected_provider_id: "3",
          selected_account_id: "12",
          selected_account_name: "account-12",
          rejected_count: 2,
          rejected_reasons: [
            { account_id: "13", account: "#13", reason: "cooldown_active" },
            { account_id: "14", account: "#14", reason: "cooldown_active" },
          ],
          scores: [
            {
              account_id: "12",
              account: "#12",
              score: 0.9,
              health: 0.7,
              latency: 0.6,
              cost: 0.8,
              quota: 0.5,
              quality: 0.4,
              sticky: 0,
              cache: 0.2,
              fairness: 1,
              risk_penalty: 0.1,
              saturation_penalty: 0,
              pareto_frontier: true,
            },
          ],
          selection_rationale: "Selected account 12 because cost was best.",
          sticky_hit: false,
          cache_affinity_hit: true,
          estimated_cost: "0.00012000",
          currency: "USD",
          warnings: ["strategy_rollout_shadow_selected"],
          logs: ["selected account-12"],
        },
      ],
      isFetching: false,
      refetch: vi.fn(),
    });
  });

  it("passes the request id URL filter to scheduler decisions query", () => {
    render(<SchedulerDecisionsPanel />, { wrapper: LanguageProvider });

    expect(mocks.useSchedulerDecisions).toHaveBeenCalledWith({
      request_id: "req-filter",
      account_id: "12",
      provider_id: "3",
      model: "gpt-4o-mini",
    });
    expect(screen.getByTitle("req:req-filter")).toBeInTheDocument();
    expect(screen.getByTitle("acct:12")).toBeInTheDocument();
    expect(screen.getByTitle("prov:3")).toBeInTheDocument();
    expect(screen.getByTitle("model:gpt-4o-mini")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o-mini")).toBeInTheDocument();
    expect(screen.getAllByText("account-12").length).toBeGreaterThan(0);
    expect(screen.getAllByText("故障转移").length).toBeGreaterThan(0);
    expect(screen.getAllByText("cooldown_active ×2").length).toBeGreaterThan(0);
    expect(screen.getByText("Selected account 12 because cost was best.")).toBeInTheDocument();
    expect(screen.getByText("分数构成")).toBeInTheDocument();
    expect(screen.getByText("frontier")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /错误日志/i })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=%2Fv1%2Fchat%2Fcompletions&f_account=12&f_provider=3&f_model=gpt-4o-mini",
    );
    expect(screen.getByRole("link", { name: /请求证据/i })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-evidence&f_request_id=req-filter",
    );
    expect(screen.getByRole("link", { name: /账号健康/i })).toHaveAttribute(
      "href",
      "/admin/accounts?view=health&f_providerId=3&f_accountId=12",
    );
    expect(screen.getByRole("link", { name: /供应商/i })).toHaveAttribute(
      "href",
      "/admin/providers?q=3",
    );
  });
});
