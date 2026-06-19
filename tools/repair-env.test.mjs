import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { parseEnv } from "./env-check.mjs";
import { repairEnvFile } from "./repair-env.mjs";

test("repair env replaces weak required secrets and preserves existing strong values", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-repair-env-"));
  const envPath = join(tempDir, ".env");
  const strongJwt = `srapi_jwt_${"x".repeat(40)}`;
  writeFileSync(
    envPath,
    readFileSync(".env.example", "utf8").replace(
      "JWT_SECRET=local_dev_jwt_secret_32_bytes_minimum_change_me",
      `JWT_SECRET=${strongJwt}`,
    ),
    { mode: 0o644 },
  );
  chmodSync(envPath, 0o644);

  const result = repairEnvFile(envPath);
  const values = parseEnv(readFileSync(envPath, "utf8"));

  assert.equal((statSync(envPath).mode & 0o077), 0);
  assert.equal(result.permissionsChanged, true);
  assert.equal(values.get("JWT_SECRET"), strongJwt);
  assert.notEqual(values.get("DATABASE_PASSWORD"), "srapi_dev_password_change_me");
  assert.notEqual(values.get("SRAPI_MASTER_KEY"), "local_dev_master_key_32_bytes_minimum_change_me");
  assert.notEqual(values.get("TOTP_ENCRYPTION_KEY"), "local_dev_totp_key_32_bytes_minimum_change_me");
  assert.notEqual(values.get("API_KEY_PEPPER"), "local_dev_api_key_pepper_change_me_32+");
  assert.notEqual(values.get("BOOTSTRAP_ADMIN_PASSWORD"), "password123");
  assert(!result.changedKeys.includes("JWT_SECRET"));
});

test("repair env command reports keys without printing generated secret values", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-repair-env-command-"));
  const envPath = join(tempDir, ".env");
  writeFileSync(envPath, readFileSync(".env.example", "utf8"), { mode: 0o644 });

  const result = spawnSync("node", ["tools/repair-env.mjs"], {
    encoding: "utf8",
    env: { ...process.env, SRAPI_REPAIR_ENV_FILE: envPath },
  });
  const values = parseEnv(readFileSync(envPath, "utf8"));

  assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  assert.match(result.stdout, /DATABASE_PASSWORD/);
  for (const key of [
    "DATABASE_PASSWORD",
    "JWT_SECRET",
    "SRAPI_MASTER_KEY",
    "TOTP_ENCRYPTION_KEY",
    "API_KEY_PEPPER",
    "BOOTSTRAP_ADMIN_PASSWORD",
  ]) {
    assert(!result.stdout.includes(values.get(key)), `${key} leaked to stdout`);
  }
});

test("env repair is exposed through Makefile, docs, and dev entrypoint", () => {
  const makefile = readFileSync("Makefile", "utf8");
  const operations = readFileSync("docs/requirements/OPERATIONS.md", "utf8");
  const qualityGates = readFileSync("docs/requirements/QUALITY_GATES.md", "utf8");
  const devPs1 = readFileSync("tools/dev.ps1", "utf8");

  assert.match(makefile, /ENV_REPAIR \?= node tools\/repair-env\.mjs/);
  assert.match(makefile, /env-repair:/);
  assert.match(operations, /make env-repair/);
  assert.match(qualityGates, /make env-repair/);
  assert.match(devPs1, /"env-repair"/);
  assert.match(devPs1, /Invoke-Step "make" @\("env-repair"\)/);
});
