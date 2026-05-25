# SRapi Observability 与告警规格

## 1. 目标

SRapi 的可观测性不是传统 Web 服务监控，而是 AI API Gateway 的运行诊断系统。它必须同时解释：

- 请求是否成功。
- 首 token 是否足够快。
- 流式响应是否完整结束。
- 哪个 Provider / Account / Model / Group 出现问题。
- Token、成本和缓存收益如何变化。
- Scheduler 为什么选择或拒绝某个账号。
- 反代账号是否出现封禁、挑战、设备异常等风险信号。

## 2. 信息架构

管理后台建议提供 `/admin/ops` 运维中心。

```txt
/admin/ops
├── overview
├── traffic
├── errors
├── providers
├── scheduler
├── alerts
└── settings
```

| 页面 | 目标 |
| --- | --- |
| Overview | 全局健康分、黄金信号、SLO 燃烧率、近期告警。 |
| Traffic | QPS、TPS、token/s、成本、Provider × Model × Group 热力图。 |
| Errors | 错误分层、错误指纹、样本、retry workbench。 |
| Providers | Provider / Account 健康矩阵、槽位、限流、凭证有效期。 |
| Scheduler | decision、reject reason、score breakdown、fallback 链路。 |
| Alerts | SLO、告警事件、通知、抑制规则。 |
| Settings | 采样率、保留期、通知通道、SLO 定义。 |

## 3. 指标模型

### 3.1 传统 RED 指标

```txt
request_count
error_count
request_duration_p50/p95/p99
inflight_requests
```

### 3.2 资源 USE 指标

```txt
cpu
memory
goroutines
db_pool_in_use
db_pool_wait
redis_pool_in_use
worker_queue_depth
```

### 3.3 AI-native 指标

```txt
ttft_ms
stream_completion_rate
tokens_per_second
input_tokens
output_tokens
cached_tokens
cache_hit_rate
model_route_saturation
provider_failover_rate
provider_error_rate
```

当前 Gateway failover 计数由 `/metrics` 暴露为：

```txt
srapi_gateway_failover_total{endpoint_family, model, provider_protocol, result}
```

该指标只使用低基数 route/model/protocol/result 标签，不得加入 API key、account id、user id、request id、prompt 或 credential。

当前 Gateway latency 由 `/metrics` 暴露为 Prometheus histogram：

```txt
srapi_gateway_request_duration_seconds_bucket{endpoint_family, model, provider_protocol, result, le}
srapi_gateway_request_duration_seconds_count{endpoint_family, model, provider_protocol, result}
srapi_gateway_request_duration_seconds_sum{endpoint_family, model, provider_protocol, result}
```

MVP bucket 固定为 `0.1`、`0.5`、`1`、`5` 秒和 `+Inf`；不得引入 API key、account id、user id、request id、prompt 或 credential label。后续若替换为 Prometheus client SDK，必须保持同名指标语义和 label 基数约束。

### 3.3.1 Trace and Log Correlation

HTTP server 会创建 OpenTelemetry server span，并使用 W3C trace context 从请求头提取/传播 trace。日志 handler 从 context 注入 `request_id`、`trace_id`、`user_id` 和 `api_key_id`，用于把 Gateway request、scheduler decision、usage log、audit log 和 provider feedback 串联起来。

Trace span attribute 只允许记录低敏诊断字段，例如 HTTP method、route、status、response size、SRapi request id、scheduler strategy、provider protocol、错误分类和 latency。不得记录原始 prompt、messages、tool arguments、API key、provider credential、Authorization header、cookie 或 payment secret。

### 3.4 经济指标

```txt
estimated_cost
actual_cost
revenue
margin_estimate
cost_per_model
cost_per_provider
cost_per_user_group
```

### 3.5 反代风险指标

```txt
reverse_proxy_challenge_required_total
reverse_proxy_captcha_required_total
reverse_proxy_session_invalid_total
reverse_proxy_account_locked_total
reverse_proxy_account_banned_total
reverse_proxy_device_unrecognized_total
reverse_proxy_geo_blocked_total
reverse_proxy_upstream_client_outdated_total
```

### 3.6 Realtime slot 指标

```txt
srapi_realtime_active_slots
srapi_realtime_active_slots_by_endpoint
srapi_realtime_slots_total{event="acquired|released|rejected"}
```

Realtime slot 指标只记录连接生命周期和低基数 endpoint 标签，不得包含原始 session affinity key、API key、credential、prompt 或 provider-specific payload。

WP-570 起，`GET /api/v1/admin/ops/realtime/slots` 提供 active realtime slot 诊断视图。WP-590 起 Redis 可用时该视图覆盖共享 Redis 的 API 节点；本地降级模式只覆盖当前节点内存。该接口返回 slot id、kind、request id、user/API key id、source endpoint、acquired time、hash 后的 affinity key、sticky account/strength 以及 endpoint/kind/API key 聚合计数；它不表示持久 upstream session 池，不返回原始 affinity key、API key、credential、prompt 或 provider-specific frame。

## 4. 错误归因

错误必须按 owner 分类：

| owner | 示例 | 是否计入 SLA |
| --- | --- | --- |
| `client` | invalid request、auth failed、model not allowed | 否 |
| `business` | balance insufficient、subscription expired、quota exceeded | 默认否 |
| `scheduler` | no available account、lease failed | 是 |
| `provider` | upstream 429/529/5xx、timeout | 是 |
| `reverse_proxy` | session invalid、account banned、challenge | 是 |
| `internal` | DB error、panic、serialization failure | 是 |

Provider 错误分类必须与 `PROVIDER_ADAPTER_SPEC.md` 和 `REVERSE_PROXY_SPEC.md` 保持一致。

## 5. SLO 模型

SLO 定义建议结构：

```yaml
id: slo_chat_availability
name: Chat Completions Availability
sli_type: availability
objective: 99.5
window_days: 28
filter:
  source_endpoint: /v1/chat/completions
  error_owner_exclude:
    - client
    - business
alert_policy: multi_window_burn_rate
```

支持的 SLI：

| 类型 | 说明 |
| --- | --- |
| `availability` | 成功请求比例。 |
| `latency` | p95/p99 duration 或 TTFT。 |
| `freshness` | 聚合数据或快照延迟。 |
| `quality` | 流式完整率、fallback 风暴率等 AI 专项指标。 |

## 6. Burn-rate 告警

错误预算燃烧率：

```txt
burn_rate = error_rate / (1 - objective)
```

必须支持多窗口多燃烧率告警：

| 严重级别 | 短窗口 | 长窗口 | 示例 |
| --- | --- | --- | --- |
| critical | 5m | 1h | 快速烧预算。 |
| warning | 30m | 6h | 中速持续劣化。 |
| ticket | 2h | 24h | 慢性问题。 |

## 7. 告警中心

告警事件必须包含：

```txt
id
rule_id
severity
status
fingerprint
summary
details_json
started_at
resolved_at
acknowledged_by
suppressed_by
```

通知通道规划：

```txt
email
webhook
dingtalk
feishu
wecom
```

通知凭证必须保存到 secret settings，不得进入前端响应。

## 8. Provider 健康矩阵

Provider 页面必须能按以下维度过滤：

```txt
provider
account_group
model
runtime_class
status
error_class
```

每个账号至少展示：

- 当前状态。
- 可用并发槽位。
- RPM/TPM 使用率。
- 最近 5 分钟错误率。
- latency p95。
- 最近 cooldown_until。
- 最近 ban signal。
- OAuth / cookie / session 有效期摘要。
- proxy / egress profile 摘要。

## 9. Scheduler 可观测性

Scheduler decision 必须支持下钻：

```txt
request_id
attempt_no
strategy
strategy_version
candidate_count
rejected_count
selected_account_id
score_breakdown
reject_reasons
compatibility_warnings
sticky_hit
cache_affinity_hit
fallback_chain
```

所有 fallback attempt 必须共享同一个 `request_id`，并递增 `attempt_no`。

## 10. 数据表规划

除 `usage_logs`、`scheduler_decisions`、`scheduler_feedbacks` 外，Phase 2 建议增加：

```txt
obs_slo_definitions
obs_alert_events
obs_alert_rules
obs_alert_silences
obs_ai_usage_rollups
obs_provider_health_snapshots
obs_error_fingerprints
obs_notification_channels
obs_notification_deliveries
```

原始日志保留期和聚合保留期必须可配置。

当前控制面 v1 已落库 `obs_slo_definitions` 和 `obs_alert_events`：

- `obs_slo_definitions` 持久化 SLO 名称、SLI 类型、比例化 objective、窗口、状态、低基数过滤条件和 burn-rate 阈值。
- `GET /api/v1/admin/ops/slo` 基于 `usage_logs` 实时返回 availability 评估证据，包括 total/good/bad requests、error rate、burn rate 和 error budget consumed。
- `obs_alert_events` 持久化告警 severity、status、fingerprint、summary、时间戳和 ack 元数据。
- `POST /api/v1/admin/ops/alerts/{id}/ack` 只写 ack 状态和 actor；audit 只记录安全摘要，不复制 `details_json`。

## 11. API 草案

```txt
GET  /api/v1/admin/ops/overview
GET  /api/v1/admin/ops/traffic
GET  /api/v1/admin/ops/errors
GET  /api/v1/admin/ops/providers
GET  /api/v1/admin/ops/scheduler/decisions
GET  /api/v1/admin/ops/scheduler/decisions/{id}
GET  /api/v1/admin/ops/realtime/slots
GET  /api/v1/admin/ops/alerts
POST /api/v1/admin/ops/alerts/{id}/ack
GET  /api/v1/admin/ops/slo
POST /api/v1/admin/ops/slo
PATCH /api/v1/admin/ops/slo/{id}
GET  /api/v1/admin/ops/settings
PATCH /api/v1/admin/ops/settings
```

已实现的 SLO 写接口使用控制台 Cookie + CSRF；SLO objective 可输入 `0.995` 或 `99.5`，响应和持久化统一为 `0.995`。

## 12. 隐私与脱敏

默认不得在观测系统中保存：

- 完整 prompt。
- 完整 messages。
- tool arguments 原文。
- Authorization header。
- Cookie。
- Provider 凭证。

如管理员显式开启调试采样，必须：

- 有保留期。
- 有采样率。
- 有脱敏处理。
- 写 audit log。

## 13. MVP 最小要求

MVP 至少实现：

- Request ID。
- Usage Log。
- Scheduler Decision 查询。
- Provider error class。
- Basic latency 和 token usage。
- `/api/v1/admin/scheduler/overview`。
- `/api/v1/admin/scheduler/decisions`。

Phase 2 补齐 Ops Dashboard、SLO、告警、Provider 健康矩阵和聚合表。
