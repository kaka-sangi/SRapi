const ZERO_DECIMAL_CURRENCIES = new Set([
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
]);

function baseSmokeConfig(env = process.env) {
  return {
    baseURL: env.SRAPI_BASE_URL || `http://127.0.0.1:${env.SERVER_PORT || "8080"}`,
    adminEmail: env.BOOTSTRAP_ADMIN_EMAIL || "admin@srapi.local",
    adminPassword: env.BOOTSTRAP_ADMIN_PASSWORD || "password123",
  };
}

async function assertHealth(config) {
  const health = await request(config, "GET", "/api/v1/health");
  if (!health.body?.request_id) {
    throw new Error("health response missing request_id");
  }
  return health;
}

async function loginAdmin(config) {
  const login = await request(config, "POST", "/api/v1/auth/login", {
    body: { email: config.adminEmail, password: config.adminPassword },
  });
  const cookie = sessionCookie(login.headers);
  const csrfToken = login.body?.data?.csrf_token;
  if (!cookie || !csrfToken) {
    throw new Error("login did not return csrf token and session cookie");
  }
  return { cookie, csrfToken };
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

async function findPaymentOrder(config, { cookie, orderNo }) {
  const response = await request(config, "GET", "/api/v1/payment/orders", { cookie });
  const orders = response.body?.data;
  if (!Array.isArray(orders)) {
    throw new Error("GET /api/v1/payment/orders did not return data array");
  }
  return orders.find((order) => order?.order_no === orderNo) || null;
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
  if (options.responseType === "text") {
    return { bodyText: text, headers: response.headers, status: response.status };
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

function minorAmount(value, code) {
  const decimal = moneyParts(value);
  const scale = zeroDecimalCurrency(code) ? 0 : 2;
  const fraction = decimal.fraction.padEnd(scale, "0").slice(0, scale);
  const normalized = `${decimal.whole}${fraction}`;
  return Number.parseInt(normalized, 10);
}

function money2(value) {
  const parts = moneyParts(value);
  const fraction = parts.fraction.padEnd(2, "0");
  if (/[^0]/.test(fraction.slice(2))) {
    throw new Error(`money value must have at most 2 decimal places: ${value}`);
  }
  return `${parts.whole}.${fraction.slice(0, 2)}`;
}

function zeroDecimalCurrency(code) {
  return ZERO_DECIMAL_CURRENCIES.has(String(code).toUpperCase());
}

export {
  addMoney,
  assertHealth,
  baseSmokeConfig,
  currentBalance,
  findPaymentOrder,
  findPaymentProvider,
  formatMoney,
  loginAdmin,
  minorAmount,
  money2,
  moneyParts,
  parseMoneyUnits,
  request,
  requiredEnv,
  sessionCookie,
  zeroDecimalCurrency,
};
