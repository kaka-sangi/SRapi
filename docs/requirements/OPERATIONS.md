# SRapi 运维与生产治理规范

## 1. 目标

本文档定义 SRapi 可自托管生产部署所需的运维能力，覆盖配置、迁移、备份恢复、健康检查、发布门禁、日志脱敏、数据生命周期和事故处理。下文各节均为当前已落地实现，本文档作为生产运维参考。

SRapi 的运维设计原则：

```txt
可恢复 > 可观测 > 可升级 > 可回滚 > 可自动化
```

## 2. 适用范围

本文档约束：

- Docker Compose 与单机部署。
- 后续 Kubernetes 部署。
- PostgreSQL / Redis 生命周期。
- Gateway、Scheduler、Provider Adapter、Reverse Proxy Runtime 的运行诊断。
- 支付、订阅、返利、用量、审计等高价值数据。

目录、配置和数据模型已按本文档要求落地；新增能力时必须沿用此处约定的扩展点。

## 3. 运维端点

生产部署必须区分 liveness、readiness 和 metrics。

```txt
GET /livez
GET /readyz
GET /metrics
GET /api/v1/health
```

| 端点 | 用途 | 认证 | 行为 |
| --- | --- | --- | --- |
| `/livez` | 进程存活探针 | 无 | 只要 HTTP server 可响应即返回 200。 |
| `/readyz` | 就绪探针 | 无 | 必须检查 PostgreSQL、Redis、关键 worker 初始化状态。 |
| `/metrics` | Prometheus 指标 | 默认无，生产可由反向代理限制 | 输出 Prometheus text format。 |
| `/api/v1/health` | 兼容健康检查 | 无 | 返回 request_id、版本、基础依赖状态。 |

`/readyz` 不得在数据库不可用、迁移未完成、Redis 连接失败、关键配置缺失时返回 200。

当前实现：

- `/livez` 只验证 HTTP server 可响应。
- `/readyz` 只接受注入的真实依赖 pinger：PostgreSQL 执行 `SELECT 1`，Redis 执行 `PING`。缺少 pinger 会返回 `not_configured`，不会用 TCP 端口可连假定依赖可用。
- 默认 `STORAGE_BACKEND=postgres` 要求启动时 PostgreSQL 可用；只有显式设置 `STORAGE_BACKEND=memory` 才会进入临时内存模式。
- release 模式在启动前拒绝弱 `JWT_SECRET`、`SRAPI_MASTER_KEY`、`API_KEY_PEPPER`、`DATABASE_PASSWORD` 和默认 `BOOTSTRAP_ADMIN_PASSWORD`。
- 容器健康检查必须调用 `/readyz`。`/srapi -healthcheck -healthcheck-path=/readyz` 是 distroless API 镜像内置的 readiness probe，不依赖 curl/wget。

## 3.1 本地环境 bootstrap

本地或单机 Docker Compose 部署前先运行：

```bash
make bootstrap-env
```

该命令使用 `tools/bootstrap-env.mjs` 从 `.env.example` 创建 `.env`，并为 `DATABASE_PASSWORD`、`JWT_SECRET`、`SRAPI_MASTER_KEY`、`API_KEY_PEPPER` 和 `BOOTSTRAP_ADMIN_PASSWORD` 生成强随机本地值。已有 `.env` 会原样保留，避免覆盖部署者轮换后的 secret；生成值不会写入终端日志，`.env` 文件权限会设置为 owner-only。生产环境仍应把这些 secret 放入平台 secret manager，并独立备份 `SRAPI_MASTER_KEY` 或 KMS 引用。

已有环境可以离线审计：

```bash
make env-check
```

该检查会拒绝缺失关键 secret、`.env.example` 弱占位值、长度过短的本地 secret，以及 group/other 可读的 `.env` 文件权限。可用 `SRAPI_ENV_CHECK_FILE=/path/to/env make env-check` 检查非默认 env 文件。

启动 Compose 或交给运维接手前可以运行部署预检：

```bash
make deploy-preflight
```

该预检会复用 `make env-check` 和 `make observability-rules-check` 的核心约束，确认 `.env.example`、`deploy/docker-compose.yml`、Prometheus/Alertmanager 配置、备份/恢复 Make targets、release smoke 和 observability profile 仍在位，并检查本机是否能找到 Docker Compose、`pg_dump`、`pg_restore`、`sha256sum` 和 `curl`。默认情况下，缺失本机工具只输出 warning，方便在无 Docker 的 CI 或开发机上继续检查静态部署配置；发布流水线可以设置 `SRAPI_DEPLOY_PREFLIGHT_STRICT_TOOLS=1` 把这些 warning 提升为失败。可用 `SRAPI_DEPLOY_PREFLIGHT_ENV_FILE=/path/to/env make deploy-preflight` 检查非默认 env 文件。

## 4. 启动就绪门禁

HTTP listener 可以启动，但 Gateway 流量不得在以下条件满足前进入主流程：

- 配置加载成功。
- PostgreSQL 连接成功。
- 数据库迁移状态可判定。
- Redis 连接成功。
- 加密主密钥可用。
- API Key、Provider Account、Scheduler 所需 repository 初始化成功。
- Reverse Proxy Runtime 的 credential decryptor、cookie jar store、HTTP client factory 初始化成功。

如果启动预算耗尽，进程必须 fail fast，不得半初始化对外服务。

### 4.1 Redis 连接护栏

Redis 承载 gateway rate limit、并发槽、scheduler lease 和 session affinity。生产部署必须显式控制单副本连接数和 I/O 超时，避免 `replicas * GOMAXPROCS` 推高 Redis 连接数。

| 环境变量 | 默认 | 用途 |
| --- | --- | --- |
| `REDIS_POOL_SIZE` | `32` | 每个 API 副本的最大 Redis 连接池大小。 |
| `REDIS_MIN_IDLE_CONNS` | `4` | 每个副本预热 idle 连接数，不得大于 `REDIS_POOL_SIZE`。 |
| `REDIS_DIAL_TIMEOUT_SECONDS` | `3` | 建连超时。 |
| `REDIS_READ_TIMEOUT_SECONDS` | `2` | Redis 读超时。 |
| `REDIS_WRITE_TIMEOUT_SECONDS` | `2` | Redis 写超时。 |
| `REDIS_POOL_TIMEOUT_SECONDS` | `3` | 等待连接池可用的超时。 |

容量估算：`api_replicas * REDIS_POOL_SIZE` 应低于 Redis 实例 `maxclients` 和代理层连接上限，并给运维客户端、Prometheus exporter、备份/故障切换连接预留余量。默认值是有界生产基线，最终应按 Redis 实例规格和 gateway QPS 复核。

Release 模式初始化 Redis 依赖时会有限重试短暂 `PING` 失败，再决定是否 fail fast；这只处理瞬时启动抖动，不会把长期不可用的 Redis 伪装成可用。

## 4.2 多副本与 HA 基线

SRapi 支持受控多副本 API 部署。当前代码已确认 batch16 worker leader-gate 就位：`internal/platform/leadergate` 使用 PostgreSQL `pg_try_advisory_lock`，`internal/app` 将 worker guard 注入周期 worker，worker 通过 `runonceguard` 执行一轮任务。没有该前置时不得把 API 扩到 `replicas > 1`。

多副本上线前置：

- API Deployment readiness probe 指向 `/readyz`。
- Redis 连接预算按 `replicas * REDIS_POOL_SIZE` 审核。
- PostgreSQL `max_connections` 按 `replicas * DATABASE_MAX_OPEN_CONNS` 审核。
- Prometheus 按 pod/job 维度抓取 `/metrics`，滚动升级期间关注 `up{job="srapi-api"}`、ready pod 数、Redis `PING` 延迟和 PostgreSQL 连接池饱和。
- 生产 PostgreSQL 使用托管服务或具备自动备份 + PITR 的 HA 方案；单容器本地卷只适合开发和小型试用。
- 生产 Redis 使用托管 Redis、Sentinel 或等价 failover；本地单 Redis 容器没有 HA。

仓库提供 `deploy/k8s/api-deployment.yaml` 与 `deploy/k8s/api-hpa.yaml` 作为可演进骨架。它们不是完整 Helm chart；生产环境仍需接入 secret manager、ingress、network policy、pod disruption budget 和托管数据面。

## 5. 迁移与回滚

### 5.1 迁移要求

数据库迁移必须满足：

- 可在空库执行。
- 可重复执行或具备明确幂等保护。
- 高风险迁移必须有回滚策略。
- 修改高增长表时必须评估锁表时间。
- 账务、支付、审计、用量表不得通过破坏性迁移丢失历史数据。

### 5.2 回滚分级

| 类型 | 要求 |
| --- | --- |
| 可逆迁移 | 必须提供 down migration。 |
| 条件可逆迁移 | 必须说明回滚前置条件和数据损失范围。 |
| 不可逆迁移 | 必须登记原因、恢复路径和备份要求。 |

### 5.3 发布前门禁

发布前至少执行：

```txt
OpenAPI lint / bundle / codegen check
数据库迁移 dry-run
后端测试
前端 typecheck
secret scan
Docker image smoke test
```

本仓库发布 smoke 入口：

```bash
make smoke-release
```

该 smoke 在已启动的 API 上检查 `/livez`、`/readyz`、`/metrics` 基线指标、管理员登录、API Key 创建，以及本地 mock Gateway 的 `/v1/models`、Chat Completions、Responses、Messages 闭环。

Gateway 限流 smoke 可单独执行：

```bash
make smoke-rate-limit
```

该 smoke 会创建 `rpm_limit=1` 的临时 Gateway API Key，调用一次 `/v1/chat/completions` 并确认成功，再调用第二次并断言返回 429、`rate_limit_error` / `rpm_limit_exceeded` 和 `Retry-After`。该检查要求 API 进程已连接 Redis-backed rate limiter；本地模式 Redis 不可用时会失败而不是静默跳过。

Gateway 限流 Redis p99 guard 可单独执行：

```bash
make rate-limit-bench RATE_LIMIT_BENCH_REDIS_ADDR=127.0.0.1:6379
```

该 guard 会先检查 Redis `PING` p99 基线，再对真实 Redis 运行 `internal/platform/ratelimit` 的 Allow、AcquireConcurrency 和 ReleaseConcurrency 热路径，并要求各自 p99 不超过 2ms。可用 `RATE_LIMIT_BENCH_SAMPLES` 调整采样数，`RATE_LIMIT_BENCH_BUDGET_MS` 调整预算，`RATE_LIMIT_BENCH_REDIS_DB` 指向可清理的测试 DB。

Gateway 跨供应商故障转移 smoke 可单独执行：

```bash
make smoke-failover
```

该 smoke 会创建两个临时 OpenAI-compatible Provider、同一个临时模型映射和两个本地 mock upstream。Primary upstream 固定返回 503，Gateway 应自动切换到 Secondary upstream 并返回成功响应；随后 smoke 会断言 `usage_logs` 出现失败/成功两条 attempt、第二个 SchedulerDecision 通过 `fallback_from_decision_id` 链到第一个 decision、`fallback_excluded` 证据存在，并且 `/metrics` 暴露正数 `srapi_gateway_failover_total`。

支付渠道闭环 smoke 可按渠道单独执行：

```bash
STRIPE_SMOKE_SECRET_KEY=... STRIPE_SMOKE_WEBHOOK_SECRET=... make smoke-payment-stripe

ALIPAY_SMOKE_APP_ID=... \
ALIPAY_SMOKE_PRIVATE_KEY='-----BEGIN RSA PRIVATE KEY-----...' \
ALIPAY_SMOKE_ALIPAY_PUBLIC_KEY='-----BEGIN PUBLIC KEY-----...' \
make smoke-payment-alipay

WECHAT_SMOKE_APP_ID=... \
WECHAT_SMOKE_MCH_ID=... \
WECHAT_SMOKE_API_V3_KEY=... \
WECHAT_SMOKE_SERIAL_NO=... \
WECHAT_SMOKE_PRIVATE_KEY='-----BEGIN RSA PRIVATE KEY-----...' \
make smoke-payment-wechat
```

每个 smoke 需要已启动的 API、管理员账号和对应渠道的测试/沙箱凭证；它们会创建临时 provider instance、走一遍用户下单 + 本地签名 webhook 回调闭环（确认订单 fulfilled、webhook 幂等、余额按配置金额入账且不二次入账），并在退出前禁用临时 provider。本地签名模式只验证 SRapi webhook 链路，不能替代渠道真实异步通知演练。各渠道所需的全部 `*_SMOKE_*` 环境变量、可选 webhook 模式开关和逐项断言以 `PAYMENT_SPEC.md` §13（测试要求，含 §4 渠道说明）为准，避免两处描述漂移。

OpenTelemetry 到 Jaeger 的可视化链路可单独执行：

```bash
make smoke-jaeger-trace
```

该 smoke 会临时启动官方 Jaeger all-in-one 容器（默认 `jaegertracing/all-in-one:1.76.0`），映射 Query/UI `16686` 和 OTLP/gRPC `4317`，通过 SRapi OTLP exporter 写入一条测试 span，再调用 Jaeger Query API `/api/traces/{trace_id}` 查回同一个 trace。测试结束会删除临时容器。它用于本地验证 Jaeger collector/query/UI 后端可见性；部署环境中的 Tempo、托管 collector 或多节点拓扑仍应按实际地址单独 smoke。

OpenTelemetry 到 Tempo 的可视化链路可单独执行：

```bash
make smoke-tempo-trace
```

该 smoke 会临时启动官方 Tempo 容器（默认 `grafana/tempo:2.9.0`），加载 `deploy/tempo-smoke.yaml`，映射 Query `13201` 和 OTLP/gRPC `14318` 到容器内 `3200` / `4317`，通过 SRapi OTLP exporter 写入一条测试 span，再调用 Tempo Query API `/api/v2/traces/{trace_id}` 查回同一个 trace。测试结束会删除临时容器。端口可通过 `TEMPO_QUERY_PORT`、`TEMPO_OTLP_PORT` 覆盖；部署环境中的托管 collector 或多节点 Tempo 拓扑仍应按实际地址单独 smoke。

## 6. 备份与恢复

### 6.1 备份对象

必须纳入备份：

- PostgreSQL。
- 加密主密钥或 KMS 引用配置。
- Provider Account 凭证密文。
- Reverse Proxy cookie jar / device fingerprint 密文。
- 支付服务商配置密文。
- S3 或对象存储中的备份元数据。

Redis 只保存可重建运行时状态，默认不作为长期备份对象。

### 6.2 备份策略

建议支持：

- 手动备份。
- 定时备份。
- S3-compatible 对象存储。
- retain-days / retain-count 保留策略。
- 备份校验和。
- 恢复前互斥锁，防止恢复期间继续写入。

当前单机 PostgreSQL 手动备份入口：

```bash
make backup-postgres BACKUP_FILE=backups/srapi-$(date +%Y%m%d%H%M%S).dump
```

该命令使用 `pg_dump --format custom` 生成备份，并写入同名 `.sha256` 校验文件。`SRAPI_MASTER_KEY`、外部 KMS key id、对象存储凭证等不写入备份文件，必须由部署者在 secret manager 或离线密钥库中独立备份。

单机 crontab 示例：

```cron
17 2 * * * cd /opt/srapi && /usr/bin/make backup-postgres BACKUP_FILE=backups/srapi-$(date +\%Y\%m\%d\%H\%M\%S).dump >> logs/backup-postgres.log 2>&1
```

Kubernetes CronJob 骨架见 `deploy/k8s/postgres-backup-cronjob.yaml`。生产应把备份文件同步到对象存储，并在同一保留策略内保存 `.sha256` 校验文件和恢复演练记录。

### 6.3 恢复要求

恢复流程必须确保：

- 恢复前停止 Gateway 写流量。
- 恢复后重新验证迁移版本。
- 恢复后清理 Redis 可重建状态。
- 恢复后重新构建 Scheduler snapshot。
- 恢复后验证管理员登录、API Key 鉴权、一次 mock Gateway 请求。

当前单机 PostgreSQL 恢复入口：

```bash
make restore-postgres BACKUP_FILE=backups/srapi-20260522120000.dump
make smoke-release
```

恢复前必须停止 Gateway 写流量和后台 worker。恢复后应重启 API 以重建 Redis scheduler lease / cache 状态，并重新运行迁移检查与 release smoke。

最小恢复验证流程：

1. 在隔离数据库上执行 `make restore-postgres BACKUP_FILE=...`。
2. 清空同环境 Redis 可重建状态，重启 API。
3. 执行迁移检查或启动 release 模式确认迁移版本可判定。
4. 运行 `make smoke-release`，确认 `/readyz`、管理员登录、API Key 创建和一次 mock gateway 请求通过。
5. 抽查一条用户余额、支付订单、usage log、provider account 密文记录，确认核心数据可读且密文未泄露。

## 7. 数据生命周期矩阵

| 数据集 | 所有者 | 保留策略 | 特殊要求 |
| --- | --- | --- | --- |
| `usage_logs` | Observability / Billing | 可配置，默认 90 天原始日志 | 可聚合后归档。 |
| `scheduler_decisions` | Scheduler | 可配置，默认 30-90 天 | 保留 score breakdown 用于诊断。 |
| `scheduler_feedbacks` | Scheduler | 可配置，默认 90 天 | 用于账号健康计算。 |
| `quality_eval_samples` | Scheduler / QualityEval | 短期；生产应按合规要求单独清理 | 仅在 `QUALITY_EVAL_ENABLED=true` 时写入，payload 为 AES-GCM 加密脱敏文本摘要。 |
| `quality_evaluations` | Scheduler / QualityEval | 可按评估窗口保留，默认调度只读最近 30 天 | 不含 prompt/output 明文，用于 account+model 质量聚合。 |
| `billing_ledger` | Billing | 永久 | 只追加，不软删。 |
| `payment_orders` | Payments | 长期保留 | Webhook、退款、争议需要追溯。 |
| `payment_audit_logs` | Payments | 长期保留 | 不得包含明文密钥。 |
| `affiliate_ledger` | Affiliate | 长期保留 | 退款补偿必须追加反向记录。 |
| `audit_logs` | Audit | 默认 180-365 天 | 高风险操作必须可追溯。 |
| `idempotency_records` | Platform | TTL 清理 | 过期后可删除。 |
| `account_health_snapshots` | Scheduler / Ops | 聚合后清理 | 可用于容量规划。 |
| `content_moderation_logs` | Risk Control | 可配置短保留 | prompt excerpt 视为敏感数据。 |
| `backup_records` | Operations | 与备份对象一致 | 删除备份时同步记录状态。 |

当前 retention worker 在持久化 store 可用时随 API 进程启动，每 24 小时清理一次：

- `usage_logs`: `DATA_RETENTION_USAGE_LOGS_DAYS`，默认 90。
- `scheduler_decisions`: `DATA_RETENTION_SCHEDULER_DECISIONS_DAYS`，默认 90。
- `scheduler_feedbacks`: `DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS`，默认 90。
- `audit_logs`: `DATA_RETENTION_AUDIT_LOGS_DAYS`，默认 365。
- `account_health_snapshots`: `DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS`，默认 90。

`auth_session_cleanup` worker 在持久化 AuthSession store 可用时随 API 进程启动，默认每 24 小时运行一次。它只处理 `expires_at <= now` 的 `active` 控制台 session，将其标记为 `expired` 并设置 `deleted_at`；登出产生的 `revoked` session 保持原状态。相关配置项为 `AUTH_SESSION_CLEANUP_INTERVAL_SECONDS`。

可选 gateway 请求转储通过 `SRAPI_REQUEST_LOG_ENABLED=true` 开启，目录由
`SRAPI_REQUEST_LOG_DIR` 控制，默认 `./logs/gateway`。该目录只存放
`request-*` / `error-*` 受控文件；后台 cleaner 按文件年龄、error 文件数量和总
managed 文件大小三层约束清理。`SRAPI_REQUEST_LOG_MAX_TOTAL_MB` 默认 512 MiB，
`0` 使用默认值，负数禁用总大小上限。该能力用于短期排障，不能替代结构化
`ops_system_logs`、`ops_error_logs` 和 `usage_logs`。

### 余额扣费

`balance_charger` worker 由 `internal/app` 在持久化 usage charge store 可用时启动。它默认每 1 分钟扫描未扣费且成功的 `usage_logs`，按 user/currency 聚合为 `billing_ledgers`，扣减用户余额并标记 `charged_at`。

相关配置项为 `BALANCE_CHARGER_INTERVAL_SECONDS`、`BALANCE_CHARGER_BATCH_LIMIT` 和 `BALANCE_CHARGER_MAX_BATCHES_PER_RUN`。默认每轮处理 `500 * 20 = 10,000` 条 pending usage，用于覆盖 10k usage/min 的生产 backlog 目标；单个 batch 仍在 Billing store 事务内完成 ledger、balance 和 usage 标记，确保吞吐配置不改变账务一致性边界。

生产相邻 PostgreSQL 压测使用 opt-in gate：

```bash
make balance-charger-pressure BALANCE_CHARGER_PRESSURE_DSN='postgres://user:pass@127.0.0.1:5432/srapi?sslmode=disable'
```

该测试会在目标 database 内创建临时 schema，写入 10,000 条成功且未扣费的 usage logs，通过真实 Ent billing store 和 `balance_charger` worker 一轮扣费，然后校验 `charged_at`、billing ledger 批次和用户余额。测试结束会删除临时 schema；不要把 DSN 写入仓库或命令历史共享日志。

### OpenTelemetry 开销门禁

OpenTelemetry HTTP tracing 的 p99 开销使用 opt-in gate 验证：

```bash
make otel-overhead-bench
```

该测试在同一进程内分别构建 no-op tracer provider 和 batch tracer provider 的 HTTP runtime，对 `/livez` 执行预热和采样请求，比较 p99 延迟增量，默认要求不超过 5ms。可通过 `OTEL_OVERHEAD_SAMPLES`、`OTEL_OVERHEAD_WARMUP`、`OTEL_OVERHEAD_BUDGET_MS` 和 `OTEL_OVERHEAD_TIMEOUT` 调整采样量、预热量、预算和测试超时。

该门禁不进入默认 `make check`，避免普通开发机抖动阻断提交；发布前、OpenTelemetry SDK/exporter 升级后、采样策略或 HTTP tracing middleware 变更后应显式运行。

### 账号健康探测

`health_probe` worker 由 `internal/app` 在持久化 account/provider store 可用时启动。它默认每 5 分钟遍历活跃 API-key provider account，调用上游 `/models` 类轻量端点，写入 `account_health_snapshots`，并在连续失败或错误率过高时给账号写入 cooldown / circuit metadata。相关配置项为 `ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS`、`ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS`、`ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT`、`ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD`、`ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT`、`ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE` 和 `ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS`。

Provider Account metadata 或 Provider config/capabilities 可以把默认 `/models` 探测提升为合成请求探测：

- `health_probe_url` / `probe_url` 指定完整探测 URL；未指定时仍使用 `{base_url}/models`。
- `health_probe_method` / `probe_method` 支持 `GET`、`HEAD`、`POST`；默认 `GET`。
- `health_probe_body` / `probe_body` 提供 JSON body；有 body 时自动补 `Content-Type: application/json`。
- `health_probe_headers` / `probe_headers` 可以添加低敏业务 header；`Authorization`、API-key、cookie、host、hop-by-hop header 会被忽略，凭证仍只来自账号 credential。
- `health_probe_expected_status_codes` / `probe_expected_status_codes` 限制成功 HTTP status；未配置时只接受 2xx。
- `health_probe_response_path` / `probe_response_path` 要求 JSON path 存在且非空；数组索引用数字段，例如 `data.0.id`。
- `health_probe_response_contains` / `probe_response_contains` 要求响应体包含指定文本。

该机制用于覆盖 sub2api channel-monitor / scheduled-test 里的“用真实请求形态做健康验证”诉求，但仍复用 SRapi 的 Provider Account、credential materialization、account health snapshot、cooldown/circuit 和 Scheduler 证据链，不新增平行监控数据面。

### SLO 告警评估

`slo_evaluator` worker 由 `internal/app` 在持久化 operations store 可用时启动。它默认每 1 分钟读取 `obs_slo_definitions`、`usage_logs` 和当前 active `obs_alert_events`，对 active availability SLO 执行多窗口 burn-rate 判断：长窗口和短窗口都超过阈值才创建或更新告警，恢复时只自动 resolve `slo.burn_rate.*` 规则生成的 active/acknowledged 告警，不会修改人工或其他规则告警。

默认短窗口为长窗口的 1/12，且不低于 1 分钟；SLO 阈值显式配置 `short_window_seconds` / `long_window_seconds` 时优先生效。相关配置项为 `SLO_EVALUATOR_INTERVAL_SECONDS` 和 `SLO_EVALUATOR_TIMEOUT_SECONDS`。告警 details 只保存 SLO id/name、severity、窗口秒数、请求计数、bad request 计数和 burn-rate 数值，不保存 API key、credential、prompt、request body、cookies 或 provider secret。

Prometheus alert rules 可以从 `deploy/prometheus-srapi-alerts.yaml` 加载。该文件基于 `srapi_ops_alert_events{severity,status}` 生成 critical 和 warning Ops posture 告警，labels 只保留低基数路由字段，排障说明和 runbook 放在 annotations。修改规则后运行：

```bash
make observability-rules-check
```

该检查会拒绝 API key、account id、user id、request id、fingerprint、rule id、prompt、credential、cookie 等高基数或敏感字段进入规则文件。

本地单机部署可以显式启用 Prometheus profile：

```bash
COMPOSE_PROFILES=observability make dev-up
```

该 profile 会用 `deploy/prometheus.yml` 抓取 API `/metrics`、加载 `deploy/prometheus-srapi-alerts.yaml`，并把触发的告警转发给同 profile 内的 Alertmanager。Prometheus 监听端口默认为 `9090`，可通过 `PROMETHEUS_PORT` 覆盖。Alertmanager 监听端口默认为 `9093`，可通过 `ALERTMANAGER_PORT` 覆盖。该 profile 是 opt-in，不影响默认 API/PostgreSQL/Redis 启动路径。

本地 Alertmanager notification route 位于 `deploy/alertmanager.yml`。默认 receiver `srapi-local-webhook` 会向宿主机 `http://host.docker.internal:19093/srapi/alerts` 发送 webhook，并设置 `send_resolved: true`，方便用本地调试接收器确认 firing 和 resolved 通知。生产部署应把 receiver 替换为实际的通知系统或中继，但 route grouping 只能保留 `service`、`severity`、`component` 这类固定低基数字段；不要把 alert id、fingerprint、rule id、API key、account id、user id、request id、prompt、credential、Authorization 或 cookie 放进 labels、grouping 或 webhook URL。

### QualityEval 在线评估

`quality_eval` worker 仅在持久化 store 可用且 `QUALITY_EVAL_ENABLED=true` 时启动。Gateway 成功完成文本请求并写入 `scheduler_feedbacks` 后，会捕获 content-safety 后的脱敏 prompt/output 摘要到 `quality_eval_samples.sample_payload_ciphertext`；禁用时不会新增样本。

worker 默认每小时按 `sample_request_hash` 稳定抽样 1% 未评估样本，调用 OpenAI-compatible Chat Completions judge model（默认 `gpt-4o-mini`）返回 `correctness` / `coherence` / `safety` 三项 0-5 分，写入 `quality_evaluations`。Scheduler Gateway 候选构建会按最近 30 天 `(account_id, model)` 平均分注入 `quality_score` / `quality_eval_score` / `quality_eval_samples` / `quality_tier`，使 decision score 中出现真实质量维度。

本地闭环可用 `make smoke-quality-eval` 验证：它使用内存 store 和本地 judge，覆盖 Gateway sample capture、worker evaluation 和 Scheduler decision quality evidence，不需要外部 judge API key。

生产启用前必须配置 `QUALITY_EVAL_OPENAI_API_KEY`，并确认 judge endpoint 的数据处理边界；该路径会把脱敏样本发送给外部评估模型。

保留天数为 `0` 时对应数据集不自动删除。账务、支付、affiliate ledger、provider credential 密文和用户核心状态不自动清理。

## 8. 日志与脱敏

日志默认不得输出：

```txt
API Key 原文
Provider API Key
OAuth access_token / refresh_token
Cookie
Authorization header
payment provider secret
JWT secret
TOTP secret
完整 prompt / messages / tool arguments
反代 device fingerprint
proxy credential
```

必须提供共享脱敏边界，HTTP handler、service、adapter、worker、audit writer 都不得各自实现不一致的脱敏逻辑。

## 9. 指标基线

`/metrics` 至少暴露：

```txt
srapi_gateway_requests_total
srapi_gateway_request_duration_seconds
srapi_gateway_request_duration_seconds_bucket
srapi_gateway_inflight_requests
srapi_gateway_errors_total
srapi_scheduler_decisions_total
srapi_provider_errors_total
srapi_provider_probe_latency_seconds
srapi_usage_tokens_total
srapi_reverse_proxy_ban_signals_total
```

AI Gateway 专项指标以 `OBSERVABILITY_SPEC.md` 为准。

当前 `/metrics` 使用 Prometheus client SDK 输出 text format，基于持久化 usage logs、scheduler decisions/leases、account health snapshots 和 Reverse Proxy Runtime 快照聚合；Gateway request duration 和 provider probe latency 暴露为 histogram，固定 bucket 为 `0.05`、`0.1`、`0.25`、`0.5`、`1`、`2.5`、`5`、`10` 秒和 `+Inf`。指标 label 只允许低基数 route/model/protocol/result/error/status 类字段，避免使用 API Key、用户邮箱、账号名、account id、prompt 或 credential。

HTTP server 启用 OpenTelemetry server span 和 W3C trace context 传播。默认不导出 trace；设置 `OTEL_TRACES_ENABLED=true` 后会通过 OTLP gRPC 发往 `OTEL_EXPORTER_OTLP_ENDPOINT`。`internal/platform/otel` 的本地 collector smoke 会启动进程内 OTLP gRPC receiver，验证 span 和 service resource attributes 能经真实 OTLP 协议 flush；`make smoke-jaeger-trace` 会把 span 写入真实 Jaeger all-in-one 后端并经 Query API 查回；`make smoke-tempo-trace` 会把 span 写入真实 Tempo 后端并经 Query API 查回。结构化日志 handler 会从 context 自动补充 `request_id`、`trace_id`、`user_id` 和 `api_key_id`，不得把原始 API Key、credential、prompt 或请求体加入日志字段。

AdminOps `ops_system_logs` 是可查询的低敏运维时间线。Gateway 上游尝试失败、no-available-account 决策、Gateway API key 认证失败以及 usage_log 写入失败必须写入该时间线；原始上游 body、prompt、header、cookie 和凭证不得进入该表，排障详情通过 `request_id` 关联 `ops_error_logs` 和请求转储文件。`gateway.auth` 认证失败事件只允许保存失败原因、入口路径、方法以及从合法 SRapi API key 格式中提取出的 `attempted_key_prefix`，不得保存完整 API key、Authorization header 或 secret 段。`RecordSystemLog` service 边界会统一清洗 `metadata`：敏感 key 会落为 `[REDACTED]`，长字符串和超大嵌套结构会截断，token 计数字段、`api_key_id`、`api_key_prefix` 和 `attempted_key_prefix` 这类低敏排障字段保留为诊断信号。

## 10. 安全运营

生产环境必须检查：

- 管理员密码不是默认值。
- JWT secret 或 RSA private key 固定且足够强。
- TOTP encryption key 固定且备份。
- Provider 凭证加密主密钥固定且可轮换。
- URL allowlist / SSRF 防护策略明确。
- 反代账号 cookie jar 和 device fingerprint 不跨账号复用。
- 管理后台不直接暴露公网，或启用强认证与 TLS。

## 11. 事故处理

必须支持按 request_id / trace_id 串联：

```txt
Gateway request
API key
user
scheduler decision
provider account
provider feedback
usage log
audit log
payment order
```

上游账号异常时，必须能区分：

- 普通 rate limit。
- 额度耗尽。
- 临时 5xx。
- session_invalid。
- account_locked。
- account_banned。
- geo_blocked。
- device_unrecognized。

反代账号状态迁移以 `REVERSE_PROXY_SPEC.md` 为准。

## 12. 生产运维能力基线

当前生产部署的运维能力基线（均已落地）：

- `/api/v1/health`。
- 基础 `/livez`、`/readyz`。
- 结构化日志和 request_id。
- secret scan。
- 数据库迁移验证。
- Docker Compose 本地启动。
- Provider 凭证和 API Key 脱敏测试。
- `/metrics`、PostgreSQL 手动备份/恢复入口、release smoke、基础数据生命周期清理、部署 preflight。
- SLO/告警控制面和 SLO burn-rate evaluator：SLO 定义和告警事件落库到 `obs_slo_definitions`、`obs_alert_events`。

告警通知投递、抑制规则和聚合 rollup 为 Roadmap（尚未实现），按 `OBSERVABILITY_SPEC.md` 后续展开。
