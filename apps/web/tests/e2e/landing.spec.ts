import { test, expect } from "@playwright/test";

test("landing page renders the sign-in form", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 2 })).toBeVisible();
  await expect(page.getByLabel(/email|邮箱/i)).toBeVisible();
  await expect(page.getByRole("button", { name: /sign in|登录/i })).toBeVisible();
});

test("protected route redirects to sign-in when unauthenticated", async ({ page }) => {
  await page.goto("/dashboard");
  await expect(page).toHaveURL(/\/(\?from=.*)?$/);
});
