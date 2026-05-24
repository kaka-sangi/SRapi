import { test, expect } from "@playwright/test";

/**
 * SRapi v0.1.0 e2e smoke: the bootstrap admin signs in through the live
 * backend and lands in the admin console.
 */
test.describe("live login", () => {
  test("bootstrap admin lands in /admin/dashboard", async ({ page }) => {
    await page.goto("/");

    await page.getByPlaceholder("operator@srapi.local").fill("admin@srapi.local");
    await page.getByPlaceholder("••••••••••••").fill("password123");
    await page.locator("#login-submit").click();

    await page.waitForURL("**/admin/dashboard", { timeout: 10_000 });
    await expect(page.getByRole("heading", { name: /admin dashboard/i })).toBeVisible();
    await expect(page.getByText(/live/i).first()).toBeVisible();
  });
});
