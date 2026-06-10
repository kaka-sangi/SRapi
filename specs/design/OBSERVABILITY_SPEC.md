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

管理后台已提供 `/admin/ops` 运维中心（页面见 `apps/web/src/app/admin/ops/`，读模型接口见 §11）。

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

当前 `/metrics` 由 Prometheus client SDK 渲染，Gateway latency 由 scrape-time 自定义 collector 暴露为 Prometheus histogram：

```txt
srapi_gateway_request_duration_seconds_bucket{endpoint_family, model, provider_protocol, result, le}
srapi_gateway_request_duration_seconds_count{endpoint_family, model, provider_protocol, result}
srapi_gateway_request_duration_seconds_sum{endpoint_family, model, provider_protocol, result}
```

bucket 固定为 `0.05`、`0.1`、`0.25`、`0.5`、`1`、`2.5`、`5`、`10` 秒和 `+Inf`；不得引入 API key、account id、user id、request id、prompt 或 credential label。

Provider account probe latency 由最新健康快照聚合为：

```txt
srapi_provider_probe_latency_seconds_bucket{provider_protocol, status, le}
srapi_provider_probe_latency_seconds_count{provider_protocol, status}
srapi_provider_probe_latency_seconds_sum{provider_protocol, status}
```

该指标只使用 provider protocol 和健康状态标签，不得加入 provider id、account id、账号名、凭证、proxy URL 或 request id。

Scheduler cost score 由已持久化的 `SchedulerDecision.scores_json` 聚合为：

```txt
srapi_scheduler_cost_score_avg{strategy}
```

该指标只使用低基数 strategy 标签，不得加入 account id、provider id、API key、user id、request id、prompt 或 credential。它用于观察 cost-aware routing 的总体趋势；逐账号排查应读取 Scheduler decision 的 score breakdown 和 request snapshot，而不是增加高基数 Prometheus label。

Scheduler strategy 运营指标由已持久化的 `scheduler_decisions` 与同 request/attempt 的 `usage_logs` 在 scrape 时聚合：

```txt
scheduler_strategy_selected_total{strategy, version}
scheduler_strategy_fallback_total{strategy, version}
scheduler_strategy_shadow_diff{strategy, version, shadow_strategy, selection}
scheduler_strategy_cost_delta{strategy, version}
scheduler_strategy_latency_delta{strategy, version}
scheduler_strategy_error_rate{strategy, version}
scheduler_strategy_reject_reason_total{strategy, version, reason}
```

这些指标只允许 strategy、strategy version、shadow strategy、current/shadow selection 和结构化 reject reason 这类低基数标签。`cost_delta` / `latency_delta` 表示选中账号分数相对同次候选集平均分数的平均差值；`error_rate` 来自同 request_id + attempt_no 的 usage 成败。不得加入 API key、account id、provider id、user id、request id、prompt、cookie 或 credential label；逐请求排查仍应读取 Scheduler decision、request snapshot 和 usage log。

Ops alert 当前状态由已持久化的 `obs_alert_events` 在 scrape 时聚合：

```txt
srapi_ops_alert_events{severity, status}
```

该指标表示当前告警事件数量，只允许 `severity` 和 `status` 两个低基数标签。不得加入 alert id、rule id、fingerprint、SLO id、user id、API key、request id、prompt 或 credential label；逐条告警排查应读取 AdminOps alert API。

默认 Prometheus 告警规则保存在 `deploy/prometheus-srapi-alerts.yaml`，覆盖 critical firing 和长期 warning firing 两类 Ops alert posture。`deploy/prometheus.yml` 是本地 compose Prometheus profile 的最小配置，会抓取 API `/metrics`、加载这些规则，并把告警发送到同 profile 内的 Alertmanager。`deploy/alertmanager.yml` 提供本地 webhook notification route，按 `service`、`severity`、`component` 聚合通知并发送 resolved 事件。规则 labels 和 Alertmanager grouping 只能使用 `severity`、`status`、`service`、`team` 和 `component` 这类固定低基数维度；排障入口、runbook 和人工动作必须放在 annotations 或接收端系统配置，不能把 alert id、fingerprint、rule id 或账号/API key/user/request 维度放进 labels 或 route grouping。

### 3.3.1 Trace and Log Correlation

HTTP server 会创建 OpenTelemetry server span，并使用 W3C trace context 从请求头提取/传播 trace。日志 handler 从 context 注入 `request_id`、`trace_id`、`user_id` 和 `api_key_id`，用于把 Gateway request、scheduler decision、usage log、audit log 和 provider feedback 串联起来。

Trace span attribute 只允许记录低敏诊断字段，例如 HTTP method、route、status、response size、SRapi request id、scheduler strategy、provider protocol、错误分类和 latency。不得记录原始 prompt、messages、tool arguments、API key、provider credential、Authorization header、cookie 或 payment secret。

关键 service span 目前覆盖 `scheduler.Schedule`、`payments.HandleWebhook` 和 `accounts.ProbeAccount`。这些 span 必须记录业务 outcome、稳定错误分类和低敏诊断字段；错误 span 必须同时设置 `error.type`、record error event，并把 span status 设为 error。后续新增业务 span 应复用 `platform/otel.StartSpan` / `EndSpan`，并保持属性命名使用 `srapi.*` 前缀。

Trace exporter 有本地 OTLP gRPC collector smoke 覆盖：`TestNewTracerProviderExportsSpansToOTLPCollector` 启动进程内 collector，启用 `OTEL_TRACES_ENABLED` 等价配置，验证 span 和 resource attributes 会经真实 OTLP 协议在 tracer provider shutdown 时 flush。Jaeger 可视化路径有 opt-in smoke 覆盖：`make smoke-jaeger-trace` 会临时启动官方 Jaeger all-in-one 容器，把 span 写入 OTLP/gRPC 4317，并通过 Jaeger Query API `/api/traces/{trace_id}` 查回。Tempo 可视化路径有 opt-in smoke 覆盖：`make smoke-tempo-trace` 会临时启动官方 Tempo 容器，把 span 写入 OTLP/gRPC，并通过 Tempo Query API `/api/v2/traces/{trace_id}` 查回。部署环境中的托管 collector、多节点 Tempo 或其他真实 tracing backend 仍需按实际拓扑单独 smoke。

Trace overhead 有 opt-in p99 guard 覆盖：`make otel-overhead-bench` 会对比 no-op tracer provider 与 batch tracer provider 下 `/livez` HTTP runtime 的 p99 延迟，并默认要求增量不超过 5ms。该测试不进入默认 `make check`，避免普通开发机抖动阻断提交；在发布前、collector/SDK 升级后或观测采样策略变更后应显式运行。

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

SLO 已落库为 `obs_slo_definitions` 并提供控制面 CRUD（§10、§11）。定义结构示例：

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

告警事件包含（`obs_alert_events` / ent `obsalertevent`，已落库；CRUD/ack 见 §11）：

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

通用指标告警规则（`obs_alert_rules`）与静默匹配器（`obs_alert_silences`）已落库并提供控制面 CRUD（见 §10、§11）。

Roadmap / 尚未实现：控制面内置外部通知通道及凭证管理。

```txt
email
webhook
dingtalk
feishu
wecom
```

> 当前外部告警投递经 Prometheus + Alertmanager 部署侧路由（见 §3.3 末尾的 `deploy/prometheus-srapi-alerts.yaml` / `deploy/alertmanager.yml`）。一旦在控制面内置上述通道，通知凭证必须保存到 secret settings，不得进入前端响应。

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

观测数据建立在 `usage_logs`、`scheduler_decisions`、`scheduler_feedbacks` 之上。下列控制面与聚合表均已落库（ent schema 见 `apps/api/ent/schema/`）：

```txt
obs_slo_definitions          # ent: obsslodefinition
obs_alert_events             # ent: obsalertevent（含 fingerprint 字段）
obs_alert_rules              # ent: obsalertrule
obs_alert_silences           # ent: obsalertsilence
account_health_snapshots     # ent: accounthealthsnapshot（Provider/账号健康快照）
account_availability_rollups # ent: accountavailabilityrollup（账号可用率聚合）
```

Roadmap / 尚未实现（验证：无对应 ent schema / 模块）：

```txt
obs_ai_usage_rollups         # AI usage/成本按维度的聚合 rollup（目前仅有账号可用率 rollup）
obs_error_fingerprints       # 独立错误指纹聚合表（当前 fingerprint 仅为 obs_alert_events 上的内联字段）
obs_notification_channels    # 控制面内置外部告警通道（email/webhook/钉钉/飞书/企业微信）
obs_notification_deliveries  # 外部告警投递记录
```

> 外部告警投递目前由 Prometheus + Alertmanager 部署侧承担（见 §3.3 末尾的 `deploy/` 配置）；控制面内置通道与投递记录是 Phase 2 项。`notifications` 模块面向事务性认证邮件与用户订阅偏好，不是 Ops 告警通道。

原始日志保留期和聚合保留期可配置（保留 worker 见 `OPERATIONS.md`）。

控制面读写细节：

- `obs_slo_definitions` 持久化 SLO 名称、SLI 类型、比例化 objective、窗口、状态、低基数过滤条件和 burn-rate 阈值。
- `GET /api/v1/admin/ops/slo` 基于 `usage_logs` 实时返回 availability 评估证据，包括 total/good/bad requests、error rate、burn rate 和 error budget consumed。
- `obs_alert_events` 持久化告警 severity、status、fingerprint、summary、时间戳和 ack 元数据。
- `obs_alert_rules` / `obs_alert_silences` 持久化通用指标告警规则（scope、阈值、severity）和静默匹配器，CRUD 见 §11 的 `/api/v1/admin/ops/alert-rules{,/{id}}` 与 `/alert-silences{,/{id}}`（service 见 `apps/api/internal/modules/operations/service/alertrules.go`）。
- `account_health_snapshots` 持久化逐账号健康快照（status、latency、probe 结果），`srapi_provider_probe_latency_seconds_*`（§3.3）由最新快照聚合。
- `account_availability_rollups` 由 `health_rollups` 模块按账号/日期桶聚合可用率。
- `POST /api/v1/admin/ops/alerts/{id}/ack` 只写 ack 状态和 actor；audit 只记录安全摘要，不复制 `details_json`。

## 11. 控制面 API

以下为已实现的 AdminOps 读模型与控制面接口（以 `packages/openapi/openapi.yaml` 的 `/api/v1/admin/ops/*` 为准；下钻接口 operationId 见同文件 `AdminOps` tag）：

```txt
GET   /api/v1/admin/ops/overview
GET   /api/v1/admin/ops/throughput-trend
GET   /api/v1/admin/ops/error-trend
GET   /api/v1/admin/ops/error-distribution
GET   /api/v1/admin/ops/latency-histogram
GET   /api/v1/admin/ops/concurrency
GET   /api/v1/admin/ops/realtime/slots
GET   /api/v1/admin/ops/system-logs
POST  /api/v1/admin/ops/system-logs/cleanup
GET   /api/v1/admin/ops/events/outbox
GET   /api/v1/admin/ops/settings
PUT   /api/v1/admin/ops/settings
GET   /api/v1/admin/ops/slo
POST  /api/v1/admin/ops/slo
GET   /api/v1/admin/ops/slo/{id}
PATCH /api/v1/admin/ops/slo/{id}
GET   /api/v1/admin/ops/alerts
GET   /api/v1/admin/ops/alert-events
POST  /api/v1/admin/ops/alerts/{id}/ack
GET   /api/v1/admin/ops/alert-rules
POST  /api/v1/admin/ops/alert-rules
PATCH /api/v1/admin/ops/alert-rules/{id}
DELETE /api/v1/admin/ops/alert-rules/{id}
GET   /api/v1/admin/ops/alert-silences
POST  /api/v1/admin/ops/alert-silences
DELETE /api/v1/admin/ops/alert-silences/{id}
```

Scheduler decision 下钻不在 `ops/` 命名空间下，而是 `GET /api/v1/admin/scheduler/decisions`（及 `/{id}`，见 §9 与 `SCHEDULER_V1_SPEC.md`）。本文 §2 中 `traffic` / `errors` / `providers` 等信息架构页面由上述 throughput/error/latency/concurrency 等读模型接口拼装，而非各自独立路由。

已实现的 SLO 写接口使用控制台 Cookie + CSRF；SLO objective 可输入 `0.995` 或 `99.5`，响应和持久化统一为 `0.995`。`ops/settings` 使用 `PUT`。

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

## 13. 实现状态

已实现（Done）：

- Request ID、trace/log correlation（§3.3.1）。
- Usage Log、Scheduler Decision 查询（`/api/v1/admin/scheduler/overview`、`/api/v1/admin/scheduler/decisions`）。
- Provider error class 与错误归因（§4）、basic latency 和 token usage。
- `/metrics`（Prometheus）含 Gateway request duration / failover / provider probe latency / scheduler cost score / scheduler strategy / ops alert 等 histogram 与 counter（§3.1–§3.3）。
- Ops Dashboard 运维中心 `/admin/ops`（§2、§11）。
- SLO 控制面（`obs_slo_definitions` + `GET/POST/PATCH /api/v1/admin/ops/slo`，实时 availability 评估）。
- 告警事件 + ack、通用指标告警规则与静默匹配器的控制面 CRUD（`obs_alert_events` / `obs_alert_rules` / `obs_alert_silences`，§7、§10、§11）。
- Provider/账号健康快照与可用率聚合（`account_health_snapshots`、`account_availability_rollups`，§8、§10）。
- Realtime slot 诊断视图（§3.6、§11）。
- OpenTelemetry trace 导出，含 OTLP collector smoke、Jaeger / Tempo opt-in 可视化 smoke 和 trace-overhead p99 guard（§3.3.1）。

Roadmap / 尚未实现（Phase 2，验证：无对应 schema/模块，见 §10）：

- AI usage/成本聚合 rollup（`obs_ai_usage_rollups`）。
- 独立错误指纹聚合表（`obs_error_fingerprints`；当前 fingerprint 为告警事件内联字段）。
- 控制面内置外部告警通知通道与投递记录（email/webhook/钉钉/飞书/企业微信；当前由 Prometheus + Alertmanager 部署侧承担，§3.3 末尾）。
