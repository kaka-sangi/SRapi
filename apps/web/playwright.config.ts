import { defineConfig, devices } from "@playwright/test";

const PORT = Number(process.env.SRAPI_WEB_E2E_PORT ?? 3100);
const BASE_URL = process.env.SRAPI_WEB_E2E_BASE_URL ?? `http://127.0.0.1:${PORT}`;

// SRapi v0.1.0 frontend e2e harness.
// Default run uses Chromium against the demo runtime (no real backend required).
export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? [["github"], ["list"]] : [["list"]],
  use: {
    baseURL: BASE_URL,
    trace: "retain-on-failure",
    video: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: process.env.SRAPI_WEB_E2E_BASE_URL
    ? undefined
    : {
        command: `next start --port ${PORT}`,
        url: BASE_URL,
        reuseExistingServer: !process.env.CI,
        timeout: 60_000,
        env: {
          NEXT_PUBLIC_SRAPI_DEMO_MODE: "true",
        },
      },
});
