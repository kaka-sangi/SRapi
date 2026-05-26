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
  minorAmount,
  moneyParts,
  parseMoneyUnits,
  request,
  requiredEnv,
  zeroDecimalCurrency,
} from "./payment-smoke-common.mjs";

function loadConfig(env = process.env) {
  return {
    ...baseSmokeConfig(env),
    stripeSecretKey: requiredEnv(env, "STRIPE_SMOKE_SECRET_KEY"),
    stripeWebhookSecret: requiredEnv(env, "STRIPE_SMOKE_WEBHOOK_SECRET"),
    amount: env.STRIPE_SMOKE_AMOUNT || "1.00",
    currency: (env.STRIPE_SMOKE_CURRENCY || "USD").toUpperCase(),
    providerName: env.STRIPE_SMOKE_PROVIDER_NAME || "stripe-smoke",
  };
}

async function main(config = loadConfig()) {
  await assertHealth(config);
  const { cookie, csrfToken } = await loginAdmin(config);

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

function isMainModule() {
  const entrypoint = process.argv[1];
  return Boolean(entrypoint) && import.meta.url === pathToFileURL(entrypoint).href;
}

export {
  addMoney,
  loadConfig,
  minorAmount,
  moneyParts,
  parseMoneyUnits,
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
