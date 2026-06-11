#!/usr/bin/env node
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const repoRoot = process.cwd();
const webSrcDir = join(repoRoot, "apps", "web", "src");
const managedRoutes = [
  "/api/v1/admin/quick-setup",
  "/api/v1/admin/models/quick-map",
  "/api/v1/admin/accounts/health-summary",
];

function main() {
  const findings = [];
  for (const file of listSourceFiles(webSrcDir)) {
    const rel = relative(repoRoot, file);
    const text = readFileSync(file, "utf8");
    for (const route of managedRoutes) {
      const line = lineNumber(text, route);
      if (line > 0) {
        findings.push(`${rel}:${line} hard-codes ${route}; use adminApi/generated SDK instead.`);
      }
    }
  }

  if (findings.length > 0) {
    console.error("Managed admin SDK routes must not be called through raw frontend paths:");
    for (const finding of findings) {
      console.error(`  - ${finding}`);
    }
    process.exit(1);
  }

  console.log(`web admin SDK route check ok (${managedRoutes.length} managed routes)`);
}

function listSourceFiles(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      out.push(...listSourceFiles(path));
      continue;
    }
    if (/\.(ts|tsx)$/.test(name) && !name.endsWith(".d.ts")) {
      out.push(path);
    }
  }
  return out;
}

function lineNumber(text, needle) {
  const index = text.indexOf(needle);
  if (index < 0) {
    return 0;
  }
  return text.slice(0, index).split(/\r?\n/).length;
}

try {
  main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
