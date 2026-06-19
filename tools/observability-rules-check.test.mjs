import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { extractAlertRules, markdownHeadingAnchors } from "./observability-rules-check.mjs";

test("observability rules check passes for repository rules", () => {
  const result = spawnSync("node", ["tools/observability-rules-check.mjs"], {
    encoding: "utf8",
  });

  assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  assert.match(result.stdout, /observability rules check ok/);
});

test("observability rules check is exposed through Makefile and quality gates", () => {
  const makefile = readFileSync("Makefile", "utf8");
  const qualityGates = readFileSync("docs/requirements/QUALITY_GATES.md", "utf8");
  const devPs1 = readFileSync("tools/dev.ps1", "utf8");

  assert.match(makefile, /observability-rules-check:/);
  assert.match(makefile, /OBSERVABILITY_RULES_CHECK/);
  assert.match(qualityGates, /make observability-rules-check/);
  assert.match(devPs1, /"observability-rules-check"/);
  assert.match(devPs1, /Invoke-Step "make" @\("observability-rules-check"\)/);
  assert.match(
    devPs1,
    /"check" \{[\s\S]*Invoke-Step "make" @\("observability-rules-check"\)[\s\S]*Invoke-Step "go" @\("test", "\.\/\.\.\."\)/,
  );
});

test("PowerShell dev entrypoint exposes the observability compose profile", () => {
  const devPs1 = readFileSync("tools/dev.ps1", "utf8");

  assert.match(devPs1, /"observability-up"/);
  assert.match(devPs1, /\$env:COMPOSE_PROFILES = "observability"/);
  assert.match(devPs1, /\$env:COMPOSE_PROFILES = \$PreviousProfiles/);
  assert.match(devPs1, /"smoke-tempo-trace"/);
  assert.match(devPs1, /Invoke-Step "make" @\("smoke-tempo-trace"\)/);
});

test("prometheus compose profile mounts the config and alert rules", () => {
  const compose = readFileSync("deploy/docker-compose.yml", "utf8");
  const prometheus = readFileSync("deploy/prometheus.yml", "utf8");
  const alertmanager = readFileSync("deploy/alertmanager.yml", "utf8");

  assert.match(compose, /prom\/prometheus:v3\.7\.3/);
  assert.match(compose, /prom\/alertmanager:v0\.29\.0/);
  assert.match(compose, /profiles: \["observability"\]/);
  assert.match(
    compose,
    /\.\/prometheus\.yml:\/etc\/prometheus\/prometheus\.yml:ro/,
  );
  assert.match(
    compose,
    /\.\/prometheus-srapi-alerts\.yaml:\/etc\/prometheus\/rules\/prometheus-srapi-alerts\.yaml:ro/,
  );
  assert.match(
    compose,
    /\.\/alertmanager\.yml:\/etc\/alertmanager\/alertmanager\.yml:ro/,
  );
  assert.match(compose, /\$\{ALERTMANAGER_PORT:-9093\}:9093/);
  assert.match(compose, /host\.docker\.internal:host-gateway/);
  assert.match(prometheus, /rule_files:/);
  assert.match(prometheus, /alerting:/);
  assert.match(prometheus, /alertmanager:9093/);
  assert.match(
    prometheus,
    /\/etc\/prometheus\/rules\/prometheus-srapi-alerts\.yaml/,
  );
  assert.match(prometheus, /metrics_path: \/metrics/);
  assert.match(prometheus, /api:8080/);
  assert.match(alertmanager, /receiver: srapi-local-webhook/);
  assert.match(alertmanager, /webhook_configs:/);
  assert.match(alertmanager, /send_resolved: true/);
  assert.match(alertmanager, /service/);
  assert.match(alertmanager, /severity/);
  assert.match(alertmanager, /component/);
});

test("alertmanager route stays low-cardinality and secret-free", () => {
  const alertmanager = readFileSync("deploy/alertmanager.yml", "utf8");

  assert.match(
    alertmanager,
    /group_by:\n    - service\n    - severity\n    - component/,
  );
  assert.doesNotMatch(
    alertmanager,
    /fingerprint|rule_id|api_key|account_id|user_id|request_id|prompt|credential|authorization|cookie/i,
  );
});

test("prometheus alert rules cover ops posture, provider, scheduler, and error evidence signals", () => {
  const rules = readFileSync("deploy/prometheus-srapi-alerts.yaml", "utf8");

  assert.match(
    rules,
    /srapi_ops_alert_events\{severity="critical",status="firing"\}/,
  );
  assert.match(
    rules,
    /srapi_ops_alert_events\{severity="warning",status="firing"\}/,
  );
  assert.match(rules, /srapi_scheduler_no_available_total/);
  assert.match(rules, /SRapiSchedulerNoAvailableAccounts/);
  assert.match(rules, /srapi_provider_errors_total/);
  assert.match(rules, /SRapiProviderErrorsSpiking/);
  assert.match(rules, /srapi_ops_error_log_queue_capacity/);
  assert.match(rules, /srapi_ops_error_log_dropped_total/);
  assert.match(rules, /srapi_ops_error_log_write_failures_total/);
  assert.match(rules, /SRapiOpsErrorLogRecorderUnavailable/);
  assert.match(rules, /SRapiOpsErrorLogRecorderDroppingEvidence/);
  assert.match(rules, /SRapiOpsErrorLogRecorderBacklogged/);
  assert.doesNotMatch(
    rules,
    /fingerprint|rule_id|api_key|account_id|user_id|request_id/i,
  );
});

test("prometheus alert rules parse into low-cardinality labels and runbooks", () => {
  const parsed = extractAlertRules(
    "deploy/prometheus-srapi-alerts.yaml",
    readFileSync("deploy/prometheus-srapi-alerts.yaml", "utf8"),
  );

  assert.deepEqual(
    parsed.map((rule) => rule.alert),
    [
      "SRapiCriticalOpsAlertsFiring",
      "SRapiWarningOpsAlertsPersisting",
      "SRapiSchedulerNoAvailableAccounts",
      "SRapiProviderErrorsSpiking",
      "SRapiOpsErrorLogRecorderUnavailable",
      "SRapiOpsErrorLogRecorderDroppingEvidence",
      "SRapiOpsErrorLogRecorderBacklogged",
    ],
  );
  for (const rule of parsed) {
    assert.equal(rule.groupLabels.has("service"), true);
    assert.equal(rule.groupLabels.has("team"), true);
    assert.equal(rule.labels.has("severity"), true);
    assert.equal(rule.labels.has("component"), true);
    assert.equal(rule.annotations.has("runbook_url"), true);
    assert.match(rule.annotations.get("runbook_url").value, /^docs\/requirements\/OPERATIONS\.md#/);
  }
});

test("prometheus alert runbooks point at existing operations headings", () => {
  const rules = extractAlertRules(
    "deploy/prometheus-srapi-alerts.yaml",
    readFileSync("deploy/prometheus-srapi-alerts.yaml", "utf8"),
  );
  const anchors = markdownHeadingAnchors(readFileSync("docs/requirements/OPERATIONS.md", "utf8"));

  for (const rule of rules) {
    const runbook = rule.annotations.get("runbook_url").value;
    const [targetPath, anchor] = runbook.split("#", 2);
    assert.equal(targetPath, "docs/requirements/OPERATIONS.md");
    assert.equal(anchors.has(anchor), true, `missing operations runbook anchor ${anchor}`);
  }
});

test("observability rules check rejects forbidden high-cardinality fields", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-observability-rules-"));
  const rulesPath = join(tempDir, "bad-rules.yaml");
  writeFileSync(
    rulesPath,
    readFileSync("deploy/prometheus-srapi-alerts.yaml", "utf8").replace(
      "component: ops",
      "account_id: acc_123",
    ),
  );
  const checker = checkerWithConstant(
    "ruleFiles",
    `[${JSON.stringify(rulesPath)}]`,
  );
  const checkerPath = join(tempDir, "check.mjs");
  writeFileSync(checkerPath, checker);

  const result = spawnSync("node", [checkerPath], { encoding: "utf8" });

  assert.notEqual(result.status, 0);
  assert.match(
    result.stderr,
    /forbidden high-cardinality or sensitive field|unsupported alert label/,
  );
});

test("observability rules check rejects missing runbook anchors", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-observability-runbook-"));
  const rulesPath = join(tempDir, "bad-rules.yaml");
  writeFileSync(
    rulesPath,
    readFileSync("deploy/prometheus-srapi-alerts.yaml", "utf8").replace(
      "docs/requirements/OPERATIONS.md#srapicriticalopsalertsfiring",
      "docs/requirements/OPERATIONS.md#missing-alert-runbook",
    ),
  );
  const checker = checkerWithConstant(
    "ruleFiles",
    `[${JSON.stringify(rulesPath)}]`,
  );
  const checkerPath = join(tempDir, "check.mjs");
  writeFileSync(checkerPath, checker);

  const result = spawnSync("node", [checkerPath], { encoding: "utf8" });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /runbook anchor #missing-alert-runbook is missing/);
});

test("observability rules check rejects unsupported alertmanager grouping", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "srapi-alertmanager-"));
  const alertmanagerPath = join(tempDir, "bad-alertmanager.yml");
  writeFileSync(
    alertmanagerPath,
    readFileSync("deploy/alertmanager.yml", "utf8").replace(
      "    - component",
      "    - account",
    ),
  );
  const checker = checkerWithConstant(
    "alertmanagerConfig",
    JSON.stringify(alertmanagerPath),
  );
  const checkerPath = join(tempDir, "check.mjs");
  writeFileSync(checkerPath, checker);

  const result = spawnSync("node", [checkerPath], { encoding: "utf8" });

  assert.notEqual(result.status, 0);
  assert.match(
    result.stderr,
    /route\.group_by uses unsupported label account|must group by component/,
  );
});

function checkerWithConstant(name, value) {
  const source = readFileSync("tools/observability-rules-check.mjs", "utf8");
  const pattern = new RegExp(`const ${name} = [^;]+;`);
  const checker = source.replace(pattern, `const ${name} = ${value};`);
  assert.notEqual(checker, source, `failed to replace ${name} in checker`);
  return checker;
}
