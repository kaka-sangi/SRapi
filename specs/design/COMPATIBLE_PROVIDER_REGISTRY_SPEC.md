# SRapi 兼容 Provider 注册表规范

## 1. 目标

本文档定义 OpenAI-compatible、Anthropic-compatible 以及其他兼容 Provider preset 的注册、路由别名、默认 base URL、认证模式、模型目录和账号类型边界。

Provider preset 的目标是减少硬编码分叉：

```txt
路径别名 + 默认配置 + 能力声明 + 账号校验 = Provider Preset
```

## 2. 核心概念

| 概念 | 说明 |
| --- | --- |
| `provider_key` | 内部稳定标识，如 `openai-compatible`、`deepseek`。 |
| `platform_family` | 兼容协议族，如 `openai_compatible`、`anthropic_compatible`、`rerank_compatible`。 |
| `route_aliases` | 可强制绑定该 preset 的路径前缀。 |
| `default_base_url` | 默认上游地址。 |
| `auth_modes` | 支持的认证方式。 |
| `model_catalog_owner` | 模型目录来源。 |
| `account_type_allowlist` | 允许的账号类型。 |

`generic-reverse-proxy` 是配置驱动的 OpenAI-compatible adapter。它读取 provider 或 account metadata 中的
`base_url`、`auth_header_template`、`body_mapping_rules` 和 `response_path_rules`，用于没有专用 adapter
但仍能用 OpenAI-compatible 请求/响应形态接入的上游。`runtime_class=api_key` 走普通 HTTP client；
其他 runtime class 走 Reverse Proxy Runtime。

## 3. OpenAI-compatible preset

内置 OpenAI-compatible preset（`internal/modules/providers/preset/registry.go`，均已随注册表发布）：

| provider_key | 默认用途 | default_base_url |
| --- | --- | --- |
| `openai-compatible` | 通用自定义 OpenAI-compatible 上游 | `https://api.openai.com/v1` |
| `openai` | 官方 API Key 上游 | `https://api.openai.com/v1` |
| `groq` | Groq OpenAI-compatible | `https://api.groq.com/openai/v1` |
| `cerebras` | Cerebras OpenAI-compatible | `https://api.cerebras.ai/v1` |
| `deepseek` | DeepSeek Chat / Reasoner | `https://api.deepseek.com` |
| `grok` | xAI Grok OpenAI-compatible | `https://api.x.ai/v1` |
| `moonshot` | Moonshot / Kimi | `https://api.moonshot.ai/v1` |
| `kimi` | Kimi route alias preset | `https://api.moonshot.ai/v1` |
| `mistral` | Mistral OpenAI-compatible | `https://api.mistral.ai/v1` |
| `openrouter` | OpenRouter | `https://openrouter.ai/api/v1` |
| `anyrouter` | AnyRouter | `https://anyrouter.dev/api/v1` |
| `qwen` | 通义千问 / DashScope OpenAI-compatible | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| `together` | Together AI OpenAI-compatible | `https://api.together.ai/v1` |
| `zhipu` | GLM / Zhipu | `https://open.bigmodel.cn/api/paas/v4` |
| `zai` | Z.AI / GLM | `https://api.z.ai/api/paas/v4` |

允许的 runtime class（`RuntimeClassAllowlist`，第三方 preset 取默认集合）：

```txt
api_key
custom_reverse_proxy
```

`openai` 一级 preset 仅暴露官方 API Key 与 `custom_reverse_proxy`。ChatGPT Web session
cookie 和 Codex CLI OAuth 分别由 `chatgpt-web`、`codex-cli` 专用 preset 承载；OpenAI /
Gemini OAuth account runtime 在 provider adapter 中显式返回 `not_supported`。
第三方 OpenAI-compatible preset 不得进入 ChatGPT Web、Codex CLI 等专用反代 runtime。

## 4. Anthropic-compatible preset

内置 Anthropic-compatible preset（均已随注册表发布）：

| provider_key | 默认用途 | default_base_url |
| --- | --- | --- |
| `anthropic-compatible` | 通用 Anthropic Messages-compatible 上游 | `https://api.anthropic.com/v1` |
| `anthropic` | Anthropic official API Key / OAuth runtime | `https://api.anthropic.com/v1` |
| `deepseek-anthropic` | DeepSeek Anthropic-compatible | `https://api.deepseek.com/anthropic` |
| `moonshot-anthropic` | Moonshot/Kimi Anthropic-compatible | `https://api.moonshot.ai/anthropic` |
| `zhipu-anthropic` | Zhipu Anthropic-compatible | `https://open.bigmodel.cn/api/anthropic` |
| `zai-anthropic` | Z.AI Anthropic-compatible | `https://api.z.ai/api/anthropic` |

`anthropic` 一级 preset 额外放开 Claude Code / Claude CLI runtime（`oauth_refresh`、`oauth_device_code`、`cli_client_token`）。`service_account_json` 在 Vertex/GCP SA 接入前不得出现在默认 allowlist。
第三方 Anthropic-compatible preset 不得进入 Claude Web / Claude Code OAuth mimicry runtime。`claude-compatible` 只能作为历史兼容 route alias（注册在 `anthropic-compatible` preset 下），不能作为新的 adapter_type 或 provider.protocol。

## 5. Rerank-compatible preset

内置 rerank preset（已发布）：

| provider_key | 默认用途 | default_base_url |
| --- | --- | --- |
| `rerank-compatible` | 通用 rerank 上游，兼容 Cohere/Jina 风格 `query` + `documents` 请求 | `https://api.cohere.com/v2` |

Rerank-compatible preset 只注册 rerank 路由别名，不自动暴露 chat、responses、images 或 moderation route。

## 6. Antigravity reverse-proxy preset

内置（platform_family `reverse_proxy_antigravity`）：

| provider_key | 默认用途 |
| --- | --- |
| `antigravity` | Antigravity desktop/IDE reverse-proxy text alias |

`antigravity` 是 reverse-proxy 客户端身份 preset，不是新的文本协议。它必须通过
`provider.protocol` 选择目标上游协议：

```txt
openai-compatible    -> /chat/completions
anthropic-compatible -> /messages
gemini-compatible    -> models/{model}:generateContent
```

registry 为 Antigravity 暴露 text alias：

```txt
/antigravity/v1
/api/provider/antigravity
/api/provider/antigravity/v1
```

并暴露 Gemini model-action alias（`GeminiRouteAliases`）：

```txt
/antigravity/v1beta
/api/provider/antigravity/v1beta
```

允许的 runtime class：

```txt
oauth_refresh
custom_reverse_proxy
```

`desktop_client_token` / `ide_plugin_token` 仅作为 legacy runtime enum 保留，默认 preset 已并入
`oauth_refresh`。`antigravity` preset 不提供通用 `default_base_url`；管理员必须在
Provider Account metadata 中配置实际 `base_url`。Gemini model-action aliases 只复用标准 Gemini Gateway
handler，并保留 alias source endpoint 作为 usage log 与 scheduler decision 证据。

## 6a. ChatGPT Web reverse-proxy preset

内置（platform_family `openai_compatible`，adapter_type `reverse-proxy-chatgpt-web`）：

| provider_key | 默认用途 | default_base_url |
| --- | --- | --- |
| `chatgpt-web` | ChatGPT Web session/cookie reverse proxy | `https://chatgpt.com` |

允许的 runtime class 为 `web_session_cookie` 与 `custom_reverse_proxy`。route alias：
`/chatgpt-web/v1`、`/api/provider/chatgpt-web`、`/api/provider/chatgpt-web/v1`。

## 6b. Bedrock Anthropic preset

内置（platform_family `bedrock_anthropic`）：

| provider_key | 默认用途 | default_base_url |
| --- | --- | --- |
| `bedrock` | Amazon Bedrock 上的 Anthropic Messages 上游 | `https://bedrock-runtime.us-east-1.amazonaws.com` |

`bedrock` 复用 Anthropic capability 集合，auth mode 为 `custom_header`（SigV4 header），允许的
runtime class 为 `api_key`。route alias：`/bedrock/v1`、
`/api/provider/bedrock`、`/api/provider/bedrock/v1`。

## 7. Preset Schema

```yaml
provider_key: deepseek
platform_family: openai_compatible
display_name: DeepSeek
route_aliases:
  - /api/provider/deepseek
  - /api/provider/deepseek/v1
default_base_url: https://api.deepseek.com
auth_modes:
  - bearer
account_type_allowlist:
  - api_key
  - upstream
model_catalog_owner: deepseek
capabilities:
  chat_completions: true
  responses: false
  messages: false
  embeddings: false
  stream: true
```

内置 registry 当前覆盖以下 key（`preset.Default()`，权威清单见
`internal/modules/providers/preset/registry.go`）：

```txt
anthropic
anthropic-compatible
antigravity
anyrouter
bedrock
cerebras
deepseek
deepseek-anthropic
grok
groq
kimi
mistral
moonshot
moonshot-anthropic
openai
openai-compatible
openrouter
qwen
rerank-compatible
together
zai
zai-anthropic
zhipu
zhipu-anthropic
```

`deepseek` 的 OpenAI-compatible 默认 base URL 为 `https://api.deepseek.com`；`deepseek-anthropic` 为 `https://api.deepseek.com/anthropic`。除 `antigravity` 这类必须由账号 metadata 指向实际客户端/反代上游的 reverse-proxy identity preset 外，其他 preset 必须同样保留显式 `default_base_url`，避免 Gateway 运行时按 provider 名称硬编码。

## 8. Auth Modes

| auth mode | Header |
| --- | --- |
| `bearer` | `Authorization: Bearer <token>` |
| `x_api_key` | `x-api-key: <token>` |
| `api_key_query` | `?key=<token>` |
| `custom_header` | 管理员配置 header name。 |

凭证值必须只存加密密文。

## 9. Route Alias 规则

Provider alias 只改变 platform context，不新增 runtime。

示例：

```txt
/api/provider/deepseek/v1/chat/completions
```

等价于：

```txt
/v1/chat/completions + forced_provider=deepseek
```

Handler 不得复制 Provider-specific 转发逻辑。

Provider alias 进入 Scheduler 前必须先应用 API Key policy，包括 `allowed_models` 与 `group_ids`。当 API Key 绑定了 `group_ids` 时，候选账号必须属于至少一个绑定的 account group；未绑定 group 的账号不得被 alias 路径调度。

## 9a. 模型目录优先级

模型目录来源优先级：

1. 管理员显式配置的 Provider Model Mapping。
2. Account `supported_models`。
3. Preset 内建模型目录。
4. 上游 live discovery。
5. 通用 fallback catalog。

`/v1/models` 必须按 API Key group、provider、model visibility 合并后返回。
`POST /api/v1/admin/accounts/{id}/discover-models` 把 live discovery 结果写入 Account `supported_models`，供后续 Provider-neutral 候选选择使用。`reverse-proxy-antigravity` 的 live discovery 通过 Reverse Proxy Runtime 使用选中账号凭证访问 `{base_url}/v1internal:fetchAvailableModels`，不是 API-key discovery。缺少 project metadata 的 Antigravity discovery 会先通过同一 selected-account runtime 请求 `{base_url}/v1internal:loadCodeAssist`，必要时请求 `{base_url}/v1internal:onboardUser`，再做模型发现；仅 `persist=true` 写回 project metadata。（实现见 `internal/httpserver/model_discovery.go`、`model_discovery_antigravity.go`。）

## 10. Upstream Endpoint Derivation

Provider Adapter 根据 preset 派生：

```txt
chat_completions_url = base_url + /chat/completions
responses_url        = base_url + /responses
responses_compact_url = base_url + /responses/compact
messages_url         = base_url + /messages
models_url           = base_url + /models
embeddings_url       = base_url + /embeddings
audio_speech_url     = base_url + /audio/speech
rerank_url           = base_url + /rerank
```

如果上游路径不兼容，preset 必须提供 explicit endpoint override。

## 11. Capability Override

能力来源按优先级合并：

```txt
account override > model_provider_mapping override > provider preset > family default
```

能力字段包括：

```txt
chat_completions
responses
responses_compact
messages
generate_content
embeddings
images
audio
audio_transcriptions
audio_speech
rerank
tools
structured_output
vision
reasoning
prompt_cache
stream
max_context_tokens
max_output_tokens
```

## 12. 新增 Preset 流程

1. 添加 preset 定义。
2. 添加 route_aliases。
3. 添加 auth mode 校验。
4. 添加默认 model catalog。
5. 添加 provider capability。
6. 添加 account validation。
7. 添加 `/v1/models` 可见性测试。
8. 添加 text request contract test。
9. 添加 stream test。
10. 更新 `GATEWAY_ROUTE_MATRIX.md`。

## 13. 安全边界

禁止：

- Compatible preset 复用反代账号 cookie / OAuth token。
- 在 preset 中硬编码 secret。
- 通过 provider alias 绕过 API Key group 权限。
- 将 upstream 详细错误直接泄漏给客户端。

## 13a. 与 Provider Adapter 的关系

Provider Adapter 实现协议转换和上游调用；Compatible Provider Registry 只提供配置、路由、能力和校验元数据。

Adapter 不得把 deepseek、groq、openrouter 等 preset 写成独立分叉，除非该 Provider 存在无法由 preset 表达的协议差异。
