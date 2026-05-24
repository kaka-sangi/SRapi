#!/usr/bin/env node
/* eslint-disable no-console */
/**
 * SRapi v0.1.0 frontend e2e harness. Invoked separately from web-check
 * because Playwright requires a downloaded browser and a long-running web
 * server; not everyone wants to pay that cost on every save.
 *
 * Steps:
 *   1. build          -> next build (provides static + server bundle for `next start`)
 *   2. install browser-> playwright install chromium (idempotent)
 *   3. e2e            -> playwright test
 *
 * Skip the install step with SRAPI_WEB_E2E_SKIP_INSTALL=1 if Chromium is already
 * cached in CI. Install OS packages explicitly with
 * `npm run test:e2e:install-deps` when setting up a new Linux runner.
 */
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");
const webDir = path.join(repoRoot, "apps", "web");
const isWindows = process.platform === "win32";
const defaultApiURL = "http://127.0.0.1:8080";
const apiURL = resolveApiURL(process.env);
const directBrowserBaseURL = (process.env.SRAPI_WEB_E2E_DIRECT_BROWSER_API ?? "").trim() === "1";

export function buildChildEnv(baseEnv, targetApiURL, options = {}) {
  const directBrowserAPI = options.directBrowserAPI === true;
  const env = {
    ...baseEnv,
    SRAPI_API_PROXY_TARGET: targetApiURL,
  };

  if (directBrowserAPI && !env.NEXT_PUBLIC_SRAPI_BASE_URL) {
    env.NEXT_PUBLIC_SRAPI_BASE_URL = targetApiURL;
  } else if (!directBrowserAPI) {
    delete env.NEXT_PUBLIC_SRAPI_BASE_URL;
  }

  return env;
}

export function resolveApiURL(env) {
  return (env.SRAPI_WEB_E2E_API_URL ?? env.SRAPI_API_PROXY_TARGET ?? defaultApiURL).replace(
    /\/+$/,
    "",
  );
}

function isMainModule() {
  const entrypoint = process.argv[1];
  return Boolean(entrypoint) && import.meta.url === pathToFileURL(entrypoint).href;
}

function run(cmd, args, label, childEnv) {
  console.log("");
  console.log(`==> web-check-e2e: ${label}`);
  const result = spawnSync(cmd, args, {
    cwd: webDir,
    stdio: "inherit",
    shell: isWindows,
    env: childEnv,
  });
  if (result.status !== 0) {
    console.error(`!! web-check-e2e: ${label} failed with exit code ${result.status ?? 1}`);
    process.exit(result.status ?? 1);
  }
}

async function requireReadyAPI(apiURL) {
  console.log("");
  console.log(`==> web-check-e2e: api preflight (${apiURL})`);

  for (const path of ["/livez", "/readyz"]) {
    const url = `${apiURL}${path}`;
    let response;
    try {
      response = await fetch(url, { signal: AbortSignal.timeout(5_000) });
    } catch (err) {
      throw new Error(
        `SRapi API is not reachable at ${url}. Start the backend first or set SRAPI_WEB_E2E_API_URL.`,
        { cause: err },
      );
    }

    if (!response.ok) {
      const body = await response.text();
      throw new Error(`${path} returned HTTP ${response.status}: ${body.slice(0, 240)}`);
    }

    const body = await response.json().catch(() => null);
    if (body?.data?.status !== "ok") {
      throw new Error(`${path} did not report data.status="ok": ${JSON.stringify(body)}`);
    }
  }
}

async function main() {
  const childEnv = buildChildEnv(process.env, apiURL, { directBrowserAPI: directBrowserBaseURL });

  await requireReadyAPI(apiURL).catch((err) => {
    console.error(`!! web-check-e2e: api preflight failed: ${err.message}`);
    process.exit(1);
  });

  run("npm", ["run", "build"], "build", childEnv);

  if (process.env.SRAPI_WEB_E2E_SKIP_INSTALL !== "1") {
    run("npm", ["run", "test:e2e:install"], "install playwright chromium", childEnv);
  }

  run("npm", ["run", "test:e2e"], "playwright test", childEnv);

  console.log("");
  console.log("==> web-check-e2e: all e2e gates passed");
}

if (isMainModule()) {
  await main();
}
