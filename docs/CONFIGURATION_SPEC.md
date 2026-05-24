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
```

要求：

- release 模式缺少关键 secret 必须启动失败。
- shutdown 必须等待 HTTP server、worker、inflight gateway request 有序退出。
- request body size 必须有全局上限和 Gateway 单独上限。

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
- Gateway API key/user RPM 和 API key TPM rate limit counters；Redis 不可用时 release mode 必须启动失败，非 release mode 会禁用 Gateway rate limit。
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
```

规则：

- HS256 时 `JWT_SECRET` 至少 32 字节。
- RS256 时必须设置 RSA private key，并暴露 `/.well-known/jwks.json`。
- TOTP secret 必须用固定 AES-256 key 加密。
- 多副本部署不得使用随机临时 TOTP key。
- Gateway API Key HMAC pepper 必须固定且至少 32 字节。
- release 模式必须拒绝默认管理员密码和开发占位密码。

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

## 11. Reverse Proxy Runtime

```txt
REVERSE_PROXY_DEFAULT_CONNECT_TIMEOUT_SECONDS=30
REVERSE_PROXY_DEFAULT_IDLE_TIMEOUT_SECONDS=120
REVERSE_PROXY_COOKIE_JAR_ENCRYPTION_ENABLED=true
REVERSE_PROXY_DEVICE_FINGERPRINT_ENCRYPTION_ENABLED=true
REVERSE_PROXY_HEADER_HYGIENE_STRICT=true
REVERSE_PROXY_EGRESS_PROFILE_STRICT=false
```

高级 TLS / HTTP/2 指纹配置以 `REVERSE_PROXY_SPEC.md` 为准。

## 12. Observability

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
```

运维实时 WebSocket 限额应作为部署级 circuit breaker。

数据保留天数为 `0` 时表示关闭对应表的自动清理。账务、支付、affiliate ledger 等追加账本不进入自动清理。

## 13. Payment

```txt
PAYMENT_ENABLED=false
PAYMENT_ORDER_TIMEOUT_MINUTES=30
PAYMENT_MAX_PENDING_ORDERS_PER_USER=3
PAYMENT_DAILY_AMOUNT_LIMIT=
```

支付服务商密钥不得通过 env 明文长期管理，优先使用后台加密 settings 或 secret manager。

## 14. Security / URL Allowlist

```txt
SECURITY_URL_ALLOWLIST_ENABLED=false
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=false
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=false
SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS=
SECURITY_URL_ALLOWLIST_PRICING_HOSTS=
SECURITY_URL_ALLOWLIST_CRS_HOSTS=
```

所有自定义 upstream、pricing、CRS URL 必须经过 SSRF 防护。

## 15. OAuth Client Credentials

```txt
GEMINI_CLI_OAUTH_CLIENT_ID=
GEMINI_CLI_OAUTH_CLIENT_SECRET=
GEMINI_OAUTH_CLIENT_ID=
GEMINI_OAUTH_CLIENT_SECRET=
ANTIGRAVITY_OAUTH_CLIENT_ID=
ANTIGRAVITY_OAUTH_CLIENT_SECRET=
```

SRapi 不得在代码仓库中内置第三方 OAuth client_secret。

## 16. Update / External Fetch

```txt
UPDATE_PROXY_URL=
MODEL_CATALOG_UPDATE_PROXY_URL=
PRICING_UPDATE_PROXY_URL=
```

外部 fetch 必须遵守 URL allowlist / SSRF 规则。

## 17. 配置变更审计

以下配置变更必须写 audit log：

- JWT / TOTP / crypto 配置。
- Provider credential。
- Payment provider config。
- URL allowlist。
- Scheduler strategy。
- Reverse Proxy Egress Profile。
- Observability notification channel。

## 18. MVP 最小要求

MVP 至少提供：

- `.env.example`。
- 本地 `config.example.yaml`。
- 配置加载校验。
- release 模式弱 secret 拒绝启动。
- 配置文档和默认值说明。
