#!/usr/bin/env node
// SRapi frontend quality gate. Runs the full serial pipeline:
//   typecheck -> lint -> unit tests (+axe) -> build -> bundle budget.
// Any failure stops the chain and exits non-zero. Invoked by `make web-check`.
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(__dirname, "..");
const webDir = join(repoRoot, "apps", "web");

const steps = [
  { name: "typecheck", cmd: "npm", args: ["run", "typecheck"], cwd: webDir },
  { name: "lint", cmd: "npm", args: ["run", "lint"], cwd: webDir },
  { name: "test", cmd: "npm", args: ["run", "test"], cwd: webDir },
  { name: "build", cmd: "npm", args: ["run", "build"], cwd: webDir },
  { name: "bundle-budget", cmd: "node", args: [join(__dirname, "bundle-budget.mjs")], cwd: repoRoot },
];

for (const step of steps) {
  console.info(`\n▶ web-check: ${step.name}`);
  const result = spawnSync(step.cmd, step.args, { cwd: step.cwd, stdio: "inherit" });
  if (result.status !== 0) {
    console.error(`\n✗ web-check failed at: ${step.name}`);
    process.exit(result.status ?? 1);
  }
}

console.info("\n✓ web-check passed");
