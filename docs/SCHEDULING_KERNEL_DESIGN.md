# SRapi Scheduling Kernel 调度内核专项设计

## 1. 设计目标

SRapi Scheduling Kernel 是 SRapi 的核心能力。它不是简单的负载均衡器，而是一个面向 AI Provider、账号池、模型能力、用户等级、额度、缓存、成本和体验的多目标调度系统。

核心目标：

```txt
在保证用户体验的同时，尽可能降低调用成本、保护账号额度、提升缓存命中、降低失败率，并让账号池长期稳定运行。
```

## 2. 核心问题

AI Gateway 的调度与传统 API Gateway 不同，主要难点包括：

- 每个账号有不同额度、并发、风控风险和可用性。
- 不同模型价格差异巨大。
- 不同 Provider 的缓存能力、错误模式和限流策略不同。
- 长上下文请求如果命中缓存，成本可能显著下降。
- 会话粘度能提升体验，但也可能导致单账号过载。
- 低价用户和高价用户需要不同服务质量。
- 部分上游账号可能随时失效、限流、封禁或需要重新授权。

所以调度内核必须同时考虑：

```txt
Account Quota
Account Health
User Plan
Model Capability
Provider Cost
Session Stickiness
Cache Affinity
Latency
Fairness
Risk
```

## 3. 调度总流程

```txt
Client Endpoint Request
  ↓
Client Endpoint Adapter
  ↓
Canonical AI Request
  ↓
RequestClassifier
  ↓
CapabilityResolver
  ↓
CandidateBuilder
  ↓
PolicyFilter
  ↓
ScoreEngine
  ↓
LeaseManager
  ↓
ProviderAdapter Dispatch
  ↓
FeedbackCollector
  ↓
Health / Quota / Usage / Cache / Sticky Update
```

## 4. 内核组件

### 4.1 RequestClassifier

负责识别请求特征。

RequestClassifier 的输入必须来自 Canonical AI Request，不得直接依赖 `/v1/chat/completions`、`/v1/responses` 或 `/v1/messages` 的源端点字段。

输入：

- 用户 ID
- API Key
- 用户套餐
- 请求模型
- 输入 token 估算
- 是否流式
- 是否工具调用
- 是否视觉请求
- 是否长上下文
- conversation id
- session id
- priority

输出：

```txt
RequestProfile
```

字段建议：

```txt
request_id
user_id
api_key_id
model
model_family
estimated_input_tokens
estimated_output_tokens
is_stream
is_long_context
requires_vision
requires_tools
requires_json
conversation_hash
session_hash
priority
strategy_hint
```

### 4.2 CapabilityResolver

负责找出可以处理该请求的模型和 Provider。

处理逻辑：

- 模型别名解析。
- RequestCapability 提取。
- ModelCapability 匹配。
- ProviderCapability 支持检查。
- EffectiveCapability 计算。
- 用户组权限检查。
- fallback model 候选生成。

能力 key、版本、状态、降级和 matching 规则以 `CAPABILITY_TAXONOMY_SPEC.md` 为准。

示例：

```txt
用户请求 gpt-4o
  ↓
解析到 internal model: openai:gpt-4o
  ↓
候选 Provider：openai、openai-compatible、openrouter
```

### 4.3 CandidateBuilder

负责构建候选账号池。

候选账号必须满足：

- Provider 匹配。
- EffectiveCapability 匹配 RequestCapability。
- 账号启用。
- 账号属于可用分组。
- 用户或 API Key 有权限。

输出：

```txt
[]AccountCandidate
```

### 4.4 PolicyFilter

硬性过滤，不满足条件直接排除。

过滤项：

- 账号禁用。
- Token 失效。
- 模型不支持。
- 账号额度耗尽。
- 用户余额不足。
- 用户订阅过期。
- API Key 禁用。
- 并发已满。
- RPM/TPM 已满。
- Circuit breaker open。
- Cooldown 未结束。
- 代理不可用。
- 地区策略不匹配。

输出：

```txt
FilteredCandidates
RejectedCandidatesWithReasons
```

### 4.5 ScoreEngine

负责多目标打分。

基础公式：

```txt
score =
  health_score       * W_health
+ quota_score        * W_quota
+ latency_score      * W_latency
+ sticky_score       * W_sticky
+ cache_score        * W_cache
+ cost_score         * W_cost
+ fairness_score     * W_fairness
+ priority_score     * W_priority
- risk_penalty
- saturation_penalty
```

所有分数建议归一化到 `0.0 - 1.0`。

最终选择先在 Cost / Latency / Quality 三目标上筛出 Pareto 前沿，再在前沿内按策略加权分排序。缺少明确输入的目标不得参与 Pareto 支配判断，避免把默认分数当成真实运行信号。前沿之外的候选仍保留在候选排序证据中，供故障转移和审计使用。

### 4.6 LeaseManager

选中账号后创建短期租约。

目的：

- 防止并发竞争。
- 预占并发额度。
- 预估 token 额度。
- 失败时可释放。
- 成功时提交实际用量。

Lease 字段：

```txt
lease_id
request_id
account_id
user_id
api_key_id
model
estimated_input_tokens
estimated_output_tokens
estimated_cost
expires_at
status
```

状态：

```txt
pending
committed
released
expired
failed
```

### 4.7 ProviderAdapter Dispatch

调度内核只选账号，不直接关心 Provider 协议细节。

Provider Adapter 负责：

- 请求转换。
- Header 注入。
- 上游调用。
- 流式响应转发。
- 错误分类。
- usage 解析。
- cache usage 解析。

### 4.8 FeedbackCollector

每次请求完成后必须回流。

成功反馈：

```txt
account_id
provider_id
model
latency_ms
input_tokens
output_tokens
cached_tokens
cost
finish_reason
status_code
```

失败反馈：

```txt
account_id
provider_id
model
error_class
status_code
latency_ms
retryable
should_cooldown
should_disable
```

## 5. 账号健康模型

每个账号维护健康状态。

状态：

```txt
healthy
warmup
degraded
rate_limited
cooling_down
suspended
dead
```

健康分计算：

```txt
health_score =
  success_rate_score * 0.40
+ latency_score      * 0.20
+ error_score        * 0.20
+ circuit_score      * 0.10
+ freshness_score    * 0.10
```

### 5.1 错误分类

错误必须分类，不同错误产生不同影响。

```txt
rate_limit
quota_exceeded
auth_failed
provider_5xx
network_error
timeout
model_unavailable
content_policy
invalid_request
unknown
```

处理建议：

- `rate_limit`：短期冷却，降低权重。
- `quota_exceeded`：额度归零，排除。
- `auth_failed`：账号禁用或标记需重新授权。
- `provider_5xx`：Provider 级降权。
- `network_error`：代理或出口降权。
- `timeout`：降低健康分。
- `invalid_request`：不惩罚账号。
- `content_policy`：不惩罚账号。

## 6. 额度模型

额度不只看是否可用，还要看水位。

建议分层：

```txt
remaining_ratio >= 70%     normal
30% - 70%                  reduced_weight
10% - 30%                  protected
< 10%                      emergency_only
0%                         excluded
```

额度分：

```txt
quota_score = f(remaining_ratio, reset_time, account_tier, request_priority)
```

策略：

- 高价值账号不要被低价值请求耗尽。
- 即将重置的账号可适当提高权重。
- 低价套餐用户不应优先使用稀缺高质量账号。
- 高级用户可在必要时使用保护水位账号。

## 7. 会话粘度模型

会话粘度用于保持连续体验和提升缓存收益。

绑定维度：

```txt
conversation_hash -> account_id
session_hash -> account_id
api_key_id + model + conversation_hash -> account_id
```

粘度类型：

```txt
hard
soft
cache_only
none
```

### 7.1 Hard Stickiness

强制使用同一账号，除非账号不可用。

适合：

- 必须依赖上游 session 的场景。
- Web OAuth 反向代理强会话。

### 7.2 Soft Stickiness

优先使用同一账号，但健康、额度、成本更重要。

适合：

- 常规 API 多轮对话。
- 可切换 Provider 的请求。

### 7.3 Cache-only Stickiness

只因缓存收益提高权重，不绑定体验。

适合：

- 长上下文 prompt cache。
- 相同系统提示词和前缀请求。

## 8. 缓存亲和模型

缓存亲和用于降低长上下文成本。

缓存记录：

```txt
provider_id
model
account_id
prompt_prefix_hash
cached_token_estimate
cache_write_time
last_hit_time
ttl
```

缓存分：

```txt
cache_score = estimated_cache_saving / estimated_total_cost
```

影响因素：

- 输入 token 越长，缓存权重越高。
- Provider cache discount 越大，缓存权重越高。
- 账号健康太差时，缓存分不能压过健康分。
- 高级用户可以牺牲部分缓存收益换稳定性。

## 9. 成本模型

成本估算：

```txt
estimated_cost =
  estimated_input_tokens  * input_price
+ estimated_output_tokens * output_price
- estimated_cached_tokens * cache_discount
+ provider_overhead
```

成本分：

```txt
cost_score = 1 - normalized_cost
```

策略：

- 免费用户：成本权重高。
- 普通用户：成本与体验平衡。
- 高级用户：体验权重高。
- 管理员可对模型、用户组、Provider 设置成本策略。

## 10. 延迟模型

延迟分参考：

- Account p50 延迟。
- Account p95 延迟。
- Provider 当前延迟。
- 网络出口延迟。
- 最近 5 分钟趋势。

```txt
latency_score = 1 - normalized_p95_latency
```

低延迟策略中，延迟权重提升。

## 11. 公平性模型

如果只选最高分账号，可能导致流量集中。

解决方式：

- Top N weighted random。
- Smooth weighted round-robin。
- Saturation penalty。
- Per-account in-flight penalty。

饱和惩罚：

```txt
saturation_penalty = current_concurrency / max_concurrency
```

## 12. 策略模板

### 12.1 Balanced

默认策略。

```txt
health: 0.30
quota: 0.20
latency: 0.15
sticky: 0.10
cache: 0.10
cost: 0.10
fairness: 0.05
```

### 12.2 Quality First

高级用户策略。

```txt
health: 0.40
latency: 0.20
quota: 0.15
sticky: 0.10
cache: 0.05
cost: 0.05
fairness: 0.05
```

### 12.3 Cost Saver

低价套餐或免费用户策略。

```txt
cost: 0.30
quota: 0.20
cache: 0.15
health: 0.15
fairness: 0.10
latency: 0.05
sticky: 0.05
```

### 12.4 Cache Affinity

长上下文策略。

```txt
cache: 0.30
sticky: 0.20
health: 0.20
quota: 0.10
cost: 0.10
latency: 0.05
fairness: 0.05
```

### 12.5 Low Latency

交互式实时策略。

```txt
latency: 0.35
health: 0.30
sticky: 0.10
quota: 0.10
cost: 0.05
cache: 0.05
fairness: 0.05
```

## 13. 重试与故障切换

重试必须谨慎，避免重复扣费或重复副作用。

可重试错误：

```txt
network_error
timeout before response
provider_5xx
rate_limit if alternative account exists
```

不可重试错误：

```txt
invalid_request
auth_failed
content_policy
quota_exceeded on user side
```

流式请求重试规则：

- 如果上游尚未返回任何 token，可以重试。
- 如果已经开始返回 token，默认不重试。
- 可以记录 partial failure。

## 14. 决策审计

每次调度要记录决策。

字段：

```txt
request_id
user_id
api_key_id
model
strategy
selected_account_id
selected_provider_id
candidate_count
rejected_count
scores_json
reject_reasons_json
sticky_hit
cache_affinity_hit
estimated_cost
created_at
```

用途：

- 调试。
- 管理后台展示。
- 评估策略效果。
- 成本优化分析。

## 15. 数据库表建议

### 15.1 scheduler_decisions

```txt
id
request_id
attempt_no
user_id
api_key_id
source_protocol
source_endpoint
target_protocol
model
strategy
strategy_version
selected_provider_id
selected_account_id
candidate_count
rejected_count
scores_json
reject_reasons_json
strategy_weights_json
compatibility_warnings_json
sticky_hit
cache_affinity_hit
estimated_cost
currency
created_at
```

### 15.2 scheduler_feedbacks

```txt
id
request_id
decision_id
attempt_no
account_id
provider_id
model
success
error_class
status_code
latency_ms
input_tokens
output_tokens
cached_tokens
actual_cost
currency
created_at
```

### 15.3 sticky_sessions

```txt
id
binding_key
binding_type
user_id
api_key_id
model
provider_id
account_id
strength
expires_at
last_seen_at
created_at
updated_at
```

### 15.4 cache_affinity_records

```txt
id
provider_id
model
account_id
prompt_prefix_hash
cached_token_estimate
cache_write_time
last_hit_time
ttl_seconds
created_at
updated_at
```

### 15.5 account_health_snapshots

```txt
id
account_id
provider_id
status
success_rate
error_rate
latency_p50_ms
latency_p95_ms
rate_limit_count
timeout_count
cooldown_until
circuit_state
snapshot_at
```

### 15.6 account_quota_snapshots

```txt
id
account_id
provider_id
quota_type
remaining
used
quota_limit
remaining_ratio
reset_at
snapshot_at
```

## 16. Redis Key 设计

```txt
scheduler:account:{id}:health
scheduler:account:{id}:quota
scheduler:account:{id}:rpm
scheduler:account:{id}:tpm
scheduler:account:{id}:concurrency
scheduler:account:{id}:circuit
scheduler:lease:{lease_id}
scheduler:sticky:{binding_key}
scheduler:cache:{prompt_prefix_hash}
scheduler:provider:{provider}:health
```

TTL 建议：

- Lease：请求超时时间 + 30 秒。
- Sticky：按会话类型，10 分钟到 7 天。
- Cache affinity：按 Provider 缓存 TTL。
- Health：短期滚动窗口 1 到 10 分钟。
- Quota：根据 Provider 刷新频率。

## 17. Provider Adapter 需要支持的调度信息

每个 Provider Adapter 需要声明：

```txt
provider_name
supported_models
supports_stream
supports_tools
supports_vision
supports_prompt_cache
supports_context_cache
rate_limit_model
quota_model
usage_parser
error_classifier
cost_model
```

这样调度内核才能做 Provider-neutral 决策。

## 18. 管理后台调度面板

建议前端提供以下卡片：

- 调度总请求数
- 平均调度耗时
- 成功率
- 失败率
- fallback 次数
- sticky 命中率
- cache affinity 命中率
- 节省成本估算
- 账号健康排行
- 账号额度水位
- Provider 错误分布
- 策略效果对比

页面：

```txt
/admin/scheduler
/admin/scheduler/decisions
/admin/scheduler/accounts
/admin/scheduler/strategies
/admin/scheduler/cache-affinity
```

## 19. MVP 实现范围

调度内核 v1 应先实现：

- RequestClassifier
- CandidateBuilder
- PolicyFilter
- ScoreEngine
- LeaseManager
- FeedbackCollector
- Balanced 策略
- Cost Saver 策略
- Soft sticky session
- Basic health score
- Basic quota score
- scheduler_decisions 记录

MVP 实现级约束以 `SCHEDULER_V1_SPEC.md` 为准。本文档描述长期设计，`SCHEDULER_V1_SPEC.md` 描述第一阶段必须通过的过滤、打分、Lease、Decision、Feedback 和测试规则。

暂缓：

- 复杂策略 DSL。
- 机器学习调度。
- 跨区域智能路由。
- 高级 cache saving 预测。

## 20. 内核成功标准

调度内核 v1 成功标准：

- 任意请求都能解释为什么选择某个账号。
- 账号故障后自动降权或熔断。
- 额度低的账号会被保护。
- 会话粘度命中时优先保持上下文。
- 长上下文请求可以优先考虑缓存亲和。
- 高级用户比免费用户获得更高成功率策略。
- 低价用户优先成本可控策略。
- 并发请求不会突破账号并发限制。
- 所有调度结果可在后台观察和审计。
