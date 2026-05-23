#!/usr/bin/env node
/* eslint-disable no-console */
/**
 * SRapi v0.1.0 frontend e2e harness. Invoked separately from web-check
 * because Playwright requires a downloaded browser and a long-running web
 * server; not everyone wants to pay that cost on every save.
 *
 * Steps:
 *   1. build          -> next build (provides static + server bundle for `next start`)
 *   2. install browser-> playwright install --with-deps chromium (idempotent)
 *   3. e2e            -> playwright test
 *
 * Skip the install step with SRAPI_WEB_E2E_SKIP_INSTALL=1 if Chromium is already
 * cached in CI.
 */
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");
const webDir = path.join(repoRoot, "apps", "web");
const isWindows = process.platform === "win32";

function run(cmd, args, label) {
  console.log("");
  console.log(`==> web-check-e2e: ${label}`);
  const result = spawnSync(cmd, args, {
    cwd: webDir,
    stdio: "inherit",
    shell: isWindows,
    env: { ...process.env, FORCE_COLOR: process.env.FORCE_COLOR ?? "1" },
  });
  if (result.status !== 0) {
    console.error(`!! web-check-e2e: ${label} failed with exit code ${result.status ?? 1}`);
    process.exit(result.status ?? 1);
  }
}

run("npm", ["run", "build"], "build");

if (process.env.SRAPI_WEB_E2E_SKIP_INSTALL !== "1") {
  run("npm", ["run", "test:e2e:install"], "install playwright chromium");
}

run("npm", ["run", "test:e2e"], "playwright test");

console.log("");
console.log("==> web-check-e2e: all e2e gates passed");
