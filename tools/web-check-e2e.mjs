#!/usr/bin/env node
// SRapi frontend e2e gate. Preflights the backend, builds, then runs Playwright
// (which boots `next start` on SRAPI_WEB_E2E_PORT and runs the specs, incl. axe).
// Invoked by `make web-check-e2e`. Higher cost than web-check; run separately.
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(__dirname, "..");
const webDir = join(repoRoot, "apps", "web");

const apiUrl = process.env.SRAPI_WEB_E2E_API_URL ?? "http://127.0.0.1:8080";

async function preflight() {
  for (const path of ["/livez", "/readyz", "/api/v1/health"]) {
    try {
      const res = await fetch(`${apiUrl}${path}`, { signal: AbortSignal.timeout(2500) });
      if (res.ok) {
        console.info(`✓ API preflight ok via ${path}`);
        return true;
      }
    } catch {
      /* try next */
    }
  }
  console.warn(`! API preflight could not reach ${apiUrl}; continuing (demo specs are offline-safe)`);
  return false;
}

function run(name, cmd, args, cwd = webDir, env = {}) {
  console.info(`\n▶ web-check-e2e: ${name}`);
  const result = spawnSync(cmd, args, { cwd, stdio: "inherit", env: { ...process.env, ...env } });
  if (result.status !== 0) {
    console.error(`\n✗ web-check-e2e failed at: ${name}`);
    process.exit(result.status ?? 1);
  }
}

await preflight();

if (process.env.SRAPI_WEB_E2E_SKIP_INSTALL !== "1") {
  run("install chromium", "npm", ["run", "test:e2e:install"]);
}
run("build", "npm", ["run", "build"]);
run("playwright", "npm", ["run", "test:e2e"], webDir, { SRAPI_API_PROXY_TARGET: apiUrl });

console.info("\n✓ web-check-e2e passed");
