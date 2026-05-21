# SRapi Scheduler v1 实现级规格

## 1. 目的

本文档把 `SCHEDULING_KERNEL_DESIGN.md` 和 `SCHEDULING_SCENARIOS.md` 收敛为 MVP 可实现、可测试、可验收的 Scheduler v1 规格。

Scheduler v1 的目标是先实现可解释、可审计、可控成本的账号选择闭环，而不是实现复杂策略 DSL 或机器学习调度。

策略扩展、版本、灰度、dry-run 和 shadow decision 以 `SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 为准。

## 2. 输入与输出

### 2.1 输入

```txt
GatewayRequest
CanonicalAIRequest
UserEntitlement
ApiKeyPolicy
ModelRegistry
ProviderCapabilities
ProviderAccountRuntimeState
PricingRule
RequestCapability
EffectiveCapability
StickyState optional
CacheAffinityState optional
```

### 2.2 输出

```txt
SchedulerDecision
Lease
RejectedCandidatesWithReasons
ScoreBreakdown
```

## 3. RequestProfile

Scheduler v1 必须把 Gateway 请求归一化为 RequestProfile。

```txt
request_id
user_id
api_key_id
source_protocol
source_endpoint
model
canonical_model
model_family
estimated_input_tokens
estimated_output_tokens
is_stream
is_long_context
requires_tools
requires_vision
requires_json
conversation_hash
session_hash
priority
strategy_hint
```

字段要求：

- `request_id` 必须由 Gateway 生成并贯穿日志、decision、feedback、usage。
- `source_protocol` 和 `source_endpoint` 只允许用于审计和兼容性诊断，不得影响 Scheduler 账号评分。
- `estimated_input_tokens` 和 `estimated_output_tokens` 可以使用 MVP 简单估算，但必须标记估算来源。
- `conversation_hash` 和 `session_hash` 必须是不可逆哈希，不得保存原始 prompt 或会话内容。
- RequestProfile 中的能力需求必须来自 `CAPABILITY_TAXONOMY_SPEC.md` 定义的 RequestCapability。

## 4. CandidateBuilder

候选账号必须满足：

```txt
provider enabled
provider_account enabled
model mapping exists
effective capability matches request capability
api key allowed model scope
user entitlement allowed model scope
account group scope matches user/api key
```

Candidate 字段：

```txt
account_id
provider_id
provider_name
adapter_type
canonical_model
upstream_model_name
capabilities
health_state
quota_state
pricing
priority
weight
risk_level
runtime_limits
```

CandidateBuilder 必须使用 `CAPABILITY_TAXONOMY_SPEC.md` 定义的 EffectiveCapability 做能力匹配，不得直接使用 Provider-specific 字段做 hard filter。

## 5. Filter Reasons

PolicyFilter 必须输出结构化拒绝原因。

```txt
account_disabled
provider_disabled
credential_invalid
needs_reauth
model_not_supported
model_not_allowed
user_balance_insufficient
subscription_expired
api_key_disabled
api_key_expired
quota_exhausted
quota_protected
rpm_limit_exceeded
tpm_limit_exceeded
concurrency_full
circuit_open
cooldown_active
proxy_unavailable
region_policy_mismatch
capability_mismatch
```

MVP 中必须至少实现：

```txt
account_disabled
credential_invalid
model_not_supported
model_not_allowed
api_key_disabled
quota_exhausted
rpm_limit_exceeded
tpm_limit_exceeded
concurrency_full
circuit_open
cooldown_active
```

## 6. 硬过滤规则

以下条件必须直接排除账号：

- `account.status` 是 `disabled`、`needs_reauth`、`suspended`、`dead`。
- 凭证解密失败或凭证验证失败。
- 账号不支持请求所需模型或能力。
- 用户或 API Key 不允许访问该模型。
- 账号额度为 0。
- 账号并发已满。
- RPM 或 TPM 已满。
- Circuit breaker open。
- Cooldown 未结束。

`invalid_request`、`content_policy` 等用户侧错误不得惩罚账号。

## 7. 打分模型

Scheduler v1 使用加权分数。

```txt
score =
  health_score   * W_health
+ quota_score    * W_quota
+ latency_score  * W_latency
+ sticky_score   * W_sticky
+ cache_score    * W_cache
+ cost_score     * W_cost
+ fairness_score * W_fairness
+ priority_score * W_priority
- risk_penalty
- saturation_penalty
```

所有子分数必须归一化到 `0.0 - 1.0`。

### 7.1 Health Score

```txt
health_score =
  success_rate_score * 0.40
+ latency_score      * 0.20
+ error_score        * 0.20
+ circuit_score      * 0.10
+ freshness_score    * 0.10
```

MVP 默认：

- 无健康数据的新账号：`health_score = 0.70`。
- `healthy`：上限 1.0。
- `degraded`：上限 0.65。
- `rate_limited`：如果未被 cooldown 过滤，上限 0.40。
- `dead`：硬过滤。

### 7.2 Quota Score

```txt
remaining_ratio >= 0.70 -> 1.00
0.30 <= remaining_ratio < 0.70 -> 0.70
0.10 <= remaining_ratio < 0.30 -> 0.35
0.00 < remaining_ratio < 0.10 -> 0.10
remaining_ratio == 0 -> hard reject
```

免费用户默认不得使用 `< 10%` 的高质量保护账号，除非没有其他账号并且策略显式允许 emergency。

### 7.3 Latency Score

MVP 使用 p95 延迟归一化：

```txt
latency_score = clamp(1 - (p95_latency_ms / latency_budget_ms), 0, 1)
```

默认：

```txt
latency_budget_ms = 10000
```

无延迟数据的新账号：`latency_score = 0.60`。

### 7.4 Cost Score

```txt
estimated_cost =
  estimated_input_tokens  * input_price_per_token
+ estimated_output_tokens * output_price_per_token
- estimated_cached_tokens * cache_discount_per_token
```

```txt
cost_score = 1 - normalized_cost
```

同一候选集内成本归一化：

```txt
normalized_cost = (candidate_cost - min_cost) / max(max_cost - min_cost, epsilon)
```

如果所有候选成本相同，`cost_score = 0.5`。

### 7.5 Sticky Score

```txt
hard sticky match -> 1.0
soft sticky match -> 0.7
cache_only match -> 0.3
no match -> 0.0
```

Hard sticky 不能绕过硬过滤。

### 7.6 Cache Score

MVP 可以只实现 cache-only 分数，不强制落库。

```txt
cache_score = clamp(estimated_cache_saving / estimated_total_cost, 0, 1)
```

当 `health_score < 0.40` 时，cache_score 不得使该账号胜出，除非无其他账号且策略显式允许。

### 7.7 Fairness 与 Saturation

```txt
saturation_penalty = current_concurrency / max_concurrency
```

MVP 可先使用 Top N weighted random：

```txt
top_n = min(3, candidate_count)
```

测试环境必须允许固定随机种子，保证断言稳定。

## 8. 策略权重

### 8.1 balanced

```txt
health: 0.30
quota: 0.20
latency: 0.15
sticky: 0.10
cache: 0.10
cost: 0.10
fairness: 0.05
priority: 0.00
```

### 8.2 cost_saver

```txt
cost: 0.30
quota: 0.20
cache: 0.15
health: 0.15
fairness: 0.10
latency: 0.05
sticky: 0.05
priority: 0.00
```

MVP 可以仅实现这两个策略。`quality_first`、`cache_affinity`、`low_latency` 保留为后续策略，但场景测试可先作为 pending 或 design tests。

## 9. Lease 状态机

Lease 用于预占账号并发和估算额度。

### 9.1 状态

```txt
pending
committed
released
expired
failed
```

### 9.2 Redis Key

```txt
scheduler:lease:{lease_id}
scheduler:account:{account_id}:concurrency
scheduler:account:{account_id}:rpm
scheduler:account:{account_id}:tpm
```

TTL：

```txt
lease_ttl = request_timeout + 30s
```

### 9.3 原子性

MVP 推荐使用 Redis Lua 或事务保证：

- 检查并发是否未满。
- 增加并发计数。
- 创建 lease。
- 设置 TTL。

如果创建失败，必须不改变并发计数。

### 9.4 释放规则

- 成功完成：`committed`，并发计数递减，提交实际 usage。
- 请求失败且未调用上游：`released`，并发计数递减。
- 上游失败：`failed`，并发计数递减，写入 feedback。
- 进程崩溃：TTL 自动过期，后台任务或下次调度修正计数。

## 10. Redis Runtime State

MVP Redis key：

```txt
scheduler:account:{id}:health
scheduler:account:{id}:quota
scheduler:account:{id}:rpm
scheduler:account:{id}:tpm
scheduler:account:{id}:concurrency
scheduler:account:{id}:circuit
scheduler:sticky:{binding_key}
scheduler:cache:{prompt_prefix_hash}
scheduler:provider:{provider}:health
```

要求：

- Redis 数据必须可由 PostgreSQL 配置和运行时反馈重建。
- 不能只依赖 Redis 作为 billing 或 usage 真实来源。
- Redis key 必须有 TTL 或明确刷新机制。

## 11. Decision 持久化

`scheduler_decisions` 必须记录：

```txt
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

约束：

- 第一次调度 `attempt_no = 1`。
- fallback 后必须使用同一 `request_id` 和递增 `attempt_no`。
- 数据库唯一约束应为 `unique(request_id, attempt_no)`。
- `strategy_weights_json` 必须保存当次决策使用的权重快照。

`scores_json` 至少包含每个候选账号的：

```txt
account_id
final_score
health_score
quota_score
latency_score
sticky_score
cache_score
cost_score
fairness_score
risk_penalty
saturation_penalty
```

## 12. Feedback 持久化

`scheduler_feedbacks` 必须记录：

```txt
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

错误分类必须与 Provider Adapter 规范一致。

## 13. 重试与 Fallback

可重试：

```txt
network_error
timeout before first byte
provider_5xx
rate_limit if alternative account exists
```

不可重试：

```txt
invalid_request
auth_failed
content_policy
quota_exceeded on user side
stream already emitted token
```

MVP 默认最多 1 次 fallback，且必须产生同一 `request_id` 下按 `attempt_no` 串联的完整 decision/feedback 链路。

## 14. 测试映射

`SCHEDULING_SCENARIOS.md` 中以下场景必须在 MVP 通过：

```txt
A 健康优先
B 额度保护
D 会话粘度命中
E 粘性账号故障后切换
J 成本优先
L 并发 Lease 限制
M RPM 限制
N 无可用账号
Q 用户余额不足
S 流式请求开始前失败
T 流式请求中途失败
```

以下场景可以先作为后续扩展或 pending：

```txt
C 高级用户使用保护水位账号
F/G Hard Sticky
H/I Cache Affinity 深度落库
K Low Latency
```

## 15. 成功标准

Scheduler v1 完成必须满足：

- 任意请求都能解释为什么选择某个账号。
- 所有拒绝账号都有结构化 reason。
- 并发不会突破账号限制。
- 无可用账号时返回明确错误。
- Provider 错误会回流到 health / quota / feedback。
- Decision 和 Feedback 可通过管理 API 查询。
- MVP 必测场景全部通过。
