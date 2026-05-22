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

## 3. OpenAI-compatible preset

第一阶段建议内置：

| provider_key | 默认用途 | 阶段 |
| --- | --- | --- |
| `openai-compatible` | 通用自定义 OpenAI-compatible 上游 | MVP |
| `openai` | 官方 API Key 或官方兼容 API | MVP |
| `groq` | Groq OpenAI-compatible | MVP preset |
| `cerebras` | Cerebras OpenAI-compatible | MVP preset |
| `deepseek` | DeepSeek Chat / Reasoner | MVP preset |
| `grok` | xAI Grok OpenAI-compatible | MVP preset |
| `moonshot` | Moonshot / Kimi | MVP preset |
| `kimi` | Kimi route alias preset | MVP preset |
| `openrouter` | OpenRouter | MVP preset |
| `anyrouter` | AnyRouter | MVP preset |
| `zhipu` | GLM / Zhipu | MVP preset |
| `zai` | Z.AI / GLM | MVP preset |

允许账号类型：

```txt
api_key
upstream
custom_reverse_proxy
```

不得进入 ChatGPT Web OAuth、Codex CLI 等 `openai` 反代专用 runtime。

## 4. Anthropic-compatible preset

第一阶段建议内置：

| provider_key | 默认用途 | 阶段 |
| --- | --- | --- |
| `anthropic-compatible` | 通用 Anthropic Messages-compatible 上游 | MVP |
| `anthropic` | Anthropic official API Key / OAuth runtime | MVP |
| `deepseek-anthropic` | DeepSeek Anthropic-compatible | MVP preset |
| `moonshot-anthropic` | Moonshot/Kimi Anthropic-compatible | MVP preset |
| `zhipu-anthropic` | Zhipu Anthropic-compatible | MVP preset |
| `zai-anthropic` | Z.AI Anthropic-compatible | MVP preset |

Anthropic-compatible preset 不得进入 Claude Web / Claude Code OAuth mimicry runtime。`claude-compatible` 只能作为历史兼容 route alias，不能作为新的 adapter_type 或 provider.protocol。

## 5. Rerank-compatible preset

WP-320 起建议内置：

| provider_key | 默认用途 | 阶段 |
| --- | --- | --- |
| `rerank-compatible` | 通用 rerank 上游，兼容 Cohere/Jina 风格 `query` + `documents` 请求 | WP-320 |

Rerank-compatible preset 只注册 rerank 路由别名，不自动暴露 chat、responses、images 或 moderation route。

## 6. Antigravity reverse-proxy preset

WP-360 起内置：

| provider_key | 默认用途 | 阶段 |
| --- | --- | --- |
| `antigravity` | Antigravity desktop/IDE reverse-proxy text alias | WP-360 |

`antigravity` 是 reverse-proxy 客户端身份 preset，不是新的文本协议。它必须通过
`provider.protocol` 选择目标上游协议：

```txt
openai-compatible    -> /chat/completions
anthropic-compatible -> /messages
gemini-compatible    -> models/{model}:generateContent
```

当前 registry 为 Antigravity 暴露 text alias：

```txt
/antigravity/v1
/api/provider/antigravity
/api/provider/antigravity/v1
```

WP-370 起还暴露 Gemini model-action alias：

```txt
/antigravity/v1beta
/api/provider/antigravity/v1beta
```

允许账号类型：

```txt
desktop_client_token
ide_plugin_token
custom_reverse_proxy
```

`antigravity` preset 不提供通用 `default_base_url`；管理员必须在 Provider Account
metadata 中配置实际 `base_url`。Gemini model-action aliases 只复用标准 Gemini Gateway
handler，并保留 alias source endpoint 作为 usage log 与 scheduler decision 证据。

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

当前内置 registry 至少必须覆盖以下 key：

```txt
anthropic
anthropic-compatible
antigravity
anyrouter
cerebras
deepseek
deepseek-anthropic
grok
groq
kimi
moonshot
moonshot-anthropic
openai
openai-compatible
openrouter
rerank-compatible
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

## 9. 模型目录优先级

模型目录来源优先级：

1. 管理员显式配置的 Provider Model Mapping。
2. Account `supported_models`。
3. Preset 内建模型目录。
4. 上游 live discovery。
5. 通用 fallback catalog。

`/v1/models` 必须按 API Key group、provider、model visibility 合并后返回。
`POST /api/v1/admin/accounts/{id}/discover-models` 可以把 live discovery 结果写入 Account `supported_models`，供后续 Provider-neutral 候选选择使用。WP-500 起，`reverse-proxy-antigravity` 的 live discovery 通过 Reverse Proxy Runtime 使用选中账号凭证访问 `{base_url}/v1internal:fetchAvailableModels`，不是 API-key discovery。

## 10. Upstream Endpoint Derivation

Provider Adapter 根据 preset 派生：

```txt
chat_completions_url = base_url + /chat/completions
responses_url        = base_url + /responses
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

## 13. 与 Provider Adapter 的关系

Provider Adapter 实现协议转换和上游调用；Compatible Provider Registry 只提供配置、路由、能力和校验元数据。

Adapter 不得把 deepseek、groq、openrouter 等 preset 写成独立分叉，除非该 Provider 存在无法由 preset 表达的协议差异。
