#!/usr/bin/env node

import crypto from "node:crypto";
import { pathToFileURL } from "node:url";

function loadConfig(env = process.env) {
  return {
    baseURL: env.SRAPI_BASE_URL || `http://127.0.0.1:${env.SERVER_PORT || "8080"}`,
    adminEmail: env.BOOTSTRAP_ADMIN_EMAIL || "admin@srapi.local",
    adminPassword: env.BOOTSTRAP_ADMIN_PASSWORD || "password123",
    stripeSecretKey: requiredEnv(env, "STRIPE_SMOKE_SECRET_KEY"),
    stripeWebhookSecret: requiredEnv(env, "STRIPE_SMOKE_WEBHOOK_SECRET"),
    amount: env.STRIPE_SMOKE_AMOUNT || "1.00",
    currency: (env.STRIPE_SMOKE_CURRENCY || "USD").toUpperCase(),
    providerName: env.STRIPE_SMOKE_PROVIDER_NAME || "stripe-smoke",
  };
}

async function main(config = loadConfig()) {
  const health = await request(config, "GET", "/api/v1/health");
  if (!health.body?.request_id) {
    throw new Error("health response missing request_id");
  }

  const login = await request(config, "POST", "/api/v1/auth/login", {
    body: { email: config.adminEmail, password: config.adminPassword },
  });
  const cookie = sessionCookie(login.headers);
  const csrfToken = login.body?.data?.csrf_token;
  if (!cookie || !csrfToken) {
    throw new Error("login did not return csrf token and session cookie");
  }

  const before = await currentBalance(config, cookie);
  let provider;
  try {
    provider = await ensureStripeProvider(config, { cookie, csrfToken });
    const test = await request(config, "POST", `/api/v1/admin/payments/providers/${provider.id}/test`, {
      cookie,
      csrfToken,
    });
    if (test.body?.data?.status !== "ok") {
      throw new Error(`stripe provider local test failed: ${JSON.stringify(test.body?.data)}`);
    }

    const orderResponse = await request(config, "POST", "/api/v1/payment/orders", {
      cookie,
      csrfToken,
      body: {
        method: "stripe_card_smoke",
        amount: config.amount,
        currency: config.currency,
        product_type: "balance_credit",
        metadata: {
          smoke: "stripe",
        },
      },
      expectedStatus: 201,
    });
    const order = orderResponse.body?.data;
    if (!order?.order_no || !order?.metadata?.stripe_checkout_session_id || !order?.metadata?.stripe_checkout_url) {
      throw new Error(`stripe order did not include checkout session metadata: ${JSON.stringify(order)}`);
    }
    if (!String(order.metadata.stripe_checkout_url).startsWith("https://checkout.stripe.com/")) {
      throw new Error(`unexpected Stripe checkout URL: ${order.metadata.stripe_checkout_url}`);
    }

    const webhook = stripeCheckoutCompletedEvent(order);
    const webhookBody = JSON.stringify(webhook);
    const webhookResult = await request(config, "POST", "/api/v1/webhooks/payments/stripe", {
      bodyText: webhookBody,
      headers: {
        "Content-Type": "application/json",
        "Stripe-Signature": stripeSignatureHeader(webhookBody, config.stripeWebhookSecret),
      },
    });
    if (webhookResult.body?.data?.handled !== true) {
      throw new Error(`stripe webhook was not handled: ${JSON.stringify(webhookResult.body)}`);
    }
    const fulfilled = webhookResult.body?.data?.order;
    if (fulfilled?.status !== "fulfilled") {
      throw new Error(`stripe webhook did not fulfill order: ${JSON.stringify(fulfilled)}`);
    }
    if (fulfilled.provider_transaction_id !== order.metadata.stripe_checkout_session_id) {
      throw new Error(`fulfilled order did not preserve Stripe session id: ${JSON.stringify(fulfilled)}`);
    }

    const duplicate = await request(config, "POST", "/api/v1/webhooks/payments/stripe", {
      bodyText: webhookBody,
      headers: {
        "Content-Type": "application/json",
        "Stripe-Signature": stripeSignatureHeader(webhookBody, config.stripeWebhookSecret),
      },
    });
    if (duplicate.body?.data?.handled !== false) {
      throw new Error(`duplicate Stripe webhook was not idempotent: ${JSON.stringify(duplicate.body)}`);
    }

    const after = await currentBalance(config, cookie);
    if (addMoney(before.balance, config.amount) !== after.balance) {
      throw new Error(`balance did not increase by ${config.amount}: before=${before.balance} after=${after.balance}`);
    }

    console.log(`stripe payment smoke ok: ${config.baseURL}`);
    console.log(`order_no: ${order.order_no}`);
    console.log(`checkout_session_id: ${order.metadata.stripe_checkout_session_id}`);
    console.log(`balance: ${before.balance} -> ${after.balance} ${after.currency}`);
  } finally {
    if (provider?.id) {
      await disableStripeProvider(config, { cookie, csrfToken, providerID: provider.id });
    }
  }
}

async function ensureStripeProvider(config, { cookie, csrfToken }) {
  const existing = await findPaymentProvider(config, { cookie, name: config.providerName });
  const body = {
    provider: "stripe",
    name: config.providerName,
    status: "active",
    config: {
      secret_key: config.stripeSecretKey,
      webhook_secret: config.stripeWebhookSecret,
      success_url: `${config.baseURL}/stripe-smoke/success`,
      cancel_url: `${config.baseURL}/stripe-smoke/cancel`,
    },
    supported_methods: ["stripe_card_smoke"],
    sort_order: 100000,
    metadata: {
      display_name: "Stripe Smoke",
      smoke: "stripe",
    },
  };
  if (existing) {
    const response = await request(config, "PATCH", `/api/v1/admin/payments/providers/${existing.id}`, {
      cookie,
      csrfToken,
      body,
    });
    return response.body.data;
  }
  const response = await request(config, "POST", "/api/v1/admin/payments/providers", {
    cookie,
    csrfToken,
    body,
    expectedStatus: 201,
  });
  return response.body.data;
}

async function disableStripeProvider(config, { cookie, csrfToken, providerID }) {
  const response = await request(config, "PATCH", `/api/v1/admin/payments/providers/${providerID}`, {
    cookie,
    csrfToken,
    body: {
      status: "disabled",
    },
  });
  if (response.body?.data?.status !== "disabled") {
    throw new Error(`failed to disable stripe smoke provider ${providerID}`);
  }
}

async function findPaymentProvider(config, { cookie, name }) {
  const response = await request(config, "GET", "/api/v1/admin/payments/providers", { cookie });
  const providers = response.body?.data;
  if (!Array.isArray(providers)) {
    throw new Error("GET /api/v1/admin/payments/providers did not return data array");
  }
  return providers.find((provider) => provider?.name === name) || null;
}

async function currentBalance(config, cookie) {
  const response = await request(config, "GET", "/api/v1/me/balance", { cookie });
  const balance = response.body?.data;
  if (!balance?.balance || !balance?.currency) {
    throw new Error(`GET /api/v1/me/balance returned invalid data: ${JSON.stringify(response.body)}`);
  }
  return balance;
}

function stripeCheckoutCompletedEvent(order) {
  const sessionID = order.metadata.stripe_checkout_session_id;
  return {
    id: `evt_srapi_smoke_${order.order_no}`,
    object: "event",
    type: "checkout.session.completed",
    data: {
      object: {
        id: sessionID,
        object: "checkout.session",
        client_reference_id: order.order_no,
        amount_total: minorAmount(order.amount, order.currency),
        currency: String(order.currency).toLowerCase(),
        metadata: {
          order_no: order.order_no,
        },
      },
    },
  };
}

function stripeSignatureHeader(body, secret) {
  const timestamp = Math.floor(Date.now() / 1000);
  const signedPayload = `${timestamp}.${body}`;
  const signature = crypto.createHmac("sha256", secret).update(signedPayload).digest("hex");
  return `t=${timestamp},v1=${signature}`;
}

function minorAmount(value, code) {
  const decimal = moneyParts(value);
  const scale = zeroDecimalCurrency(code) ? 0 : 2;
  const fraction = decimal.fraction.padEnd(scale, "0").slice(0, scale);
  const normalized = `${decimal.whole}${fraction}`;
  return Number.parseInt(normalized, 10);
}

function addMoney(left, right) {
  return formatMoney(parseMoneyUnits(left) + parseMoneyUnits(right));
}

function parseMoneyUnits(value) {
  const parts = moneyParts(value);
  return Number.parseInt(parts.whole, 10) * 100000000 + Number.parseInt(parts.fraction.padEnd(8, "0"), 10);
}

function formatMoney(units) {
  const whole = Math.trunc(units / 100000000);
  const fraction = String(units % 100000000).padStart(8, "0");
  return `${whole}.${fraction}`;
}

function moneyParts(value) {
  const raw = String(value).trim();
  const match = raw.match(/^([0-9]+)(?:\.([0-9]{1,8}))?$/);
  if (!match) {
    throw new Error(`invalid decimal money value: ${value}`);
  }
  return {
    whole: match[1],
    fraction: match[2] || "",
  };
}

function zeroDecimalCurrency(code) {
  return new Set([
    "BIF",
    "CLP",
    "DJF",
    "GNF",
    "JPY",
    "KMF",
    "KRW",
    "MGA",
    "PYG",
    "RWF",
    "UGX",
    "VND",
    "VUV",
    "XAF",
    "XOF",
    "XPF",
  ]).has(String(code).toUpperCase());
}

async function request(config, method, path, options = {}) {
  const headers = {
    Accept: "application/json",
    ...(options.headers || {}),
  };
  let body;
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(options.body);
  } else if (options.bodyText !== undefined) {
    body = options.bodyText;
  }
  if (options.cookie) {
    headers.Cookie = options.cookie;
  }
  if (options.csrfToken) {
    headers["X-CSRF-Token"] = options.csrfToken;
  }

  const response = await fetch(new URL(path, config.baseURL), {
    method,
    headers,
    body,
  });
  const expectedStatus = options.expectedStatus || 200;
  const expectedStatuses = Array.isArray(expectedStatus) ? expectedStatus : [expectedStatus];
  const text = await response.text();
  if (!expectedStatuses.includes(response.status)) {
    throw new Error(`${method} ${path} expected ${expectedStatuses.join(" or ")}, got ${response.status}: ${text}`);
  }
  let parsedBody;
  if (text) {
    try {
      parsedBody = JSON.parse(text);
    } catch {
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

function requiredEnv(env, name) {
  const value = env[name];
  if (!value || value.trim() === "") {
    throw new Error(`${name} is required`);
  }
  return value.trim();
}

function isMainModule() {
  const entrypoint = process.argv[1];
  return Boolean(entrypoint) && import.meta.url === pathToFileURL(entrypoint).href;
}

export {
  addMoney,
  formatMoney,
  loadConfig,
  minorAmount,
  moneyParts,
  parseMoneyUnits,
  requiredEnv,
  stripeCheckoutCompletedEvent,
  stripeSignatureHeader,
  zeroDecimalCurrency,
};

if (isMainModule()) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}
