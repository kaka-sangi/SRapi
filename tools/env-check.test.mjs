import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { checkEnvFile, parseEnv } from "./env-check.mjs";
import { rewriteEnv } from "./bootstrap-env.mjs";

test("env check accepts bootstrap generated env files", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-env-check-good-"));
  const envPath = join(tempDir, ".env");
  writeFileSync(envPath, rewriteEnv(readFileSync(".env.example", "utf8")), {
    mode: 0o600,
  });

  assert.deepEqual(checkEnvFile(envPath), []);
});

test("env check rejects weak placeholders and permissive permissions", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-env-check-bad-"));
  const envPath = join(tempDir, ".env");
  writeFileSync(envPath, readFileSync(".env.example", "utf8"), { mode: 0o644 });
  chmodSync(envPath, 0o644);

  const findings = checkEnvFile(envPath);

  assert(findings.some((finding) => finding.includes("permissions")));
  assert(findings.some((finding) => finding.includes("DATABASE_PASSWORD")));
  assert(findings.some((finding) => finding.includes("JWT_SECRET")));
  assert(
    findings.some((finding) => finding.includes("TOTP_ENCRYPTION_KEY")),
  );
  assert(
    findings.some((finding) => finding.includes("BOOTSTRAP_ADMIN_PASSWORD")),
  );
});

test("env check command supports explicit env file path", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-env-check-command-"));
  const envPath = join(tempDir, ".env");
  writeFileSync(envPath, rewriteEnv(readFileSync(".env.example", "utf8")), {
    mode: 0o600,
  });

  const result = spawnSync("node", ["tools/env-check.mjs"], {
    encoding: "utf8",
    env: { ...process.env, SRAPI_ENV_CHECK_FILE: envPath },
  });

  assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  assert.match(result.stdout, /env check ok/);
});

test("env parser handles comments and quoted values", () => {
  const values = parseEnv(`
    # comment
    JWT_SECRET="quoted-secret"
    API_KEY_PEPPER='single-quoted'
  `);

  assert.equal(values.get("JWT_SECRET"), "quoted-secret");
  assert.equal(values.get("API_KEY_PEPPER"), "single-quoted");
});
