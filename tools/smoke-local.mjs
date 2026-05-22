#!/usr/bin/env node

const baseURL = process.env.SRAPI_BASE_URL || `http://127.0.0.1:${process.env.SERVER_PORT || '8080'}`;
const adminEmail = process.env.BOOTSTRAP_ADMIN_EMAIL || 'admin@srapi.local';
const adminPassword = process.env.BOOTSTRAP_ADMIN_PASSWORD || 'password123';
const releaseSmoke = process.argv.includes('--release');

async function main() {
  const health = await request('GET', '/api/v1/health');
  if (!health.body?.request_id || health.body?.data?.status === undefined) {
    throw new Error('health response missing request_id or status');
  }
  if (releaseSmoke) {
    await smokeReleaseEndpoints();
  }

  const login = await request('POST', '/api/v1/auth/login', {
    body: {
      email: adminEmail,
      password: adminPassword,
    },
  });
  const csrfToken = login.body?.data?.csrf_token;
  const cookie = sessionCookie(login.headers);
  if (!csrfToken || !cookie) {
    throw new Error('login did not return csrf token and session cookie');
  }

  const apiKey = await request('POST', '/api/v1/api-keys', {
    cookie,
    csrfToken,
    body: {
      name: `local-smoke-${Date.now()}`,
      scopes: ['gateway:invoke'],
    },
    expectedStatus: 201,
  });
  const plaintextKey = apiKey.body?.data?.plaintext_key;
  if (!plaintextKey) {
    throw new Error('api key creation did not return plaintext_key');
  }

  const models = await request('GET', '/v1/models', {
    bearer: plaintextKey,
  });
  if (!Array.isArray(models.body?.data) || models.body.data.length === 0) {
    throw new Error('/v1/models returned no models');
  }

  const chat = await request('POST', '/v1/chat/completions', {
    bearer: plaintextKey,
    body: {
      model: 'gpt-4o-mini',
      messages: [{ role: 'user', content: 'local smoke gateway call' }],
    },
  });
  const content = chat.body?.choices?.[0]?.message?.content;
  if (typeof content !== 'string' || !content.includes('local smoke gateway call')) {
    throw new Error('mock gateway chat response did not include echoed prompt');
  }

  const responses = await request('POST', '/v1/responses', {
    bearer: plaintextKey,
    body: {
      model: 'gpt-4o-mini',
      input: 'local smoke responses call',
    },
  });
  const responseContent = responses.body?.output?.[0]?.content?.[0]?.text;
  if (typeof responseContent !== 'string' || !responseContent.includes('local smoke responses call')) {
    throw new Error('mock gateway Responses response did not include echoed prompt');
  }

  const messages = await request('POST', '/v1/messages', {
    bearer: plaintextKey,
    body: {
      model: 'gpt-4o-mini',
      max_tokens: 128,
      messages: [{ role: 'user', content: 'local smoke messages call' }],
    },
  });
  const messageContent = messages.body?.content?.[0]?.text;
  if (typeof messageContent !== 'string' || !messageContent.includes('local smoke messages call')) {
    throw new Error('mock gateway Messages response did not include echoed prompt');
  }

  console.log(`local smoke ok: ${baseURL}`);
  console.log(`health request_id: ${health.body.request_id}`);
  console.log(`models: ${models.body.data.map((model) => model.id).join(', ')}`);
  console.log('gateway endpoints: chat_completions, responses, messages');
  if (releaseSmoke) {
    console.log('release endpoints: livez, readyz, metrics');
  }
}

async function smokeReleaseEndpoints() {
  const livez = await request('GET', '/livez');
  if (livez.body?.data?.status !== 'ok') {
    throw new Error('/livez did not report ok');
  }
  const readyz = await request('GET', '/readyz');
  if (readyz.body?.data?.status !== 'ok') {
    throw new Error('/readyz did not report ok');
  }
  const metrics = await rawRequest('GET', '/metrics');
  for (const metric of [
    'srapi_gateway_requests_total',
    'srapi_gateway_request_duration_seconds',
    'srapi_gateway_inflight_requests',
    'srapi_gateway_errors_total',
    'srapi_scheduler_decisions_total',
    'srapi_provider_errors_total',
    'srapi_usage_tokens_total',
    'srapi_reverse_proxy_ban_signals_total',
  ]) {
    if (!metrics.text.includes(metric)) {
      throw new Error(`/metrics missing ${metric}`);
    }
  }
}

async function request(method, path, options = {}) {
  const headers = {
    Accept: 'application/json',
  };
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  if (options.cookie) {
    headers.Cookie = options.cookie;
  }
  if (options.csrfToken) {
    headers['X-CSRF-Token'] = options.csrfToken;
  }
  if (options.bearer) {
    headers.Authorization = `Bearer ${options.bearer}`;
  }

  const response = await fetch(new URL(path, baseURL), {
    method,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  const text = await response.text();
  let body;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch (error) {
      throw new Error(`${method} ${path} returned non-JSON body: ${text.slice(0, 200)}`);
    }
  }
  const expectedStatus = options.expectedStatus || 200;
  if (response.status !== expectedStatus) {
    throw new Error(`${method} ${path} expected ${expectedStatus}, got ${response.status}: ${text}`);
  }
  return { body, headers: response.headers };
}

async function rawRequest(method, path) {
  const response = await fetch(new URL(path, baseURL), { method });
  const text = await response.text();
  if (response.status !== 200) {
    throw new Error(`${method} ${path} expected 200, got ${response.status}: ${text}`);
  }
  return { text, headers: response.headers };
}

function sessionCookie(headers) {
  const cookie = headers.get('set-cookie');
  if (!cookie) {
    return '';
  }
  return cookie.split(';', 1)[0];
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
