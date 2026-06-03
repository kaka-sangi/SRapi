#!/usr/bin/env node
// SRapi frontend bundle budget gate.
// Reads apps/web/bundle-budget.json and checks the emitted JS chunks under
// apps/web/.next/static against each group's byte cap. Run after `next build`;
// web-check.mjs invokes it automatically. Exit 1 on any breach.
import { readFile, readdir, stat } from "node:fs/promises";
import { join, dirname, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(__dirname, "..");
const webDir = join(repoRoot, "apps", "web");
const budgetPath = join(webDir, "bundle-budget.json");
const staticDir = join(webDir, ".next", "static");

/** Recursively collect every file under `dir` (absolute paths). */
async function walk(dir) {
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return [];
  }
  const out = [];
  for (const entry of entries) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...(await walk(full)));
    } else {
      out.push(full);
    }
  }
  return out;
}

/** Convert a simple glob (supports ** and *) to a RegExp anchored to staticDir. */
function globToRegExp(glob) {
  const escaped = glob
    .replace(/[.+^${}()|[\]\\]/g, "\\$&")
    .replace(/\*\*\//g, "::DOUBLESLASH::")
    .replace(/\*\*/g, "::DOUBLE::")
    .replace(/\*/g, "[^/]*")
    .replace(/::DOUBLESLASH::/g, "(?:.*/)?")
    .replace(/::DOUBLE::/g, ".*");
  return new RegExp(`^${escaped}$`);
}

async function collectFiles(globs) {
  const all = await walk(staticDir);
  const matchers = globs.map(globToRegExp);
  return all.filter((abs) => {
    const rel = relative(staticDir, abs).split(sep).join("/");
    return matchers.some((re) => re.test(rel));
  });
}

async function main() {
  const budget = JSON.parse(await readFile(budgetPath, "utf8"));
  const groups = budget.groups ?? {};
  let failed = false;

  for (const [name, group] of Object.entries(groups)) {
    const globs = group.files ?? [];
    const metric = group.metric ?? "total";
    const maxBytes = group.max_bytes ?? 0;
    const matched = await collectFiles(globs);
    const sizes = await Promise.all(matched.map((f) => stat(f).then((s) => s.size)));
    const total = sizes.reduce((a, b) => a + b, 0);
    const largest = sizes.reduce((a, b) => Math.max(a, b), 0);
    const value = metric === "largest" ? largest : total;
    const valueLabel = `${(value / 1024).toFixed(0)} KiB`;
    const limitLabel = `${(maxBytes / 1024).toFixed(0)} KiB`;
    if (value > maxBytes) {
      failed = true;
      console.error(`✗ ${name}: ${valueLabel} > ${limitLabel} (${matched.length} files)`);
    } else {
      console.info(`✓ ${name}: ${valueLabel} <= ${limitLabel} (${matched.length} files)`);
    }
  }

  if (failed) process.exit(1);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
