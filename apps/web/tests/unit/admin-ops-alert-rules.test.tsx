import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminOpsPage from "@/app/admin/ops/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { OpsAlertRule } from "@/lib/sdk-types";

const storage = new Map<string, string>();
Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
});

const mocks = vi.hoisted(() => ({
  baselineRule: {
    id: "1",
    name: "SRapi Provider timeout baseline",
    metric_type: "request_count",
    operator: "gt",
    threshold: 10,
    severity: "warning",
    enabled: true,
    window_seconds: 300,
    cooldown_seconds: 600,
    min_request_count: 1,
    scope: {
      source_endpoint: "",
      model: "",
      error_class: "timeout",
    },
    builtin_baseline: true,
    baseline_key: "provider.timeout",
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:00:00Z",
  } satisfies OpsAlertRule,
  customRule: {
    id: "2",
    name: "Custom chat error rate",
    metric_type: "error_rate",
    operator: "gt",
    threshold: 0.2,
    severity: "critical",
    enabled: false,
    window_seconds: 600,
    cooldown_seconds: 900,
    min_request_count: 20,
    scope: {
      source_endpoint: "/v1/chat/completions",
      model: "gpt-ops",
      error_class: "",
    },
    builtin_baseline: false,
    baseline_key: "",
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:00:00Z",
  } satisfies OpsAlertRule,
}));

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(window.location.search),
  useRouter: () => ({ replace: vi.fn() }),
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/admin/slo-form-dialog", () => ({ SloFormDialog: () => null }));
vi.mock("@/components/admin/alert-rule-form-dialog", () => ({ AlertRuleFormDialog: () => null }));
vi.mock("@/components/admin/alert-silence-form-dialog", () => ({
  AlertSilenceFormDialog: () => null,
}));
vi.mock("@/components/admin/ops-notification-channel-form-dialog", () => ({
  OpsNotificationChannelFormDialog: () => null,
}));
vi.mock("@/components/admin/ops-log-cleanup-dialog", () => ({ OpsLogCleanupDialog: () => null }));
vi.mock("@/components/admin/ops-latency-histogram-chart", () => ({
  OpsLatencyHistogramChart: () => null,
}));
vi.mock("@/components/admin/ops-error-distribution-chart", () => ({
  OpsErrorDistributionChart: () => null,
}));
vi.mock("@/components/admin/ops-channel-monitor", () => ({ MonitorContent: () => null }));
vi.mock("@/components/admin/ops-scheduled-tests", () => ({ ScheduledTestsContent: () => null }));
vi.mock("@/components/admin/ops-strategy", () => ({ StrategyContent: () => null }));
vi.mock("@/components/features/scheduler-decisions-panel", () => ({
  SchedulerDecisionsPanel: () => null,
}));
vi.mock("@/app/admin/ops/evidence-chain-health", () => ({
  OpsEvidenceChainHealth: () => null,
}));

vi.mock("@/hooks/admin-queries/ops-charts", () => ({
  useOpsLatencyHistogram: () => ({ data: { buckets: [] }, isLoading: false }),
  useOpsErrorDistribution: () => ({ data: { items: [] }, isLoading: false }),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useOpsSlos: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useOpsAlerts: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useAcknowledgeAlert: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useOpsThroughput: () => ({ data: { points: [] }, isLoading: false }),
  useOpsErrorTrend: () => ({ data: { points: [] }, isLoading: false }),
  useUpdateOpsSettings: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useOpsAlertRules: () => ({
    data: { data: [mocks.baselineRule, mocks.customRule] },
    isLoading: false,
    isError: false,
  }),
  useDeleteOpsAlertRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteOpsSlo: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useOpsAlertSilences: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useDeleteOpsAlertSilence: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useOpsNotificationChannels: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useOpsNotificationDeliveries: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useDeleteOpsNotificationChannel: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useOpsSystemLogHealth: () => ({ data: undefined, isLoading: false }),
}));

function wrap({ children }: PropsWithChildren) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LanguageProvider>{children}</LanguageProvider>
    </QueryClientProvider>
  );
}

describe("AdminOpsPage alert rules", () => {
  beforeEach(() => {
    storage.clear();
    window.history.replaceState(null, "", "/admin/ops");
  });

  it("shows built-in baseline metadata without marking custom rules", () => {
    render(<AdminOpsPage />, { wrapper: wrap });

    expect(screen.getByText("SRapi Provider timeout baseline")).toBeInTheDocument();
    expect(screen.getByText("内置基线")).toBeInTheDocument();
    expect(screen.getByText(/provider\.timeout/)).toBeInTheDocument();
    expect(screen.getByText("Custom chat error rate")).toBeInTheDocument();
    expect(screen.getAllByText("内置基线")).toHaveLength(1);
  });
});
