import assert from "node:assert/strict";
import { test } from "node:test";
import { buildChildEnv, resolveApiURL } from "./web-check-e2e.mjs";

test("resolveApiURL prefers the explicit e2e API target", () => {
  const apiURL = resolveApiURL({
    SRAPI_WEB_E2E_API_URL: "https://api.example.test/",
    SRAPI_API_PROXY_TARGET: "http://127.0.0.1:8080",
  });

  assert.equal(apiURL, "https://api.example.test");
});

test("buildChildEnv sends the e2e API target to the Next proxy by default", () => {
  const childEnv = buildChildEnv(
    { NEXT_PUBLIC_SRAPI_BASE_URL: "https://stale-browser-api.example.test" },
    "https://api.example.test",
  );

  assert.equal(childEnv.SRAPI_API_PROXY_TARGET, "https://api.example.test");
  assert.equal(childEnv.NEXT_PUBLIC_SRAPI_BASE_URL, undefined);
});

test("buildChildEnv can opt into a direct browser SDK API target", () => {
  const childEnv = buildChildEnv({}, "https://api.example.test", { directBrowserAPI: true });

  assert.equal(childEnv.SRAPI_API_PROXY_TARGET, "https://api.example.test");
  assert.equal(childEnv.NEXT_PUBLIC_SRAPI_BASE_URL, "https://api.example.test");
});
