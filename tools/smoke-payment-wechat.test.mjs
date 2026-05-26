import assert from "node:assert/strict";
import crypto from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  assertWechatOrder,
  loadConfig,
  normalizeWechatMode,
  providerConfig,
  publicKeyFromPrivateKey,
  signedWechatNotification,
  wechatTradeSuccessNotification,
  wechatTradeType,
} from "./smoke-payment-wechat.mjs";

const API_V3_KEY = "0123456789abcdef0123456789abcdef";

function wechatEnv(extra = {}) {
  return {
    WECHAT_SMOKE_APP_ID: "wx_app_123",
    WECHAT_SMOKE_MCH_ID: "mch_123",
    WECHAT_SMOKE_API_V3_KEY: API_V3_KEY,
    WECHAT_SMOKE_SERIAL_NO: "merchant_serial_123",
    WECHAT_SMOKE_PRIVATE_KEY: "merchant-private-key-for-test",
    ...extra,
  };
}

function rsaKeys() {
  const { privateKey, publicKey } = crypto.generateKeyPairSync("rsa", { modulusLength: 2048 });
  return {
    privateKeyPEM: privateKey.export({ type: "pkcs1", format: "pem" }),
    publicKeyPEM: publicKey.export({ type: "spki", format: "pem" }),
  };
}

test("WeChat payment smoke config requires explicit merchant credentials", () => {
  assert.throws(
    () => loadConfig({}),
    /WECHAT_SMOKE_APP_ID is required/,
  );

  const config = loadConfig(wechatEnv({ SERVER_PORT: "18080" }));

  assert.equal(config.baseURL, "http://127.0.0.1:18080");
  assert.equal(config.adminEmail, "admin@srapi.local");
  assert.equal(config.adminPassword, "password123");
  assert.equal(config.amount, "1.00");
  assert.equal(config.currency, "CNY");
  assert.equal(config.providerName, "wechat-smoke");
  assert.equal(config.method, "wechat_smoke");
  assert.equal(config.mode, "native");
});

test("WeChat payment smoke config validates APIv3 key and mode-specific fields", () => {
  assert.throws(
    () => loadConfig(wechatEnv({ WECHAT_SMOKE_API_V3_KEY: "too-short" })),
    /WECHAT_SMOKE_API_V3_KEY must be 32 bytes/,
  );
  assert.throws(
    () => loadConfig(wechatEnv({ WECHAT_SMOKE_MODE: "h5" })),
    /WECHAT_SMOKE_PAYER_CLIENT_IP is required for H5 mode/,
  );
  assert.throws(
    () => loadConfig(wechatEnv({ WECHAT_SMOKE_MODE: "jsapi" })),
    /WECHAT_SMOKE_PAYER_OPENID is required for JSAPI mode/,
  );

  assert.equal(loadConfig(wechatEnv({ WECHAT_SMOKE_MODE: "wechat_h5", WECHAT_SMOKE_PAYER_CLIENT_IP: "127.0.0.1" })).mode, "h5");
  assert.equal(loadConfig(wechatEnv({ WECHAT_SMOKE_MODE: "wechat_jsapi", WECHAT_SMOKE_PAYER_OPENID: "openid_123" })).mode, "jsapi");
});

test("WeChat local webhook mode requires an explicit platform signing key", () => {
  assert.throws(
    () => loadConfig(wechatEnv({ WECHAT_SMOKE_LOCAL_WEBHOOK: "1" })),
    /WECHAT_SMOKE_PLATFORM_PRIVATE_KEY is required/,
  );
});

test("WeChat provider config carries prepay and optional verifier settings", () => {
  const keys = rsaKeys();
  const config = loadConfig(wechatEnv({
    SRAPI_BASE_URL: "https://api.example",
    WECHAT_SMOKE_MODE: "h5",
    WECHAT_SMOKE_PAYER_CLIENT_IP: "203.0.113.10",
    WECHAT_SMOKE_H5_APP_NAME: "SRapi",
    WECHAT_SMOKE_LOCAL_WEBHOOK: "1",
    WECHAT_SMOKE_PLATFORM_PRIVATE_KEY: keys.privateKeyPEM,
    WECHAT_SMOKE_PLATFORM_PUBLIC_KEY_ID: "PUB_KEY_ID_TEST",
  }));

  assert.deepEqual(providerConfig(config), {
    app_id: "wx_app_123",
    mch_id: "mch_123",
    api_v3_key: API_V3_KEY,
    serial_no: "merchant_serial_123",
    private_key: "merchant-private-key-for-test",
    notify_url: "https://api.example/api/v1/webhooks/payments/wechat",
    mode: "h5",
    description: "SRapi WeChat Pay smoke",
    payer_client_ip: "203.0.113.10",
    h5_app_name: "SRapi",
    wechatpay_public_key: keys.publicKeyPEM,
    wechatpay_public_key_id: "PUB_KEY_ID_TEST",
  });
  assert.equal(publicKeyFromPrivateKey(keys.privateKeyPEM), keys.publicKeyPEM);
});

test("WeChat order assertion matches Native, H5, and JSAPI metadata", () => {
  assert.doesNotThrow(() => assertWechatOrder({
    order_no: "pay_native_123",
    metadata: {
      checkout_session_id: "pay_native_123",
      checkout_url: "weixin://wxpay/bizpayurl?pr=abc",
      wechat_pay_url: "weixin://wxpay/bizpayurl?pr=abc",
      wechat_code_url: "weixin://wxpay/bizpayurl?pr=abc",
      wechat_pay_mode: "native",
    },
  }, { mode: "native" }));

  assert.doesNotThrow(() => assertWechatOrder({
    order_no: "pay_h5_123",
    metadata: {
      checkout_session_id: "pay_h5_123",
      checkout_url: "https://wx.tenpay.com/cgi-bin/mmpayweb-bin/checkmweb?prepay_id=1",
      wechat_pay_url: "https://wx.tenpay.com/cgi-bin/mmpayweb-bin/checkmweb?prepay_id=1",
      wechat_h5_url: "https://wx.tenpay.com/cgi-bin/mmpayweb-bin/checkmweb?prepay_id=1",
      wechat_pay_mode: "h5",
    },
  }, { mode: "h5" }));

  assert.doesNotThrow(() => assertWechatOrder({
    order_no: "pay_jsapi_123",
    metadata: {
      checkout_session_id: "wx_prepay_123",
      wechat_app_id: "wx_app_123",
      wechat_package: "prepay_id=wx_prepay_123",
      wechat_pay_sign: "signed",
      wechat_prepay_id: "wx_prepay_123",
      wechat_pay_mode: "jsapi",
    },
  }, { mode: "jsapi" }));

  assert.throws(
    () => assertWechatOrder({
      order_no: "pay_native_123",
      metadata: {
        checkout_session_id: "pay_native_123",
        checkout_url: "weixin://wxpay/bizpayurl?pr=abc",
        wechat_pay_url: "weixin://wxpay/bizpayurl?pr=abc",
        wechat_code_url: "weixin://wxpay/bizpayurl?pr=other",
        wechat_pay_mode: "native",
      },
    }, { mode: "native" }),
    /wechat native code URL metadata mismatch/,
  );
});

test("WeChat APIv3 notification encrypts resource and signs exact header message", () => {
  const keys = rsaKeys();
  const config = {
    apiV3Key: API_V3_KEY,
    eventID: "evt_wechat_paid_1",
    platformPrivateKey: keys.privateKeyPEM,
    platformPublicKeyID: "PUB_KEY_ID_TEST",
    resourceNonce: "notify123456",
    signNonce: "signnonce123",
  };
  const notification = signedWechatNotification(config, {
    out_trade_no: "pay_123",
    transaction_id: "420000000020260526000001",
    trade_state: "SUCCESS",
    amount: {
      total: 1234,
      currency: "CNY",
    },
  });
  const body = JSON.parse(notification.body);
  const encrypted = Buffer.from(body.resource.ciphertext, "base64");
  const authTag = encrypted.subarray(encrypted.length - 16);
  const ciphertext = encrypted.subarray(0, encrypted.length - 16);
  const decipher = crypto.createDecipheriv("aes-256-gcm", Buffer.from(API_V3_KEY, "utf8"), Buffer.from(body.resource.nonce, "utf8"));
  decipher.setAAD(Buffer.from(body.resource.associated_data, "utf8"));
  decipher.setAuthTag(authTag);
  const plaintext = Buffer.concat([decipher.update(ciphertext), decipher.final()]).toString("utf8");
  const transaction = JSON.parse(plaintext);
  const message = `${notification.headers["Wechatpay-Timestamp"]}\n${notification.headers["Wechatpay-Nonce"]}\n${notification.body}\n`;

  assert.equal(body.id, "evt_wechat_paid_1");
  assert.equal(body.event_type, "TRANSACTION.SUCCESS");
  assert.equal(transaction.out_trade_no, "pay_123");
  assert.equal(transaction.amount.total, 1234);
  assert.equal(
    crypto.verify("RSA-SHA256", Buffer.from(message, "utf8"), keys.publicKeyPEM, Buffer.from(notification.headers["Wechatpay-Signature"], "base64")),
    true,
  );
});

test("WeChat success notification preserves reconciliation fields", () => {
  const keys = rsaKeys();
  const config = {
    appID: "wx_app_123",
    mchID: "mch_123",
    apiV3Key: API_V3_KEY,
    mode: "native",
    eventID: "evt_wechat_paid_2",
    transactionID: "420000000020260526000002",
    platformPrivateKey: keys.privateKeyPEM,
    platformPublicKeyID: "PUB_KEY_ID_TEST",
  };
  const notification = wechatTradeSuccessNotification(config, {
    order_no: "pay_123",
    amount: "12.34000000",
    currency: "CNY",
  });
  const body = JSON.parse(notification.body);

  assert.equal(body.id, "evt_wechat_paid_2");
  assert.equal(notification.headers["Wechatpay-Serial"], "PUB_KEY_ID_TEST");
  assert.match(notification.headers["Wechatpay-Signature"], /^[A-Za-z0-9+/=]+$/);
});

test("WeChat mode helpers normalize checkout mode and trade type", () => {
  assert.equal(normalizeWechatMode("wechat_h5"), "h5");
  assert.equal(normalizeWechatMode("wechat_jsapi"), "jsapi");
  assert.equal(normalizeWechatMode("anything-else"), "native");
  assert.equal(wechatTradeType("h5"), "MWEB");
  assert.equal(wechatTradeType("jsapi"), "JSAPI");
  assert.equal(wechatTradeType("native"), "NATIVE");
});

test("WeChat payment smoke script keeps cleanup, idempotency, and real-prepay boundaries", () => {
  const script = readFileSync("tools/smoke-payment-wechat.mjs", "utf8");

  assert.match(script, /finally\s*{\s*if \(provider\?\.id\) {\s*await disableWechatProvider/s);
  assert.match(script, /WECHAT_SMOKE_APP_ID/);
  assert.match(script, /WECHAT_SMOKE_LOCAL_WEBHOOK/);
  assert.match(script, /duplicate\.body\?\.data\?\.handled !== false/);
  assert.match(script, /fulfilled\.provider_transaction_id !== config\.transactionID/);
  assert.match(script, /balance did not increase by/);
});
