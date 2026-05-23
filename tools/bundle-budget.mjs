#!/usr/bin/env node
/* eslint-disable no-console */
/**
 * SRapi v0.1.0 frontend bundle-size budget enforcer.
 *
 * Usage: `node tools/bundle-budget.mjs` (run after `next build`).
 *
 * Reads `apps/web/bundle-budget.json`, walks `apps/web/.next/static/chunks/**`,
 * and fails non-zero if any glob group exceeds its `max_bytes`. Output is a
 * compact summary so reviewers can spot regressions quickly.
 */
import { readFile, readdir, stat } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");
const webDir = path.join(repoRoot, "apps", "web");
const buildRoot = path.join(webDir, ".next");
const budgetFile = path.join(webDir, "bundle-budget.json");

function fmtBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

async function walkFiles(dir) {
  const out = [];
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...(await walkFiles(full)));
    } else if (entry.isFile()) {
      out.push(full);
    }
  }
  return out;
}

// Convert a glob (e.g. `static/chunks/main-app-*.js`, `static/chunks/**/*.js`)
// into a RegExp anchored to a path relative to `rootDir`.
//
// We use placeholder tokens so the multi-step transform never re-rewrites
// its own intermediate output.
function globToRegExp(pattern) {
  const DOUBLE = "__DOUBLESTAR__";
  const SINGLE = "__SINGLESTAR__";
  const QUESTION = "__QMARK__";
  const tokenized = pattern
    .replace(/\*\*/g, DOUBLE)
    .replace(/\*/g, SINGLE)
    .replace(/\?/g, QUESTION);
  const escaped = tokenized.replace(/[.+^${}()|[\]\\]/g, "\\$&");
  const expanded = escaped
    .replace(new RegExp(`${DOUBLE}/`, "g"), "(?:.*/)?") // `**/` -> any segments
    .replace(new RegExp(DOUBLE, "g"), ".*")
    .replace(new RegExp(SINGLE, "g"), "[^/]*")
    .replace(new RegExp(QUESTION, "g"), "[^/]");
  return new RegExp(`^${expanded}$`);
}

async function listFiles(rootDir, patterns) {
  const all = await walkFiles(rootDir);
  const regexps = patterns.map(globToRegExp);
  return all.filter((file) => {
    const rel = path.relative(rootDir, file).replace(/\\/g, "/");
    return regexps.some((re) => re.test(rel));
  });
}

async function main() {
  const raw = await readFile(budgetFile, "utf8");
  const budget = JSON.parse(raw);
  const staticRoot = path.join(buildRoot, "static");

  const failures = [];
  console.log("==> bundle-budget");
  for (const [groupName, group] of Object.entries(budget.groups)) {
    const files = await listFiles(staticRoot, group.files);
    let total = 0;
    let largest = { file: "", size: 0 };
    for (const file of files) {
      const s = await stat(file);
      total += s.size;
      if (s.size > largest.size) {
        largest = { file: path.relative(staticRoot, file), size: s.size };
      }
    }
    const metric = group.metric ?? "total";
    const observed = metric === "largest" ? largest.size : total;
    const ok = observed <= group.max_bytes;
    const symbol = ok ? "ok" : "!!";
    console.log(
      `  [${symbol}] ${groupName} (${metric}): ${fmtBytes(observed)} ` +
        `(${files.length} files; largest ${fmtBytes(largest.size)} ${largest.file || "n/a"}), ` +
        `cap ${fmtBytes(group.max_bytes)}`,
    );
    if (!ok) {
      failures.push({
        group: groupName,
        metric,
        observed,
        max_bytes: group.max_bytes,
        largest,
      });
    }
  }

  if (failures.length > 0) {
    console.error("");
    console.error("!! bundle-budget exceeded:");
    for (const f of failures) {
      console.error(
        `   ${f.group} (${f.metric}): ${fmtBytes(f.observed)} > cap ${fmtBytes(f.max_bytes)}`,
      );
      if (f.metric !== "largest" && f.largest && f.largest.size > 0) {
        console.error(`     largest single chunk: ${fmtBytes(f.largest.size)} ${f.largest.file}`);
      }
    }
    console.error("");
    console.error(
      "Update apps/web/bundle-budget.json only if the increase is intentional and reviewed.",
    );
    process.exit(1);
  }

  console.log("==> bundle-budget: all groups within cap");
}

main().catch((err) => {
  console.error("bundle-budget failed:", err);
  process.exit(2);
});
