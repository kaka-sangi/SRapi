import type {
  CreateTlsProfileRequest,
  TlsProfile,
} from "../../../../packages/sdk/typescript/src/types.gen";

// Mirrors supportedTLSTemplates in the backend tls_profiles service (alias
// duplicates like android_okhttp_11 / none omitted). Kept in sync so the form
// only ever offers a value egress will accept.
export const TLS_TEMPLATES = [
  "default",
  "chrome",
  "chrome_auto",
  "chrome_120",
  "chrome_133",
  "firefox",
  "firefox_auto",
  "firefox_120",
  "safari",
  "safari_auto",
  "safari_16",
  "safari_16_0",
  "ios",
  "ios_auto",
  "ios_14",
  "android_11_okhttp",
  "randomized",
  "randomized_alpn",
  "randomized_no_alpn",
];

// Mirrors supportedHTTPVersionPolicies (the require_h2 family the egress layer
// rejects is intentionally excluded).
export const HTTP_VERSION_POLICIES = ["auto", "prefer_h2", "prefer_h1", "require_h1"];

export interface TlsProfileFormState {
  name: string;
  tls_template: string;
  http_version_policy: string;
  user_agent: string;
  enabled: boolean;
  extra_headers: Record<string, string>;
}

export function emptyTlsProfileForm(): TlsProfileFormState {
  return {
    name: "",
    tls_template: "chrome_auto",
    http_version_policy: "prefer_h2",
    user_agent: "",
    enabled: true,
    extra_headers: {},
  };
}

export function tlsProfileFormFromProfile(profile: TlsProfile): TlsProfileFormState {
  return {
    name: profile.name,
    tls_template: profile.tls_template || "default",
    http_version_policy: profile.http_version_policy || "prefer_h2",
    user_agent: profile.user_agent ?? "",
    enabled: profile.enabled,
    extra_headers: profile.extra_headers ?? {},
  };
}

export function buildTlsProfileBody(form: TlsProfileFormState): CreateTlsProfileRequest {
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  return {
    name,
    tls_template: form.tls_template,
    http_version_policy: form.http_version_policy,
    user_agent: form.user_agent.trim(),
    extra_headers: form.extra_headers,
    enabled: form.enabled,
  };
}
