#!/usr/bin/env node
import { existsSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { pathToFileURL } from "node:url";

const envPath = process.env.SRAPI_BOOTSTRAP_ENV_FILE || ".env";
const examplePath = process.env.SRAPI_BOOTSTRAP_ENV_EXAMPLE || ".env.example";

const replacements = new Map([
  ["DATABASE_PASSWORD", () => randomSecret("db")],
  ["JWT_SECRET", () => randomSecret("jwt")],
  ["SRAPI_MASTER_KEY", () => randomSecret("master")],
  ["API_KEY_PEPPER", () => randomSecret("pepper")],
  ["BOOTSTRAP_ADMIN_PASSWORD", () => randomPassword()],
]);

function main() {
  if (existsSync(envPath)) {
    console.log(`${envPath} already exists; leaving it unchanged.`);
    return;
  }
  assert(statSync(examplePath).isFile(), `${examplePath} is missing`);

  const output = rewriteEnv(readFileSync(examplePath, "utf8"));
  writeFileSync(envPath, output, { mode: 0o600, flag: "wx" });
  console.log(`${envPath} created with generated local secrets.`);
}

function rewriteEnv(content) {
  const seen = new Set();
  const lines = content.split("\n").map((line) => {
    const match = line.match(/^([A-Z0-9_]+)=(.*)$/);
    if (!match) {
      return line;
    }
    const [, key] = match;
    const replacement = replacements.get(key);
    if (!replacement) {
      return line;
    }
    seen.add(key);
    return `${key}=${replacement()}`;
  });

  for (const key of replacements.keys()) {
    assert(seen.has(key), `${examplePath} is missing ${key}`);
  }
  return lines.join("\n");
}

function randomSecret(prefix) {
  return `srapi_${prefix}_${randomBytes(32).toString("base64url")}`;
}

function randomPassword() {
  return `srapi_admin_${randomBytes(24).toString("base64url")}`;
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

export { rewriteEnv };

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
