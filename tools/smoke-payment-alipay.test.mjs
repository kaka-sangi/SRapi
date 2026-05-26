import assert from "node:assert/strict";
import crypto from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  alipayRSASign,
  alipayTradeSuccessNotification,
  assertAlipayOrder,
  loadConfig,
  money2,
  providerConfig,
  publicKeyFromPrivateKey,
} from "./smoke-payment-alipay.mjs";

function alipayEnv(extra = {}) {
  return {
    ALIPAY_SMOKE_APP_ID: "app_test_123",
    ALIPAY_SMOKE_PRIVATE_KEY: "merchant-private-key-for-test",
    ALIPAY_SMOKE_ALIPAY_PUBLIC_KEY: "alipay-public-key-for-test",
    ...extra,
  };
}

test("Alipay payment smoke config requires explicit merchant credentials", () => {
  assert.throws(
    () => loadConfig({}),
    /ALIPAY_SMOKE_APP_ID is required/,
  );

  const config = loadConfig(alipayEnv({ SERVER_PORT: "18080" }));

  assert.equal(config.baseURL, "http://127.0.0.1:18080");
  assert.equal(config.adminEmail, "admin@srapi.local");
  assert.equal(config.adminPassword, "password123");
  assert.equal(config.amount, "1.00");
  assert.equal(config.currency, "CNY");
  assert.equal(config.providerName, "alipay-smoke");
  assert.equal(config.method, "alipay_smoke");
});

test("Alipay local webhook mode requires an explicit notification signing key", () => {
  assert.throws(
    () => loadConfig(alipayEnv({ ALIPAY_SMOKE_LOCAL_WEBHOOK: "1" })),
    /ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY is required/,
  );
});

test("Alipay local webhook mode derives the verifier key from the notification private key", () => {
  const { privateKey, publicKey } = crypto.generateKeyPairSync("rsa", { modulusLength: 2048 });
  const privateKeyPEM = privateKey.export({ type: "pkcs1", format: "pem" });
  const expectedPublicKey = publicKey.export({ type: "spki", format: "pem" });
  const config = loadConfig(alipayEnv({
    ALIPAY_SMOKE_LOCAL_WEBHOOK: "1",
    ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY: privateKeyPEM,
  }));

  assert.equal(config.alipayPublicKey, expectedPublicKey);
  assert.equal(publicKeyFromPrivateKey(privateKeyPEM), expectedPublicKey);
});

test("Alipay provider config keeps checkout and callback settings explicit", () => {
  const config = loadConfig(alipayEnv({
    SRAPI_BASE_URL: "https://api.example",
    ALIPAY_SMOKE_GATEWAY_URL: "https://openapi.alipay.test/gateway.do",
    ALIPAY_SMOKE_PRODUCTION: "false",
  }));

  assert.deepEqual(providerConfig(config), {
    app_id: "app_test_123",
    private_key: "merchant-private-key-for-test",
    alipay_public_key: "alipay-public-key-for-test",
    notify_url: "https://api.example/api/v1/webhooks/payments/alipay",
    return_url: "https://api.example/alipay-smoke/return",
    mode: "page",
    subject: "SRapi Alipay smoke",
    body: "SRapi balance top-up smoke",
    gateway_url: "https://openapi.alipay.test/gateway.do",
    production: "false",
  });
});

test("Alipay order assertion requires signed page-pay checkout metadata", () => {
  const checkoutURL =
    "https://openapi.alipay.test/gateway.do?method=alipay.trade.page.pay&sign_type=RSA2&sign=signed";

  assert.doesNotThrow(() => assertAlipayOrder({
    order_no: "pay_123",
    metadata: {
      checkout_url: checkoutURL,
      alipay_pay_url: checkoutURL,
    },
  }));
  assert.throws(
    () => assertAlipayOrder({
      order_no: "pay_123",
      metadata: {
        checkout_url: "https://openapi.alipay.test/gateway.do?method=alipay.trade.wap.pay",
        alipay_pay_url: "https://openapi.alipay.test/gateway.do?method=alipay.trade.wap.pay",
      },
    }),
    /unexpected Alipay checkout method/,
  );
});

test("Alipay RSA2 signing ignores signature fields and signs sorted key-value text", () => {
  const { privateKey, publicKey } = crypto.generateKeyPairSync("rsa", { modulusLength: 2048 });
  const fields = {
    b: "two",
    sign_type: "RSA2",
    a: "one",
    sign: "ignored",
  };
  const signature = alipayRSASign(fields, privateKey.export({ type: "pkcs1", format: "pem" }), ["sign", "sign_type"]);
  const verifier = crypto.createVerify("RSA-SHA256");
  verifier.update("a=one&b=two");

  assert.equal(verifier.verify(publicKey.export({ type: "pkcs1", format: "pem" }), signature, "base64"), true);
});

test("Alipay local success notification preserves reconciliation fields", () => {
  const { privateKey } = crypto.generateKeyPairSync("rsa", { modulusLength: 2048 });
  const config = loadConfig(alipayEnv({
    ALIPAY_SMOKE_LOCAL_WEBHOOK: "1",
    ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY: privateKey.export({ type: "pkcs1", format: "pem" }),
    ALIPAY_SMOKE_NOTIFY_ID: "notify_paid_1",
    ALIPAY_SMOKE_TRADE_NO: "2026052622001400000001",
  }));
  const payload = alipayTradeSuccessNotification(config, {
    order_no: "pay_123",
    amount: "12.34000000",
  });

  assert.equal(payload.app_id, "app_test_123");
  assert.equal(payload.notify_id, "notify_paid_1");
  assert.equal(payload.notify_type, "trade_status_sync");
  assert.equal(payload.out_trade_no, "pay_123");
  assert.equal(payload.trade_no, "2026052622001400000001");
  assert.equal(payload.trade_status, "TRADE_SUCCESS");
  assert.equal(payload.total_amount, "12.34");
  assert.equal(payload.sign_type, "RSA2");
  assert.match(payload.sign, /^[A-Za-z0-9+/=]+$/);
});

test("Alipay money helper rejects effective sub-cent amounts", () => {
  assert.equal(money2("12.34000000"), "12.34");
  assert.equal(money2("12"), "12.00");
  assert.throws(() => money2("12.345"), /at most 2 decimal places/);
});

test("Alipay payment smoke script keeps cleanup, idempotency, and local webhook boundaries", () => {
  const script = readFileSync("tools/smoke-payment-alipay.mjs", "utf8");

  assert.match(script, /finally\s*{\s*if \(provider\?\.id\) {\s*await disableAlipayProvider/s);
  assert.match(script, /parsed\.searchParams\.get\("method"\) !== "alipay\.trade\.page\.pay"/);
  assert.match(script, /ALIPAY_SMOKE_LOCAL_WEBHOOK/);
  assert.match(script, /webhookResult\.bodyText\.trim\(\) !== "success"/);
  assert.match(script, /duplicate\.bodyText\.trim\(\) !== "success"/);
  assert.match(script, /fulfilled\.provider_transaction_id !== config\.tradeNo/);
  assert.match(script, /duplicate Alipay webhook changed balance/);
  assert.match(script, /balance did not increase by/);
});
