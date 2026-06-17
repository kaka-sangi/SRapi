import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AvailabilityBadge } from "@/app/admin/proxies/page";

// AvailabilityBadge picks a colour tone based on three thresholds — the
// admin's rules of thumb for proxy health at a glance. These thresholds are
// also documented in en.ts/zh.ts, so changing them should change the test.
describe("AvailabilityBadge", () => {
  it("renders the percentage with the success tone above 95%", () => {
    const { container } = render(<AvailabilityBadge pct={97} />);
    expect(container.textContent).toContain("97%");
    expect(container.querySelector(".text-srapi-success")).not.toBeNull();
  });

  it("renders the warning tone in the 70-94% band", () => {
    const { container } = render(<AvailabilityBadge pct={80} />);
    expect(container.textContent).toContain("80%");
    expect(container.querySelector(".text-srapi-warning")).not.toBeNull();
  });

  it("renders the error tone below 70%", () => {
    const { container } = render(<AvailabilityBadge pct={50} />);
    expect(container.textContent).toContain("50%");
    expect(container.querySelector(".text-srapi-error")).not.toBeNull();
  });

  it("treats exactly 95 as success and exactly 70 as warning (inclusive)", () => {
    const exactSuccess = render(<AvailabilityBadge pct={95} />);
    expect(exactSuccess.container.querySelector(".text-srapi-success")).not.toBeNull();
    const exactWarning = render(<AvailabilityBadge pct={70} />);
    expect(exactWarning.container.querySelector(".text-srapi-warning")).not.toBeNull();
  });
});
