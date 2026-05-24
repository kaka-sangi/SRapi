import { expect, test } from "@playwright/test";
import { ADMIN_ROUTE_SMOKE_TARGETS } from "@/lib/routes";

async function signInAsBootstrapAdmin(page: import("@playwright/test").Page) {
  await page.goto("/");
  await page.getByPlaceholder("operator@srapi.local").fill("admin@srapi.local");
  await page.getByPlaceholder("••••••••••••").fill("password123");
  await page.locator("#login-submit").click();
  await page.waitForURL("**/admin/dashboard", { timeout: 10_000 });
  await expect(page.getByRole("heading", { name: /admin dashboard/i })).toBeVisible();
}

test.describe("admin route smoke", () => {
  test("all admin pages render live without console failures", async ({ page }) => {
    test.setTimeout(90_000);

    const consoleIssues: string[] = [];
    const pageErrors: string[] = [];
    const failedRequests: string[] = [];

    page.on("console", (message) => {
      if (message.type() !== "error" && message.type() !== "warning") {
        return;
      }
      const text = message.text();
      if (/Download the React DevTools/i.test(text)) {
        return;
      }
      consoleIssues.push(`${message.type()}: ${text}`);
    });
    page.on("pageerror", (error) => {
      pageErrors.push(error.message);
    });
    page.on("requestfailed", (request) => {
      const url = request.url();
      if (url.includes("/__nextjs_font/")) {
        return;
      }
      if (url.includes("/srapi-health")) {
        return;
      }
      const errorText = request.failure()?.errorText ?? "failed";
      if (errorText === "net::ERR_ABORTED") {
        return;
      }
      failedRequests.push(`${request.method()} ${url}: ${errorText}`);
    });

    await signInAsBootstrapAdmin(page);

    for (const target of ADMIN_ROUTE_SMOKE_TARGETS) {
      const response = await page.goto(target.path, {
        waitUntil: "domcontentloaded",
        timeout: 30_000,
      });
      expect(response?.status(), target.path).toBeLessThan(400);
      await expect(page).toHaveURL(new RegExp(`${target.path.replaceAll("/", "\\/")}$`));
      await expect(page.locator("h1")).toHaveText(target.heading);
      await expect(page.locator("#login-submit")).toHaveCount(0);
      await expect(page.getByText(/Admin API request failed/i)).toHaveCount(0);
    }

    expect(pageErrors).toEqual([]);
    expect(consoleIssues).toEqual([]);
    expect(failedRequests).toEqual([]);
  });
});
