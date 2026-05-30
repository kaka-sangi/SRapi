#!/usr/bin/env node
import { existsSync, readFileSync, statSync } from "node:fs";
import { pathToFileURL } from "node:url";

const envPath = process.env.SRAPI_ENV_CHECK_FILE || ".env";
const requiredKeys = [
  "DATABASE_PASSWORD",
  "JWT_SECRET",
  "SRAPI_MASTER_KEY",
  "TOTP_ENCRYPTION_KEY",
  "API_KEY_PEPPER",
  "BOOTSTRAP_ADMIN_PASSWORD",
];
const weakValues = new Map([
  [
    "DATABASE_PASSWORD",
    new Set(["", "srapi_dev_password_change_me", "postgres", "password"]),
  ],
  [
    "JWT_SECRET",
    new Set(["", "local_dev_jwt_secret_32_bytes_minimum_change_me"]),
  ],
  [
    "SRAPI_MASTER_KEY",
    new Set(["", "local_dev_master_key_32_bytes_minimum_change_me"]),
  ],
  [
    "TOTP_ENCRYPTION_KEY",
    new Set(["", "local_dev_totp_key_32_bytes_minimum_change_me"]),
  ],
  ["API_KEY_PEPPER", new Set(["", "local_dev_api_key_pepper_change_me_32+"])],
  [
    "BOOTSTRAP_ADMIN_PASSWORD",
    new Set(["", "password123", "admin", "admin123"]),
  ],
]);

function main() {
  const findings = checkEnvFile(envPath);
  if (findings.length > 0) {
    for (const finding of findings) {
      console.error(finding);
    }
    process.exit(1);
  }
  console.log(`${envPath} env check ok`);
}

function checkEnvFile(file) {
  const findings = [];
  if (!existsSync(file)) {
    return [`${file} is missing; run make bootstrap-env first`];
  }
  const stat = statSync(file);
  if (!stat.isFile()) {
    return [`${file} is not a file`];
  }
  if ((stat.mode & 0o077) !== 0) {
    findings.push(`${file} permissions must not grant group/other access`);
  }

  const values = parseEnv(readFileSync(file, "utf8"));
  for (const key of requiredKeys) {
    if (!values.has(key)) {
      findings.push(`${file} is missing ${key}`);
      continue;
    }
    const value = values.get(key) ?? "";
    const weak = weakValues.get(key) ?? new Set();
    if (weak.has(value)) {
      findings.push(`${file} has weak placeholder value for ${key}`);
    }
    if (value.length < 32) {
      findings.push(`${file} ${key} must be at least 32 characters`);
    }
  }
  return findings;
}

function parseEnv(content) {
  const values = new Map();
  for (const rawLine of content.split("\n")) {
    const line = rawLine.trim();
    if (line === "" || line.startsWith("#")) {
      continue;
    }
    const index = line.indexOf("=");
    if (index <= 0) {
      continue;
    }
    const key = line.slice(0, index);
    const value = line.slice(index + 1);
    values.set(key, unquoteEnvValue(value));
  }
  return values;
}

function unquoteEnvValue(value) {
  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    return value.slice(1, -1);
  }
  return value;
}

export { checkEnvFile, parseEnv };

if (isDirectExecution()) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

function isDirectExecution() {
  return (
    process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href
  );
}
