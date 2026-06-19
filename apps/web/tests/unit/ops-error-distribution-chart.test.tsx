import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OpsErrorDistributionChart } from "@/components/admin/ops-error-distribution-chart";

describe("OpsErrorDistributionChart", () => {
  it("renders error-class investigation links when provided", () => {
    render(
      <OpsErrorDistributionChart
        title="Error distribution"
        emptyLabel="No errors"
        totalLabel="errors"
        ownerLabels={{
          provider: "Provider",
          client: "Client",
          platform: "Platform",
          other: "Other",
        }}
        items={[
          { error_class: "server_bad", owner: "provider", count: 7, share: 0.7 },
          { error_class: "timeout", owner: "platform", count: 3, share: 0.3 },
        ]}
        investigationHref={(item) => `/admin/logs?tab=error&q=${item.error_class}`}
      />,
    );

    expect(screen.getByRole("link", { name: /server_bad/ })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=server_bad",
    );
  });
});
