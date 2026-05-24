#!/usr/bin/env node

import http from "node:http";
import { once } from "node:events";
import { pathToFileURL } from "node:url";

const baseURL = process.env.SRAPI_BASE_URL || `http://127.0.0.1:${process.env.SERVER_PORT || "8080"}`;
const adminEmail = process.env.BOOTSTRAP_ADMIN_EMAIL || "admin@srapi.local";
const adminPassword = process.env.BOOTSTRAP_ADMIN_PASSWORD || "password123";

export const CORE_GATEWAY_SMOKE_ENDPOINTS = Object.freeze([
  { name: "models", method: "GET", path: "/v1/models" },
  { name: "chat_completions", method: "POST", path: "/v1/chat/completions" },
  { name: "responses", method: "POST", path: "/v1/responses" },
  { name: "messages", method: "POST", path: "/v1/messages" },
]);

export const RELEASE_GATEWAY_SMOKE_ENDPOINTS = Object.freeze([
  ...CORE_GATEWAY_SMOKE_ENDPOINTS,
  { name: "anthropic_count_tokens", method: "POST", path: "/v1/messages/count_tokens" },
  { name: "embeddings", method: "POST", path: "/v1/embeddings" },
  { name: "image_generations", method: "POST", path: "/v1/images/generations" },
  { name: "image_edits", method: "POST", path: "/v1/images/edits" },
  { name: "image_variations", method: "POST", path: "/v1/images/variations" },
  { name: "audio_transcriptions", method: "POST", path: "/v1/audio/transcriptions" },
  { name: "audio_speech", method: "POST", path: "/v1/audio/speech" },
  { name: "moderations", method: "POST", path: "/v1/moderations" },
  { name: "rerank", method: "POST", path: "/v1/rerank" },
  { name: "gemini_models", method: "GET", path: "/v1beta/models" },
  { name: "gemini_generate_content", method: "POST", path: "/v1beta/models/{model}:generateContent" },
  {
    name: "gemini_stream_generate_content",
    method: "POST",
    path: "/v1beta/models/{model}:streamGenerateContent",
  },
  { name: "gemini_count_tokens", method: "POST", path: "/v1beta/models/{model}:countTokens" },
]);

export const RELEASE_GATEWAY_SMOKE_TARGETS = Object.freeze([
  {
    name: "local-smoke-rerank",
    displayName: "Local Smoke Rerank",
    adapterType: "rerank-compatible",
    protocol: "rerank-compatible",
    providerCapabilities: { rerank: true },
    modelCapabilities: [{ key: "rerank", level: "required", status: "stable", version: "v1" }],
    upstreamModelName: "rerank-smoke-upstream",
    accountCredential: { api_key: "rerank-smoke-secret" },
    accountBasePath: "/v1",
  },
  {
    name: "local-smoke-anthropic-count",
    displayName: "Local Smoke Anthropic Count",
    adapterType: "anthropic-compatible",
    protocol: "anthropic-compatible",
    providerCapabilities: { token_counting: true },
    modelCapabilities: [{ key: "token_counting", level: "required", status: "stable", version: "v1" }],
    upstreamModelName: "anthropic-count-smoke-upstream",
    accountCredential: { api_key: "anthropic-count-smoke-secret" },
    accountBasePath: "/v1",
  },
  {
    name: "local-smoke-gemini-count",
    displayName: "Local Smoke Gemini Count",
    adapterType: "gemini-compatible",
    protocol: "gemini-compatible",
    providerCapabilities: { token_counting: true },
    modelCapabilities: [
      { key: "streaming", level: "required", status: "stable", version: "v1" },
      { key: "token_counting", level: "required", status: "stable", version: "v1" },
    ],
    upstreamModelName: "gemini-count-smoke-upstream",
    accountCredential: { api_key: "gemini-count-smoke-secret" },
    accountBasePath: "/v1beta",
  },
]);

const RELEASE_GATEWAY_SMOKE_TARGET_NAMES = new Set(RELEASE_GATEWAY_SMOKE_TARGETS.map((target) => target.name));
const RELEASE_GATEWAY_SMOKE_MODEL_NAMES = new Set(RELEASE_GATEWAY_SMOKE_TARGETS.map((target) => `${target.name}-model`));
const RELEASE_GATEWAY_SMOKE_ACCOUNT_NAMES = new Set(
  RELEASE_GATEWAY_SMOKE_TARGETS.map((target) => `${target.name}-account`),
);

async function main() {
  const releaseSmoke = process.argv.includes("--release");
  const health = await request("GET", "/api/v1/health");
  if (!health.body?.request_id || health.body?.data?.status === undefined) {
    throw new Error("health response missing request_id or status");
  }
  if (releaseSmoke) {
    await smokeReleasePlatformEndpoints();
  }

  const login = await request("POST", "/api/v1/auth/login", {
    body: {
      email: adminEmail,
      password: adminPassword,
    },
  });
  const csrfToken = login.body?.data?.csrf_token;
  const cookie = sessionCookie(login.headers);
  if (!csrfToken || !cookie) {
    throw new Error("login did not return csrf token and session cookie");
  }

  await disableActiveSmokeApiKeys({ cookie, csrfToken });
  if (releaseSmoke) {
    await disableLegacySmokeGatewayTargets({ cookie, csrfToken });
  }
  const apiKey = await createSmokeApiKey({ cookie, csrfToken });
  const plaintextKey = apiKey.plaintextKey;

  let models;
  try {
    models = await smokeCoreGatewayEndpoints(plaintextKey);
    if (releaseSmoke) {
      await smokeReleaseGatewayEndpoints({
        bearer: plaintextKey,
        cookie,
        csrfToken,
      });
    }
  } finally {
    await disableSmokeApiKey({ cookie, csrfToken, apiKeyID: apiKey.id });
    if (releaseSmoke) {
      await disableFixedSmokeGatewayTargets({ cookie, csrfToken });
    }
    await assertNoActiveSmokeResidue({ cookie });
  }

  console.log(`local smoke ok: ${baseURL}`);
  console.log(`health request_id: ${health.body.request_id}`);
  console.log(`models: ${models.body.data.map((model) => model.id).join(", ")}`);
  console.log(`gateway endpoints: ${CORE_GATEWAY_SMOKE_ENDPOINTS.map((endpoint) => endpoint.name).join(", ")}`);
  if (releaseSmoke) {
    console.log("release endpoints: livez, readyz, metrics");
    console.log(
      `release gateway endpoints: ${RELEASE_GATEWAY_SMOKE_ENDPOINTS.map((endpoint) => endpoint.name).join(", ")}`,
    );
  }
}

async function createSmokeApiKey({ cookie, csrfToken }) {
  const apiKey = await request("POST", "/api/v1/api-keys", {
    cookie,
    csrfToken,
    body: {
      name: `local-smoke-${Date.now()}`,
      scopes: ["gateway:invoke"],
    },
    expectedStatus: 201,
  });
  const plaintextKey = apiKey.body?.data?.plaintext_key;
  if (!plaintextKey) {
    throw new Error("api key creation did not return plaintext_key");
  }
  const id = apiKey.body?.data?.api_key?.id;
  if (!id) {
    throw new Error("api key creation did not return api_key.id");
  }
  return { id, plaintextKey };
}

async function disableSmokeApiKey({ cookie, csrfToken, apiKeyID }) {
  const response = await request("PATCH", `/api/v1/api-keys/${apiKeyID}`, {
    cookie,
    csrfToken,
    body: {
      status: "disabled",
    },
  });
  if (response.body?.data?.status !== "disabled") {
    throw new Error(`PATCH /api/v1/api-keys/${apiKeyID} did not disable the smoke api key`);
  }
}

async function disableActiveSmokeApiKeys({ cookie, csrfToken }) {
  const response = await request("GET", "/api/v1/api-keys?page_size=200&status=active", { cookie });
  const keys = response.body?.data;
  if (!Array.isArray(keys)) {
    throw new Error("GET /api/v1/api-keys did not return a data array");
  }
  for (const key of keys) {
    if (typeof key?.id === "string" && typeof key.name === "string" && key.name.startsWith("local-smoke-")) {
      await disableSmokeApiKey({ cookie, csrfToken, apiKeyID: key.id });
    }
  }
}

async function disableLegacySmokeGatewayTargets({ cookie, csrfToken }) {
  await disableLegacySmokeAdminResources({
    cookie,
    csrfToken,
    path: "/api/v1/admin/accounts",
    nameField: "name",
    fixedNames: RELEASE_GATEWAY_SMOKE_ACCOUNT_NAMES,
  });
  await disableLegacySmokeAdminResources({
    cookie,
    csrfToken,
    path: "/api/v1/admin/models",
    nameField: "canonical_name",
    fixedNames: RELEASE_GATEWAY_SMOKE_MODEL_NAMES,
  });
  await disableLegacySmokeAdminResources({
    cookie,
    csrfToken,
    path: "/api/v1/admin/providers",
    nameField: "name",
    fixedNames: RELEASE_GATEWAY_SMOKE_TARGET_NAMES,
  });
}

async function disableFixedSmokeGatewayTargets({ cookie, csrfToken }) {
  await disableFixedSmokeAdminResources({
    cookie,
    csrfToken,
    path: "/api/v1/admin/accounts",
    nameField: "name",
    fixedNames: RELEASE_GATEWAY_SMOKE_ACCOUNT_NAMES,
  });
  await disableFixedSmokeAdminResources({
    cookie,
    csrfToken,
    path: "/api/v1/admin/models",
    nameField: "canonical_name",
    fixedNames: RELEASE_GATEWAY_SMOKE_MODEL_NAMES,
  });
  await disableFixedSmokeAdminResources({
    cookie,
    csrfToken,
    path: "/api/v1/admin/providers",
    nameField: "name",
    fixedNames: RELEASE_GATEWAY_SMOKE_TARGET_NAMES,
  });
}

async function disableFixedSmokeAdminResources({ cookie, csrfToken, path, nameField, fixedNames }) {
  const response = await request("GET", path, { cookie });
  const resources = response.body?.data;
  if (!Array.isArray(resources)) {
    throw new Error(`GET ${path} did not return a data array`);
  }
  for (const resource of resources) {
    const name = resource?.[nameField];
    if (
      typeof resource?.id === "string" &&
      typeof name === "string" &&
      fixedNames.has(name) &&
      resource.status === "active"
    ) {
      await updateAdminResource("PATCH", `${path}/${resource.id}`, {
        cookie,
        csrfToken,
        body: { status: "disabled" },
        expectedStatus: "disabled",
      });
    }
  }
}

async function disableLegacySmokeAdminResources({ cookie, csrfToken, path, nameField, fixedNames }) {
  const response = await request("GET", path, { cookie });
  const resources = response.body?.data;
  if (!Array.isArray(resources)) {
    throw new Error(`GET ${path} did not return a data array`);
  }
  for (const resource of resources) {
    const name = resource?.[nameField];
    if (
      typeof resource?.id === "string" &&
      typeof name === "string" &&
      name.startsWith("local-smoke-") &&
      !fixedNames.has(name) &&
      resource.status === "active"
    ) {
      await updateAdminResource("PATCH", `${path}/${resource.id}`, {
        cookie,
        csrfToken,
        body: { status: "disabled" },
        expectedStatus: "disabled",
      });
    }
  }
}

async function assertNoActiveSmokeResidue({ cookie }) {
  const activeKeys = await listActiveSmokeAPIKeys({ cookie });
  const activeAccounts = await listActiveSmokeAdminResources({
    cookie,
    path: "/api/v1/admin/accounts",
    nameField: "name",
  });
  const activeModels = await listActiveSmokeAdminResources({
    cookie,
    path: "/api/v1/admin/models",
    nameField: "canonical_name",
  });
  const activeProviders = await listActiveSmokeAdminResources({
    cookie,
    path: "/api/v1/admin/providers",
    nameField: "name",
  });
  const residue = [...activeKeys, ...activeAccounts, ...activeModels, ...activeProviders];
  if (residue.length > 0) {
    throw new Error(`smoke cleanup left active local-smoke resources: ${residue.join(", ")}`);
  }
}

async function listActiveSmokeAPIKeys({ cookie }) {
  const response = await request("GET", "/api/v1/api-keys?page_size=200&status=active", { cookie });
  const keys = response.body?.data;
  if (!Array.isArray(keys)) {
    throw new Error("GET /api/v1/api-keys did not return a data array");
  }
  return keys
    .filter((key) => typeof key?.name === "string" && key.name.startsWith("local-smoke-"))
    .map((key) => `api_key:${key.name}`);
}

async function listActiveSmokeAdminResources({ cookie, path, nameField }) {
  const response = await request("GET", path, { cookie });
  const resources = response.body?.data;
  if (!Array.isArray(resources)) {
    throw new Error(`GET ${path} did not return a data array`);
  }
  return resources
    .filter((resource) => {
      const name = resource?.[nameField];
      return typeof name === "string" && name.startsWith("local-smoke-") && resource.status === "active";
    })
    .map((resource) => `${path}:${resource[nameField]}`);
}

async function smokeReleasePlatformEndpoints() {
  const livez = await request("GET", "/livez");
  if (livez.body?.data?.status !== "ok") {
    throw new Error("/livez did not report ok");
  }
  const readyz = await request("GET", "/readyz");
  if (readyz.body?.data?.status !== "ok") {
    throw new Error("/readyz did not report ok");
  }
  const metrics = await request("GET", "/metrics", { responseType: "text" });
  for (const metric of [
    "srapi_gateway_requests_total",
    "srapi_gateway_request_duration_seconds",
    "srapi_gateway_inflight_requests",
    "srapi_gateway_errors_total",
    "srapi_scheduler_decisions_total",
    "srapi_provider_errors_total",
    "srapi_usage_tokens_total",
    "srapi_reverse_proxy_ban_signals_total",
  ]) {
    if (!metrics.text.includes(metric)) {
      throw new Error(`/metrics missing ${metric}`);
    }
  }
}

async function smokeCoreGatewayEndpoints(bearer) {
  const models = await request("GET", "/v1/models", {
    bearer,
  });
  if (!Array.isArray(models.body?.data) || models.body.data.length === 0) {
    throw new Error("/v1/models returned no models");
  }

  const chat = await request("POST", "/v1/chat/completions", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      messages: [{ role: "user", content: "local smoke gateway call" }],
    },
  });
  const content = chat.body?.choices?.[0]?.message?.content;
  if (typeof content !== "string" || !content.includes("local smoke gateway call")) {
    throw new Error("mock gateway chat response did not include echoed prompt");
  }

  const responses = await request("POST", "/v1/responses", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      input: "local smoke responses call",
    },
  });
  const responseContent = responses.body?.output?.[0]?.content?.[0]?.text;
  if (typeof responseContent !== "string" || !responseContent.includes("local smoke responses call")) {
    throw new Error("mock gateway Responses response did not include echoed prompt");
  }

  const messages = await request("POST", "/v1/messages", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      max_tokens: 128,
      messages: [{ role: "user", content: "local smoke messages call" }],
    },
  });
  const messageContent = messages.body?.content?.[0]?.text;
  if (typeof messageContent !== "string" || !messageContent.includes("local smoke messages call")) {
    throw new Error("mock gateway Messages response did not include echoed prompt");
  }

  return models;
}

async function smokeReleaseGatewayEndpoints({ bearer, cookie, csrfToken }) {
  await smokeOpenAICompatiblePassThroughEndpoints(bearer);
  await smokeGeminiTextEndpoints(bearer);
  await withSmokeUpstream(async (upstreamURL) => {
    const rerankModel = await ensureGatewayTarget({
      cookie,
      csrfToken,
      target: withUpstreamMetadata(RELEASE_GATEWAY_SMOKE_TARGETS[0], upstreamURL),
    });
    await smokeRerankEndpoint(bearer, rerankModel);

    const anthropicCountModel = await ensureGatewayTarget({
      cookie,
      csrfToken,
      target: withUpstreamMetadata(RELEASE_GATEWAY_SMOKE_TARGETS[1], upstreamURL),
    });
    await smokeAnthropicCountTokensEndpoint(bearer, anthropicCountModel);

    const geminiCountModel = await ensureGatewayTarget({
      cookie,
      csrfToken,
      target: withUpstreamMetadata(RELEASE_GATEWAY_SMOKE_TARGETS[2], upstreamURL),
    });
    await smokeGeminiCountTokensEndpoint(bearer, geminiCountModel);
  });
}

async function smokeOpenAICompatiblePassThroughEndpoints(bearer) {
  const embeddings = await request("POST", "/v1/embeddings", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      input: "local smoke embeddings call",
    },
  });
  if (!Array.isArray(embeddings.body?.data) || embeddings.body.data.length !== 1) {
    throw new Error("/v1/embeddings returned no embedding data");
  }
  const vector = embeddings.body.data[0].embedding;
  if (!Array.isArray(vector) || vector.length === 0 || typeof vector[0] !== "number") {
    throw new Error("/v1/embeddings returned an invalid embedding vector");
  }

  const imageGeneration = await request("POST", "/v1/images/generations", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      prompt: "local smoke image generation call",
      n: 1,
      size: "256x256",
    },
  });
  assertImageResponse("/v1/images/generations", imageGeneration.body);

  const imageEdit = await request("POST", "/v1/images/edits", {
    bearer,
    formData: imageFormData({
      model: "gpt-4o-mini",
      prompt: "local smoke image edit call",
      imageField: "image",
    }),
  });
  assertImageResponse("/v1/images/edits", imageEdit.body);

  const imageVariation = await request("POST", "/v1/images/variations", {
    bearer,
    formData: imageFormData({
      model: "gpt-4o-mini",
      imageField: "image",
    }),
  });
  assertImageResponse("/v1/images/variations", imageVariation.body);

  const audioTranscription = await request("POST", "/v1/audio/transcriptions", {
    bearer,
    formData: audioFormData({
      model: "gpt-4o-mini",
      responseFormat: "json",
    }),
  });
  if (typeof audioTranscription.body?.text !== "string" || !audioTranscription.body.text.includes("smoke.wav")) {
    throw new Error("/v1/audio/transcriptions returned an invalid transcription");
  }

  const audioSpeech = await request("POST", "/v1/audio/speech", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      input: "local smoke speech call",
      voice: "alloy",
      response_format: "wav",
    },
    responseType: "arrayBuffer",
  });
  if (audioSpeech.bytes.length === 0 || !audioSpeech.headers.get("content-type")?.startsWith("audio/")) {
    throw new Error("/v1/audio/speech returned invalid audio");
  }

  const moderations = await request("POST", "/v1/moderations", {
    bearer,
    body: {
      model: "gpt-4o-mini",
      input: "local smoke moderation call",
    },
  });
  if (!Array.isArray(moderations.body?.results) || typeof moderations.body.results[0]?.flagged !== "boolean") {
    throw new Error("/v1/moderations returned invalid results");
  }
}

async function smokeGeminiTextEndpoints(bearer) {
  const models = await request("GET", "/v1beta/models", { bearer });
  if (!Array.isArray(models.body?.models) || models.body.models.length === 0) {
    throw new Error("/v1beta/models returned no models");
  }

  const generate = await request("POST", "/v1beta/models/gpt-4o-mini:generateContent", {
    bearer,
    body: geminiContentBody("local smoke gemini call"),
  });
  const text = generate.body?.candidates?.[0]?.content?.parts?.[0]?.text;
  if (typeof text !== "string" || !text.includes("local smoke gemini call")) {
    throw new Error("/v1beta/models/{model}:generateContent did not include echoed prompt");
  }

  const stream = await request("POST", "/v1beta/models/gpt-4o-mini:streamGenerateContent", {
    bearer,
    body: geminiContentBody("local smoke gemini stream call"),
    responseType: "text",
  });
  if (!stream.text.includes("data:") || !stream.text.includes("local smoke gemini stream call")) {
    throw new Error("/v1beta/models/{model}:streamGenerateContent returned invalid SSE");
  }
}

async function smokeRerankEndpoint(bearer, model) {
  const rerank = await request("POST", "/v1/rerank", {
    bearer,
    body: {
      model,
      query: "what is srapi",
      documents: [
        "Payment processors settle card orders.",
        { text: "SRapi is a self-hosted AI API gateway.", source: "docs" },
      ],
      top_n: 1,
      return_documents: true,
    },
  });
  const first = rerank.body?.results?.[0];
  if (rerank.body?.model !== model || first?.index !== 1 || first?.document?.source !== "docs") {
    throw new Error("/v1/rerank returned invalid rerank results");
  }
}

async function smokeAnthropicCountTokensEndpoint(bearer, model) {
  const count = await request("POST", "/v1/messages/count_tokens", {
    bearer,
    body: {
      model,
      system: "count only",
      messages: [{ role: "user", content: "local smoke anthropic count call" }],
    },
  });
  if (count.body?.input_tokens !== 23) {
    throw new Error("/v1/messages/count_tokens returned invalid token count");
  }
}

async function smokeGeminiCountTokensEndpoint(bearer, model) {
  const count = await request("POST", `/v1beta/models/${model}:countTokens`, {
    bearer,
    body: geminiContentBody("local smoke gemini count call"),
  });
  if (count.body?.totalTokens !== 31) {
    throw new Error("/v1beta/models/{model}:countTokens returned invalid token count");
  }
}

function withUpstreamMetadata(target, upstreamURL) {
  return {
    ...target,
    accountMetadata: { base_url: `${upstreamURL}${target.accountBasePath}` },
  };
}

async function ensureGatewayTarget({ cookie, csrfToken, target }) {
  const provider = await ensureProvider({ cookie, csrfToken, target });
  const modelName = `${target.name}-model`;
  const model = await ensureModel({ cookie, csrfToken, target, modelName });
  await ensureModelMapping({ cookie, csrfToken, model, provider, target });
  await ensureProviderAccount({ cookie, csrfToken, provider, target });
  return modelName;
}

async function ensureProvider({ cookie, csrfToken, target }) {
  const body = {
    display_name: target.displayName,
    adapter_type: target.adapterType,
    protocol: target.protocol,
    status: "active",
    capabilities: target.providerCapabilities,
  };
  const existing = await findAdminResource("/api/v1/admin/providers", "name", target.name, { cookie });
  if (existing) {
    return updateAdminResource("PATCH", `/api/v1/admin/providers/${existing.id}`, {
      cookie,
      csrfToken,
      body,
    });
  }
  return createAdminResource("POST", "/api/v1/admin/providers", {
    cookie,
    csrfToken,
    body: {
      name: target.name,
      ...body,
    },
  });
}

async function ensureModel({ cookie, csrfToken, target, modelName }) {
  const body = {
    display_name: `${target.displayName} Model`,
    status: "active",
    capabilities: target.modelCapabilities,
  };
  const existing = await findAdminResource("/api/v1/admin/models", "canonical_name", modelName, { cookie });
  if (existing) {
    return updateAdminResource("PATCH", `/api/v1/admin/models/${existing.id}`, {
      cookie,
      csrfToken,
      body,
    });
  }
  return createAdminResource("POST", "/api/v1/admin/models", {
    cookie,
    csrfToken,
    body: {
      canonical_name: modelName,
      ...body,
    },
  });
}

async function ensureModelMapping({ cookie, csrfToken, model, provider, target }) {
  const response = await request("POST", `/api/v1/admin/models/${model.id}/mappings`, {
    cookie,
    csrfToken,
    body: {
      provider_id: String(provider.id),
      upstream_model_name: target.upstreamModelName,
      status: "active",
    },
    expectedStatus: [201, 409],
  });
  if (response.status === 201 && !response.body?.data?.id) {
    throw new Error(`POST /api/v1/admin/models/${model.id}/mappings did not return data.id`);
  }
}

async function ensureProviderAccount({ cookie, csrfToken, provider, target }) {
  const body = {
    name: `${target.name}-account`,
    runtime_class: "api_key",
    credential: target.accountCredential,
    metadata: target.accountMetadata,
    status: "active",
    priority: 100,
    weight: 1,
  };
  const existing = await findAdminResource(
    `/api/v1/admin/accounts?provider_id=${encodeURIComponent(String(provider.id))}`,
    "name",
    body.name,
    { cookie },
  );
  if (existing) {
    return updateAdminResource("PATCH", `/api/v1/admin/accounts/${existing.id}`, {
      cookie,
      csrfToken,
      body,
    });
  }
  return createAdminResource("POST", "/api/v1/admin/accounts", {
    cookie,
    csrfToken,
    body: {
      provider_id: String(provider.id),
      ...body,
    },
  });
}

async function createAdminResource(method, path, options) {
  const response = await request(method, path, {
    ...options,
    expectedStatus: 201,
  });
  const data = response.body?.data;
  if (!data?.id) {
    throw new Error(`${method} ${path} did not return data.id`);
  }
  return data;
}

async function updateAdminResource(method, path, options) {
  const response = await request(method, path, {
    ...options,
    expectedStatus: 200,
  });
  const data = response.body?.data;
  if (!data?.id) {
    throw new Error(`${method} ${path} did not return data.id`);
  }
  if (options.expectedStatus && data.status !== options.expectedStatus) {
    throw new Error(`${method} ${path} expected status ${options.expectedStatus}, got ${data.status}`);
  }
  return data;
}

async function findAdminResource(path, field, value, { cookie }) {
  const response = await request("GET", path, { cookie });
  const resources = response.body?.data;
  if (!Array.isArray(resources)) {
    throw new Error(`GET ${path} did not return a data array`);
  }
  return resources.find((resource) => resource?.[field] === value) || null;
}

function assertImageResponse(endpoint, body) {
  const first = body?.data?.[0];
  if (!first || (typeof first.url !== "string" && typeof first.b64_json !== "string")) {
    throw new Error(`${endpoint} returned no image payload`);
  }
}

function imageFormData({ model, prompt, imageField }) {
  const form = new FormData();
  form.append("model", model);
  if (prompt) {
    form.append("prompt", prompt);
  }
  form.append("size", "256x256");
  form.append(imageField, pngBlob(), "smoke.png");
  return form;
}

function audioFormData({ model, responseFormat }) {
  const form = new FormData();
  form.append("model", model);
  form.append("response_format", responseFormat);
  form.append("file", new Blob([Buffer.from("RIFF-SRAPI-SMOKE-AUDIO")], { type: "audio/wav" }), "smoke.wav");
  return form;
}

function pngBlob() {
  return new Blob([Buffer.from("PNG-SRAPI-SMOKE-IMAGE")], { type: "image/png" });
}

function geminiContentBody(text) {
  return {
    contents: [
      {
        role: "user",
        parts: [{ text }],
      },
    ],
  };
}

async function withSmokeUpstream(fn) {
  const server = http.createServer(async (req, res) => {
    try {
      await handleSmokeUpstreamRequest(req, res);
    } catch (error) {
      res.writeHead(500, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: { message: error.message } }));
    }
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  const upstreamURL = `http://127.0.0.1:${address.port}`;
  try {
    return await fn(upstreamURL);
  } finally {
    await new Promise((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
  }
}

async function handleSmokeUpstreamRequest(req, res) {
  const url = new URL(req.url, "http://127.0.0.1");
  if (req.method === "POST" && url.pathname === "/v1/rerank") {
    const payload = await readJSON(req);
    const docs = Array.isArray(payload.documents) ? payload.documents : [];
    writeJSON(res, {
      id: "rerank_local_smoke",
      model: payload.model || "rerank-smoke-upstream",
      results: [
        {
          index: Math.min(1, Math.max(0, docs.length - 1)),
          relevance_score: 0.93,
          document: { text: "SRapi is a self-hosted AI API gateway.", source: "docs" },
        },
      ],
      usage: { prompt_tokens: 11, total_tokens: 11 },
    });
    return;
  }
  if (req.method === "POST" && url.pathname === "/v1/messages/count_tokens") {
    writeJSON(res, {
      input_tokens: 23,
      cache_creation_input_tokens: 2,
    });
    return;
  }
  if (req.method === "POST" && url.pathname.startsWith("/v1beta/models/") && url.pathname.endsWith(":countTokens")) {
    writeJSON(res, {
      totalTokens: 31,
      cachedContentTokenCount: 3,
      promptTokensDetails: [{ modality: "TEXT", tokenCount: 28 }],
    });
    return;
  }
  writeJSON(res, { error: { message: `unexpected smoke upstream route ${req.method} ${url.pathname}` } }, 404);
}

async function readJSON(req) {
  const chunks = [];
  for await (const chunk of req) {
    chunks.push(chunk);
  }
  const raw = Buffer.concat(chunks).toString("utf8");
  if (raw.trim() === "") {
    return {};
  }
  return JSON.parse(raw);
}

function writeJSON(res, body, status = 200) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

async function request(method, path, options = {}) {
  const headers = {
    Accept: options.responseType === "arrayBuffer" ? "*/*" : "application/json",
    ...(options.headers || {}),
  };
  let body;
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(options.body);
  } else if (options.formData !== undefined) {
    body = options.formData;
  }
  if (options.cookie) {
    headers.Cookie = options.cookie;
  }
  if (options.csrfToken) {
    headers["X-CSRF-Token"] = options.csrfToken;
  }
  if (options.bearer) {
    headers.Authorization = `Bearer ${options.bearer}`;
  }

  const response = await fetch(new URL(path, baseURL), {
    method,
    headers,
    body,
  });
  const expectedStatus = options.expectedStatus || 200;
  const expectedStatuses = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];

  if (options.responseType === "arrayBuffer") {
    const bytes = Buffer.from(await response.arrayBuffer());
    if (!expectedStatuses.includes(response.status)) {
      throw new Error(`${method} ${path} expected ${expectedStatuses.join(" or ")}, got ${response.status}: ${bytes.toString("utf8").slice(0, 200)}`);
    }
    return { bytes, headers: response.headers, status: response.status };
  }

  const text = await response.text();
  if (!expectedStatuses.includes(response.status)) {
    throw new Error(`${method} ${path} expected ${expectedStatuses.join(" or ")}, got ${response.status}: ${text}`);
  }
  if (options.responseType === "text") {
    return { text, headers: response.headers, status: response.status };
  }

  let parsedBody;
  if (text) {
    try {
      parsedBody = JSON.parse(text);
    } catch (error) {
      throw new Error(`${method} ${path} returned non-JSON body: ${text.slice(0, 200)}`);
    }
  }
  return { body: parsedBody, headers: response.headers, status: response.status };
}

function sessionCookie(headers) {
  const cookie = headers.get("set-cookie");
  if (!cookie) {
    return "";
  }
  return cookie.split(";", 1)[0];
}

function isMainModule() {
  const entrypoint = process.argv[1];
  return Boolean(entrypoint) && import.meta.url === pathToFileURL(entrypoint).href;
}

if (isMainModule()) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}
