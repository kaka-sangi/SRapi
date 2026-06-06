# SRapi 产品基调与文案规范 (v0.1.0)

本文件定义 SRapi v0.1.0 起统一的产品定位、语气、文案规则与中英文术语表，是控制台、官网、API 错误信息、文档与营销文案的唯一基调来源。

`docs/FRONTEND_DESIGN_SYSTEM.md` 负责视觉，`docs/PRODUCT_TONE.md` 负责语气，两者共同决定 SRapi 的产品观感。

---

## 1. 一句话定位

**English** — *SRapi is a self-hosted AI gateway. One endpoint, every provider, your accounts, your control.*

**中文** — *SRapi 是一个自托管 AI 网关。一个入口，接入所有服务商，账号自管，调度可控。*

## 2. 价值描述（30 字版本）

- EN: *Route OpenAI, Anthropic, Gemini and CLI/web-session accounts through one OpenAI-compatible endpoint, with built-in scheduling, quotas and audit logs.*
- 中文：*通过一个 OpenAI 兼容的接口，接入 OpenAI、Anthropic、Gemini 以及 CLI / 反代账号，自带调度、配额与用量审计。*

## 3. 产品基调

SRapi 不是一个堆满酷炫词汇的 AI SaaS 落地页，而是一个**自托管运维台**：用户群体是工程师、平台团队、独立开发者。基调用三个词概括：

1. **冷静（Calm）** — 不夸张、不打鸡血、不堆叠 emoji 与 buzzword。
2. **直白（Plain）** — 用通用词替代行话；说"API 密钥"而不是"加密通道密钥"。
3. **可信（Trustworthy）** — 涉及凭据、计费、合规时给出明确边界与限制。

> 美学参照：Anthropic 的产品文档、Stripe 的开发者文案、Linear 的设置页。
> 反例：传统 SaaS 营销页（"赋能"、"降本增效"、"AI 驱动的下一代"）、过度学术化（"Operator"、"Cryptographic Vault"、"Lease"、"Epoch"）、机翻味浓的中文（"租约延迟"、"事务遥测审计"）。

---

## 4. 英文文案规则

| 规则 | 做 | 不做 |
| --- | --- | --- |
| 使用第二人称 | "Your API keys", "Sign in" | "Operator credentials", "Subject identification" |
| 用名词的本名 | "API key", "provider account", "request" | "channel", "credential vessel", "invocation" |
| 动词朴素 | "Sign in", "Create", "Test connection" | "Authenticate", "Deploy", "Verify link" |
| 状态精确 | "Active", "Disabled", "Rate limited" | "Operational", "Suspended", "Throttled tier" |
| 错误友善 | "Wrong email or password." | "Authentication rejected. Verify credentials." |
| 计费用普通词 | "Cost", "Tokens", "Spend" | "Yield", "Routed debit", "Transactional cost" |
| 调度术语保留 | "Scheduler", "Decision", "Latency" | "Lease scribe", "Epoch matrix", "Routing coeff" |
| 大小写克制 | 句首大写，专名大写 | 整段 UPPERCASE 强调；小标签除外 |

**保留的专业术语**（已写入 OpenAPI / 调度规范，不要改名）：scheduler, decision, candidate, provider, provider account, model, usage log, capability, runtime class, OAuth refresh, reverse proxy, rate limit, quota, SLO, burn rate.

## 5. 中文文案规则

| 规则 | 做 | 不做 |
| --- | --- | --- |
| 称谓 | 控制台用 "你"；正式邮件用 "您" | 控制台里突然冒出 "您"，与英文 "you" 不对齐 |
| 术语本名 | "API 密钥"、"上游账号"、"请求" | "通道"、"凭证保险库"、"调用" |
| 动词朴素 | "登录"、"新建"、"测试连接" | "身份验证"、"部署"、"校验链路" |
| 状态精确 | "启用"、"已停用"、"被限流" | "操作中"、"暂停态"、"流量受限态" |
| 计费 | "花费"、"Token"、"已用" | "事务成本"、"重定向令牌"、"借记额度" |
| 标点 | 中英文混排时数字与英文前后留半角空格 | 数字紧贴中文："$42.50USD" |
| 学术词慎用 | 用 "调度器"、"决策"、"候选" | "智能调度记录仪"、"租约决策证据" |

**英文术语处理**：保留不译的词包括：`API`、`OpenAI`、`Anthropic`、`Gemini`、`Token`、`SLO`、`OAuth`、`HMAC`、`SSE`、`WebSocket`、`Scheduler`（中文文档中可写"调度器"，UI 里两者皆可，但保持单页一致）。

---

## 6. 词表（English → English / 中文 → 中文）

下列对应表是 v0.1.0 控制台必须遵循的替换规则。当代码、文档、错误信息中出现"不做"列里的词，应在 v0.1.0 之前全部替换为"做"列。

### 6.1 身份与登录

| 不做 (旧) | 做 (新 EN) | 做 (新 中文) |
| --- | --- | --- |
| Operator Console | Admin | 管理后台 |
| Developer Console | Workspace | 工作台 |
| Verify Operator Credentials | Sign in to SRapi | 登录 SRapi |
| Operator Identity | Email | 邮箱 |
| Console Security Passphrase | Password | 密码 |
| Authenticate | Sign in | 登录 |
| Decrypting Session... | Signing in... | 登录中... |
| Authentication rejected. Verify credentials. | Wrong email or password. | 邮箱或密码不正确。 |
| Quick Test Environments (Local Console) | Demo accounts (local only) | 本地演示账号 |
| Admin Account / Developer Account | Sign in as admin / Sign in as developer | 以管理员身份登录 / 以开发者身份登录 |
| Terminate Session | Sign out | 退出登录 |
| Operator Name | Account | 账户 |

### 6.2 凭据与密钥

| 不做 | 做 (EN) | 做 (中文) |
| --- | --- | --- |
| Cryptographic Credentials Vault | API keys | API 密钥 |
| Token Registry & Gateway Authorizations | Manage your API keys | 管理 API 密钥 |
| Channel / API Key Channel | API key | API 密钥 |
| Generate Channel Key / Deploy Channel | Create API key | 新建 API 密钥 |
| Active API Channels | Your API keys | 你的 API 密钥 |
| Querying tenant key registry... | Loading API keys... | 加载中... |
| Plaintext Secret Key Generated | API key created | 已创建 API 密钥 |
| Copy Plaintext Key | Copy key | 复制密钥 |
| Key Identifier | Name | 名称 |
| Prefix Value | Prefix | 前缀 |
| Allowed Target Models | Allowed models | 可用模型 |
| Scope Account Groups (CSV) | Account groups (comma-separated) | 账号组（用逗号分隔） |
| Revoke / Toggle | Disable / Enable | 停用 / 启用 |

### 6.3 用量与计费

| 不做 | 做 (EN) | 做 (中文) |
| --- | --- | --- |
| Transactional Telemetry Auditing | Usage | 用量 |
| SLA Invocations Ledger & Audit Evidence | Request logs | 请求日志 |
| Audited Traffic / Invocations Evaluated | Requests | 请求总数 |
| Router SLA / Routing Success Coeff | Success rate | 成功率 |
| Payload Routed / Total Integrated Tokens | Total tokens | 累计 Token |
| Financial Cost / Estimated Debit / Yield Cost | Cost | 总花费 |
| Source Path / Source Endpoint | Endpoint | 接入点 |
| Rerouted Tokens | Tokens | Token |
| Transactional Cost | Cost | 花费 |
| 200 OK Only / System Errors Only | Successful / Failed | 成功 / 失败 |
| All Model Scopes / All Response States | All models / All statuses | 全部模型 / 全部状态 |
| Showing {filtered} of {total} events | {filtered} / {total} requests | {filtered} / {total} 条 |
| Fetching audit evidence... | Loading requests... | 加载中... |
| No Matching Traffic Found | No requests match | 没有匹配的请求 |

### 6.4 上游账号

| 不做 | 做 (EN) | 做 (中文) |
| --- | --- | --- |
| Upstream Adapter Mappings | Provider accounts | 上游账号 |
| Large Language Model Credentials Pool | Manage upstream provider accounts | 管理上游账号 |
| JSON Schema Specifications / Operator Declarations | Configuration example | 配置示例 |
| Provision Schema (.json) | Example accounts JSON | 账号配置示例 (JSON) |
| Write-Only Cryptographic Guarantee | Credentials are write-only | 凭据只写存储，无法回查 |
| Verify Link / Verifying... | Test connection / Testing... | 测试连接 / 测试中... |
| Verified | OK | 正常 |
| Rejected (401 Auth) | Failed (401) | 失败（401） |
| Class | Type | 类型 |
| Proxy Endpoint | Base URL | Base URL |
| Scope Maps | Models | 模型 |
| Lease Latency (Avg) | Avg latency | 平均延迟 |
| Quota Remainder | Quota left | 剩余配额 |
| Resolving active upstream accounts... | Loading provider accounts... | 加载上游账号... |

### 6.5 调度与决策

| 不做 | 做 (EN) | 做 (中文) |
| --- | --- | --- |
| Adaptive Dispatch Interface & Architectural Control | Gateway overview | 网关总览 |
| Real-Time Decision Registry & Fallback Evidence | Live scheduling decisions | 实时调度决策 |
| Dynamic Scheduling Diagnostics | Scheduler decisions | 调度决策 |
| Dispatcher Scribe & Simulator | Routing decisions | 调度决策 |
| Configure Dispatch Simulation | Decision filters | 决策筛选 |
| Execute Dispatch Pipeline | Refresh decisions | 刷新调度决策 |
| Initiate / Pause Live Simulation Feed | Live data / Paused | 实时数据 / 已暂停 |
| Awaiting routing instructions... | Send real traffic to record decisions. | 发出真实请求以记录调度决策。 |
| Dispatch Trace Log | Trace log | 调度日志 |
| Epoch Index: Live | Live | 实时 |
| Lease Candidates Scores | Candidate scores | 候选评分 |
| Failover / Excluded Candidates | Rejected candidates | 被排除的候选 |
| Scheduler Engine Reasoning Logs | Scheduler logs | 调度器日志 |
| Leased Upstream | Selected | 选中上游 |
| Routed | Dispatched | 已分发 |
| Accessing lease logs... | Loading decisions... | 加载调度决策... |

### 6.6 控制台与导航

| 不做 | 做 (EN) | 做 (中文) |
| --- | --- | --- |
| Specification Portal | SRapi | SRapi |
| v0.1 Core Studio | v0.1.0 | v0.1.0 |
| Live API / Demo Data | Live / API offline | 实时数据 / API 离线 |
| Smoke Evidence: Complete / not complete | Self-check: passing / not passing | 自检：通过 / 未通过 |
| Constraints Matrix | Checks | 检查项 |
| Healthy Traffic Registry | Real traffic recorded | 已记录真实流量 |
| Upstream Scheduler Routing | Scheduler routed real upstreams | 调度器已分发到真实上游 |
| Console Diagnostic Instructions | Next steps | 下一步 |
| Synthesizing developer metrics... / Decrypting control plane telemetry... | Loading... | 加载中... |
| Operator CLI Diagnostic Reference | CLI quick reference | CLI 快速参考 |

### 6.7 调度决策输出

调度决策输出的每一行不再使用 `[INFO]` `[SCHEDULER]` `[EVALUATE]` `[FILTER]` `[RESOLVED]` 这种工业控制器风格的全大写标签；改为可读的单词或一致的小写 tag：

```text
[1/5] request received  id=req_xxx model=claude-3-7-sonnet
[2/5] scheduler         capability ok, candidates=3
[3/5] candidates        scoring 3 accounts
[4/5] excluded          openai-pro-02 cooldown not expired
[5/5] selected          claude-sonnet-01  score=0.94
                        - health 1.00 (w 0.3) -> 0.300
                        - quota  0.85 (w 0.2) -> 0.170
                        - cache  0.90 (w 0.1) -> 0.090
                        - sticky 1.00 (w 0.1) -> 0.100
                        - cost   0.92 (w 0.1) -> 0.092
```

中文版用相同结构：`请求受理 / 调度器 / 候选评分 / 已排除 / 已选中`。

---

## 7. 错误信息样板

错误信息须满足三段式：**发生了什么 → 为什么 → 你可以怎么办**。

| 场景 | EN | 中文 |
| --- | --- | --- |
| 邮箱密码错误 | Wrong email or password. Try again. | 邮箱或密码不正确，请重试。 |
| 缺字段 | Email and password are required. | 请输入邮箱和密码。 |
| 上游 401 | Provider rejected the credential (401). Update the API key in the provider account. | 上游凭据被拒（401），请在该上游账号下更新 API 密钥。 |
| 上游限流 | Provider is rate limited. SRapi will retry with another account. | 上游被限流，SRapi 会自动尝试其他账号。 |
| 没有候选 | No provider account can serve this model right now. | 当前没有可用上游账号能服务该模型。 |
| 自检未通过 | SRapi has not yet recorded real traffic on the gateway. Send a request to verify your setup. | SRapi 还没有记录到真实网关流量，先发一个请求来验证配置。 |

---

## 8. 落地与持续治理

本文件是产品基调的唯一来源；下面的去行话 / 词表 / 状态文案统一过滤已落地，并作为持续约束保留。

**已落地（基线）**

- [x] 本文件作为产品基调的单一来源
- [x] 所有用户可见文案集中在 `apps/web/src/i18n/messages/`（`en.ts` / `zh.ts`）与 `apps/web/src/context/LanguageContext.tsx`，整体符合本文 §4 / §5 规则
- [x] §6 词表替换已应用到全站文案，旧的行话词条（Operator Console / Channel / Vault / Telemetry 等）不再出现在 UI
- [x] 后端状态枚举统一经 `apps/web/src/lib/status-badge.ts`（`statusLabel` + `quietStatusFor`）渲染，配合 i18n 的 `status` 命名空间；徽章与表格不再直接暴露原始枚举（`needs_reauth` / `suspended` / `monitor` 等）
- [x] OpenAPI 错误样板与 README 简介与本文 §1 一句话定位对齐

> 文案不再枚举到单个页面文件。早期的 `components/DashboardLayout.tsx` 已被 `components/layout/app-shell.tsx` / `admin-shell.tsx` 取代，页面集也已远超最初的 7 个；治理对象是**全部用户可见文案**，而非某个固定页面清单。

**持续约束（每次改动）**

- [ ] 任何新增或修改的用户可见文案，先经过本文 §4 / §5 / §6 校对再合入
- [ ] 新出现的后端状态 / 模式枚举要在 i18n 的 `status` 命名空间补齐译名，并经 `statusLabel` 渲染，不得把原始 token 直接显示
- [ ] 新增 / 改动的 API 错误信息满足 §7 三段式

> 任何 PR 引入新的用户可见文案时，应在 PR 描述里链接到本文件并声明对应词条。
