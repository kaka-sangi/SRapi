#!/usr/bin/env node

import crypto from "node:crypto";
import { pathToFileURL } from "node:url";
import {
  addMoney,
  assertHealth,
  baseSmokeConfig,
  currentBalance,
  findPaymentProvider,
  loginAdmin,
  money2,
  request,
  requiredEnv,
} from "./payment-smoke-common.mjs";

function loadConfig(env = process.env) {
  const localWebhook = env.ALIPAY_SMOKE_LOCAL_WEBHOOK === "1" || env.ALIPAY_SMOKE_LOCAL_WEBHOOK === "true";
  const notifyPrivateKey = env.ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY || "";
  const appID = requiredEnv(env, "ALIPAY_SMOKE_APP_ID");
  const privateKey = requiredEnv(env, "ALIPAY_SMOKE_PRIVATE_KEY");
  const alipayPublicKey = localWebhook
    ? publicKeyFromPrivateKey(requiredEnv(env, "ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY"))
    : requiredEnv(env, "ALIPAY_SMOKE_ALIPAY_PUBLIC_KEY");
  const config = {
    ...baseSmokeConfig(env),
    appID,
    privateKey,
    alipayPublicKey,
    amount: env.ALIPAY_SMOKE_AMOUNT || "1.00",
    currency: (env.ALIPAY_SMOKE_CURRENCY || "CNY").toUpperCase(),
    providerName: env.ALIPAY_SMOKE_PROVIDER_NAME || "alipay-smoke",
    method: env.ALIPAY_SMOKE_METHOD || "alipay_smoke",
    mode: env.ALIPAY_SMOKE_MODE || "page",
    gatewayURL: env.ALIPAY_SMOKE_GATEWAY_URL || "",
    production: env.ALIPAY_SMOKE_PRODUCTION || "",
    subject: env.ALIPAY_SMOKE_SUBJECT || "SRapi Alipay smoke",
    body: env.ALIPAY_SMOKE_BODY || "SRapi balance top-up smoke",
    returnURL: env.ALIPAY_SMOKE_RETURN_URL || "",
    notifyURL: env.ALIPAY_SMOKE_NOTIFY_URL || "",
    localWebhook,
    notifyPrivateKey,
    notifyID: env.ALIPAY_SMOKE_NOTIFY_ID || "",
    tradeNo: env.ALIPAY_SMOKE_TRADE_NO || "",
  };
  if (config.currency !== "CNY") {
    throw new Error("ALIPAY_SMOKE_CURRENCY must be CNY");
  }
  if (localWebhook) {
    if (!config.notifyID.trim()) {
      config.notifyID = `notify_srapi_smoke_${Date.now()}`;
    }
    if (!config.tradeNo.trim()) {
      config.tradeNo = `20260526220014${Date.now()}`;
    }
  }
  return config;
}

function publicKeyFromPrivateKey(privateKey) {
  return crypto.createPublicKey(privateKey).export({ type: "spki", format: "pem" });
}

async function main(config = loadConfig()) {
  await assertHealth(config);
  const { cookie, csrfToken } = await loginAdmin(config);
  const before = await currentBalance(config, cookie);
  let provider;
  try {
    provider = await ensureAlipayProvider(config, { cookie, csrfToken });
    const test = await request(config, "POST", `/api/v1/admin/payments/providers/${provider.id}/test`, {
      cookie,
      csrfToken,
    });
    if (test.body?.data?.status !== "ok") {
      throw new Error(`alipay provider local test failed: ${JSON.stringify(test.body?.data)}`);
    }

    const orderResponse = await request(config, "POST", "/api/v1/payment/orders", {
      cookie,
      csrfToken,
      body: {
        method: config.method,
        amount: config.amount,
        currency: config.currency,
        product_type: "balance_credit",
        metadata: {
          smoke: "alipay",
        },
      },
      expectedStatus: 201,
    });
    const order = orderResponse.body?.data;
    assertAlipayOrder(order);

    if (config.localWebhook) {
      const payload = alipayTradeSuccessNotification(config, order);
      const webhookResult = await request(config, "POST", "/api/v1/webhooks/payments/alipay", {
        body: payload,
        responseType: "text",
      });
      if (webhookResult.bodyText.trim() !== "success") {
        throw new Error(`alipay webhook did not return success ack: ${webhookResult.bodyText}`);
      }
      const fulfilled = await findPaymentOrder(config, { cookie, orderNo: order.order_no });
      if (fulfilled?.status !== "fulfilled") {
        throw new Error(`alipay webhook did not fulfill order: ${JSON.stringify(fulfilled)}`);
      }
      if (fulfilled.provider_transaction_id !== config.tradeNo) {
        throw new Error(`fulfilled order did not preserve Alipay trade_no: ${JSON.stringify(fulfilled)}`);
      }
      const after = await currentBalance(config, cookie);
      if (addMoney(before.balance, config.amount) !== after.balance) {
        throw new Error(`balance did not increase by ${config.amount}: before=${before.balance} after=${after.balance}`);
      }

      const duplicate = await request(config, "POST", "/api/v1/webhooks/payments/alipay", {
        body: payload,
        responseType: "text",
      });
      if (duplicate.bodyText.trim() !== "success") {
        throw new Error(`duplicate Alipay webhook did not return success ack: ${duplicate.bodyText}`);
      }
      const afterDuplicate = await currentBalance(config, cookie);
      if (afterDuplicate.balance !== after.balance) {
        throw new Error(`duplicate Alipay webhook changed balance: before=${after.balance} after=${afterDuplicate.balance}`);
      }
      console.log(`alipay local webhook smoke ok: ${before.balance} -> ${after.balance} ${after.currency}`);
    } else {
      console.log("alipay checkout smoke ok; local webhook mode disabled");
    }

    console.log(`alipay payment smoke ok: ${config.baseURL}`);
    console.log(`order_no: ${order.order_no}`);
    console.log(`checkout_url: ${order.metadata.checkout_url}`);
  } finally {
    if (provider?.id) {
      await disableAlipayProvider(config, { cookie, csrfToken, providerID: provider.id });
    }
  }
}

async function findPaymentOrder(config, { cookie, orderNo }) {
  const response = await request(config, "GET", "/api/v1/payment/orders", { cookie });
  const orders = response.body?.data;
  if (!Array.isArray(orders)) {
    throw new Error("GET /api/v1/payment/orders did not return data array");
  }
  return orders.find((order) => order?.order_no === orderNo) || null;
}

async function ensureAlipayProvider(config, { cookie, csrfToken }) {
  const existing = await findPaymentProvider(config, { cookie, name: config.providerName });
  const body = {
    provider: "alipay",
    name: config.providerName,
    status: "active",
    config: providerConfig(config),
    supported_methods: [config.method],
    sort_order: 100001,
    limits: {
      currency: "CNY",
    },
    metadata: {
      display_name: "Alipay Smoke",
      smoke: "alipay",
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

async function disableAlipayProvider(config, { cookie, csrfToken, providerID }) {
  const response = await request(config, "PATCH", `/api/v1/admin/payments/providers/${providerID}`, {
    cookie,
    csrfToken,
    body: {
      status: "disabled",
    },
  });
  if (response.body?.data?.status !== "disabled") {
    throw new Error(`failed to disable alipay smoke provider ${providerID}`);
  }
}

function providerConfig(config) {
  const out = {
    app_id: config.appID,
    private_key: config.privateKey,
    alipay_public_key: config.alipayPublicKey,
    notify_url: config.notifyURL || `${config.baseURL}/api/v1/webhooks/payments/alipay`,
    return_url: config.returnURL || `${config.baseURL}/alipay-smoke/return`,
    mode: config.mode,
    subject: config.subject,
    body: config.body,
  };
  if (config.gatewayURL) {
    out.gateway_url = config.gatewayURL;
  }
  if (config.production) {
    out.production = config.production;
  }
  return out;
}

function assertAlipayOrder(order) {
  const checkoutURL = order?.metadata?.checkout_url;
  if (!order?.order_no || !checkoutURL || !order?.metadata?.alipay_pay_url) {
    throw new Error(`alipay order did not include checkout URL metadata: ${JSON.stringify(order)}`);
  }
  if (order.metadata.alipay_pay_url !== checkoutURL) {
    throw new Error(`alipay checkout URL metadata mismatch: ${JSON.stringify(order.metadata)}`);
  }
  const parsed = new URL(checkoutURL);
  if (parsed.searchParams.get("method") !== "alipay.trade.page.pay") {
    throw new Error(`unexpected Alipay checkout method: ${checkoutURL}`);
  }
  if (!parsed.searchParams.get("sign") || parsed.searchParams.get("sign_type") !== "RSA2") {
    throw new Error(`Alipay checkout URL missing RSA2 signature: ${checkoutURL}`);
  }
}

function alipayTradeSuccessNotification(config, order) {
  const payload = {
    app_id: config.appID,
    charset: "utf-8",
    notify_id: config.notifyID,
    notify_type: "trade_status_sync",
    out_trade_no: order.order_no,
    total_amount: money2(order.amount),
    trade_no: config.tradeNo,
    trade_status: "TRADE_SUCCESS",
    version: "1.0",
  };
  payload.sign_type = "RSA2";
  payload.sign = alipayRSASign(payload, config.notifyPrivateKey, ["sign", "sign_type", "alipay_cert_sn"]);
  return payload;
}

function alipayRSASign(fields, privateKey, ignores = []) {
  const ignored = new Set(ignores);
  const signingText = Object.keys(fields)
    .filter((key) => !ignored.has(key))
    .flatMap((key) => {
      const value = String(fields[key]).trim();
      return value ? [`${key}=${value}`] : [];
    })
    .sort()
    .join("&");
  return crypto.createSign("RSA-SHA256").update(signingText).sign(privateKey, "base64");
}

function isMainModule() {
  const entrypoint = process.argv[1];
  return Boolean(entrypoint) && import.meta.url === pathToFileURL(entrypoint).href;
}

export {
  alipayRSASign,
  alipayTradeSuccessNotification,
  assertAlipayOrder,
  loadConfig,
  money2,
  providerConfig,
  publicKeyFromPrivateKey,
};

if (isMainModule()) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}
