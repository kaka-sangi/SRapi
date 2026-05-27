import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  CORE_GATEWAY_SMOKE_ENDPOINTS,
  CORE_GATEWAY_SMOKE_TARGETS,
  FAILOVER_GATEWAY_SMOKE_TARGETS,
  FAILOVER_SMOKE_ENDPOINT,
  RATE_LIMIT_SMOKE_ENDPOINT,
  RELEASE_GATEWAY_SMOKE_ENDPOINTS,
  RELEASE_GATEWAY_SMOKE_TARGETS,
} from "./smoke-local.mjs";

const releaseEndpointPaths = new Set(RELEASE_GATEWAY_SMOKE_ENDPOINTS.map((endpoint) => endpoint.path));

test("core gateway smoke keeps the MVP route family explicit", () => {
  assert.deepEqual(
    CORE_GATEWAY_SMOKE_ENDPOINTS.map((endpoint) => `${endpoint.method} ${endpoint.path}`),
    [
      "GET /v1/models",
      "POST /v1/chat/completions",
      "POST /v1/responses",
      "POST /v1/messages",
    ],
  );
});

test("core gateway smoke includes DB-backed raw Responses streaming", () => {
  assert.deepEqual(
    CORE_GATEWAY_SMOKE_TARGETS.map((target) => target.name),
    ["local-smoke-responses-stream"],
  );
  const target = CORE_GATEWAY_SMOKE_TARGETS[0];
  assert.equal(target.adapterType, "native-openai");
  assert.equal(target.protocol, "openai-compatible");
  assert.equal(target.providerCapabilities.streaming, true);
  assert.equal(target.providerCapabilities.responses, true);
  assert.equal(target.providerConfigSchema.native_responses, true);
  assert.equal(target.accountMetadata.responses_require_terminal_event, true);
  assert.equal(target.mappingCapabilityOverride.some((capability) => capability.key === "responses"), true);
});

test("release gateway smoke covers every non-websocket public gateway route in OpenAPI", () => {
  const contract = readFileSync("packages/openapi/openapi.yaml", "utf8");
  const publicGatewayPaths = [...contract.matchAll(/^  (\/(?:v1|v1beta)\/[^:]+):$/gm)]
    .map((match) => match[1])
    .filter((path) => path !== "/v1/responses/ws" && path !== "/v1/realtime")
    .map((path) => path.replace(/\{model\}/g, "{model}"));

  const missing = publicGatewayPaths.filter((path) => !releaseEndpointPaths.has(path));

  assert.deepEqual(missing, []);
});

test("release gateway smoke covers provider-native token counting routes", () => {
  assert.equal(releaseEndpointPaths.has("/v1/messages/count_tokens"), true);
  assert.equal(releaseEndpointPaths.has("/v1beta/models/{model}:countTokens"), true);
});

test("rate-limit smoke targets one OpenAI-compatible request path", () => {
  assert.deepEqual(RATE_LIMIT_SMOKE_ENDPOINT, {
    name: "rate_limit",
    method: "POST",
    path: "/v1/chat/completions",
  });
});

test("failover smoke uses two stable OpenAI-compatible targets", () => {
  assert.deepEqual(FAILOVER_SMOKE_ENDPOINT, {
    name: "failover",
    method: "POST",
    path: "/v1/chat/completions",
  });
  assert.deepEqual(
    FAILOVER_GATEWAY_SMOKE_TARGETS.map((target) => target.name),
    ["local-smoke-failover-primary", "local-smoke-failover-secondary"],
  );
  assert.deepEqual(
    FAILOVER_GATEWAY_SMOKE_TARGETS.map((target) => target.priority),
    [10, 100],
  );
  for (const target of FAILOVER_GATEWAY_SMOKE_TARGETS) {
    assert.equal(target.adapterType, "openai-compatible");
    assert.equal(target.protocol, "openai-compatible");
    assert.equal(target.providerCapabilities.chat_completions, true);
  }
});

test("release gateway smoke uses stable reusable admin targets", () => {
  assert.deepEqual(
    RELEASE_GATEWAY_SMOKE_TARGETS.map((target) => target.name),
    ["local-smoke-rerank", "local-smoke-anthropic-count", "local-smoke-gemini-count"],
  );
  for (const target of RELEASE_GATEWAY_SMOKE_TARGETS) {
    assert.equal(target.name.includes("${"), false);
    assert.equal(target.name.includes("Date"), false);
    assert.equal(target.name.includes("random"), false);
    assert.match(`${target.name}-model`, /^local-smoke-[a-z-]+-model$/);
  }
});

test("core smoke creates and cleans fixed raw stream gateway target", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /const CORE_GATEWAY_SMOKE_TARGET_NAMES = new Set/);
  assert.match(script, /if \(!rateLimitSmoke && !failoverSmoke\) {\s*await disableFixedSmokeGatewayTargets/s);
  assert.match(script, /smokeResponsesRawStreamEndpoint/);
  assert.match(script, /RESPONSES_RAW_STREAM_SMOKE_SSE/);
  assert.match(script, /response\.output_text\.delta/);
  assert.match(script, /sequence_number/);
  assert.match(script, /response_id/);
  assert.match(script, /item_id/);
  assert.match(script, /stream\.text !== RESPONSES_RAW_STREAM_SMOKE_SSE/);
});

test("local smoke disables its temporary gateway api key", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /finally\s*{\s*await disableSmokeApiKey/s);
  assert.match(script, /PATCH", `\/api\/v1\/api-keys\/\$\{apiKeyID\}`/);
  assert.match(script, /status: "disabled"/);
});

test("rate-limit smoke creates a one-rpm api key and expects 429 retry-after", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /process\.argv\.includes\("--rate-limit"\)/);
  assert.match(script, /rpmLimit: rateLimitSmoke \? 1 : undefined/);
  assert.match(script, /body\.rpm_limit = rpmLimit/);
  assert.match(script, /expectedStatus: 429/);
  assert.match(script, /headers\.get\("retry-after"\)/);
  assert.match(script, /rpm_limit_exceeded/);
});

test("failover smoke verifies attempt evidence and failover metrics", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /process\.argv\.includes\("--failover"\)/);
  assert.match(script, /withFailoverSmokeUpstreams/);
  assert.match(script, /primary unavailable/);
  assert.match(script, /expected one call to each failover upstream/);
  assert.match(script, /\/api\/v1\/admin\/usage-logs\?model=/);
  assert.match(script, /attempt_no === 1/);
  assert.match(script, /attempt_no === 2/);
  assert.match(script, /fallback_from_decision_id/);
  assert.match(script, /fallback_excluded/);
  assert.match(script, /srapi_gateway_failover_total/);
});

test("local smoke disables old active smoke api keys before creating another one", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");
  const cleanupIndex = script.indexOf("await disableActiveSmokeApiKeys({ cookie, csrfToken });");
  const createIndex = script.indexOf("const apiKey = await createSmokeApiKey");

  assert.notEqual(cleanupIndex, -1);
  assert.notEqual(createIndex, -1);
  assert.ok(cleanupIndex < createIndex);
  assert.match(script, /\/api\/v1\/api-keys\?page_size=200&status=active/);
  assert.match(script, /key\.name\.startsWith\("local-smoke-"\)/);
});

test("release smoke disables legacy random gateway targets but preserves fixed targets", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /const RELEASE_GATEWAY_SMOKE_TARGET_NAMES = new Set/);
  assert.match(script, /const RELEASE_GATEWAY_SMOKE_MODEL_NAMES = new Set/);
  assert.match(script, /const RELEASE_GATEWAY_SMOKE_ACCOUNT_NAMES = new Set/);
  assert.match(script, /if \(releaseSmoke\) {\s*await disableLegacySmokeGatewayTargets/s);
  assert.match(script, /name\.startsWith\("local-smoke-"\)/);
  assert.match(script, /!fixedNames\.has\(name\)/);
  assert.match(script, /body: { status: "disabled" }/);
});

test("release smoke disables fixed gateway targets after the smoke run", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /finally\s*{[\s\S]*if \(releaseSmoke\) {\s*await disableFixedSmokeGatewayTargets/s);
  assert.match(script, /async function disableFixedSmokeGatewayTargets/);
  assert.match(script, /fixedNames\.has\(name\)/);
  assert.match(script, /resource\.status === "active"/);
});

test("failover smoke disables fixed gateway targets before and after the run", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /const FAILOVER_GATEWAY_SMOKE_TARGET_NAMES = new Set/);
  assert.match(script, /const FAILOVER_GATEWAY_SMOKE_ACCOUNT_NAMES = new Set/);
  assert.match(script, /const FAILOVER_GATEWAY_SMOKE_MODEL_NAME = "local-smoke-failover-model"/);
  assert.match(script, /if \(failoverSmoke\) {\s*await disableFixedSmokeGatewayTargets/s);
  assert.match(script, /targetNames: FAILOVER_GATEWAY_SMOKE_TARGET_NAMES/);
  assert.match(script, /modelNames: new Set\(\[FAILOVER_GATEWAY_SMOKE_MODEL_NAME\]\)/);
  assert.match(script, /accountNames: FAILOVER_GATEWAY_SMOKE_ACCOUNT_NAMES/);
});

test("local smoke asserts no active smoke residue before exiting", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /finally\s*{[\s\S]*await assertNoActiveSmokeResidue\(\{ cookie \}\);/);
  assert.match(script, /async function assertNoActiveSmokeResidue/);
  assert.match(script, /smoke cleanup left active local-smoke resources/);
  assert.match(script, /\/api\/v1\/api-keys\?page_size=200&status=active/);
  assert.match(script, /name\.startsWith\("local-smoke-"\) && resource\.status === "active"/);
});

test("admin resource disable verifies the returned disabled status", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /expectedStatus: "disabled"/);
  assert.match(script, /data\.status !== options\.expectedStatus/);
});
