import { defineConfig, devices } from "@playwright/test";

const PORT = Number(process.env.SRAPI_WEB_E2E_PORT ?? 3100);
const BASE_URL = process.env.SRAPI_WEB_E2E_BASE_URL ?? `http://127.0.0.1:${PORT}`;

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: BASE_URL,
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: `npm run build && npm run start -- --port ${PORT}`,
    url: BASE_URL,
    timeout: 120_000,
    reuseExistingServer: !process.env.CI,
    env: { SRAPI_API_PROXY_TARGET: process.env.SRAPI_WEB_E2E_API_URL ?? "http://127.0.0.1:8080" },
  },
});
