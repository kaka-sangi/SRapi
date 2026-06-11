#!/usr/bin/env node
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const repoRoot = process.cwd();
const httpserverDir = join(repoRoot, "apps", "api", "internal", "httpserver");
const openapiPath = join(repoRoot, "packages", "openapi", "openapi.yaml");
const copilotSpecPath = join(repoRoot, "apps", "api", "internal", "modules", "copilot", "openapi.spec.yaml");

const methods = new Set(["GET", "POST", "PUT", "PATCH", "DELETE"]);

function main() {
  const registered = collectRegisteredAdminRoutes(httpserverDir);
  const documented = collectOpenAPIAdminOperations(readFileSync(openapiPath, "utf8"));

  const registeredKeys = new Set(registered.keys());
  const documentedKeys = new Set(documented.keys());
  const missing = [...registeredKeys].filter((key) => !documentedKeys.has(key)).sort();
  const extra = [...documentedKeys].filter((key) => !registeredKeys.has(key)).sort();
  const metadataFindings = checkAdminOperationMetadata(documented);
  const copilotSynced = readFileSync(openapiPath, "utf8") === readFileSync(copilotSpecPath, "utf8");

  const findings = [];
  if (missing.length > 0) {
    findings.push("Admin routes missing from packages/openapi/openapi.yaml:");
    for (const key of missing) {
      findings.push(`  - ${key} (${relative(repoRoot, registered.get(key).file)})`);
    }
  }
  if (extra.length > 0) {
    findings.push("Admin OpenAPI operations without a registered route:");
    for (const key of extra) {
      findings.push(`  - ${key}`);
    }
  }
  if (metadataFindings.length > 0) {
    findings.push("Admin OpenAPI operation metadata findings:");
    findings.push(...metadataFindings.map((finding) => `  - ${finding}`));
  }
  if (!copilotSynced) {
    findings.push("apps/api/internal/modules/copilot/openapi.spec.yaml is out of sync; run make openapi-codegen.");
  }

  if (findings.length > 0) {
    console.error(findings.join("\n"));
    process.exit(1);
  }

  console.log(`admin OpenAPI coverage ok (${registered.size} registered routes, ${documented.size} documented operations)`);
}

function collectRegisteredAdminRoutes(dir) {
  const routes = new Map();
  for (const file of listGoFiles(dir)) {
    const text = readFileSync(file, "utf8");
    const routePattern = /HandleFunc\("(?<method>GET|POST|PUT|PATCH|DELETE) (?<path>\/api\/v1\/admin[^" ]*)"/g;
    for (const match of text.matchAll(routePattern)) {
      const method = match.groups.method;
      const path = match.groups.path;
      routes.set(routeKey(method, path), { method, path, file });
    }
  }
  return routes;
}

function listGoFiles(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    const stat = statSync(path);
    if (stat.isDirectory()) {
      out.push(...listGoFiles(path));
    } else if (name.endsWith(".go") && !name.endsWith("_test.go")) {
      out.push(path);
    }
  }
  return out;
}

function collectOpenAPIAdminOperations(yaml) {
  const operations = new Map();
  const lines = yaml.split(/\r?\n/);
  let currentPath = "";
  let currentMethod = "";
  let currentOperation = null;

  for (const line of lines) {
    const pathMatch = /^  (?<path>\/api\/v1\/admin.*):\s*$/.exec(line);
    if (pathMatch) {
      flushOperation(operations, currentPath, currentMethod, currentOperation);
      currentPath = pathMatch.groups.path;
      currentMethod = "";
      currentOperation = null;
      continue;
    }

    const anyPathMatch = /^  \S.*:\s*$/.exec(line);
    if (anyPathMatch && !pathMatch) {
      flushOperation(operations, currentPath, currentMethod, currentOperation);
      currentPath = "";
      currentMethod = "";
      currentOperation = null;
      continue;
    }

    if (!currentPath) {
      continue;
    }

    const methodMatch = /^    (?<method>get|post|put|patch|delete):\s*$/.exec(line);
    if (methodMatch) {
      flushOperation(operations, currentPath, currentMethod, currentOperation);
      currentMethod = methodMatch.groups.method.toUpperCase();
      currentOperation = { operationId: "", summary: "", tags: 0, responses: new Set(), requestBodySchema: "" };
      continue;
    }

    if (!currentOperation) {
      continue;
    }

    const operationIdMatch = /^      operationId:\s*(?<value>\S.*)\s*$/.exec(line);
    if (operationIdMatch) {
      currentOperation.operationId = operationIdMatch.groups.value.trim();
      continue;
    }
    const summaryMatch = /^      summary:\s*(?<value>\S.*)\s*$/.exec(line);
    if (summaryMatch) {
      currentOperation.summary = summaryMatch.groups.value.trim();
      continue;
    }
    if (/^      tags:\s*$/.test(line)) {
      currentOperation.tags += 1;
      continue;
    }
    const responseMatch = /^        (?<code>"?[1-5][0-9][0-9]"?|default):\s*$/.exec(line);
    if (responseMatch) {
      currentOperation.responses.add(responseMatch.groups.code.replaceAll('"', ""));
      continue;
    }
    if (/^              \$ref:\s*"#\/components\/schemas\/[^"]+"\s*$/.test(line)) {
      currentOperation.requestBodySchema = line.trim();
    }
  }
  flushOperation(operations, currentPath, currentMethod, currentOperation);
  return operations;
}

function flushOperation(operations, path, method, operation) {
  if (!path || !method || !operation || !methods.has(method)) {
    return;
  }
  operations.set(routeKey(method, path), { path, method, ...operation });
}

function checkAdminOperationMetadata(operations) {
  const findings = [];
  const operationIds = new Map();
  for (const [key, operation] of operations) {
    if (!operation.operationId) {
      findings.push(`${key} is missing operationId`);
    } else if (operationIds.has(operation.operationId)) {
      findings.push(`${key} duplicates operationId ${operation.operationId} from ${operationIds.get(operation.operationId)}`);
    } else {
      operationIds.set(operation.operationId, key);
    }
    if (!operation.summary) {
      findings.push(`${key} is missing summary`);
    }
    if (operation.tags === 0) {
      findings.push(`${key} is missing tags`);
    }
    const hasErrorResponse = [...operation.responses].some((code) => code === "default" || code.startsWith("4") || code.startsWith("5"));
    if (!hasErrorResponse) {
      findings.push(`${key} is missing a 4xx/5xx/default error response`);
    }
  }
  return findings;
}

function routeKey(method, path) {
  return `${method} ${path}`;
}

try {
  main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
