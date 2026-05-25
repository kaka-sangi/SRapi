# SRapi Scheduler 策略扩展规范

## 1. 目标

本文档定义 Scheduler 策略的扩展、配置、版本、灰度、仿真和审计规则。

目标：

- 新增调度策略不修改 Gateway。
- 新增调度策略不修改 Provider Adapter。
- Scheduler core 保持 Provider-neutral。
- 策略可解释、可审计、可回滚。
- 支持用户组、API Key、模型、Provider 维度的策略覆盖。

## 2. 策略边界

Strategy 可以决定：

- 分数权重。
- 候选排序规则。
- Top N 选择规则。
- fallback 顺序。
- 风险惩罚。
- sticky / cache affinity 偏好。
- emergency 模式是否允许。

Strategy 不得：

- 直接调用 Provider。
- 解密账号凭证。
- 修改用户余额。
- 绕过硬过滤。
- 访问 HTTP request 原始 body。
- 使用 source endpoint 作为 Provider 偏置条件。
- 保存原始 prompt。

## 3. StrategyRegistry

Scheduler 必须通过 StrategyRegistry 获取策略。

接口能力：

```txt
ListStrategies
GetStrategy
ResolveStrategyForRequest
ValidateStrategyConfig
SimulateStrategy
GetStrategyVersion
```

解析优先级：

```txt
request strategy_hint
api_key strategy override
user_group strategy override
model strategy override
global default strategy
```

任何 override 都不得突破用户权益、API Key scope、模型能力和账号硬过滤。

## 4. Strategy Descriptor

策略描述结构：

```txt
id
name
version
status
description
owner
config_schema_json
default_config_json
risk_level
created_at
updated_at
```

状态：

```txt
draft
active
deprecated
disabled
```

内置策略：

```txt
balanced
cost_saver
latency_first
quota_protect
sticky_first
cache_affinity_first
premium_quality
emergency_fallback
```

MVP 必须实现：

```txt
balanced
cost_saver
```

## 5. Strategy Config

策略配置必须是可验证 schema。

基础字段：

```txt
weights
hard_rules
soft_preferences
fallback_rules
randomization
risk_controls
observability
```

权重示例：

```txt
health_weight
quota_weight
latency_weight
sticky_weight
cache_weight
cost_weight
fairness_weight
priority_weight
risk_penalty_weight
saturation_penalty_weight
```

权重约束：

- 所有权重必须在 `0.0 - 1.0`。
- 总权重必须可归一化。
- 禁止负权重，除非字段明确是 penalty。
- 配置必须保存版本快照。

## 6. 策略执行阶段

策略执行流程：

```txt
RequestProfile
  ↓
CandidateBuilder
  ↓
HardFilter
  ↓
Strategy.ResolveWeights
  ↓
ScoreEngine
  ↓
Strategy.RankCandidates
  ↓
Strategy.SelectWinner
  ↓
LeaseManager
  ↓
Decision Persist
```

Strategy 只能作用于 hard filter 之后的候选集。

## 7. Hard Rule 不可覆盖

以下规则任何策略不得覆盖：

- account disabled。
- provider disabled。
- credential invalid。
- model not supported。
- model not allowed。
- user entitlement denied。
- api key disabled / expired。
- account concurrency full。
- circuit open。
- cooldown active。
- credential needs reauth。
- region policy mismatch。

Emergency strategy 只能在明确允许的硬规则内放宽，例如使用低额度保护账号，不得使用无效凭证或禁用账号。

## 8. 策略作用域

策略可以绑定到：

```txt
global
user_group
api_key
model
model_alias
provider
account_group
```

优先级：

```txt
api_key > user_group > model_alias > model > provider > global
```

如果多个 group 命中，以 `GATEWAY_ROUTE_MATRIX.md` 的 group 解析结果为准。

## 9. Strategy Version

每次策略配置变更必须生成新版本：

```txt
balanced@v1
balanced@v2
cost_saver@v1
```

`scheduler_decisions` 必须记录：

```txt
strategy
strategy_version
strategy_config_hash
strategy_weights_json
```

K1.6.2 起新的真实 Scheduler attempt 会同步写入 `scheduler_request_snapshots`，
保存可回放的 RequestProfile 和 CandidateSnapshot。历史 decision 只有在存在 snapshot 时
才能按当时策略版本和候选集复现；旧的 decision-only 行只能用于报表统计。

## 10. Dry-run 与 Shadow Decision

策略上线前必须支持 dry-run：

```txt
POST /api/v1/admin/scheduler/simulate
```

Shadow decision：

- 不影响真实路由。
- 使用同一 RequestProfile 和候选集。
- 记录 shadow winner、score diff、cost diff、risk diff。
- 可选传入 `shadow_rollout_percent` 和 `rollout_key`，预览同一请求在稳定百分比分流下是否会命中 shadow strategy；响应只返回 key hash，不返回原始 rollout key。
- 用于评估新策略。

Shadow decision 不得创建 Lease。

## 11. A/B 与灰度

策略灰度维度：

```txt
user_group
api_key_prefix_hash
model
provider
percentage
```

要求：

- 分流必须稳定，同一 key 在窗口期内命中同一策略。
- 可以一键回滚到上一 active version。
- 灰度策略必须在 Observability 中显示效果。
- K1.6 的 `POST /api/v1/admin/scheduler/simulate` 已提供稳定百分比分流预览，用于验证 bucket、命中结果和 key hash；它仍是 dry-run，不改变真实 Gateway 策略。

## 12. Fallback 策略

Fallback 发生条件：

- Lease 获取失败。
- 上游 429 / 5xx 可重试。
- Provider timeout。
- streaming 建连失败。
- selected account 被并发占满。

Fallback 规则：

```txt
same_provider_next_account
same_model_next_provider
compatible_model_fallback
emergency_account_group
fail_fast
```

Fallback 必须使用同一 `request_id`，递增 `attempt_no`。

不得 fallback 到不满足 required capability 的候选。

## 13. Sticky 与 Cache Affinity

策略可以调节：

```txt
sticky_weight
hard_sticky_enabled
soft_sticky_ttl
cache_affinity_weight
cache_affinity_ttl
```

限制：

- sticky 不得绕过 hard filter。
- cache affinity 不得让健康分过低账号胜出，除非 emergency explicitly allowed。
- 会话哈希必须不可逆。

## 14. Cost 控制

策略可以使用：

```txt
estimated_cost
actual_cost_history
user_plan
provider_price
cache_saving_estimate
```

禁止：

- 用低成本账号绕过用户购买的模型权益。
- 在未告知的情况下把高质量模型降级为低质量模型。
- 修改 Billing ledger。

模型 fallback 或降级必须由 Gateway / Model Policy 产生 compatibility warning。

## 15. Risk 控制

Risk penalty 来源：

```txt
reverse_proxy_runtime
account_ban_history
recent_auth_errors
proxy_instability
provider_policy_risk
new_unverified_adapter
experimental_capability
```

策略可以增加 risk penalty，不得移除安全模块强制 penalty。

## 16. 策略存储

建议新增 `scheduler_strategies`：

```txt
id
name
version
status
scope_type
scope_id
config_json
config_hash
description
created_by
created_at
updated_at
activated_at
deprecated_at
```

索引：

```txt
unique(name, version, scope_type, scope_id)
index(status, scope_type, scope_id)
index(name, status)
```

也可在 MVP 先用配置文件 seed，但 decision 中必须记录策略版本和权重快照。

K1.2 起运行时已经读取 `status=active`、`scope_type=global` 且 `scope_id IS NULL` 的
`scheduler_strategies` 行，并在每次调度和管理员策略列表读取前刷新到
`StrategyRegistry`。同名多 active 版本按 `activated_at`、`updated_at` / `created_at`、
`id` 选择最新行；内置 seed 仍作为本地 / memory store fallback。K1.6 起已支持单请求
shadow dry-run 和稳定 rollout 预览；K1.6.2 起新的真实调度会持久化脱敏
`scheduler_request_snapshots`；K1.6.3 起 `POST /api/v1/admin/scheduler/replay`
可用这些 snapshot 重建 RequestProfile + CandidateSnapshot 做历史策略回放。

历史 replay 接口必须只对存在 snapshot 的决策声称可重算。没有 snapshot 的旧
`scheduler_decisions` 行缺少当时完整候选集，只能用于报表对比，不能声称完成“历史重算新策略”。API key、model、provider 等 scoped override 和真实流量百分比灰度仍属于后续范围。

## 17. Admin API

当前 OpenAPI 已落地单请求 dry-run 和 snapshot-backed 历史 replay：

```txt
POST   /api/v1/admin/scheduler/simulate
POST   /api/v1/admin/scheduler/replay
```

后续策略管理接口预留：

```txt
GET    /api/v1/admin/scheduler/strategies
POST   /api/v1/admin/scheduler/strategies
GET    /api/v1/admin/scheduler/strategies/{id}
PATCH  /api/v1/admin/scheduler/strategies/{id}
POST   /api/v1/admin/scheduler/strategies/{id}/activate
GET    /api/v1/admin/scheduler/strategies/{id}/versions
```

写操作必须进入 Audit。

## 18. 可观测性

策略指标：

```txt
scheduler_strategy_selected_total
scheduler_strategy_fallback_total
scheduler_strategy_shadow_diff
scheduler_strategy_cost_delta
scheduler_strategy_latency_delta
scheduler_strategy_error_rate
scheduler_strategy_reject_reason_total
```

Ops Dashboard 必须能按 strategy / version 过滤 decision、fallback、错误率和成本。

## 19. 测试要求

每个策略必须覆盖：

- 固定随机种子下选择稳定。
- hard filter 不可绕过。
- 权重归一化。
- Top N weighted random。
- fallback attempt_no 递增。
- strategy version 记录。
- shadow decision 不创建 Lease。
- A/B 分流稳定。
- rollback 后使用旧版本。
- `SCHEDULING_SCENARIOS.md` 中对应场景。

## 20. 新策略引入清单

新增策略前必须完成：

- Strategy descriptor。
- Config schema。
- 默认配置。
- 风险等级。
- dry-run 测试。
- golden decision tests。
- 观测指标。
- 回滚方案。
- 文档更新。
