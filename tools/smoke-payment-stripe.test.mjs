import assert from "node:assert/strict";
import crypto from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  addMoney,
  loadConfig,
  minorAmount,
  stripeCheckoutCompletedEvent,
  stripeSignatureHeader,
  zeroDecimalCurrency,
} from "./smoke-payment-stripe.mjs";

test("Stripe payment smoke config requires explicit Stripe credentials", () => {
  assert.throws(
    () => loadConfig({}),
    /STRIPE_SMOKE_SECRET_KEY is required/,
  );

  const config = loadConfig({
    SERVER_PORT: "18080",
    STRIPE_SMOKE_SECRET_KEY: "stripe-secret-for-test",
    STRIPE_SMOKE_WEBHOOK_SECRET: "stripe-webhook-for-test",
  });

  assert.equal(config.baseURL, "http://127.0.0.1:18080");
  assert.equal(config.adminEmail, "admin@srapi.local");
  assert.equal(config.adminPassword, "password123");
  assert.equal(config.amount, "1.00");
  assert.equal(config.currency, "USD");
  assert.equal(config.providerName, "stripe-smoke");
});

test("Stripe checkout completed event preserves order reconciliation fields", () => {
  const event = stripeCheckoutCompletedEvent({
    order_no: "pay_123",
    amount: "12.34",
    currency: "USD",
    metadata: {
      stripe_checkout_session_id: "cs_test_123",
    },
  });

  assert.equal(event.id, "evt_srapi_smoke_pay_123");
  assert.equal(event.type, "checkout.session.completed");
  assert.equal(event.data.object.id, "cs_test_123");
  assert.equal(event.data.object.client_reference_id, "pay_123");
  assert.equal(event.data.object.amount_total, 1234);
  assert.equal(event.data.object.currency, "usd");
  assert.deepEqual(event.data.object.metadata, { order_no: "pay_123" });
});

test("Stripe money helpers match decimal and zero-decimal currencies", () => {
  assert.equal(minorAmount("1.23", "USD"), 123);
  assert.equal(minorAmount("123", "JPY"), 123);
  assert.equal(zeroDecimalCurrency("jpy"), true);
  assert.equal(addMoney("1.00000000", "0.25"), "1.25000000");
});

test("Stripe signature header uses Stripe webhook timestamp and v1 HMAC shape", () => {
  const body = JSON.stringify({ id: "evt_test", type: "checkout.session.completed" });
  const secret = "stripe-webhook-for-test";
  const header = stripeSignatureHeader(body, secret);
  const match = header.match(/^t=([0-9]+),v1=([0-9a-f]{64})$/);

  assert.ok(match, `unexpected Stripe signature header: ${header}`);
  const expected = crypto
    .createHmac("sha256", secret)
    .update(`${match[1]}.${body}`)
    .digest("hex");
  assert.equal(match[2], expected);
});

test("Stripe payment smoke script keeps cleanup, idempotency, and checkout assertions", () => {
  const script = readFileSync("tools/smoke-payment-stripe.mjs", "utf8");

  assert.match(script, /finally\s*{\s*if \(provider\?\.id\) {\s*await disableStripeProvider/s);
  assert.match(script, /startsWith\("https:\/\/checkout\.stripe\.com\/"\)/);
  assert.match(script, /duplicate\.body\?\.data\?\.handled !== false/);
  assert.match(script, /fulfilled\.provider_transaction_id !== order\.metadata\.stripe_checkout_session_id/);
  assert.match(script, /balance did not increase by/);
});
