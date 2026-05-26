# SRapi 配置与环境变量规范

## 1. 目标

本文档定义 SRapi 配置系统、环境变量、默认值、安全约束和生产部署建议。

配置来源优先级建议：

```txt
explicit env > config file > database settings > built-in default
```

敏感配置不得通过前端 API 明文返回。

## 2. 配置分层

| 层级 | 示例 | 说明 |
| --- | --- | --- |
| 启动配置 | server、database、redis、crypto | 进程启动前必须确定。 |
| 运行配置 | scheduler、gateway、observability | 可由数据库 settings 管理，需版本化。 |
| Secret 配置 | JWT、OAuth、payment、proxy | 必须加密或通过 secret manager 注入。 |
| 用户配置 | pricing、model mappings、provider accounts | 由管理后台维护。 |

## 3. Server

```txt
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_MODE=release
SERVER_SHUTDOWN_TIMEOUT_SECONDS=45
SERVER_MAX_REQUEST_BODY_SIZE=268435456
STORAGE_BACKEND=postgres
```

要求：

- release 模式缺少关键 secret 必须启动失败。
- shutdown 必须等待 HTTP server、worker、inflight gateway request 有序退出。
- request body size 必须有全局上限和 Gateway 单独上限。
- `STORAGE_BACKEND=postgres` 是默认持久化路径；`STORAGE_BACKEND=memory` 只用于显式本地/测试的临时内存模式，`release` 模式禁止使用。

## 4. Logging

```txt
LOG_LEVEL=info
LOG_FORMAT=json
LOG_SERVICE_NAME=srapi
LOG_ENV=production
LOG_CALLER=true
LOG_OUTPUT_TO_STDOUT=true
LOG_OUTPUT_TO_FILE=false
LOG_ROTATION_MAX_SIZE_MB=100
LOG_ROTATION_MAX_BACKUPS=10
LOG_ROTATION_MAX_AGE_DAYS=7
LOG_ROTATION_COMPRESS=true
LOG_SAMPLING_ENABLED=false
```

日志必须通过统一脱敏器处理。

结构化日志 handler 会从 request context 自动追加安全字段：`request_id`、`trace_id`、`user_id`、`api_key_id`。这些字段用于运维串联，不得替代鉴权，也不得加入原始 API Key、credential、prompt、request body 或 provider secret。

## 4.1 OpenTelemetry

```txt
OTEL_SERVICE_NAME=srapi
OTEL_SERVICE_VERSION=dev
OTEL_ENVIRONMENT=local
OTEL_TRACES_ENABLED=false
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_TRACES_SAMPLE_RATIO=1
OTEL_BATCH_TIMEOUT_SECONDS=5
```

默认只安装本进程 tracer provider 和 HTTP server span，不向外部 collector 导出 trace。生产启用 trace 导出时必须设置 `OTEL_TRACES_ENABLED=true` 并指向内部 OTLP gRPC collector；`OTEL_TRACES_SAMPLE_RATIO` 必须在 `0` 到 `1` 之间。外部 collector、Jaeger、Tempo 等后端不应接收包含原始 prompt 或 credential 的 span attribute。

## 5. Database

```txt
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_USER=srapi
DATABASE_PASSWORD=
DATABASE_DBNAME=srapi
DATABASE_SSLMODE=disable
DATABASE_MAX_OPEN_CONNS=256
DATABASE_MAX_IDLE_CONNS=128
DATABASE_CONN_MAX_LIFETIME_MINUTES=30
DATABASE_CONN_MAX_IDLE_TIME_MINUTES=5
```

生产要求：

- `DATABASE_PASSWORD` 必填且不得为弱口令。
- 连接池上限必须按 PostgreSQL `max_connections` 和实例数计算。
- 迁移执行和应用启动要有清晰边界。

## 6. Redis

```txt
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_POOL_SIZE=1024
REDIS_MIN_IDLE_CONNS=10
REDIS_ENABLE_TLS=false
```

Redis 保存：

- API Key auth cache。
- Gateway API key/user RPM、API key TPM、provider account RPM/TPM rate limit counters，以及 API key / provider account concurrency ZSet leases；Redis 不可用时 release mode 必须启动失败，非 release mode 会禁用 Gateway rate limit。
- account lease。
- realtime WebSocket slot lifecycle。
- sticky session。
- circuit breaker state。
- short-term health window。

Redis 不得成为唯一真实来源。

## 7. Auth / JWT / TOTP

```txt
JWT_SIGNING_ALGORITHM=HS256
JWT_SECRET=
JWT_RSA_PRIVATE_KEY_PEM=
JWT_ACCESS_TOKEN_EXPIRE_MINUTES=60
TOTP_ENCRYPTION_KEY=
API_KEY_PEPPER=
BOOTSTRAP_ADMIN_EMAIL=admin@srapi.local
BOOTSTRAP_ADMIN_PASSWORD=
BOOTSTRAP_ADMIN_NAME=Admin
AUTH_SESSION_CLEANUP_INTERVAL_SECONDS=86400
```

规则：

- HS256 时 `JWT_SECRET` 至少 32 字节。
- RS256 时必须设置 RSA private key，并暴露 `/.well-known/jwks.json`。
- TOTP secret 必须用固定 AES-256 key 加密。
- 多副本部署不得使用随机临时 TOTP key。
- Gateway API Key HMAC pepper 必须固定且至少 32 字节。
- release 模式必须拒绝默认管理员密码和开发占位密码。
- 过期控制台 session 由 `auth_session_cleanup` worker 周期性标记为 `expired` 并软删除；该 worker 只在持久化 AuthSession store 可用时启动，默认每 24 小时运行一次。

## 8. Crypto

```txt
SRAPI_MASTER_KEY=
SRAPI_KMS_PROVIDER=none
SRAPI_KMS_KEY_ID=
```

加密对象：

- Provider credentials。
- cookie jar。
- device fingerprint。
- proxy URL。
- payment provider config。
- secret settings。

密文必须记录 version，以支持轮换。

## 9. Gateway

```txt
GATEWAY_MAX_BODY_SIZE=268435456
GATEWAY_REQUEST_TIMEOUT_SECONDS=600
GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS=120
GATEWAY_REALTIME_MAX_OPEN_SLOTS=0
GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY=0
GATEWAY_MAX_CONNS_PER_HOST=2048
GATEWAY_MAX_IDLE_CONNS=8192
GATEWAY_MAX_IDLE_CONNS_PER_HOST=4096
GATEWAY_FORCE_CODEX_CLI=false
```

`GATEWAY_FORCE_CODEX_CLI` 会影响所有 Responses 请求，只能作为部署级兜底开关。
Realtime slot limit 配置为 `0` 表示不启用该维度限制；生产部署建议按实例容量设置全局和单 API key 上限，防止长连接耗尽 Gateway 资源。
Redis 可用时 realtime slot lifecycle 使用 Redis-backed store，限额和 AdminOps active slot 视图跨 API 节点生效；local 模式 Redis 不可用时降级为内存 store，release 模式 Redis 不可用必须启动失败。

## 10. Scheduler

```txt
GATEWAY_SCHEDULING_STICKY_SESSION_MAX_WAITING=3
GATEWAY_SCHEDULING_STICKY_SESSION_WAIT_TIMEOUT=120s
GATEWAY_SCHEDULING_FALLBACK_MAX_WAITING=100
GATEWAY_SCHEDULING_FALLBACK_WAIT_TIMEOUT=30s
GATEWAY_SCHEDULING_LOAD_BATCH_ENABLED=true
GATEWAY_SCHEDULING_SLOT_CLEANUP_INTERVAL=30s
GATEWAY_SCHEDULING_DB_FALLBACK_ENABLED=true
GATEWAY_SCHEDULING_DB_FALLBACK_TIMEOUT_SECONDS=0
GATEWAY_SCHEDULING_DB_FALLBACK_MAX_QPS=0
GATEWAY_SCHEDULING_OUTBOX_POLL_INTERVAL_SECONDS=1
GATEWAY_SCHEDULING_FULL_REBUILD_INTERVAL_SECONDS=300
```

Scheduler snapshot / outbox 配置必须能防止缓存失效时全量打爆数据库。

## 11. QualityEval

```txt
QUALITY_EVAL_ENABLED=false
QUALITY_EVAL_INTERVAL_SECONDS=3600
QUALITY_EVAL_TIMEOUT_SECONDS=30
QUALITY_EVAL_BATCH_LIMIT=100
QUALITY_EVAL_SAMPLE_PERCENT=1
QUALITY_EVAL_JUDGE_MODEL=gpt-4o-mini
QUALITY_EVAL_JUDGE_TIMEOUT_SECONDS=20
QUALITY_EVAL_OPENAI_BASE_URL=https://api.openai.com/v1
QUALITY_EVAL_OPENAI_API_KEY=
```

规则：

- 默认关闭；启用时必须设置 `QUALITY_EVAL_OPENAI_API_KEY`，否则启动配置校验失败。
- 仅持久化 PostgreSQL store 会由 `internal/app` 启动 `quality_eval` worker。内存模式可用于测试 HTTP 捕获和服务逻辑，不作为生产评估闭环。
- worker 默认每小时从未评估的 `quality_eval_samples` 中按 `sample_request_hash` 稳定抽样 1%，单批最多 100 条，单条 judge 调用 30 秒总超时、20 秒上游 HTTP 超时。
- 样本捕获只保存 content-safety 之后的脱敏文本摘要，并以 `SRAPI_MASTER_KEY` 派生密钥加密；禁用时不新增样本，但已有 `quality_evaluations` 仍可继续作为 Scheduler quality 分数输入。

## 11.1 Payment Smoke

```txt
STRIPE_SMOKE_SECRET_KEY=
STRIPE_SMOKE_WEBHOOK_SECRET=
STRIPE_SMOKE_AMOUNT=1.00
STRIPE_SMOKE_CURRENCY=USD
STRIPE_SMOKE_PROVIDER_NAME=stripe-smoke

ALIPAY_SMOKE_APP_ID=
ALIPAY_SMOKE_PRIVATE_KEY=
ALIPAY_SMOKE_ALIPAY_PUBLIC_KEY=
ALIPAY_SMOKE_AMOUNT=1.00
ALIPAY_SMOKE_CURRENCY=CNY
ALIPAY_SMOKE_PROVIDER_NAME=alipay-smoke
ALIPAY_SMOKE_METHOD=alipay_smoke
ALIPAY_SMOKE_MODE=page
ALIPAY_SMOKE_GATEWAY_URL=
ALIPAY_SMOKE_NOTIFY_URL=
ALIPAY_SMOKE_RETURN_URL=
ALIPAY_SMOKE_LOCAL_WEBHOOK=0
ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY=
```

这些变量只供 `make smoke-payment-stripe` 使用，不参与 API 进程启动配置。`STRIPE_SMOKE_SECRET_KEY` 和 `STRIPE_SMOKE_WEBHOOK_SECRET` 应来自 CI secret 或临时 shell 环境；不要把真实 Stripe test-mode secret 长期写入共享 `.env`。脚本会把密钥写入临时 payment provider instance 的加密 config，运行结束禁用该 provider。

`ALIPAY_SMOKE_*` 只供 `make smoke-payment-alipay` 使用，不参与 API 进程启动配置。脚本会创建或更新临时 Alipay provider instance，验证 Page Pay RSA2 checkout URL，并在退出前禁用该 provider。`ALIPAY_SMOKE_LOCAL_WEBHOOK=1` 时必须提供 `ALIPAY_SMOKE_NOTIFY_PRIVATE_KEY`；脚本会从该私钥派生临时验签公钥写入 provider config，再提交本地签名的 `TRADE_SUCCESS` 通知来验证 SRapi webhook 验签、`success` 应答、履约、余额入账和重复通知幂等。该模式不能替代支付宝沙箱真实回调演练。

## 12. Balance Charger

```txt
BALANCE_CHARGER_INTERVAL_SECONDS=60
BALANCE_CHARGER_BATCH_LIMIT=500
BALANCE_CHARGER_MAX_BATCHES_PER_RUN=20
```

规则：

- `balance_charger` 只在持久化 PostgreSQL store 可用时启动，把未扣费 `usage_logs` 聚合成 `billing_ledgers` 并标记 `charged_at`。
- worker 每轮最多处理 `BALANCE_CHARGER_BATCH_LIMIT * BALANCE_CHARGER_MAX_BATCHES_PER_RUN` 条 pending usage；默认值覆盖 10,000 usage/min 的规格目标。
- 单个 batch 仍按 user/currency 聚合并在 Billing store 事务内写 ledger、扣余额和标记 usage，避免把 worker 吞吐配置泄漏到账务一致性边界。

## 13. Reverse Proxy Runtime

```txt
REVERSE_PROXY_DEFAULT_CONNECT_TIMEOUT_SECONDS=30
REVERSE_PROXY_DEFAULT_IDLE_TIMEOUT_SECONDS=120
REVERSE_PROXY_COOKIE_JAR_ENCRYPTION_ENABLED=true
REVERSE_PROXY_DEVICE_FINGERPRINT_ENCRYPTION_ENABLED=true
REVERSE_PROXY_HEADER_HYGIENE_STRICT=true
REVERSE_PROXY_EGRESS_PROFILE_STRICT=false
```

高级 TLS / HTTP/2 指纹配置以 `REVERSE_PROXY_SPEC.md` 为准。

## 14. Observability

```txt
OPS_ENABLED=true
OPS_WS_MAX_CONNS=100
OPS_WS_MAX_CONNS_PER_IP=20
DASHBOARD_AGGREGATION_ENABLED=true
DASHBOARD_AGGREGATION_INTERVAL_SECONDS=60
DASHBOARD_AGGREGATION_RETENTION_USAGE_LOGS_DAYS=90
DASHBOARD_AGGREGATION_RETENTION_HOURLY_DAYS=180
DASHBOARD_AGGREGATION_RETENTION_DAILY_DAYS=730
DATA_RETENTION_USAGE_LOGS_DAYS=90
DATA_RETENTION_SCHEDULER_DECISIONS_DAYS=90
DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS=90
DATA_RETENTION_AUDIT_LOGS_DAYS=365
DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS=90
ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS=300
ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS=10
ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT=8
ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD=3
ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT=50
ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE=3
ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS=300
SLO_EVALUATOR_INTERVAL_SECONDS=60
SLO_EVALUATOR_TIMEOUT_SECONDS=30
```

运维实时 WebSocket 限额应作为部署级 circuit breaker。

数据保留天数为 `0` 时表示关闭对应表的自动清理。账务、支付、affiliate ledger 等追加账本不进入自动清理。

账号健康探测 worker 默认每 5 分钟探测一次活跃 API-key provider account，单个探测 10 秒超时，并发上限为 8。连续 3 次失败或最小样本数内错误率超过 50% 时写入 unhealthy 快照和 cooldown metadata，scheduler 会基于 `cooldown_active` / `circuit_open` 避开该账号。

SLO evaluator worker 默认每 1 分钟读取 `obs_slo_definitions` 与 `usage_logs`，用 availability SLO 的长/短窗口 burn-rate 阈值生成、更新或自动恢复 `obs_alert_events`。单次 evaluation 默认 30 秒超时；告警 details 只包含 SLO id/name、窗口秒数、请求计数和 burn-rate 数值，不包含 API key、credential、prompt、request body 或 provider secret。

## 15. Payment

```txt
PAYMENT_ENABLED=false
PAYMENT_ORDER_TIMEOUT_MINUTES=30
PAYMENT_MAX_PENDING_ORDERS_PER_USER=3
PAYMENT_DAILY_AMOUNT_LIMIT=
```

支付服务商密钥不得通过 env 明文长期管理，优先使用后台加密 settings 或 secret manager。

## 16. Security / URL Allowlist

```txt
SECURITY_URL_ALLOWLIST_ENABLED=false
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=false
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=false
SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS=
SECURITY_URL_ALLOWLIST_PRICING_HOSTS=
SECURITY_URL_ALLOWLIST_CRS_HOSTS=
```

所有自定义 upstream、pricing、CRS URL 必须经过 SSRF 防护。

## 17. OAuth Client Credentials

```txt
GEMINI_CLI_OAUTH_CLIENT_ID=
GEMINI_CLI_OAUTH_CLIENT_SECRET=
GEMINI_OAUTH_CLIENT_ID=
GEMINI_OAUTH_CLIENT_SECRET=
ANTIGRAVITY_OAUTH_CLIENT_ID=
ANTIGRAVITY_OAUTH_CLIENT_SECRET=
```

SRapi 不得在代码仓库中内置第三方 OAuth client_secret。

## 18. Update / External Fetch

```txt
UPDATE_PROXY_URL=
MODEL_CATALOG_UPDATE_PROXY_URL=
PRICING_UPDATE_PROXY_URL=
```

外部 fetch 必须遵守 URL allowlist / SSRF 规则。

## 19. 配置变更审计

以下配置变更必须写 audit log：

- JWT / TOTP / crypto 配置。
- Provider credential。
- Payment provider config。
- URL allowlist。
- Scheduler strategy。
- Reverse Proxy Egress Profile。
- Observability notification channel。

## 20. MVP 最小要求

MVP 至少提供：

- `.env.example`。
- 本地 `config.example.yaml`。
- 配置加载校验。
- release 模式弱 secret 拒绝启动。
- 配置文档和默认值说明。
