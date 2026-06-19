#!/usr/bin/env node
import { readFileSync, statSync } from "node:fs";
import { dirname, normalize, sep } from "node:path";
import { pathToFileURL } from "node:url";

const ruleFiles = ["deploy/prometheus-srapi-alerts.yaml"];
const prometheusConfig = "deploy/prometheus.yml";
const alertmanagerConfig = "deploy/alertmanager.yml";
const composeConfig = "deploy/docker-compose.yml";

const requiredPhrases = [
  "srapi_ops_alert_events",
  "srapi_scheduler_no_available_total",
  "SRapiCriticalOpsAlertsFiring",
  "SRapiWarningOpsAlertsPersisting",
  "SRapiSchedulerNoAvailableAccounts",
  "runbook_url:",
];

const forbiddenPatterns = [
  /\bapi[_-]?key\b/i,
  /\baccount[_-]?id\b/i,
  /\bprovider[_-]?id\b/i,
  /\buser[_-]?id\b/i,
  /\brequest[_-]?id\b/i,
  /\bfingerprint\b/i,
  /\brule[_-]?id\b/i,
  /\bcredential\b/i,
  /\bauthorization\b/i,
  /\bcookie\b/i,
  /\bprompt\b/i,
  /\bmessages?\b/i,
];

const allowedLabelNames = new Set([
  "severity",
  "status",
  "service",
  "team",
  "component",
]);
const allowedAlertmanagerGroupLabels = new Set([
  "service",
  "severity",
  "component",
]);

function main() {
  const combined = ruleFiles.map(readRuleFile).join("\n");

  for (const phrase of requiredPhrases) {
    assert(combined.includes(phrase), `observability rules missing ${phrase}`);
  }
  for (const pattern of forbiddenPatterns) {
    assert(
      !pattern.test(combined),
      `observability rules include forbidden high-cardinality or sensitive field: ${pattern}`,
    );
  }
  for (const file of ruleFiles) {
    assertRuleLabelNames(file, readFileSync(file, "utf8"));
  }
  assertPrometheusConfig(readRuleFile(prometheusConfig));
  assertAlertmanagerConfig(readRuleFile(alertmanagerConfig));
  assertComposeConfig(readRuleFile(composeConfig));

  console.log("observability rules check ok");
}

function assertRuleLabelNames(file, content) {
  const rules = extractAlertRules(file, content);
  assert(rules.length > 0, `${file} does not define any alert rules`);
  for (const rule of rules) {
    assert(
      rule.expr !== "",
      `${file}:${rule.line} alert ${rule.alert} is missing expr`,
    );
    assert(
      rule.annotations.has("runbook_url"),
      `${file}:${rule.line} alert ${rule.alert} is missing runbook_url annotation`,
    );
    assertRunbookURL(file, rule);
    for (const pattern of forbiddenPatterns) {
      assert(
        !pattern.test(rule.expr),
        `${file}:${rule.line} alert ${rule.alert} expression includes forbidden field: ${pattern}`,
      );
    }
    for (const [key, line] of rule.labels) {
      assert(
        allowedLabelNames.has(key),
        `${file}:${line} alert ${rule.alert} uses unsupported alert label ${key}`,
      );
    }
    for (const [key, line] of rule.groupLabels) {
      assert(
        allowedLabelNames.has(key),
        `${file}:${line} group label ${key} is not allowed`,
      );
    }
  }
}

function readRuleFile(file) {
  assert(statSync(file).isFile(), `${file} is missing`);
  return readFileSync(file, "utf8");
}

function assertRunbookURL(file, rule) {
  const runbook = rule.annotations.get("runbook_url");
  assert(
    runbook?.value,
    `${file}:${rule.line} alert ${rule.alert} has an empty runbook_url annotation`,
  );
  const [targetPath, anchor] = runbook.value.split("#", 2);
  assert(
    targetPath.endsWith(".md"),
    `${file}:${runbook.line} alert ${rule.alert} runbook_url must point to a Markdown file`,
  );
  assert(
    anchor,
    `${file}:${runbook.line} alert ${rule.alert} runbook_url must include a heading anchor`,
  );
  const normalizedPath = normalize(targetPath);
  assert(
    !normalizedPath.startsWith(`..${sep}`) && !normalizedPath.includes(`${sep}..${sep}`),
    `${file}:${runbook.line} alert ${rule.alert} runbook_url must stay inside the repository`,
  );
  assert(
    statSync(normalizedPath).isFile(),
    `${file}:${runbook.line} alert ${rule.alert} runbook target ${targetPath} is missing`,
  );
  const markdown = readFileSync(normalizedPath, "utf8");
  const anchors = markdownHeadingAnchors(markdown);
  assert(
    anchors.has(anchor),
    `${file}:${runbook.line} alert ${rule.alert} runbook anchor #${anchor} is missing in ${targetPath}`,
  );
  assert(
    dirname(normalizedPath) === "docs/requirements",
    `${file}:${runbook.line} alert ${rule.alert} runbook must live in docs/requirements`,
  );
}

function markdownHeadingAnchors(markdown) {
  const anchors = new Set();
  for (const line of markdown.split("\n")) {
    const match = line.match(/^#{1,6}\s+(.+?)\s*#*\s*$/);
    if (!match) continue;
    anchors.add(markdownAnchor(match[1]));
  }
  return anchors;
}

function markdownAnchor(heading) {
  return heading
    .trim()
    .toLowerCase()
    .replace(/`([^`]+)`/g, "$1")
    .replace(/[^\p{Letter}\p{Number}\s-]/gu, "")
    .trim()
    .replace(/\s+/g, "-");
}

function assertPrometheusConfig(content) {
  assert(
    content.includes("rule_files:"),
    `${prometheusConfig} must load SRapi alert rules`,
  );
  assert(
    content.includes("/etc/prometheus/rules/prometheus-srapi-alerts.yaml"),
    `${prometheusConfig} must reference prometheus-srapi-alerts.yaml`,
  );
  assert(
    content.includes("metrics_path: /metrics"),
    `${prometheusConfig} must scrape /metrics`,
  );
  assert(
    content.includes("api:8080"),
    `${prometheusConfig} must scrape the api compose service`,
  );
  assert(
    content.includes("alerting:"),
    `${prometheusConfig} must route alerts to Alertmanager`,
  );
  assert(
    content.includes("alertmanager:9093"),
    `${prometheusConfig} must target the alertmanager compose service`,
  );
}

function assertAlertmanagerConfig(content) {
  assert(
    content.includes("route:"),
    `${alertmanagerConfig} must define an alert route`,
  );
  assert(
    content.includes("receiver: srapi-local-webhook"),
    `${alertmanagerConfig} must route to the SRapi local webhook receiver`,
  );
  assert(
    content.includes("group_by:"),
    `${alertmanagerConfig} must group alerts by low-cardinality labels`,
  );
  assertAlertmanagerGroupBy(content);
  assert(
    content.includes("webhook_configs:"),
    `${alertmanagerConfig} must define a webhook receiver`,
  );
  assert(
    content.includes("send_resolved: true"),
    `${alertmanagerConfig} must forward resolved notifications`,
  );
  for (const pattern of forbiddenPatterns) {
    assert(
      !pattern.test(content),
      `${alertmanagerConfig} includes forbidden high-cardinality or sensitive field: ${pattern}`,
    );
  }
}

function assertAlertmanagerGroupBy(content) {
  const match = content.match(
    /^  group_by:\n((?:    - [A-Za-z_][A-Za-z0-9_]*\n?)+)/m,
  );
  assert(
    match,
    `${alertmanagerConfig} must define route.group_by as a YAML list`,
  );
  const labels = match[1]
    .trim()
    .split("\n")
    .map((line) =>
      line
        .replace(/^- /, "")
        .replace(/^    - /, "")
        .trim(),
    )
    .filter(Boolean);
  for (const label of allowedAlertmanagerGroupLabels) {
    assert(
      labels.includes(label),
      `${alertmanagerConfig} must group by ${label}`,
    );
  }
  for (const label of labels) {
    assert(
      allowedAlertmanagerGroupLabels.has(label),
      `${alertmanagerConfig} route.group_by uses unsupported label ${label}`,
    );
  }
}

function assertComposeConfig(content) {
  assert(
    content.includes("prom/prometheus:v3.7.3"),
    `${composeConfig} must pin the Prometheus image`,
  );
  assert(
    content.includes("prom/alertmanager:v0.29.0"),
    `${composeConfig} must pin the Alertmanager image`,
  );
  const observabilityProfileCount =
    content.match(/profiles: \["observability"\]/g)?.length ?? 0;
  assert(
    observabilityProfileCount >= 2,
    `${composeConfig} Prometheus and Alertmanager services must stay opt-in`,
  );
  assert(
    content.includes("./prometheus.yml:/etc/prometheus/prometheus.yml:ro"),
    `${composeConfig} must mount prometheus.yml read-only`,
  );
  assert(
    content.includes(
      "./prometheus-srapi-alerts.yaml:/etc/prometheus/rules/prometheus-srapi-alerts.yaml:ro",
    ),
    `${composeConfig} must mount SRapi alert rules read-only`,
  );
  assert(
    content.includes(
      "./alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro",
    ),
    `${composeConfig} must mount alertmanager.yml read-only`,
  );
  assert(
    content.includes("${ALERTMANAGER_PORT:-9093}:9093"),
    `${composeConfig} must expose Alertmanager on a configurable port`,
  );
}

function extractAlertRules(file, content) {
  const lines = content.split("\n");
  const rules = [];
  let groupLabels = new Map();
  let current = null;
  let section = "";

  for (const [offset, line] of lines.entries()) {
    const lineNumber = offset + 1;
    if (/^    labels:\s*$/.test(line)) {
      section = "group_labels";
      continue;
    }
    if (/^    rules:\s*$/.test(line)) {
      section = "rules";
      continue;
    }
    if (/^        labels:\s*$/.test(line)) {
      section = "rule_labels";
      continue;
    }
    if (/^        annotations:\s*$/.test(line)) {
      section = "annotations";
      continue;
    }

    const alertMatch = line.match(/^      - alert:\s*(\S.*)$/);
    if (alertMatch) {
      current = {
        alert: alertMatch[1].trim(),
        line: lineNumber,
        expr: "",
        labels: new Map(),
        annotations: new Map(),
        groupLabels,
      };
      rules.push(current);
      section = "rule";
      continue;
    }

    const groupLabelMatch = line.match(
      /^      ([A-Za-z_][A-Za-z0-9_]*):\s*(\S.*)$/,
    );
    if (section === "group_labels" && groupLabelMatch) {
      groupLabels.set(groupLabelMatch[1], lineNumber);
      continue;
    }
    if (!current) {
      continue;
    }
    const exprMatch = line.match(/^        expr:\s*(\S.*)$/);
    if (exprMatch) {
      current.expr = exprMatch[1].trim();
      section = "rule";
      continue;
    }
    const ruleLabelMatch = line.match(
      /^          ([A-Za-z_][A-Za-z0-9_]*):\s*(\S.*)$/,
    );
    if (section === "rule_labels" && ruleLabelMatch) {
      current.labels.set(ruleLabelMatch[1], lineNumber);
      continue;
    }
    if (section === "annotations" && ruleLabelMatch) {
      current.annotations.set(ruleLabelMatch[1], {
        line: lineNumber,
        value: ruleLabelMatch[2].trim(),
      });
    }
  }

  assert(groupLabels.size > 0, `${file} does not define group labels`);
  return rules;
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

export { extractAlertRules, markdownAnchor, markdownHeadingAnchors };

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
