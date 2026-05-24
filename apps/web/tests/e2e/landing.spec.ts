import { test, expect } from "@playwright/test";

/**
 * SRapi v0.1.0 e2e smoke: landing / login page renders the calm tone copy.
 */
test.describe("landing page", () => {
  test("renders v0.1.0 brand and production sign-in", async ({ page }) => {
    await page.goto("/");

    await expect(page.getByText("SRapi.").first()).toBeVisible();
    await expect(
      page.getByRole("heading", { name: /one endpoint, every provider/i }),
    ).toBeVisible();

    await expect(page.getByRole("heading", { name: /sign in to srapi/i })).toBeVisible();
    await expect(page.getByLabel(/email/i)).toBeVisible();
    await expect(page.getByLabel(/password/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /^sign in$/i })).toBeVisible();
    await expect(page.getByRole("button", { name: /sign in as admin/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /sign in as developer/i })).toHaveCount(0);
  });

  test("forbids the deprecated academic copy", async ({ page }) => {
    await page.goto("/");
    const html = await page.content();
    expect(html).not.toContain("Verify Operator Credentials");
    expect(html).not.toContain("Cryptographic Credentials Vault");
    expect(html).not.toContain("Adaptive dispatch");
    expect(html).not.toContain("SPECIFICATION PORTAL");
  });
});
