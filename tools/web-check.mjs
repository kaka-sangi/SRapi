#!/usr/bin/env node
/* eslint-disable no-console */
/**
 * SRapi v0.1.0 frontend quality harness.
 *
 * Runs the gates the frontend must pass before a release. Each step is a
 * thin wrapper around `npm run <script>` so the same gates run locally,
 * in CI, and from `make web-check`.
 *
 * Steps in order:
 *   1. typecheck   -> tsc --noEmit
 *   2. lint        -> eslint
 *   3. unit tests  -> vitest run (includes axe-core unit smoke)
 *   4. build       -> next build (production output, also exercises CSP headers)
 *
 * Playwright e2e is intentionally NOT run here. e2e needs a live server and
 * Chromium download; `make web-check-e2e` runs that separately.
 *
 * Skip a step locally with SRAPI_WEB_CHECK_SKIP=lint,build (etc).
 *
 * Exit code is the first non-zero step's exit code.
 */
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");
const webDir = path.join(repoRoot, "apps", "web");

const skip = new Set(
  (process.env.SRAPI_WEB_CHECK_SKIP ?? "")
    .split(",")
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean),
);

const steps = [
  { name: "typecheck", cmd: "npm", args: ["run", "typecheck"], cwd: webDir },
  { name: "lint", cmd: "npm", args: ["run", "lint"], cwd: webDir },
  { name: "test", cmd: "npm", args: ["run", "test"], cwd: webDir },
  { name: "build", cmd: "npm", args: ["run", "build"], cwd: webDir },
  {
    name: "bundle-budget",
    cmd: "node",
    args: [path.join(here, "bundle-budget.mjs")],
    cwd: repoRoot,
  },
];

const isWindows = process.platform === "win32";

function header(name) {
  console.log("");
  console.log(`==> web-check: ${name}`);
}

let firstFailure = 0;
for (const step of steps) {
  if (skip.has(step.name)) {
    console.log(`==> web-check: ${step.name} (skipped via SRAPI_WEB_CHECK_SKIP)`);
    continue;
  }
  header(step.name);
  const result = spawnSync(step.cmd, step.args, {
    cwd: step.cwd ?? webDir,
    stdio: "inherit",
    shell: isWindows,
    env: { ...process.env, FORCE_COLOR: process.env.FORCE_COLOR ?? "1" },
  });
  if (result.status !== 0) {
    firstFailure = result.status ?? 1;
    console.error(`!! web-check: ${step.name} failed with exit code ${firstFailure}`);
    break;
  }
}

if (firstFailure === 0) {
  console.log("");
  console.log("==> web-check: all gates passed");
}
process.exit(firstFailure);
