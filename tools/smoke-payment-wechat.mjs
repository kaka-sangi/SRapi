#!/usr/bin/env node

import crypto from "node:crypto";
import { pathToFileURL } from "node:url";
import {
  addMoney,
  assertHealth,
  baseSmokeConfig,
  currentBalance,
  findPaymentOrder,
  findPaymentProvider,
  loginAdmin,
  minorAmount,
  request,
  requiredEnv,
} from "./payment-smoke-common.mjs";

function loadConfig(env = process.env) {
  const localWebhook = env.WECHAT_SMOKE_LOCAL_WEBHOOK === "1" || env.WECHAT_SMOKE_LOCAL_WEBHOOK === "true";
  const mode = normalizeWechatMode(env.WECHAT_SMOKE_MODE || "native");
  const config = {
    ...baseSmokeConfig(env),
    appID: requiredEnv(env, "WECHAT_SMOKE_APP_ID"),
    mchID: requiredEnv(env, "WECHAT_SMOKE_MCH_ID"),
    apiV3Key: requiredEnv(env, "WECHAT_SMOKE_API_V3_KEY"),
    serialNo: requiredEnv(env, "WECHAT_SMOKE_SERIAL_NO"),
    privateKey: requiredEnv(env, "WECHAT_SMOKE_PRIVATE_KEY"),
    amount: env.WECHAT_SMOKE_AMOUNT || "1.00",
    currency: (env.WECHAT_SMOKE_CURRENCY || "CNY").toUpperCase(),
    providerName: env.WECHAT_SMOKE_PROVIDER_NAME || "wechat-smoke",
    method: env.WECHAT_SMOKE_METHOD || "wechat_smoke",
    mode,
    notifyURL: env.WECHAT_SMOKE_NOTIFY_URL || "",
    description: env.WECHAT_SMOKE_DESCRIPTION || "SRapi WeChat Pay smoke",
    payerClientIP: env.WECHAT_SMOKE_PAYER_CLIENT_IP || "",
    payerOpenID: env.WECHAT_SMOKE_PAYER_OPENID || "",
    h5Type: env.WECHAT_SMOKE_H5_TYPE || "",
    h5AppName: env.WECHAT_SMOKE_H5_APP_NAME || "",
    h5AppURL: env.WECHAT_SMOKE_H5_APP_URL || "",
    h5BundleID: env.WECHAT_SMOKE_H5_BUNDLE_ID || "",
    h5PackageName: env.WECHAT_SMOKE_H5_PACKAGE_NAME || "",
    localWebhook,
    platformPrivateKey: env.WECHAT_SMOKE_PLATFORM_PRIVATE_KEY || "",
    platformPublicKeyID: env.WECHAT_SMOKE_PLATFORM_PUBLIC_KEY_ID || "PUB_KEY_ID_SMOKE",
    eventID: env.WECHAT_SMOKE_EVENT_ID || "",
    transactionID: env.WECHAT_SMOKE_TRANSACTION_ID || "",
  };
  if (config.currency !== "CNY") {
    throw new Error("WECHAT_SMOKE_CURRENCY must be CNY");
  }
  if (Buffer.byteLength(config.apiV3Key, "utf8") !== 32) {
    throw new Error("WECHAT_SMOKE_API_V3_KEY must be 32 bytes");
  }
  if (config.mode === "h5" && !config.payerClientIP.trim()) {
    throw new Error("WECHAT_SMOKE_PAYER_CLIENT_IP is required for H5 mode");
  }
  if (config.mode === "jsapi" && !config.payerOpenID.trim()) {
    throw new Error("WECHAT_SMOKE_PAYER_OPENID is required for JSAPI mode");
  }
  if (localWebhook) {
    config.platformPrivateKey = requiredEnv(env, "WECHAT_SMOKE_PLATFORM_PRIVATE_KEY");
    if (!config.eventID.trim()) {
      config.eventID = `evt_srapi_wechat_smoke_${Date.now()}`;
    }
    if (!config.transactionID.trim()) {
      config.transactionID = `420000000020260526${Date.now()}`;
    }
  }
  return config;
}

async function main(config = loadConfig()) {
  await assertHealth(config);
  const { cookie, csrfToken } = await loginAdmin(config);
  const before = await currentBalance(config, cookie);
  let provider;
  try {
    provider = await ensureWechatProvider(config, { cookie, csrfToken });
    const test = await request(config, "POST", `/api/v1/admin/payments/providers/${provider.id}/test`, {
      cookie,
      csrfToken,
    });
    if (test.body?.data?.status !== "ok") {
      throw new Error(`wechat provider local test failed: ${JSON.stringify(test.body?.data)}`);
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
          smoke: "wechat",
        },
      },
      expectedStatus: 201,
    });
    const order = orderResponse.body?.data;
    assertWechatOrder(order, config);

    if (config.localWebhook) {
      const payload = wechatTradeSuccessNotification(config, order);
      const webhookResult = await request(config, "POST", "/api/v1/webhooks/payments/wechat", {
        bodyText: payload.body,
        headers: {
          "Content-Type": "application/json",
          ...payload.headers,
        },
      });
      if (webhookResult.body?.data?.handled !== true) {
        throw new Error(`wechat webhook was not handled: ${JSON.stringify(webhookResult.body)}`);
      }
      const fulfilled = webhookResult.body?.data?.order;
      if (fulfilled?.status !== "fulfilled") {
        throw new Error(`wechat webhook did not fulfill order: ${JSON.stringify(fulfilled)}`);
      }
      if (fulfilled.provider_transaction_id !== config.transactionID) {
        throw new Error(`fulfilled order did not preserve WeChat transaction_id: ${JSON.stringify(fulfilled)}`);
      }

      const after = await currentBalance(config, cookie);
      if (addMoney(before.balance, config.amount) !== after.balance) {
        throw new Error(`balance did not increase by ${config.amount}: before=${before.balance} after=${after.balance}`);
      }

      const duplicate = await request(config, "POST", "/api/v1/webhooks/payments/wechat", {
        bodyText: payload.body,
        headers: {
          "Content-Type": "application/json",
          ...payload.headers,
        },
      });
      if (duplicate.body?.data?.handled !== false) {
        throw new Error(`duplicate WeChat webhook was not idempotent: ${JSON.stringify(duplicate.body)}`);
      }
      const afterDuplicate = await currentBalance(config, cookie);
      if (afterDuplicate.balance !== after.balance) {
        throw new Error(`duplicate WeChat webhook changed balance: before=${after.balance} after=${afterDuplicate.balance}`);
      }
      const listed = await findPaymentOrder(config, { cookie, orderNo: order.order_no });
      if (listed?.status !== "fulfilled") {
        throw new Error(`wechat fulfilled order was not listed as fulfilled: ${JSON.stringify(listed)}`);
      }
      console.log(`wechat local webhook smoke ok: ${before.balance} -> ${after.balance} ${after.currency}`);
    } else {
      console.log("wechat prepay smoke ok; local webhook mode disabled");
    }

    console.log(`wechat payment smoke ok: ${config.baseURL}`);
    console.log(`order_no: ${order.order_no}`);
    console.log(`mode: ${config.mode}`);
    if (order.metadata.checkout_url) {
      console.log(`checkout_url: ${order.metadata.checkout_url}`);
    }
  } finally {
    if (provider?.id) {
      await disableWechatProvider(config, { cookie, csrfToken, providerID: provider.id });
    }
  }
}

async function ensureWechatProvider(config, { cookie, csrfToken }) {
  const existing = await findPaymentProvider(config, { cookie, name: config.providerName });
  const body = {
    provider: "wechat",
    name: config.providerName,
    status: "active",
    config: providerConfig(config),
    supported_methods: [config.method],
    sort_order: 100002,
    limits: {
      currency: "CNY",
    },
    metadata: {
      display_name: "WeChat Pay Smoke",
      smoke: "wechat",
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

async function disableWechatProvider(config, { cookie, csrfToken, providerID }) {
  const response = await request(config, "PATCH", `/api/v1/admin/payments/providers/${providerID}`, {
    cookie,
    csrfToken,
    body: {
      status: "disabled",
    },
  });
  if (response.body?.data?.status !== "disabled") {
    throw new Error(`failed to disable wechat smoke provider ${providerID}`);
  }
}

function providerConfig(config) {
  const out = {
    app_id: config.appID,
    mch_id: config.mchID,
    api_v3_key: config.apiV3Key,
    serial_no: config.serialNo,
    private_key: config.privateKey,
    notify_url: config.notifyURL || `${config.baseURL}/api/v1/webhooks/payments/wechat`,
    mode: config.mode,
    description: config.description,
  };
  optionalSet(out, "payer_client_ip", config.payerClientIP);
  optionalSet(out, "payer_openid", config.payerOpenID);
  optionalSet(out, "h5_type", config.h5Type);
  optionalSet(out, "h5_app_name", config.h5AppName);
  optionalSet(out, "h5_app_url", config.h5AppURL);
  optionalSet(out, "h5_bundle_id", config.h5BundleID);
  optionalSet(out, "h5_package_name", config.h5PackageName);
  if (config.localWebhook) {
    out.wechatpay_public_key = publicKeyFromPrivateKey(config.platformPrivateKey);
    out.wechatpay_public_key_id = config.platformPublicKeyID;
  }
  return out;
}

function assertWechatOrder(order, config) {
  const metadata = order?.metadata || {};
  const mode = config.mode || "native";
  if (!order?.order_no || !metadata.checkout_session_id || metadata.wechat_pay_mode !== mode) {
    throw new Error(`wechat order did not include expected session metadata: ${JSON.stringify(order)}`);
  }
  if (mode === "jsapi") {
    for (const key of ["wechat_app_id", "wechat_package", "wechat_pay_sign", "wechat_prepay_id"]) {
      if (!metadata[key]) {
        throw new Error(`wechat jsapi order missing ${key}: ${JSON.stringify(metadata)}`);
      }
    }
    return;
  }
  const checkoutURL = metadata.checkout_url;
  if (!checkoutURL || metadata.wechat_pay_url !== checkoutURL) {
    throw new Error(`wechat order did not include checkout URL metadata: ${JSON.stringify(order)}`);
  }
  if (mode === "native" && metadata.wechat_code_url !== checkoutURL) {
    throw new Error(`wechat native code URL metadata mismatch: ${JSON.stringify(metadata)}`);
  }
  if (mode === "h5" && metadata.wechat_h5_url !== checkoutURL) {
    throw new Error(`wechat h5 URL metadata mismatch: ${JSON.stringify(metadata)}`);
  }
}

function wechatTradeSuccessNotification(config, order) {
  return signedWechatNotification(config, {
    appid: config.appID,
    mchid: config.mchID,
    out_trade_no: order.order_no,
    transaction_id: config.transactionID,
    trade_state: "SUCCESS",
    trade_type: wechatTradeType(config.mode),
    amount: {
      total: minorAmount(order.amount, order.currency),
      currency: order.currency,
    },
  });
}

function signedWechatNotification(config, transaction) {
  const plaintext = JSON.stringify(transaction);
  const associatedData = "transaction";
  const resourceNonce = config.resourceNonce || "notify123456";
  const cipher = crypto.createCipheriv("aes-256-gcm", Buffer.from(config.apiV3Key, "utf8"), Buffer.from(resourceNonce, "utf8"));
  cipher.setAAD(Buffer.from(associatedData, "utf8"));
  const encrypted = Buffer.concat([cipher.update(plaintext, "utf8"), cipher.final()]);
  const ciphertext = Buffer.concat([encrypted, cipher.getAuthTag()]).toString("base64");
  const body = JSON.stringify({
    id: config.eventID,
    create_time: new Date().toISOString(),
    event_type: "TRANSACTION.SUCCESS",
    resource_type: "encrypt-resource",
    summary: "transaction success",
    resource: {
      algorithm: "AEAD_AES_256_GCM",
      ciphertext,
      associated_data: associatedData,
      nonce: resourceNonce,
      original_type: "transaction",
    },
  });
  const timestamp = String(Math.floor(Date.now() / 1000));
  const signNonce = config.signNonce || crypto.randomBytes(12).toString("hex");
  const message = `${timestamp}\n${signNonce}\n${body}\n`;
  const signature = crypto.sign("RSA-SHA256", Buffer.from(message, "utf8"), config.platformPrivateKey).toString("base64");
  return {
    body,
    headers: {
      "Wechatpay-Nonce": signNonce,
      "Wechatpay-Serial": config.platformPublicKeyID,
      "Wechatpay-Signature": signature,
      "Wechatpay-Timestamp": timestamp,
    },
  };
}

function publicKeyFromPrivateKey(privateKey) {
  return crypto.createPublicKey(privateKey).export({ type: "spki", format: "pem" });
}

function normalizeWechatMode(value) {
  switch (String(value).trim().toLowerCase()) {
    case "h5":
    case "wap":
    case "wechat_h5":
    case "wechat_wap":
      return "h5";
    case "jsapi":
    case "wechat_jsapi":
    case "wechat_mini_program":
      return "jsapi";
    default:
      return "native";
  }
}

function wechatTradeType(mode) {
  switch (normalizeWechatMode(mode)) {
    case "h5":
      return "MWEB";
    case "jsapi":
      return "JSAPI";
    default:
      return "NATIVE";
  }
}

function optionalSet(target, key, value) {
  if (String(value || "").trim()) {
    target[key] = value;
  }
}

function isMainModule() {
  const entrypoint = process.argv[1];
  return Boolean(entrypoint) && import.meta.url === pathToFileURL(entrypoint).href;
}

export {
  assertWechatOrder,
  loadConfig,
  normalizeWechatMode,
  providerConfig,
  publicKeyFromPrivateKey,
  signedWechatNotification,
  wechatTradeSuccessNotification,
  wechatTradeType,
};

if (isMainModule()) {
  main().catch((error) => {
    console.error(error.message);
    process.exit(1);
  });
}
