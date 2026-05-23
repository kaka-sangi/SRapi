import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

/**
 * SRapi v0.1.0 page-level a11y smoke: no serious or critical WCAG 2 AA
 * violations on the landing page or the workspace.
 */
const SEVERITY_GATE = ["critical", "serious"] as const;

test("landing page has no serious axe violations", async ({ page }) => {
  await page.goto("/");
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa"])
    .analyze();
  const blocking = results.violations.filter((v) =>
    SEVERITY_GATE.includes(v.impact as (typeof SEVERITY_GATE)[number]),
  );
  expect(blocking, JSON.stringify(blocking, null, 2)).toEqual([]);
});

test("workspace (after demo login) has no serious axe violations", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /sign in as developer/i }).click();
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await page.waitForURL("**/dashboard", { timeout: 10_000 });

  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa"])
    .analyze();
  const blocking = results.violations.filter((v) =>
    SEVERITY_GATE.includes(v.impact as (typeof SEVERITY_GATE)[number]),
  );
  expect(blocking, JSON.stringify(blocking, null, 2)).toEqual([]);
});
