#!/usr/bin/env node
import { chmodSync, existsSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { randomPassword, randomSecret } from "./bootstrap-env.mjs";
import { checkEnvFile, parseEnv } from "./env-check.mjs";

const envPath = process.env.SRAPI_REPAIR_ENV_FILE || ".env";
const examplePath = process.env.SRAPI_REPAIR_ENV_EXAMPLE || ".env.example";

const repairers = new Map([
  ["DATABASE_PASSWORD", () => randomSecret("db")],
  ["JWT_SECRET", () => randomSecret("jwt")],
  ["SRAPI_MASTER_KEY", () => randomSecret("master")],
  ["TOTP_ENCRYPTION_KEY", () => randomSecret("totp")],
  ["API_KEY_PEPPER", () => randomSecret("pepper")],
  ["BOOTSTRAP_ADMIN_PASSWORD", () => randomPassword()],
]);

function main() {
  const result = repairEnvFile(envPath, examplePath);
  if (result.changedKeys.length === 0 && !result.permissionsChanged) {
    console.log(`${envPath} env already passes repair checks.`);
    return;
  }
  const changes = [];
  if (result.permissionsChanged) changes.push("permissions");
  changes.push(...result.changedKeys);
  console.log(`${envPath} repaired: ${changes.join(", ")}`);
}

function repairEnvFile(file, exampleFile = ".env.example") {
  if (!existsSync(file)) {
    assert(statSync(exampleFile).isFile(), `${exampleFile} is missing`);
    writeFileSync(file, readFileSync(exampleFile, "utf8"), { mode: 0o600, flag: "wx" });
  }

  const beforeStat = statSync(file);
  assert(beforeStat.isFile(), `${file} is not a file`);
  const permissionsChanged = (beforeStat.mode & 0o077) !== 0;
  if (permissionsChanged) chmodSync(file, 0o600);

  const initialContent = readFileSync(file, "utf8");
  const values = parseEnv(initialContent);
  const findings = checkEnvFile(file);
  const keysToRepair = repairKeys(findings, values);
  if (keysToRepair.size === 0) {
    return { changedKeys: [], permissionsChanged };
  }

  const nextContent = rewriteEnvValues(initialContent, keysToRepair);
  writeFileSync(file, nextContent, { mode: 0o600 });

  const remaining = checkEnvFile(file);
  assert(
    remaining.length === 0,
    `failed to repair ${file}: ${remaining.join("; ")}`,
  );
  return {
    changedKeys: [...keysToRepair.keys()],
    permissionsChanged,
  };
}

function repairKeys(findings, values) {
  const keys = new Map();
  for (const key of repairers.keys()) {
    const needsRepair = findings.some(
      (finding) =>
        finding.includes(`missing ${key}`) ||
        finding.includes(`placeholder value for ${key}`) ||
        finding.includes(`${key} must be at least 32 characters`),
    );
    if (needsRepair || !values.has(key)) keys.set(key, repairers.get(key)());
  }
  return keys;
}

function rewriteEnvValues(content, replacements) {
  const seen = new Set();
  const lines = content.split("\n").map((line) => {
    const match = line.match(/^([A-Z0-9_]+)=.*$/);
    if (!match) return line;
    const key = match[1];
    if (!replacements.has(key)) return line;
    seen.add(key);
    return `${key}=${replacements.get(key)}`;
  });

  for (const [key, value] of replacements) {
    if (!seen.has(key)) lines.push(`${key}=${value}`);
  }
  return lines.join("\n");
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

export { repairEnvFile, repairKeys, rewriteEnvValues };

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
