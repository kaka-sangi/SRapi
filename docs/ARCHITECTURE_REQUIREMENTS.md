# SRapi 架构要求与 Harness

## 1. 目标

本文档把 SRapi MVP 的架构要求改写成可执行门禁。

原则很简单：

```txt
架构要求先写清楚，harness 再把它卡住。
```

## 2. 必须满足的要求

### 2.1 启动入口

- `cmd/srapi/main.go` 必须保持薄入口。
- `cmd` 只能负责配置、日志、`internal/app` 组装、启动和优雅退出。
- `cmd` 不得直接依赖业务模块、Ent 查询、数据库实现或 HTTP handler 细节。
- `internal/app` 负责把启动所需对象装配成可运行进程。
- `internal/app` 可以装配 persistence 实现，但只能通过 store contract 注入 HTTP runtime，不得把 Ent query 泄漏给业务模块或 handler。
- `internal/app` 必须管理后台 worker 生命周期；worker 启动前必须拿到 contract/store 注入，关闭时必须先停止 worker，再关闭 HTTP、数据库和 Redis。

### 2.2 分层边界

- `internal/modules/*/contract` 只能依赖稳定 DTO、标准库和少量白名单模块 contract。
- `internal/modules/*/service` 只能通过 contract 访问别的模块。
- `internal/httpserver` 负责协议层，不得直接访问 Ent 或业务 repository。
- `internal/platform` 只提供基础设施能力，不得反向依赖业务模块。

### 2.3 数据与迁移

- PostgreSQL 是业务真实来源。
- Redis 只保存可重建状态。
- Ent schema 必须能应用到空库。
- Ent-backed repository 必须实现模块 contract，放在 `internal/persistence/entstore/*`，不得把 Ent import 泄漏回 `internal/modules/*`。
- 用户、API Key、Provider、Model、Account、Usage、Scheduler Decision / Feedback、Audit、Billing Ledger、Domain Events Outbox / Inbox 必须有 Ent-backed store。
- Domain Events Outbox 必须具备可测试的分发闭环：pending 可被 dispatcher 选择，成功后标记 published，失败后记录 attempt_count、last_error、next_retry_at，到期后允许重试。
- Domain Events Outbox worker 必须由 `internal/app` 在 persistent Event store 可用时启动；本地降级为内存 runtime 时不得启动持久 outbox worker。
- Scheduler Lease 和 realtime slot lifecycle 属于可重建运行时状态；release 模式必须使用 Redis-backed store，本地 Redis 不可用时才允许降级为内存 lease/slot store。
- 本地模式允许启动期应用 Ent schema 以支持一键开发；release 模式必须依赖已应用的正式迁移。
- `apps/api/migrations` 必须保留为版本化迁移目录。
- PostgreSQL release 迁移必须和 Ent schema 保持一致；修改 `apps/api/ent/schema` 后必须先运行 `make ent-generate`，再运行 `make migration-diff MIGRATION_NAME=00000N_subject` 生成下一条 up migration。
- 每个 release 迁移必须提供 down migration；初始迁移的 down migration 必须覆盖当前 Ent table list。
- `apps/api/atlas.hcl` 是 Ent -> Atlas -> PostgreSQL migration diff 的项目配置；`make migration-diff` 和 `make migration-hash` 必须保留显式 pin 的 Atlas CLI。

### 2.4 启动与配置

- release 模式必须拒绝弱 secret。
- 本地开发默认配置必须能启动。
- `readyz` 至少要能区分数据库和 Redis 可达性。

### 2.5 Provider 兼容注册表

- `openai-compatible` 和 `anthropic-compatible` 必须有显式 preset。
- preset 必须包含 route alias、默认 base URL、auth mode、模型目录所有者和账号 allowlist。
- preset 新增必须回写文档。

### 2.6 版本策略

- Go 使用最新稳定版本；当前批准版本为 `1.26.3`。
- `apps/api/go.mod` 与 `apps/api/Dockerfile` 的 Go 版本必须一致。
- Go 依赖按最新可用版本推进，不为兼容旧运行时降级。
- 代码生成工具必须显式 pin 版本；升级后必须重新跑 drift check。

## 3. Harness

### 3.1 快速架构门禁

```txt
make architecture-check
```

它当前覆盖：

- config release gate。
- Go / Dockerfile 版本一致性。
- cmd 入口依赖边界。
- app bootstrap 结构。
- app 到 Ent-backed store 的 contract 注入。
- app 到后台 worker 的生命周期编排。
- HTTP server 不直接 import Ent。
- worker 不得直接 import Ent、HTTP server 或 persistence 实现；只能依赖模块 contract / service。
- Ent store 只能依赖 Ent、标准库、同层 entstore package 和模块 contract。
- Redis store 只能依赖 Redis client、标准库、同层 redisstore package 和模块 contract。
- platform db / redis 启动 client。
- module contract 边界。
- crypto / logger 基础平台抽象。
- Ent 空库迁移应用。
- Users / API keys / Providers / Models / Accounts / Usage / Scheduler / Audit / Billing / Events Ent repository contract 实现。
- Domain Events Outbox 发布成功、失败重试和到期重试状态迁移。
- Domain Events Outbox worker 轮询分发、inbox 去重、processed / failed 状态记录和 app 装配规则。
- Redis-backed Scheduler Lease 原子获取、释放、过期释放并发。
- Redis-backed realtime slot store 跨实例限额、跨实例释放、过期释放和安全摘要。
- runtime rebuild 后仍能读回 API Key、Provider、Model、Account、Usage、Scheduler Decision、Billing Ledger、Outbox Event 和 Audit Log。
- compatible provider preset 注册表。
- HTTP 启动层基础测试。

### 3.2 迁移门禁

```txt
make migration-check
```

它当前覆盖：

- Ent schema 可应用到空库。
- `apps/api/migrations/postgres/up/000001_initial_schema.sql` 与 Ent 生成的 PostgreSQL DDL 无漂移。
- `apps/api/migrations/postgres/down/000001_initial_schema.sql` 覆盖当前 Ent table list。
- `apps/api/migrations/postgres/up` 和 `apps/api/migrations/postgres/down` 文件成对，编号连续，新增迁移从 `000002_*` 开始。

### 3.3 全量门禁

```txt
make check
```

它继续作为完整质量门禁，覆盖 diff check、OpenAPI、SDK、Ent、migration check、architecture-check、code-quality-check、Go test 和 secret scan。

### 3.4 代码质量门禁

```txt
make code-quality-check
```

它当前覆盖：

- Go 文件 `gofmt` 漂移。
- `go vet ./...`。
- `git diff --check`。
- `make check` 必须包含架构、代码质量、API test、生成物漂移、migration 和 secret scan 门禁。
- secret scan 必须覆盖生成的 OpenAPI/SDK artifacts 和 lockfile。
- 非生成生产 Go 文件最大 2180 行。
- 非生成生产 Go 函数最大 210 行。
- 仓库文本文件必须是 UTF-8、以换行结尾、无尾随空白。
- Node / shell 脚本必须能通过语法检查。
- Dockerfile / Compose 必须满足基础容器卫生：镜像 tag 显式，运行镜像非 root。
- 生产 Go 代码不得新增 `TODO`、`FIXME`、`HACK`、`XXX` 这类投机标记。
- 生产 Go 代码不得在已记录的启动装配逃生口之外使用 `panic` / `recover`。

`code-quality-check` 和 `architecture-check` 分工不同：前者卡住代码形态和静态质量，后者卡住模块边界、启动装配、persistence/worker/HTTP ownership 等架构不变量。生成代码不进入 size 阈值，但仍必须通过对应 codegen drift check。

### 3.5 本地 Gateway Smoke

```txt
make smoke-gateway
```

它当前覆盖：

- `/api/v1/health` 返回 `request_id` 和健康状态。
- 管理员登录返回 session cookie 和 CSRF token。
- 控制台创建 API Key 且只在创建响应返回 plaintext。
- `/v1/models` 使用 API Key 可读取本地 seed 模型。
- `/v1/chat/completions`、`/v1/responses`、`/v1/messages` 均可通过本地 mock Provider Account 完成最小 Gateway 调用。

## 4. 证据映射

| 要求 | 证据 |
| --- | --- |
| cmd 入口薄 | `apps/api/internal/architecture/architecture_test.go`, `apps/api/internal/app` |
| 模块 contract 边界 | `apps/api/internal/architecture/architecture_test.go` |
| Go 版本策略 | `apps/api/internal/architecture/architecture_test.go`, `apps/api/go.mod`, `apps/api/Dockerfile` |
| app bootstrap / persistence 注入 | `apps/api/internal/app/app_test.go`, `apps/api/internal/httpserver/runtime_persistence_test.go` |
| app worker 生命周期 | `apps/api/internal/app/app_test.go`, `apps/api/internal/workers/outbox/worker_test.go` |
| platform db / redis | `apps/api/internal/platform/db`, `apps/api/internal/platform/redis` |
| 空库迁移可应用 | `make migration-check`, `apps/api/internal/platform/db/migration_test.go` |
| release 版本化迁移无漂移 | `make migration-check`, `apps/api/internal/platform/db/migration_test.go`, `apps/api/atlas.hcl`, `apps/api/migrations/postgres` |
| Ent-backed repository | `apps/api/internal/persistence/entstore/*`, `apps/api/internal/persistence/entstore/runtime_stores_test.go` |
| Domain Events Outbox / Inbox 分发 / 重试 | `apps/api/internal/modules/events/service/service_test.go`, `apps/api/internal/workers/outbox/worker_test.go`, `apps/api/internal/persistence/entstore/runtime_stores_test.go` |
| Redis-backed Scheduler Lease | `apps/api/internal/persistence/redisstore/scheduler`, `apps/api/internal/app/app_test.go` |
| Redis-backed realtime slots | `apps/api/internal/persistence/redisstore/realtime`, `apps/api/internal/modules/realtime/service`, `apps/api/internal/app/app_test.go` |
| 运行期数据重启持久化 | `apps/api/internal/httpserver/runtime_persistence_test.go` |
| release 配置门禁 | `apps/api/internal/config/config_test.go` |
| logger / crypto 平台层 | `apps/api/internal/platform/logger`, `apps/api/internal/platform/crypto` |
| compatible preset 注册表 | `apps/api/internal/modules/providers/preset/registry_test.go` |
| 启动 / health / readiness / local gateway smoke | `apps/api/internal/httpserver` 测试、`apps/api/internal/app/app_test.go`、`deploy/docker-compose.yml` 与 `tools/smoke-local.mjs` |

## 5. 变更规则

- 改架构边界，先改文档，再补 harness。
- 改 harness 覆盖范围，必须同步文档映射。
- 改 Ent schema 或 release 迁移，必须跑 `make migration-check`。
- 新增业务前，至少跑一次 `make architecture-check`。
