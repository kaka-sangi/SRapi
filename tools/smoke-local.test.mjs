import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  CORE_GATEWAY_SMOKE_ENDPOINTS,
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

test("local smoke disables its temporary gateway api key", () => {
  const script = readFileSync("tools/smoke-local.mjs", "utf8");

  assert.match(script, /finally\s*{\s*await disableSmokeApiKey/s);
  assert.match(script, /PATCH", `\/api\/v1\/api-keys\/\$\{apiKeyID\}`/);
  assert.match(script, /status: "disabled"/);
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
