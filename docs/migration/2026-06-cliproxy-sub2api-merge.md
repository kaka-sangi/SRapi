# sub2api / CLIProxyAPI → SRapi 功能迁移计划

> 起源：用户要求把 `/home/senran/Desktop/sub2api` 与 `/home/senran/Desktop/CLIProxyAPI` 中真正有用的功能特化迁移到 SRapi。
> 截稿日：2026-06-21。

## 1. 背景与判断

通过对三个仓库的并行扫描，得到以下事实：

- **SRapi 远比初判成熟**：46 个后端 modules、53 张 PostgreSQL 迁移、70+ 个 Next.js 页面，已覆盖 15 个上游 provider（OpenAI / Anthropic / Grok / Gemini / Kimi / Qwen / Zhipu / DeepSeek / Mistral / Groq / Together / OpenRouter / Antigravity / ChatGPT Web / Bedrock）、6 个控制台 OAuth（GitHub / Google / OIDC / LinuxDo / WeChat / DingTalk）、4 个支付（Stripe / Alipay / WeChat / EasyPay）、TLS 指纹、思考预算归一化、Sticky 会话、Codex CLI / Realtime WS、`count_tokens`、`/v1/responses` 与 `responses/compact` 等。
- **sub2api 与 SRapi 同类**（同样是 AI 网关，**不是订阅转换器**），核心引擎重合度极高，大部分能力 SRapi 已有更工程化的实现。
- **CLIProxyAPI 与 SRapi 的 reverse-proxy 模块重合**，conductor / selector / 翻译注册表对应 SRapi 的 scheduler + `provider_adapters/translator`。

因此**真正可迁移的增量很小但精准**。无脑全量"复刻"是错误工程判断；这里只挑选 SRapi 实际缺失、且能带来明确价值的能力。

## 2. 排除项（不做，并说明理由）

| 候选 | 来源 | 决策 | 理由 |
|------|------|------|------|
| 指数退避冷却 | CLIProxyAPI | **不做** | SRapi 现"尊重 upstream retry-after + 5 次/10min 滑动窗口升级"更精确；指数退避会无视 upstream Retry-After 头，反而过度限流。 |
| 翻译器注册表重写 | CLIProxyAPI | **不做** | SRapi 已有 `provider_adapters/translator/registry.go`，结构等价。 |
| Round-robin / Fill-first Selector | CLIProxyAPI | **不做** | SRapi `scheduler` 已是多因子打分（health/quota/latency/cache affinity/cost/concurrency），优于纯轮询。 |
| Plugin C ABI | CLIProxyAPI | **不做** | 非 SRapi 工程风格；如需扩展点用 Go interface + DI。 |
| TUI / bubbletea | CLIProxyAPI | **不做** | SRapi 已有完整 Next.js 控制台。 |
| Git/S3 凭据存储 | CLIProxyAPI | **不做** | SRapi 用 Ent 加密落库（`credential_crypto.go`）。 |
| 更多 OAuth provider | sub2api | **不做** | 已有 6 个；缺失的 Microsoft/Apple/Discord 不在用户当前主市场。 |
| 更多支付 provider | sub2api | **不做** | Stripe + Alipay + WeChat + EasyPay 已覆盖主市场；Airwallex / PayPal 留作未来按需扩展。 |
| 视频端点（/v1/videos） | CLIProxyAPI | **不做** | SRapi Grok preset 已支持 `KeyVideos`；OpenAI 端不主推 Sora。 |
| channel_monitors 请求模板 | sub2api | **已有，取消** | `apps/api/internal/modules/channel_monitors/contract/contract.go` 已实现 `Template/CreateTemplate/UpdateTemplate`。 |

## 3. 迁移面（执行清单）

### 3.1 Vertex AI provider（来源：CLIProxyAPI `/internal/auth/vertex/`）

**为什么需要**：SRapi 当前仅支持 Google AI Studio (`generativelanguage.googleapis.com`)，缺 GCP Vertex AI (`aiplatform.googleapis.com`)。企业客户普遍只能用 Vertex（合规、配额、Service Account 模型）。

**做法**：

- 新增 `PlatformFamilyVertexCompatible` 与 `vertexPreset()` in `providers/preset/registry.go`。
- 端点：`https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/publishers/google/models/{model}:generateContent`。
- 新增 `RuntimeClassServiceAccountJSON`（accounts 模块）；元数据：`project_id`、`region`、`service_account_json`（落库前加密 + key 规整）。
- 服务账号密钥规整工具从 `internal/auth/vertex/vertex_credentials.go` 移植到 `apps/api/internal/modules/accounts/service/vertex_keys.go`，处理 PKCS#1/PKCS#8/PEM 头损坏等真实问题。
- Adapter：复用 `PlatformFamilyGeminiCompatible` 的 Gemini 翻译器，请求层注入 Bearer = OAuth2 access token（用 service-account JWT 兑换）。
- 模型 catalog：`gemini-2.5-pro`、`gemini-2.5-flash`、`gemini-3-pro-preview`、`gemini-3-pro-high/low` 等。
- Admin UI：accounts 表单新增 Vertex 配置组（project / region / 上传 service-account JSON）。

### 3.2 冷却升级为 (account, model) 双键（来源：CLIProxyAPI conductor）

**为什么需要**：当前 `rate_limit_cooldown` 以 `accountID` 为键，账号 A 在 `gemini-2.5-pro` 上 429 会屏蔽其 `gemini-2.5-flash` 调用，浪费配额。

**做法**：

- 改 `rate_limit_cooldown/service/service.go`：key 由 `int64` 改为 `struct{ AccountID int64; Model string }`；`model == ""` 表示"全模型"级（兼容现网行为）。
- `RecordRateLimitHit(accountID, model, retryAfter)` 与 `IsAccountInCooldown(accountID, model)` 同时考虑两级 entry。
- 调用方：gateway 入口在记录 429 时传入解析后的 `canonical_model`。scheduler 候选过滤同样按模型查询。
- 测试：覆盖"同账号不同模型互不影响"+"全账号级冷却同时阻止所有模型"。

### 3.3 Gemini/Vertex 配额耗尽时切换 project（来源：CLIProxyAPI `quota-exceeded: switch-project`）

**为什么需要**：Gemini OAuth / Vertex 上一个 Google 账号通常绑定多个 GCP project；单 project 配额耗尽是常见错误，自动切换可避免对客户失败。

**做法**：

- accounts 表 metadata 新字段 `project_ids: []string`（首项为 primary）。
- 新增 `accounts/service` 内 `RotateProject(accountID)` 接口：游标推进到下一 project_id。
- gateway 上当 upstream 返回 `RESOURCE_EXHAUSTED` / `quotaExceeded` 时调用 `RotateProject`，并把当前 (account, project) 加入 short cooldown（30 min 默认）。
- 仅对 `gemini` / `vertex` / `antigravity` provider 生效。
- Admin UI：accounts 编辑页 project_ids 列表，可拖拽排序、即时移除。

### 3.4 Codex reasoning 重放缓存（来源：CLIProxyAPI `cache/codex_reasoning_replay_cache.go`）

**为什么需要**：Codex 多轮对话中，前一轮的 reasoning 摘要会在下一轮重复发送，浪费上行带宽与上游 token 计费。

**做法**：

- 新模块 `apps/api/internal/modules/reasoning_cache/`（合并到 gateway 也可；新模块边界更清晰）。
- 实现：`Put(sessionID, reasoningHash, summary)` / `Get(sessionID, reasoningHash)`，存储用 process-local LRU + Redis 旁路（TTL 30min）。
- gateway codex pipeline 在出站前替换重复 reasoning 块为引用 hash；上游若需要完整内容，仍按需展开。
- 命中率埋点接入 `/admin/ops/diagnostics`。

### 3.5 OpenAI Moderation API 接入（来源：sub2api `service/content_moderation.go`）

**为什么需要**：SRapi 现 `content_safety` 只有本地正则（PII redaction / prompt-injection keyword）。OpenAI Moderation 提供分类（hate / harassment / sexual / self-harm / violence）+ 分数，是 enterprise 客户的合规要求。

**做法**：

- contract 新增 `ModerationProvider` 接口（`Classify(ctx, texts) → []ClassificationResult`），实现 `openai_moderation`（用户配置 API key 调 `https://api.openai.com/v1/moderations`）。
- 落库结果缓存（hash → result，TTL 1h）避免重复请求。
- 配置：`enabled`、`mode: shadow|enforce`、`thresholds: map[category]float64`、`api_key_ref`、`cache_ttl`。
- 与现有正则规则联动：先正则（PII redact），再 moderation 分类。
- Admin UI：`/admin/risk-control` 增加 Moderation tab；显示最近 24h 触发分布。

### 3.6 维护模式（来源：sub2api `backend_mode_enabled`）

**为什么需要**：发布、迁库、上游事故时，需要一键阻止普通用户登录与 `/v1/*` 请求，只放行 admin。当前完全没有。

**做法**：

- settings 表新键 `maintenance: { enabled: bool, message: string, allow_admin: bool, expected_recovery_at: timestamptz }`。
- 中间件 `internal/httpserver/middleware/maintenance.go`：
  - `/api/v1/auth/login` 非 admin 返回 503 + JSON。
  - `/v1/*` 网关全部返回 503 + 公告 JSON。
  - `/api/v1/admin/*` 与 `/api/v1/auth/admin-*` 放行。
- Admin 设置页新增"维护"块（toggle + 文案 + 计划恢复时间）。
- 前端：未登录顶部公告条 + 登录页 banner（命中 503 时展示原因）。

### 3.7 关联 ROOTCAUSE 修复（顺手做）

迁移过程中触碰到的高严重项，必须在同 PR 内修而非掩盖：

- Codex import 丢弃 `Priority/Weight/RiskLevel`（与 3.1 邻近）。
- 音频端点未应用 preset.base_url（与 3.1 同 adapter 体系）。
- `gateway/channel_filter` 在 settings transient error 时吞掉所有候选（与 3.6 维护模式互斥）。
- `quick-setup` provider 无 `openai-compatible` 时静默回退（与 3.1 新增 Vertex 路径相关）。

### 3.8 前端整理（信息密度提升）

不"乱加按钮"，而是把稀疏页拉齐到密集页的信息密度：

- `/admin/announcements`：列表 + 行内编辑/发布；当前几乎只有富文本编辑器。
- `/admin/logs` ↔ `/admin/ops/system-logs`：合并为单页，左侧 facet 过滤，右侧表+详情，去掉重复。
- `/admin/gateway-policies` + `/admin/gateway-resources`：合并为"网关运行参数"单页，多 tab。
- `/available-channels`：补容量/延迟/最近错误率三列。
- `/account`：增加 OAuth 绑定状态、2FA 状态、API key 总览三块；当前只是单 form。
- 为 3.1 / 3.5 / 3.6 新功能加最小必要入口：Vertex 在 accounts 表单；moderation 在 risk-control tab；maintenance 在 settings 既有页内 block。**不**新开顶层菜单。

## 4. 执行顺序与提交节奏

| 阶段 | 内容 | 预期 commit |
|------|------|-------------|
| 1 | 文档落地（本文件） + 任务清单 | 1 |
| 2 | 维护模式（独立 + 最小风险） | 1 |
| 3 | OpenAI Moderation 适配（独立模块） | 1 |
| 4 | per-(account, model) 冷却（局部改造） | 1 |
| 5 | Vertex AI provider（核心新增） | 1-2 |
| 6 | 项目切换（依赖 Vertex） | 1 |
| 7 | Codex reasoning 缓存 | 1 |
| 8 | 前端密度整理 | 1-2 |
| 9 | 顺手 ROOTCAUSE 合并 | 散落 |

每阶段：`make check` / 关键测试 / `git commit` / `git push`。

## 5. 不会承诺的事

- 这次会话不保证全部完成；超出能力的阶段会在 task list 留 in-progress，下次会话续。
- 不会引入 backward-compat 的兼容层——直接调整 API/Schema；同一次 commit 内修齐全链路。
