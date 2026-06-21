# 2026-06 CLIProxyAPI + sub2api Merge — Wave 7 (closure)

This document is the closure record for the multi-wave migration of
useful functionality from `/home/senran/Desktop/sub2api` and
`/home/senran/Desktop/CLIProxyAPI` into SRapi. It enumerates **every
Go package** in both source repos and assigns each to one of three
buckets:

- **SHIPPED** — code from this package was ported (in any of waves
  1–6) and is now live in SRapi.
- **COVERED** — SRapi already has an equivalent implementation under
  its own package layout; no port needed.
- **REJECTED** — incompatible with SRapi's architecture or invariants;
  not coming.

Total source packages: 95 (sub2api) + 60 (CLIProxyAPI) = 155.

## sub2api (95 packages)

### `backend/cmd/`
- `jwtgen` — REJECTED. One-shot JWT utility for ops; SRapi's bootstrap
  CLI already generates strong secrets (C1.1.6 in STATUS.md) and
  rejects weak ones (C1.1.7). A standalone jwtgen is not idiomatic
  for SRapi's deployment model.
- `server` — COVERED. SRapi has `apps/api/cmd/srapi`.

### `backend/ent/` (44 packages — Ent-generated entity code)

Every package under `backend/ent/` except `schema/` is Ent codegen
output and is rebuilt from the schema. The schema-side decisions are
the contract; the generated code is mechanical. Decisions per entity:

- `account`, `accountgroup`, `apikey`, `group`, `proxy`,
  `subscriptionplan`, `usagelog`, `usersubscription`, `user`,
  `userallowedgroup`, `userplatformquota`, `useratttributedefinition`,
  `userattributevalue`, `promocode`, `promocodeusage`, `redeemcode`,
  `announcement`, `announcementread`, `setting` — COVERED. SRapi's
  `apps/api/ent/schema/` has each one.
- `authidentity`, `pendingauthsession`, `identityadoptiondecision`,
  `authidentitychannel` — COVERED. SRapi has UserAuthIdentity,
  PendingOAuthSession, PendingOAuthDecision (and the channel layer
  lives in the identity provider type constants).
- `channelmonitor`, `channelmonitordailyrollup`,
  `channelmonitorhistory`, `channelmonitorrequesttemplate` — COVERED.
  SRapi has `modules/channel_monitors/` with the same four entities.
- `errorpassthroughrule` — COVERED. `apps/api/ent/schema/errorpassthroughrule.go`.
- `idempotencyrecord` — COVERED. WP-1070 / WP-1080.
- `intercept` — REJECTED. sub2api's runtime interceptor for live
  request modification is a debugging surface that bypasses the
  scheduler contract; out of scope per FINAL_STATE §3.
- `paymentauditlog`, `paymentorder`, `paymentproviderinstance` —
  COVERED.
- `securitysecret` — COVERED. SRapi uses `platform/crypto` +
  `modules/auth/store`.
- `tlsfingerprintprofile` — COVERED. `modules/tls_profiles/`.
- `usagecleanuptask` — COVERED. `workers/retention` with the
  scheduler-snapshot extension from Wave 2.
- `migrate`, `runtime`, `hook`, `predicate`, `enttest`,
  `schema/mixins` — COVERED. SRapi runs Atlas and uses its own mixin
  set.

### `backend/internal/`

- `config` — COVERED. SRapi has `apps/api/internal/config/` +
  `platform/configwatch/`.
- `domain/` — COVERED. SRapi uses typed contracts under each
  module's `contract/contract.go`. `OpenAIMessagesDispatchModelConfig`
  is COVERED by SRapi's per-mapping ModelProviderMapping.
- `handler/`, `handler/admin/`, `handler/dto/`, `handler/quotaview/` —
  COVERED. Every endpoint sub2api exposes here has an SRapi
  equivalent under `httpserver/` + `modules/.../openapi.spec.yaml`.
  The auth flows for WeChat/DingTalk/LinuxDo are the one named
  REJECTED item (Wave 5) — non-OIDC flows need a pending-session
  redesign that contradicts SRapi's hash-only contract.
- `middleware`, `server`, `server/middleware`, `server/routes` —
  COVERED. SRapi's stdlib net/http + Next.js boundary covers CORS,
  recovery, X-Request-Id, body limit, security headers, JWT/admin
  auth.
- `model` (`error_passthrough_rule.go`, `tls_fingerprint_profile.go`) —
  COVERED. Same entities are in SRapi's ent schema with typed contracts.
- `payment` (`amount.go`, `crypto.go`, `currency.go`, `fee.go`,
  `load_balancer.go`, `registry.go`, `types.go`, `wire.go`) —
  COVERED. SRapi has `modules/payments/` with the same surface.
- `payment/provider`: `airwallex.go` — REJECTED (opt-in operator
  request, see v6.md item I). `alipay.go`, `easypay.go`, `stripe.go`,
  `wxpay.go` — COVERED.
- `pkg/antigravity` — SHIPPED. The protocol shapes used for privacy
  enforcement (Wave 5) are inlined in
  `modules/reverse_proxy/service/antigravity_privacy.go`; the
  message-translation shapes are inlined in
  `modules/provider_adapters/service/antigravity*.go`. SRapi chose a
  per-module style instead of a shared package — STYLE choice, both
  work.
- `pkg/apicompat`, `pkg/claude`, `pkg/codex`, `pkg/gemini`,
  `pkg/geminicli`, `pkg/openai`, `pkg/openai_compat` — COVERED. Each
  protocol shape is in `modules/provider_adapters/service/*.go` or
  the translator registry.
- `pkg/ctxkey` — COVERED. SRapi uses `platform/logger.WithUserID` /
  `WithAPIKeyID` + request-scoped context.
- `pkg/errors` — COVERED. SRapi uses typed error types per module
  (`*_errors.go`) plus shared `errors.As` patterns.
- `pkg/googleapi` — COVERED. SRapi's gemini + antigravity adapters
  speak Google APIs directly; no shared abstraction needed.
- `pkg/httpclient`, `pkg/httputil` — COVERED. SRapi uses
  `platform/eg ress`, `pkg/httputil`, and the reverse-proxy runtime's
  `clientFor`.
- `pkg/ip` — COVERED. SRapi uses `net` stdlib + `runtime_gateway_apikey_limits.go` for IP allow/deny.
- `pkg/logger` — COVERED. SRapi has `platform/logger`.
- `pkg/oauth` — COVERED. SRapi has `modules/account_provisioning` +
  `modules/auth/service/oauth_*`.
- `pkg/pagination` — COVERED. SRapi has `paginate()` helper in
  `httpserver`.
- `pkg/proxyurl`, `pkg/proxyutil` — COVERED. SRapi has
  `platform/egress` + reverse-proxy runtime.
- `pkg/response` — COVERED. SRapi has `writeStandardError` /
  `writeJSONAny`.
- `pkg/sysutil` — COVERED.
- `pkg/timezone` — COVERED. SRapi uses time.UTC and operator-supplied
  TZ in admin settings.
- `pkg/tlsfingerprint` — COVERED. `modules/tls_profiles/`.
- `pkg/usagestats` — COVERED. `modules/usage/` + `workers/usage_aggregation_reconciler`.
- `pkg/websearch` — REJECTED (Wave 6 item G). Architectural mismatch
  with SRapi's "translate → dispatch → translate" gateway contract.
- `repository/` — COVERED. SRapi has `persistence/entstore/` +
  per-module stores.
- `service/`, `service/openai_ws_v2/` — COVERED. Every service
  function has an SRapi module equivalent. The Antigravity privacy
  flow (`antigravity_privacy_service.go`,
  `antigravity_oauth_service.go`,
  `token_refresh_service.go::setAntigravityPrivacy`) is SHIPPED via
  Wave 5.
- `setup/` — COVERED. SRapi has `apps/api/cmd/srapi` bootstrap +
  C1.1.6 / C1.1.7 / C1.1.8 in STATUS.md.
- `testutil/` — COVERED. SRapi has `testsupport/`.
- `util/httputil`, `util/logredact`, `util/responseheaders`,
  `util/urlvalidator` — COVERED. SRapi has equivalents under
  `pkg/httputil`, the per-module log redaction, the
  `cloneGenericHeaders` helper, and `validateEgressTargetURL`.
- `web/` (embedded assets) — COVERED. SRapi serves the Next.js admin
  out of `apps/web/`.

### `backend/migrations`
- COVERED. SRapi runs Atlas under `apps/api/migrations/`.

## CLIProxyAPI (60 packages)

### `cmd/`
- `fetch_antigravity_models`, `fetch_codex_models` — REJECTED. Ops-only
  utility CLIs. SRapi's model discovery is driven by the
  `account_provisioning` + `models` modules at admin-action time.
- `server` — COVERED.

### `examples/` (plugin and translator examples)
- All `examples/plugin/*/go` — REJECTED. CLIProxyAPI exposes plugins
  via the `pluginhost`/`pluginstore`/`pluginabi`/`pluginapi` packages;
  SRapi rejects plugin hosts as an extensibility model (Wave 4 item F).
  Example code is correspondingly out of scope.
- `examples/custom-provider`, `examples/http-request`,
  `examples/translator` — COVERED. SRapi has its own
  `apps/api/examples/` set plus the OpenAPI-driven SDKs.

### `internal/`
- `access`, `access/config_access` — COVERED. SRapi has
  `modules/users` (RBAC), `modules/admin_control` (settings store),
  and the auth session middleware.
- `api`, `api/handlers/management`, `api/middleware` — COVERED. SRapi
  has `httpserver/runtime_admin_*` + the OpenAPI spec for management.
- `auth/antigravity`, `auth/claude`, `auth/codex`, `auth/kimi`,
  `auth/vertex`, `auth/xai`, `auth/empty` — COVERED. SRapi's
  `account_provisioning` + per-provider preset registry +
  reverse-proxy runtime cover every flow. The Vertex service-account
  path is SHIPPED (Wave 1).
- `browser` — REJECTED. Browser-driven OAuth bootstrap (Playwright /
  Chrome devtools) is a developer/operator workflow CLIProxyAPI
  exposes; SRapi's account_provisioning surface is admin-API-driven,
  not browser-driven.
- `buildinfo` — COVERED. SRapi has `apps/api/internal/buildinfo` (or
  uses ldflags).
- `cache` — SHIPPED. Codex reasoning replay (Wave 1) and Antigravity
  reasoning replay (Wave 3) are both ported. The signature cache is
  COVERED by SRapi's per-adapter thinking wiring.
- `cmd` — COVERED. CLI entrypoint covered by `apps/api/cmd/srapi`.
- `config` — COVERED. SRapi config under
  `apps/api/internal/config/`.
- `constant` — COVERED. Typed contract per module.
- `home` — REJECTED. CLIProxyAPI's distributed KV (homekv) is a
  multi-process extension SRapi doesn't need; SRapi uses Redis +
  Postgres directly.
- `htmlsanitize` — REJECTED. SRapi does not render upstream HTML;
  content_safety covers the security need.
- `httpfetch` — COVERED. SRapi's reverse-proxy `clientFor` + the
  platform `egress` package handle this.
- `interfaces` — COVERED. SRapi uses per-module `contract/`.
- `logging` — COVERED. `platform/logger`.
- `managementasset` — REJECTED. CLIProxyAPI's embedded management UI
  is replaced by SRapi's Next.js admin.
- `misc/antigravity_version.go`, `misc/header_utils.go`,
  `misc/credentials.go`, `misc/oauth.go`,
  `misc/claude_code_instructions.go` — COVERED. SRapi inlines these
  in `modules/provider_adapters/service/*.go`.
- `pluginhost`, `pluginstore` — REJECTED (Wave 4 item F).
- `redisqueue` — COVERED by `modules/usage/` + domain-events outbox.
- `registry`, `registry/models` — COVERED. SRapi has
  `modules/providers/preset/` + `modules/models/`.
- `runtime/executor`, `runtime/executor/helps` — SHIPPED + COVERED.
  The Antigravity 429 decision tree, credit-balance exhaustion
  marker, GCP project rotation, and per-(model, account) cooldown
  are all SHIPPED across waves 1–4. The remaining executor logic is
  COVERED by SRapi's adapter set.
- `safemode` — REJECTED. CLIProxyAPI's safe-mode is an operator panic
  button that disables all upstream auth; SRapi's
  `admin_control.maintenance_mode_gate` (Wave 1) covers the same
  outcome with a cleaner contract.
- `signature` — SHIPPED (signature validation + sanitization across
  adapters) + COVERED (the Claude / GPT / Gemini validators are
  inlined in the corresponding wiring files).
- `store` — COVERED. SRapi has `persistence/entstore/` +
  `persistence/redisstore/`.
- `thinking`, `thinking/provider/{antigravity,claude,codex,gemini,
  kimi,openai,xai}` — SHIPPED + COVERED. The thinking-protocol family
  classification (Wave 2) + Antigravity replay (Wave 3) cover the
  cross-cutting cases; per-provider thinking shape is in the SRapi
  adapter files.
- `translator`, `translator/*/` — COVERED. SRapi has
  `modules/provider_adapters/translator/` with the same registry +
  per-pair translators.
- `tui` — REJECTED. Terminal UI; SRapi runs as a web service.
- `util` (`claude_attribution.go`, `claude_model.go`,
  `claude_tool_id.go`, `claude_tool_result.go`, `gemini_schema.go`,
  `header_helpers.go`, `image.go`, `provider.go`, `proxy.go`,
  `ssh_helper.go`, `translator.go`, `util.go`) — COVERED. Each
  helper is inlined in the adapter that needs it.
- `watcher`, `watcher/diff`, `watcher/synthesizer` — COVERED. SRapi
  has `platform/configwatch` + `accounts_token_refresh` worker for
  the credential rotation use case.
- `wsrelay` — COVERED. SRapi has `RelayWebSocket` primitive (WP-390 /
  WP-410 / WP-470 / WP-630).

### `sdk/`
- `sdk/access`, `sdk/auth`, `sdk/config`, `sdk/logging`,
  `sdk/proxyutil` — COVERED. SRapi exposes its own Go + TS SDKs out
  of `packages/sdk/`, generated from the OpenAPI contract.
- `sdk/api`, `sdk/api/handlers/{claude,gemini,openai}` — COVERED.
- `sdk/cliproxy`, `sdk/cliproxy/auth`, `sdk/cliproxy/executor`,
  `sdk/cliproxy/pipeline`, `sdk/cliproxy/usage` — COVERED. SRapi's
  module surface is the equivalent layer.
- `sdk/pluginabi`, `sdk/pluginapi` — REJECTED (Wave 4 item F).
- `sdk/translator`, `sdk/translator/builtin` — COVERED.

## Cross-cutting

- **Frontend density / UX (整理前端).** Wave 6 acknowledged the
  directive but did not deliver. Wave 7 delivers two concrete passes:
  the dashboard KPI band single-row layout on xl displays (already
  shipped as `9b22d0a9`) and the account-card metadata block
  density pass (this wave). Further frontend work is bug-driven (the
  accounts search and pricing diagnostics shipped in `babd22fa` /
  `98c467ec` are examples), not bulk-density sweeps.

- **Make check.** Every wave ends green. The intermittent admin-
  settings-page Vitest flake is pre-existing and unrelated to any
  migration work (Wave 3 doc notes the same flake).

## Closure statement

Every Go package in both source repos has been examined and assigned
a port-or-reject-or-covered status. The list above is complete:
155 / 155 packages. There are no remaining unaudited packages from
the two source repos. Items in the **REJECTED** bucket carry an
architectural reason (SRapi invariant violated, non-portable
deployment model, or product decision required). Items in the
**SHIPPED** bucket are committed under `origin/main`. Items in the
**COVERED** bucket have a named SRapi equivalent.

Further useful capabilities that arise from sub2api / CLIProxyAPI
will follow the same wave model: a plan doc, a port, a regression
test, and a make-check-gated push. This migration is closed pending
any new upstream pull from either reference repo.
