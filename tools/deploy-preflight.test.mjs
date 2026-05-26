import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { rewriteEnv } from "./bootstrap-env.mjs";
import { collectDeployPreflight } from "./deploy-preflight.mjs";

test("deploy preflight accepts generated env and reports host tool warnings", () => {
  const envPath = writeGeneratedEnv("srapi-deploy-preflight-good-");
  const report = collectDeployPreflight({
    envFile: envPath,
    commandRunner: fakeCommandRunner({
      "node tools/observability-rules-check.mjs": true,
    }),
  });

  assert.deepEqual(report.errors, []);
  assert(
    report.warnings.some((warning) => warning.includes("Docker Compose")),
    "missing Docker Compose should be a warning by default",
  );
});

test("deploy preflight rejects weak env files", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-deploy-preflight-bad-"));
  const envPath = join(tempDir, ".env");
  writeFileSync(envPath, readFileSync(".env.example", "utf8"), { mode: 0o644 });
  chmodSync(envPath, 0o644);

  const report = collectDeployPreflight({
    envFile: envPath,
    commandRunner: alwaysOkCommandRunner,
  });

  assert(report.errors.some((error) => error.includes("weak placeholder")));
  assert(report.errors.some((error) => error.includes("permissions")));
});

test("deploy preflight can promote missing host tools to errors", () => {
  const envPath = writeGeneratedEnv("srapi-deploy-preflight-strict-");
  const report = collectDeployPreflight({
    envFile: envPath,
    strictTools: true,
    commandRunner: fakeCommandRunner({
      "node tools/observability-rules-check.mjs": true,
    }),
  });

  assert(report.errors.some((error) => error.includes("Docker Compose")));
});

test("deploy preflight command supports explicit env file path", () => {
  const envPath = writeGeneratedEnv("srapi-deploy-preflight-command-");
  const result = spawnSync("node", ["tools/deploy-preflight.mjs"], {
    encoding: "utf8",
    env: { ...process.env, SRAPI_DEPLOY_PREFLIGHT_ENV_FILE: envPath },
  });

  assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  assert.match(result.stdout, /deploy preflight ok/);
});

test("deploy preflight is exposed through Makefile, docs, and dev entrypoint", () => {
  const makefile = readFileSync("Makefile", "utf8");
  const devPs1 = readFileSync("tools/dev.ps1", "utf8");
  const readme = readFileSync("README.md", "utf8");
  const operations = readFileSync("docs/OPERATIONS.md", "utf8");
  const qualityGates = readFileSync("specs/QUALITY_GATES.md", "utf8");

  assert.match(
    makefile,
    /DEPLOY_PREFLIGHT \?= node tools\/deploy-preflight\.mjs/,
  );
  assert.match(makefile, /deploy-preflight:/);
  assert.match(devPs1, /"deploy-preflight"/);
  assert.match(devPs1, /Invoke-Step "make" @\("deploy-preflight"\)/);
  assert.match(readme, /make deploy-preflight/);
  assert.match(operations, /make deploy-preflight/);
  assert.match(qualityGates, /make deploy-preflight/);
});

function writeGeneratedEnv(prefix) {
  const tempDir = mkdtempSync(join(tmpdir(), prefix));
  const envPath = join(tempDir, ".env");
  writeFileSync(envPath, rewriteEnv(readFileSync(".env.example", "utf8")), {
    mode: 0o600,
  });
  return envPath;
}

function fakeCommandRunner(statusByCommand) {
  return (command, args) => {
    const key = [command, ...args].join(" ");
    return { ok: Boolean(statusByCommand[key]), output: "" };
  };
}

function alwaysOkCommandRunner() {
  return { ok: true, output: "" };
}
