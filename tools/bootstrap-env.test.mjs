import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { rewriteEnv } from "./bootstrap-env.mjs";

test("bootstrap env rewrites weak local placeholders", () => {
  const output = rewriteEnv(readFileSync(".env.example", "utf8"));

  assert.doesNotMatch(output, /srapi_dev_password_change_me/);
  assert.doesNotMatch(
    output,
    /local_dev_jwt_secret_32_bytes_minimum_change_me/,
  );
  assert.doesNotMatch(
    output,
    /local_dev_master_key_32_bytes_minimum_change_me/,
  );
  assert.doesNotMatch(output, /local_dev_api_key_pepper_change_me_32\+/);
  assert.doesNotMatch(output, /BOOTSTRAP_ADMIN_PASSWORD=password123/);
  for (const key of [
    "DATABASE_PASSWORD",
    "JWT_SECRET",
    "SRAPI_MASTER_KEY",
    "API_KEY_PEPPER",
    "BOOTSTRAP_ADMIN_PASSWORD",
  ]) {
    const value = envValue(output, key);
    assert.ok(
      value.length >= 32,
      `${key} should be generated as a strong local value`,
    );
  }
});

test("bootstrap env creates private .env without printing generated secrets", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-bootstrap-env-"));
  const envPath = join(tempDir, ".env");
  const result = spawnSync("node", ["tools/bootstrap-env.mjs"], {
    encoding: "utf8",
    env: {
      ...process.env,
      SRAPI_BOOTSTRAP_ENV_FILE: envPath,
      SRAPI_BOOTSTRAP_ENV_EXAMPLE: ".env.example",
    },
  });

  assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  const output = readFileSync(envPath, "utf8");
  assert.match(result.stdout, /created with generated local secrets/);
  assert.doesNotMatch(
    result.stdout,
    /srapi_admin_|srapi_jwt_|srapi_master_|srapi_pepper_|srapi_db_/,
  );
  assert.equal(statSync(envPath).mode & 0o777, 0o600);
  assert.equal(
    envValue(output, "BOOTSTRAP_ADMIN_PASSWORD").startsWith("srapi_admin_"),
    true,
  );
});

test("bootstrap env leaves an existing .env unchanged", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-bootstrap-env-existing-"));
  const envPath = join(tempDir, ".env");
  const existing = "JWT_SECRET=keep_existing\n";
  const first = spawnSync("node", [
    "-e",
    `require("node:fs").writeFileSync(${JSON.stringify(envPath)}, ${JSON.stringify(existing)})`,
  ]);
  assert.equal(first.status, 0);

  const result = spawnSync("node", ["tools/bootstrap-env.mjs"], {
    encoding: "utf8",
    env: {
      ...process.env,
      SRAPI_BOOTSTRAP_ENV_FILE: envPath,
      SRAPI_BOOTSTRAP_ENV_EXAMPLE: ".env.example",
    },
  });

  assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  assert.match(result.stdout, /already exists; leaving it unchanged/);
  assert.equal(readFileSync(envPath, "utf8"), existing);
});

function envValue(content, key) {
  const match = content.match(new RegExp(`^${key}=(.*)$`, "m"));
  assert.ok(match, `${key} is missing`);
  return match[1];
}
