# SRapi 2api 迁移指南

本文面向已经使用 `/home/senran/Desktop/sub2api`、`/home/senran/Desktop/CLIProxyAPI`、`/home/senran/Desktop/chatgpt2api` 这类 2api / 反代项目的部署者。SRapi 迁移目标不是把这些项目的本地进程接进来，而是把相同的账号池、模型映射、请求模拟和兼容 API 能力沉到 SRapi 的 Provider Account、Scheduler、Provider Adapter 与 Reverse Proxy Runtime 边界里。

## 1. 术语边界

在 SRapi 中，反代 / 2api 的定义只有一种：

```txt
SRapi Gateway compatible endpoint
  -> Canonical AI Request / policy / entitlement
  -> Scheduler selects Provider Account
  -> Provider Adapter builds official-client upstream shape
  -> Reverse Proxy Runtime sends request with selected account credential
  -> real upstream official API / web / CLI / IDE endpoint
```

这里的 selected account credential 指 SRapi Provider Account 中配置并加密保存的 OAuth、session、desktop、CLI 或 IDE credential。客户端只拿 SRapi API Key 调用 `/v1/*`、`/v1beta/*` 等兼容接口。

明确不属于 SRapi 反代：

- 不是把本地 Codex / Claude Code / Antigravity 客户端作为 SRapi 的下游入口。
- 不是在 Gateway service 为 Codex / Claude Code / Antigravity 增加本地 DTO。
- 不是把调用方传来的 `Authorization`、`Cookie` 或本地 CLI token 透传给上游。
- 不是绕过 Scheduler 直接按 provider 名称选账号。

权威定义见 `docs/2API_REVERSE_PROXY_DEFINITION.md` 和 `docs/REVERSE_PROXY_SPEC.md`。

## 2. 概念映射

| 既有 2api 项目概念 | SRapi 对应概念 |
| --- | --- |
| account pool / token pool | Provider Account + Account Group |
| upstream provider type | Provider `adapter_type`、`protocol`、`runtime_class` |
| OAuth / session / desktop / CLI token | Provider Account encrypted credential |
| model alias / route alias | Model Registry + Model Mapping + Provider preset aliases |
| sticky conversation / session id | Scheduler `session_affinity_key`、`sticky_strength`、`sticky_account_id` |
| quota / cooldown / ban signal | Account health, quota snapshot, cooldown, circuit state, Scheduler feedback |
| OpenAI-compatible output | Gateway response renderer for Chat Completions, Responses, Messages, Gemini, etc. |
| anti-feature headers / client fingerprint | Provider Adapter official-client shape + Reverse Proxy Runtime egress profile |

`sub2api`、`CLIProxyAPI`、`chatgpt2api` 可以作为语义参考：它们证明 2api 的核心是“用账号材料模拟官方客户端请求真实上游，再渲染兼容 API”。SRapi 仍保留自己的模块边界，不复制它们的 Gateway service 结构。

## 3. 迁移顺序

1. 创建 Provider 或选择 preset：为 OpenAI-compatible、Anthropic-compatible、Gemini-compatible、Codex CLI、Claude Code CLI、ChatGPT Web、Antigravity 等上游建立 Provider。
2. 建立 Model Registry：用 SRapi canonical model 名称承载客户端可见模型，例如 `gpt-4o-mini`。
3. 建立 Model Mapping：把 canonical model 映射到每个 Provider Account 的 upstream model、route family 和 capability。
4. 导入 Provider Account：通过管理 API 或控制台导入 OAuth/session/desktop/CLI/IDE credential。导入接口是写入型控制面，不要把这些凭证放进客户端示例、SDK 示例、日志或环境变量。
5. 绑定 Account Group：把 Gateway API Key 可用的账号池绑定到合适的 group，让 Scheduler 能按模型、能力、健康、额度和粘度选择账号。
6. 验证账号：用 account test / discovery 能力确认 selected Provider Account 可以访问真实上游，必要时让 Antigravity discovery 走 `loadCodeAssist` / `onboardUser` / `fetchAvailableModels`。
7. 切客户端：把原来指向 2api 服务的 base URL 改成 SRapi，继续使用 OpenAI / Anthropic / Gemini 兼容路径。
8. 观察证据链：检查 Scheduler decision、usage log、account health、quota snapshot、audit log 和 AdminOps realtime slot。

## 4. 兼容调用面

客户端迁移优先使用这些稳定入口：

- OpenAI-compatible: `GET /v1/models`、`POST /v1/chat/completions`、`POST /v1/responses`
- Anthropic-compatible: `POST /v1/messages`、`POST /v1/messages/count_tokens`
- Gemini-compatible: `GET /v1beta/models`、`POST /v1beta/models/{model}:countTokens`
- Realtime / WebSocket diagnostics: `GET /api/v1/admin/ops/realtime/slots`

示例见 `examples/README.md`。示例只展示 `SRAPI_BASE_URL`、`SRAPI_API_KEY`、可选 `SRAPI_ADMIN_SESSION` 和 `SRAPI_CSRF_TOKEN`，不会展示真实上游 credential。

## 5. Codex / Claude Code / Antigravity 注意点

Codex CLI、Claude Code CLI 和 Antigravity 的本地安装可以用于理解官方客户端行为、抓取请求形状或做人工对照分析，但 SRapi 的运行时接入方式仍是：

- Provider Adapter 构造目标官方客户端 endpoint、header、body、stream 或 WSS shape。
- Reverse Proxy Runtime 使用选中 Provider Account 的 OAuth/session/desktop/CLI/IDE credential 请求真实上游。
- Gateway 继续只处理兼容 API、Canonical AI Request、policy、entitlement 和 Scheduler 证据。

Codex CLI 和 Claude Code CLI 账号已经支持 sub2api 风格的 refresh-token-only 导入：创建、导入或更新 `runtime_class=oauth_refresh` 且 `upstream_client=codex_cli` / `claude_code_cli` 的 Provider Account 时，可以只提交 credential 中的 `refresh_token`。SRapi 会先通过 Reverse Proxy Runtime 按对应官方客户端 OAuth token endpoint、client ID 和请求体形态换取 `access_token`，再加密保存完整 OAuth 状态；管理响应、导出、audit、usage 和 Scheduler evidence 不返回 token 明文。账号 metadata 仍应配置 upstream `base_url`，可选配置 `user_agent`、`proxy_id` 和测试用或私有部署用的 `oauth_token_url` / `oauth_client_id` 覆盖值。

Antigravity 账号也支持 refresh-token-only 导入：创建、导入或更新 `runtime_class=oauth_refresh`、`upstream_client=antigravity_desktop`、`adapter_type=reverse-proxy-antigravity` 的 Provider Account 时，可以只提交 `refresh_token`，但必须在加密 credential 中提供 `oauth_client_secret` / `client_secret`，SRapi 不硬编码 Google OAuth client secret。Antigravity 账号 metadata 仍需配置 Cloud Code `base_url` 和 `project_id` / `antigravity_project_id` / `cloudaicompanion_project`，项目 bootstrap 仍通过已有 discovery 流程处理。

因此新增这些 provider 能力时，应把 upstream-specific shape 放在 Provider Adapter / runtime adapter 中，把凭证注入、header hygiene、cookie jar、proxy egress 和 WSS relay 放在 Reverse Proxy Runtime 中。Gateway service 不新增 Codex / Claude Code / Antigravity 本地 DTO。

## 6. 安全与合规

- 客户端只使用 SRapi API Key；上游账号材料只存在于 SRapi Provider Account credential。
- 日志、audit、usage、scheduler decision 和 AdminOps 响应不得包含 OAuth/session/cookie/token 明文。
- 调用方的 `Authorization`、`Cookie`、`X-SRapi-*`、`X-Gateway-*`、`X-Forwarded-*`、`Via` 等 header 不得进入上游身份。
- 反代运行时可能与目标上游服务条款冲突；部署者自行承担账号、地区、网络出口和自动化调用风险。
