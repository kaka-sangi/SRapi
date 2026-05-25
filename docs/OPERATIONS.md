# SRapi 运维与生产治理规范

## 1. 目标

本文档定义 SRapi 从 MVP 走向可自托管生产部署时必须具备的运维能力，覆盖配置、迁移、备份恢复、健康检查、发布门禁、日志脱敏、数据生命周期和事故处理。

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

MVP 可以只实现最小子集，但目录、配置和数据模型必须为本文档预留扩展点。

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
- `/readyz` 通过注入 pinger 或 TCP fallback 检查 PostgreSQL 和 Redis。
- 默认 `STORAGE_BACKEND=postgres` 要求启动时 PostgreSQL 可用；只有显式设置 `STORAGE_BACKEND=memory` 才会进入临时内存模式。
- release 模式在启动前拒绝弱 `JWT_SECRET`、`SRAPI_MASTER_KEY`、`API_KEY_PEPPER`、`DATABASE_PASSWORD` 和默认 `BOOTSTRAP_ADMIN_PASSWORD`。

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

### 账号健康探测

`health_probe` worker 由 `internal/app` 在持久化 account/provider store 可用时启动。它默认每 5 分钟遍历活跃 API-key provider account，调用上游 `/models` 类轻量端点，写入 `account_health_snapshots`，并在连续失败或错误率过高时给账号写入 cooldown / circuit metadata。相关配置项为 `ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS`、`ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS`、`ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT`、`ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD`、`ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT`、`ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE` 和 `ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS`。

### QualityEval 在线评估

`quality_eval` worker 仅在持久化 store 可用且 `QUALITY_EVAL_ENABLED=true` 时启动。Gateway 成功完成文本请求并写入 `scheduler_feedbacks` 后，会捕获 content-safety 后的脱敏 prompt/output 摘要到 `quality_eval_samples.sample_payload_ciphertext`；禁用时不会新增样本。

worker 默认每小时按 `sample_request_hash` 稳定抽样 1% 未评估样本，调用 OpenAI-compatible Chat Completions judge model（默认 `gpt-4o-mini`）返回 `correctness` / `coherence` / `safety` 三项 0-5 分，写入 `quality_evaluations`。Scheduler Gateway 候选构建会按最近 30 天 `(account_id, model)` 平均分注入 `quality_score` / `quality_tier`，使 decision score 中出现真实质量维度。

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
srapi_gateway_inflight_requests
srapi_gateway_errors_total
srapi_scheduler_decisions_total
srapi_provider_errors_total
srapi_usage_tokens_total
srapi_reverse_proxy_ban_signals_total
```

AI Gateway 专项指标以 `OBSERVABILITY_SPEC.md` 为准。

当前 `/metrics` 使用 Prometheus text format，基于持久化 usage logs、scheduler decisions/leases 和 Reverse Proxy Runtime 快照聚合，避免使用 API Key、用户邮箱、账号名、prompt 或 credential 作为 label。

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

## 12. MVP 最小要求

MVP 必须至少实现：

- `/api/v1/health`。
- 基础 `/livez`、`/readyz`。
- 结构化日志和 request_id。
- secret scan。
- 数据库迁移验证。
- Docker Compose 本地启动。
- Provider 凭证和 API Key 脱敏测试。

Phase 2 起必须补齐 `/metrics`、备份恢复、发布 smoke、数据生命周期清理和 SLO 告警。

当前 Phase 2 已补齐 `/metrics`、PostgreSQL 手动备份/恢复入口、release smoke、基础数据生命周期清理，以及 SLO/告警控制面 v1。SLO 定义和告警事件落库到 `obs_slo_definitions`、`obs_alert_events`；告警通知、抑制规则和聚合 rollup 仍按 `OBSERVABILITY_SPEC.md` 后续展开。
