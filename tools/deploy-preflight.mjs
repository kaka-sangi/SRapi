#!/usr/bin/env node
import { existsSync, readFileSync, statSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { isAbsolute, join } from "node:path";
import { pathToFileURL } from "node:url";
import { checkEnvFile } from "./env-check.mjs";

const defaultEnvFile =
  process.env.SRAPI_DEPLOY_PREFLIGHT_ENV_FILE ||
  process.env.SRAPI_ENV_CHECK_FILE ||
  ".env";
const defaultStrictTools =
  process.env.SRAPI_DEPLOY_PREFLIGHT_STRICT_TOOLS === "1";

const requiredFiles = [
  ".env.example",
  "Makefile",
  "deploy/docker-compose.yml",
  "deploy/k8s/api-deployment.yaml",
  "deploy/k8s/api-hpa.yaml",
  "deploy/k8s/postgres-backup-cronjob.yaml",
  "deploy/prometheus.yml",
  "deploy/prometheus-srapi-alerts.yaml",
  "deploy/alertmanager.yml",
];

const requiredMakeTargets = [
  "bootstrap-env",
  "env-check",
  "deploy-preflight",
  "dev-up",
  "dev-down",
  "smoke-release",
  "backup-postgres",
  "restore-postgres",
  "observability-rules-check",
];

function main() {
  const report = collectDeployPreflight({
    envFile: defaultEnvFile,
    strictTools: defaultStrictTools,
  });
  printReport(report);
  if (report.errors.length > 0) {
    process.exit(1);
  }
}

function collectDeployPreflight({
  envFile = defaultEnvFile,
  root = ".",
  strictTools = false,
  commandRunner = defaultCommandRunner,
} = {}) {
  const errors = [];
  const warnings = [];

  for (const file of requiredFiles) {
    assertFile(root, file, errors);
  }

  for (const finding of checkEnvFile(resolvePath(root, envFile))) {
    errors.push(`env: ${finding}`);
  }

  const makefile = readText(root, "Makefile", errors);
  if (makefile !== "") {
    assertMakefile(makefile, errors);
  }

  const compose = readText(root, "deploy/docker-compose.yml", errors);
  if (compose !== "") {
    assertComposeConfig(compose, errors);
  }

  assertObservabilityRules(commandRunner, root, errors);
  collectToolFindings(commandRunner, strictTools, errors, warnings);

  return { errors, warnings };
}

function assertFile(root, file, errors) {
  const path = resolvePath(root, file);
  if (!existsSync(path)) {
    errors.push(`${file} is missing`);
    return;
  }
  if (!statSync(path).isFile()) {
    errors.push(`${file} is not a file`);
  }
}

function readText(root, file, errors) {
  const path = resolvePath(root, file);
  try {
    return readFileSync(path, "utf8");
  } catch (error) {
    errors.push(`${file} cannot be read: ${errorMessage(error)}`);
    return "";
  }
}

function assertMakefile(content, errors) {
  for (const target of requiredMakeTargets) {
    const pattern = new RegExp(`(^|\\n)${escapeRegExp(target)}:`);
    if (!pattern.test(content)) {
      errors.push(`Makefile is missing ${target} target`);
    }
  }
  for (const phrase of [
    "DEPLOY_PREFLIGHT ?= node tools/deploy-preflight.mjs",
    "$(DEPLOY_PREFLIGHT)",
    "pg_dump",
    "pg_restore",
    "sha256sum",
  ]) {
    if (!content.includes(phrase)) {
      errors.push(`Makefile is missing ${phrase}`);
    }
  }
}

function assertComposeConfig(content, errors) {
  const required = [
    ["postgres service", /^  postgres:/m],
    ["redis service", /^  redis:/m],
    ["api service", /^  api:/m],
    ["prometheus service", /^  prometheus:/m],
    ["alertmanager service", /^  alertmanager:/m],
    [
      "api readiness healthcheck",
      /test: \["CMD", "\/srapi", "-healthcheck", "-healthcheck-path=\/readyz"\]/,
    ],
    ["api restart policy", /api:\n(?:    .+\n)*?    restart: unless-stopped/m],
    ["postgres restart policy", /postgres:\n(?:    .+\n)*?    restart: unless-stopped/m],
    ["redis restart policy", /redis:\n(?:    .+\n)*?    restart: unless-stopped/m],
    ["resource limits", /resources:\n        limits:/],
    ["redis pool size env", /REDIS_POOL_SIZE: \$\{REDIS_POOL_SIZE:-32\}/],
    [
      "redis read timeout env",
      /REDIS_READ_TIMEOUT_SECONDS: \$\{REDIS_READ_TIMEOUT_SECONDS:-2\}/,
    ],
    [
      "postgres health dependency",
      /postgres:\n        condition: service_healthy/,
    ],
    ["redis health dependency", /redis:\n        condition: service_healthy/],
    ["observability profile", /profiles: \["observability"\]/],
    [
      "database password interpolation",
      /DATABASE_PASSWORD: \$\{DATABASE_PASSWORD:-/,
    ],
    ["jwt secret interpolation", /JWT_SECRET: \$\{JWT_SECRET:-/],
    ["master key interpolation", /SRAPI_MASTER_KEY: \$\{SRAPI_MASTER_KEY:-/],
    ["api key pepper interpolation", /API_KEY_PEPPER: \$\{API_KEY_PEPPER:-/],
    [
      "bootstrap admin password interpolation",
      /BOOTSTRAP_ADMIN_PASSWORD: \$\{BOOTSTRAP_ADMIN_PASSWORD:\?/,
    ],
  ];
  for (const [name, pattern] of required) {
    if (!pattern.test(content)) {
      errors.push(`deploy/docker-compose.yml is missing ${name}`);
    }
  }
}

function assertObservabilityRules(commandRunner, root, errors) {
  const result = commandRunner(
    "node",
    ["tools/observability-rules-check.mjs"],
    {
      cwd: root,
    },
  );
  if (!result.ok) {
    errors.push(
      `observability rules check failed${formatCommandOutput(result.output)}`,
    );
  }
}

function collectToolFindings(commandRunner, strictTools, errors, warnings) {
  const target = strictTools ? errors : warnings;
  const dockerCompose =
    commandRunner("docker", ["compose", "version"]).ok ||
    commandRunner("docker-compose", ["--version"]).ok;
  if (!dockerCompose) {
    target.push(
      "Docker Compose command not found; install the docker compose plugin or docker-compose before running make dev-up",
    );
  }

  for (const tool of ["pg_dump", "pg_restore", "sha256sum", "curl"]) {
    if (!commandRunner(tool, ["--version"]).ok) {
      target.push(
        `${tool} not found; related backup/restore or smoke target may fail`,
      );
    }
  }
}

function defaultCommandRunner(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    encoding: "utf8",
  });
  return {
    ok: result.status === 0,
    output: [result.stdout, result.stderr, result.error?.message]
      .filter(Boolean)
      .join("\n")
      .trim(),
  };
}

function printReport(report) {
  for (const warning of report.warnings) {
    console.warn(`warning: ${warning}`);
  }
  for (const error of report.errors) {
    console.error(`error: ${error}`);
  }
  if (report.errors.length === 0) {
    console.log("deploy preflight ok");
  }
}

function resolvePath(root, file) {
  return isAbsolute(file) ? file : join(root, file);
}

function formatCommandOutput(output) {
  if (!output) {
    return "";
  }
  const firstLine = output.split("\n").find(Boolean);
  return firstLine ? `: ${firstLine}` : "";
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export { collectDeployPreflight, defaultCommandRunner };

if (isDirectExecution()) {
  try {
    main();
  } catch (error) {
    console.error(errorMessage(error));
    process.exit(1);
  }
}

function isDirectExecution() {
  return (
    process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href
  );
}
