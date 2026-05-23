import { test, expect } from "@playwright/test";

/**
 * SRapi v0.1.0 e2e smoke: demo developer login routes to the workspace and
 * shows the new tone copy.
 */
test.describe("demo login", () => {
  test("developer demo lands in /dashboard", async ({ page }) => {
    await page.goto("/");

    await page.getByPlaceholder("operator@srapi.local").fill("developer@srapi.local");
    await page.getByPlaceholder("••••••••••••").fill("password123");
    await page.getByRole("button", { name: /^sign in$/i }).click();

    await page.waitForURL("**/dashboard", { timeout: 10_000 });
    await expect(page.getByRole("heading", { name: /your account at a glance/i })).toBeVisible();
    await expect(page.getByRole("link", { name: /^api keys$/i })).toBeVisible();
  });

  test("admin demo lands in /admin", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: /sign in as admin/i }).click();
    await page.getByRole("button", { name: /^sign in$/i }).click();

    await page.waitForURL("**/admin", { timeout: 10_000 });
    await expect(page.getByRole("heading", { name: /gateway overview/i })).toBeVisible();
  });
});
