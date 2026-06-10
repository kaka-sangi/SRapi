# Goal：部署/扩展/运维就绪 —— Redis 连接护栏 + 真实 readiness + 资源/HA 基线 + standalone 镜像（第十七批 · NFR 部署就绪）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：16-agent NFR 对抗核查后认定 SRapi「正确性地基扎实，但多副本就绪度有确定性炸弹」。生产就绪结论是**不能直接多副本部署**，两个硬阻塞之一就在部署/运维侧：(1) Redis 客户端裸默认 `Options`（只设 Addr/Password/DB，PoolSize/超时全走 go-redis 默认），而 Redis 承载 rate-limit/并发槽/scheduler lease/session affinity 这些**网关热路径同步依赖**——一旦 Redis 抖动，热路径会按默认 ~3s 读超时长挂，且多副本×GOMAXPROCS 会对 Redis 开过多连接，与 DB 池显式上界 `MaxOpenConns=25` 形成对比（`redis.go:26-30`）。(2) `/readyz` 的依赖探测在没有注入 pinger 时退化为 500ms 纯 TCP 拨号（端口可连即 `ok`，发现不了库只读/认证失败/连接池耗尽），compose 健康检查又只跑恒 200 的 `/livez`——**滚动升级时无法真实摘流**，坏副本会继续吃流量。(3) 部署侧无任何资源 `limits`/`restart`/HA：`deploy/docker-compose.yml:33` 的 api 服务无 `replicas`/`restart`/`resources.limits`，Postgres/Redis 各单容器单本地卷无副本/failover/PITR——对一个收钱系统，Postgres 单点是 RPO 灾难；且**当前没有任何东西阻止运维 `docker compose up --scale api=3`**，一旦真这么做就会撞上 batch16 之前的 worker N 倍探测炸弹。(4) 前端控制台未用 `output:'standalone'`，运行镜像全量 COPY `node_modules`，且代理目标 `SRAPI_API_PROXY_TARGET` 在 build 期烤进 routes-manifest，一镜像无法跨环境复用。
> 在 N 副本/高 QPS/大数据量下会怎样（这就是本批要消灭的确定性后果）：Redis 默认池在「N 副本 × 每副本 PoolSize=10×GOMAXPROCS」下会对单 Redis 实例开出几百~上千连接、Redis 抖动时无超时护栏让每个网关请求在限流/并发槽查询处长挂 ~3s 直至雪崩；假 readiness 让滚动升级把流量打到尚未就绪/已半死的副本；无 restart/limits 让单副本 OOM 后不自愈、且一个副本的内存暴涨能拖垮宿主机上的 Postgres/Redis；无多副本护栏让运维「以为能横向扩」却在解锁瞬间触发上游风控封号。
> 前置依赖：**batch16（worker leader-gate + 竞态收口）必须先落地**——leader-gate 是「解锁多副本」的硬前提，否则本批 rank16 一旦把部署文档/manifest 从「禁止 replicas>1」改成「可多副本」，就会立刻让 14 个 worker 在每个副本无条件全启、对同一批上游账号发 N 倍探测/拉额度（NFR 排名第 1 的 high 风险）。本批的解锁动作必须在确认 leader-gate 已就位后才执行。
> 关联：与 NFR program 其余批次同源——batch16（并发安全地基，本批的硬前置）；batch18（热路径去全表扫，单副本容量天花板）；batch19（容量治理：可观测去全表扫 + retention + 一线告警，本批 rank16 的 k8s readiness 探针与 batch19 rank19 的「up{job=srapi-api}==0」黄金信号告警互补）。复用的现成原语：`migrator.go:67` 的 `pg_advisory_lock` 范式（batch16 leader-gate 的底座，本批不重复实现）、`platform/redis` 客户端、`httpserver` 已有的 `dependencyPinger` 接口与 `WithDatabasePinger`/`WithRedisPinger` 注入点、`Makefile` 已存在但未被 workflow 调用的 `deploy-preflight`。

---

<role_and_context>
你是 SRapi 仓库的资深后端 / SRE 工程师。SRapi 是一个对标 sub2api 的 AI 网关 / 计费平台。
技术栈与拓扑：
- 后端 Go + ent ORM（`apps/api`，OpenAPI-first：先改 `packages/openapi/openapi.yaml` 再生成）。HTTP 在 `apps/api/internal/httpserver/`；app 装配在 `apps/api/internal/app/`（`app.New` 打开依赖、`startWorkers` 启 14 个周期 worker）；配置在 `apps/api/internal/config/`；周期 worker 在 `apps/api/internal/workers/`（14 个）；基础设施在 `apps/api/internal/platform/{db,redis,otel,ratelimit}`。
- 持久化：`entstore`（Postgres，真实后端）+ `redisstore`（rate-limit / 并发槽 / scheduler lease / session affinity）。
- 前端：Next.js + TypeScript（`apps/web`，路由 `app/`，代理 route handler 运行期读 `SRAPI_API_PROXY_TARGET`）。
- 部署：`deploy/`（docker-compose + nginx + prometheus / tempo / alertmanager）；`make dev-up` 起本地栈；登录 admin@srapi.local / Admin1234。
本批维度是**部署 / 扩展 / 运维就绪（deployment NFR）**，强调点落在：连接资源护栏（Redis 池 / 超时，与 DB 池 `MaxOpenConns=25` 对齐）、真实健康探测（滚动升级摘流正确性）、资源 / restart / HA 基线、多副本解锁的护栏与前置确认、镜像可移植性（standalone + 运行期配置）、发布供应链（多 arch 镜像 + SBOM + 漏洞扫描）。这些都是「让系统能被安全地部署和扩容」而非新增业务能力。
架构红线（由守门测试 + gofmt 强制，本批后端代码改动小、基本不增大文件）：跨模块只允许 import 目标模块的 `contract` 层；`contract` 禁止 import Ent / 生成的 OpenAPI DTO / HTTP server 包；worker 只能 import 模块 contract/service；单文件硬上限 `maxRuntimeFileLines=2200`（runtime_* 文件）、`runtime_http.go` 兼容层上限 120 行；单函数上限 `maxProductionFuncLines=210`；gofmt 必须全过；Go 1.26.3。
凭证 AES-GCM 加密不得改明文。绝不抄 sub2api 源码，只借鉴能力与算法思路。
</role_and_context>

<objective>
目标（可衡量最终态）：在 batch16 已让 14 个 worker 多副本安全的前提下，补齐「真的能开多副本并安全运维」的部署护栏，使 SRapi 从「只能锁死单副本跑」升级为「可受控多副本 + 滚动升级正确摘流 + 单副本 OOM 自愈 + 一镜像跨环境」。完成后：(a) Redis 客户端的 `PoolSize`/`MinIdleConns`/`DialTimeout`/`ReadTimeout`/`WriteTimeout`/`PoolTimeout` 全部从 `config` 可配、有生产默认值，与 DB 池一样有显式容量护栏；(b) `/readyz` 对 DB/Redis 做真实查询级探测（DB `SELECT 1`、Redis `PING`），库只读 / 认证失败 / 连接池耗尽时返回非 200，compose/k8s 健康检查指向 `/readyz`；(c) compose 有 `restart:unless-stopped` + `resources.limits` 基线，部署文档与 k8s manifest 骨架在**确认 leader-gate 已就位后**显式从「禁止 replicas>1」改为「可多副本」，backup 接定时任务；(d) 前端运行镜像只 COPY standalone 产物、体积显著下降、代理目标运行期可改实现一镜像多环境；(e) 有 tag 触发的 release workflow 产出带 git-sha 标签的多 arch 镜像 + SBOM + 漏洞扫描 + 调用 `deploy-preflight`。
动机：这些 NFR 风险在规模下是确定性而非概率性后果。Redis 默认池在 N 副本 × GOMAXPROCS 下确定会对单实例开出数百连接、抖动时确定让热路径按 ~3s 默认读超时挂死并雪崩；假 readiness 确定让滚动升级把流量打到未就绪副本；无 restart/limits 确定让 OOM 副本不自愈并拖垮同宿主的 PG/Redis；无多副本护栏确定让运维 `--scale` 后撞上游 N 倍探测封号。本批把每一条都换成「有显式上界 / 有真实探测 / 有自愈 / 有解锁前置确认」的护栏。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元，每项带真实文件锚点；二选一的项必须落定到无中间态）：

**1. rank9（high）：Redis 客户端连接 / 超时护栏（先行）**
- `apps/api/internal/platform/redis/redis.go:19-32` 的 `Open()` 当前 `redis.NewClient(&redis.Options{...})` 只设 `Addr`/`Password`/`DB`，其余全走 go-redis 默认（PoolSize=10×GOMAXPROCS、Read/Write≈3s、Dial≈5s）。
- 在 `apps/api/internal/config/`（`DependencyConfig` 或新增 `RedisConfig` 字段）暴露并从 env 读取 `PoolSize`/`MinIdleConns`/`DialTimeout`/`ReadTimeout`/`WriteTimeout`/`PoolTimeout`，给生产默认值（如 `ReadTimeout`/`WriteTimeout` 2–3s、`DialTimeout` 3–5s、`PoolTimeout` 略大于 ReadTimeout、`PoolSize` 显式而非随核数膨胀），在 `Open()` 里全部塞进 `redis.Options`。
- 与 DB 池显式上界（`config.go` 的 `MaxOpenConns=25` / `db.go` 池设置）形成对称护栏；compose env 暴露对应 `REDIS_*` 键（默认值有界）。

**2. rank18（low，但与 rank9 同为滚动升级前提）：真实 readiness + Redis 瞬断不立即退进程**
- `/readyz` 路径：`checkDependencies`（`server.go:756`）经 `probeStatus`（`runtime_state.go:1055`）已**优先调注入的 `dependencyPinger.Ping`**、仅在 pinger 为 nil 时退化到 `tcpStatus`（`server.go:772`，500ms 纯 TCP 拨号）。app.go 已注入 `WithRedisPinger(redisClient)`（go-redis `PING`，真实）与 `WithDatabasePinger`（`app.go:174-178`）。**本批要确保的真实性**：(i) DB pinger 的 `Ping` 是真实查询级探测（确认 `dbClient.Ping` 走 `SELECT 1` 或连接 ping 等价，必要时升级为 `SELECT 1`，发现「端口在但库只读 / 认证失败 / 连接池耗尽」）；(ii) 不再有「pinger 为 nil → 退化 TCP」让端口可连即误报 `ok` 的生产路径（release 模式必须注入真实 pinger）；(iii) 探测超时合理（不复用 500ms 而是给查询留余量）。
- compose / k8s 健康检查从 `/livez`（恒 200，`main.go -healthcheck`）改指向 `/readyz`（`deploy/docker-compose.yml` 的 api healthcheck），使滚动升级按真实就绪态摘流。
- release 模式 Redis 瞬断：`app.go:369`（scheduler lease）等处 release 模式对 Redis ping 失败直接 `return nil, fmt.Errorf(...)` 让 `app.New` 失败、进程退出。对**瞬断**（非永久不可用）考虑有限重试 / 退避后再决定是否退出，而非首次 ping 失败即退（避免 Redis 抖动一下整个 API 进程被拉下）。

**3. rank16（med）：资源 / restart / HA 基线 + 多副本解锁（受 leader-gate 前置约束）**
- `deploy/docker-compose.yml:33` 的 api 服务加 `restart: unless-stopped` + `deploy.resources.limits`（CPU/内存基线）+ 可选 `GOMAXPROCS` env；Postgres/Redis 同加 `restart`。
- 生产选型与文档：明确「托管 Postgres（自动备份 + PITR）+ 托管 / 哨兵 Redis」或提供 k8s manifest / helm 骨架（API `Deployment` 多副本 + readiness 探针指向 `/readyz` + HPA），作为可选部署形态骨架（不要求完整生产 helm，提供可演进的骨架即可）。
- `make backup-postgres`（`Makefile`）接定时任务示例并在文档说明 restore 验证流程。
- **多副本解锁（关键护栏动作）**：当前部署文档应处于「禁止 replicas>1，直到 leader-gate 落地」的锁定态。本批**仅在确认 batch16 leader-gate 已就位**后，把部署文档与 k8s manifest 从「禁止 replicas>1」改为「leader-gate 已就绪，可受控多副本」并写明前置条件与监控点；若 leader-gate 尚未确认就位，则保持锁定态并在 progress 注明阻塞，不得擅自解锁。

**4. rank17（med）：前端 standalone 镜像 + 代理地址运行期化**
- `apps/web/next.config.ts` 设 `output: 'standalone'`；`apps/web/Dockerfile` 运行阶段只 COPY `.next/standalone` + `.next/static` + `public`（替代全量 COPY `node_modules` + `npm start`），体积 / 冷启大降。
- 代理目标 `SRAPI_API_PROXY_TARGET` 确认走运行期 env（route handler 已运行期读取），核对 `next.config` 的 `rewrites` 不再把目标烤进 build，实现一镜像多环境。

**5. rank24（low）：发布流水线（可与其余并行）**
- `.github/workflows/` 现仅 `ci.yml` 单 check job。新增 release workflow（如 `release.yml`，tag 触发）：多 arch 构建 API + web 镜像并推仓库（带 git-sha / 版本标签）+ 生成 SBOM + 镜像漏洞扫描 + 调用已存在的 `deploy-preflight`（`Makefile`，当前未被任何 workflow 调用）；可选自动部署 staging。

明确不做（out-of-scope，均由其它 NFR 批涵盖或本批刻意以「护栏 / 骨架」处置，非永久搁置）：
- 不做 worker leader-gate 本体与 balance_charger/outbox/idempotency 竞态收口——那是 **batch16**，且是本批 rank16 解锁多副本的硬前置。
- 不做热路径去全表扫（`recordGatewayAccountSnapshots`/调度候选 N+1/定价缓存）——那是 **batch18**（单副本容量天花板根因）。
- 不做可观测去全表扫（`/metrics` 真 counter、`/v1/usage` DB 聚合）、retention 覆盖、一线黄金信号告警——那是 **batch19**；本批只把 k8s readiness 探针就位，告警留 batch19。
- 不实现完整生产级 helm chart / 多 region / 灾备演练——本批只提供 k8s manifest 骨架（多副本 + readiness + HPA 形状），生产化属独立运维批。
- 不改任何业务能力、计费维度、调度决策、OpenAPI 业务契约。

> 决策注记（受 open_decisions 影响的默认假设）：
> - **默认目标是支持受控多副本**（open_decisions「是否要多副本/水平扩展」是 batch16+17 是否为上线前置的分水岭）：故本批把 leader-gate（batch16）就位视为「解锁 replicas>1」的硬前提，部署护栏按「将要开多副本」设计。若运维明确长期单副本，本批仍全部有价值（Redis 护栏 / 真实 readiness / restart / standalone 与副本数无关），唯 rank16 的解锁动作改为「保持锁定 replicas=1 但补齐护栏」。
> - **HA/RPO/RTO 默认假设**（open_decisions「HA/RPO/RTO 要求」未定）：本批默认提供「托管 DB + 定时备份 + PITR 推荐 + k8s 骨架」作为可选形态，不强制上 Postgres operator；最终选型（托管 vs 单点 + pg_dump）由运维拍板，本批产出的是「能演进到任一形态」的骨架与文档，不锁死。
> - **Redis 超时默认值待运维确认**：本批给出有界的生产默认（如 ReadTimeout 2–3s），但具体数值与 PoolSize 上界标注为「可调，默认值待运维按 Redis 实例规格确认」，不硬编码为不可改。
</scope>

<constraints>
途中不得改变 / 必须守住：
- **绝不破坏已验证的多副本安全正确性**：计费扣费 `ChargeUsage` 已是 Serializable 隔离 + `charged_at IS NULL` 在 select 与 UpdateMany 两端条件 claim + 行数校验，多副本不会重复扣费（`billing/store.go:133`/`:220`）；限流 / 并发槽 / scheduler lease 已是 Redis 原子 Lua 脚本，跨副本天然一致。**本批是部署护栏批，根本不碰这些存储 / 调度逻辑的正确性语义**——Redis 池 / 超时只改连接资源参数，不改任何 Lua 脚本或 claim 条件；改 Redis 超时不得让原子操作变成「超时后部分执行」的不一致（超时是连接层取消，不改脚本原子性，但须确认热路径对 Redis 超时错误的处理是「拒绝/降级」而非「当成成功」）。
- 金额一律 string + big.Rat 8 位定点，绝不改回 float（本批不碰金额代码，列此为红线提醒）。
- **行为等价**：本批主要是配置 / 部署 / 健康探测改动，不涉及热路径业务逻辑重构；凡触及 Go 代码（redis.Options 装配、readiness 探测、release 重试），改动前后对外行为（API 响应结构、健康响应字段语义、调度决策、SSE 流帧）必须不变；readiness 的语义变化（从「TCP 可连」到「查询成功」）是**修正而非改契约**——HTTP 响应结构（`healthData`/`Dependencies` 字段）保持，只让 `status` 更真实。
- 不破坏现有 OpenAPI 响应：`/healthz`/`/readyz`/`/livez` 的响应 DTO 结构不变；如需新增依赖状态枚举值先说明。
- ent schema 本批不需要改；若意外需要改，`go generate ./ent/...` 后同步 store/mock（含 memory store `DeletedAt` 过滤、Store-mock codegen 已知坑）。
- 不得修改与本任务无关的 `*_test.go`；新增能力（readiness 真实探测、redis 池配置解析）配新测试。
- 新增依赖（如 SBOM 工具、镜像扫描 action）须先说明理由；Go 侧本批原则上不引入新依赖（go-redis 的 Options 字段已存在）。
</constraints>

<success_criteria>
完成需同时满足（每条给出 agent 自己能贴出的可观察证据，评估器不自己跑命令）：
1. `cd apps/api && go build ./... && go vet ./...`，退出码 0（贴尾部）。
2. **Redis 池 / 超时从 env 可配且生效（rank9）**：贴 `redis.go` 改后片段证明 `redis.Options` 现填入 `PoolSize`/`MinIdleConns`/`DialTimeout`/`ReadTimeout`/`WriteTimeout`/`PoolTimeout`；贴 `config` 解析片段证明这些键从 env 读取且有有界默认值；贴一条测试证明「设了 env → Options 拿到对应值」「未设 → 落到默认而非 go-redis 裸默认」（表驱动 config 解析测试 + Open 装配断言）；贴 compose 新增的 `REDIS_*` env。
3. **真实 readiness 可证（rank18）**：贴测试 / 说明证明 `/readyz` 在「DB pinger Ping 失败（模拟只读 / 连接池耗尽 / 认证失败）」或「Redis PING 失败」时 `aggregateStatus` 返回非 `ok`、`handleReady` 返回 503；并证明 release 模式不再有「pinger 为 nil → 退化 500ms TCP → 端口可连即 ok」的生产路径（贴 app.go 注入证据 + 探测路径说明）；贴 compose/k8s 健康检查已指向 `/readyz`（非 `/livez`）的 diff。
4. **release 模式 Redis 瞬断处置可证（rank18）**：贴改后片段证明 Redis ping 失败在 release 模式走「有限重试 / 退避」而非首次失败即 `app.New` 退出（或如评估后决定保留 fail-fast，则贴明确决策说明与理由，二选一落定，不留中间态）。
5. **资源 / restart / HA 基线可证（rank16）**：贴 `deploy/docker-compose.yml` 改后片段证明 api（及 PG/Redis）有 `restart: unless-stopped` + `resources.limits`；贴 k8s manifest 骨架（API Deployment + readiness 探针指向 `/readyz` + HPA）；贴 backup 定时任务示例 + restore 验证说明。
6. **多副本解锁护栏可证（rank16，关键）**：贴部署文档片段证明——**仅当确认 batch16 leader-gate 已就位**，文档 / manifest 才从「禁止 replicas>1」改为「leader-gate 已就绪，可受控多副本」并列出前置条件 + 监控点；若 leader-gate 未确认就位，则贴出「保持锁定 replicas=1」的护栏文字 + progress 中的阻塞说明（诚实，不擅自解锁）。
7. **前端 standalone 可证（rank17）**：贴 `next.config.ts` 的 `output:'standalone'`；贴 `apps/web/Dockerfile` 运行阶段只 COPY standalone 产物的 diff；贴证据（`docker build` 产出层 / 镜像体积对比，或 `.next/standalone` 目录确认）证明运行镜像不再全量 COPY `node_modules`；贴说明证明 `SRAPI_API_PROXY_TARGET` 运行期可改（同镜像换 env 验证）。
8. **release workflow 可证（rank24）**：贴 `.github/workflows/release.yml` 证明 tag 触发 → 多 arch 镜像构建（带 git-sha 标签）+ SBOM + 漏洞扫描 + 调用 `deploy-preflight`；说明新增 action 依赖理由。
9. `go test ./...` 全绿（贴尾部）；`gofmt -l` 空；`git status` 干净（除预期改动），`git diff --stat` 贴出。
（success_hint 细化：Redis 池 / 超时从 env 可配且生效、默认值有界；`/readyz` 在 DB 只读 / 连接池耗尽时返回非 200 且有真实 PING 可证；compose 有 restart + limits；部署文档 / k8s manifest 标注多副本已就绪；web 运行镜像体积显著下降且同镜像跨环境；release workflow 产出带 git-sha 标签的多 arch 镜像。）
或 30 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
按依赖顺序、每单元自带测试并单独 commit；message 末尾加 Co-Authored-By 行：
1. 阶段一（连接护栏 + 真实健康，滚动升级前提先行）：rank9 Redis 池 / 超时配置（config 解析 + Open 装配 + 测试）→ rank18 真实 readiness（确认 / 升级 DB pinger 为查询级、消除 nil-pinger 退化 TCP 的生产路径、compose/k8s 健康检查指向 `/readyz`、release 模式 Redis 瞬断有限重试）。这两项是多副本滚动升级正确摘流的前提，必须先于解锁。每项独立 commit + 测试。
2. 阶段二（资源 / HA 基线 + 多副本解锁）：rank16——先加 `restart`/`resources.limits`/backup 定时任务（与副本数无关，先做）；再做 k8s manifest 骨架；**最后**做多副本解锁文档动作（前置：grep / 确认 batch16 leader-gate 已就位，否则保持锁定并记录阻塞）。一个或拆多个 commit。
3. 阶段三（前端镜像瘦身）：rank17 next standalone + Dockerfile 改 + 代理运行期化。一个 commit。
4. 阶段四（发布流水线，可与前并行）：rank24 release workflow。一个 commit。
说明：本批后端 Go 改动小、基本不增大文件，不需要先按 batch15 拆文件；若意外要给 `runtime_state.go`/`server.go` 增不少行，先确认离 2200 红线余量充足（这两个文件远小于核心巨文件，正常无虞）。
</sequencing>

<artifact>
交付物：feat 分支（如 `feat/sub2api-parity-batch17`）若干语义化 commit + 具体产物——
- `apps/api/internal/platform/redis/redis.go` 填全 `redis.Options`（池 / 超时）+ `apps/api/internal/config/` 新增 Redis 池 / 超时配置字段与解析；
- `apps/api/internal/httpserver/`（`server.go`/`runtime_state.go`）真实 readiness 探测（查询级 DB pinger、消除 nil-pinger TCP 退化）+ `apps/api/internal/app/app.go` release 模式 Redis 瞬断有限重试；
- `deploy/docker-compose.yml` restart + resources.limits + 健康检查指向 `/readyz` + `REDIS_*` env；k8s manifest 骨架（Deployment + readiness + HPA）；backup 定时任务示例；部署文档（多副本解锁护栏 / HA 选型）；
- `apps/web/next.config.ts`（`output:'standalone'`）+ `apps/web/Dockerfile`（只 COPY standalone）；
- `.github/workflows/release.yml`（多 arch 镜像 + SBOM + 扫描 + deploy-preflight）；
- 新测试（config 解析表驱动测试、readiness 探测在 pinger 失败时返回 503 的测试、Open 装配断言）；
- progress.txt 收尾：列每个 rank 的最终处置、证据位置、多副本解锁的决策（已解锁 / 因 leader-gate 未就位保持锁定）、HA 选型默认假设、未做的 stretch（完整 helm / 自动部署）。
</artifact>

<guardrails>
绝不：删 / 改既有测试让其通过；hardcode 让测试 / 健康检查恒过；`--no-verify` 跳校验或 `git push --force`；把金额改回 float；**破坏计费多副本安全语义**（不碰 `ChargeUsage` 的 Serializable + 条件 claim、不碰 Redis Lua 原子脚本）；为「让 Redis 不超时」而把超时设到无意义的大值假装稳定；伪造未实测的部署改善（如没真跑 `docker build` 就声称镜像体积下降、没真模拟 pinger 失败就声称 `/readyz` 返回 503）；**在 leader-gate 未确认就位时擅自把部署文档 / manifest 解锁为 replicas>1**（这会让 batch16 之前的 N 倍上游探测炸弹被打开）。
先问我：破坏现有 OpenAPI 响应结构（健康响应 DTO 字段 / 新增依赖状态枚举）；删除现有顶层路由 / 健康端点；任何不可逆数据迁移；解锁多副本前确认 leader-gate 已就位（若无法确认，保持锁定并问我）；向真实上游或真实托管 DB/Redis 发起需真实凭证的调用。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt（做了什么 / 证据在哪 / Redis 默认值取值 / readiness 探测变化 / 多副本解锁决策 / 下一步）+ `git commit`。
新 context 开始：先 `pwd`、`git log --oneline -10`、读 progress.txt，再 `cd apps/api && go build ./...` 确认编译态，确认 batch16 leader-gate 是否已就位（grep `pg_try_advisory_lock` / leaderGate），从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 30 turns 仍未达成，停下汇总：已完成阶段、Redis 池 / 超时是否可配生效、`/readyz` 是否真实探测、compose/k8s 资源 / restart / readiness 状态、多副本解锁决策（已解锁 / 保持锁定及原因）、前端 standalone 状态、release workflow 状态、剩余阻塞项及原因。
</stop_clause>

<references>
关键文件（path:line，以仓库根为基准；行数为本批审计 + 实测，部分行号会随改动漂移，以函数名为准）：
- Redis 池护栏（rank9）：`apps/api/internal/platform/redis/redis.go:19-32`（`Open()`，`redis.NewClient(&redis.Options{Addr,Password,DB})` 仅三字段，其余 go-redis 默认）；`apps/api/internal/config/config.go`（新增 Redis 池 / 超时配置，对照 DB 池 `MaxOpenConns=25` 显式上界先例）；`apps/api/internal/platform/db`（DB 池设置先例）。
- 真实 readiness（rank18）：`apps/api/internal/httpserver/server.go:732`（`handleReady`）/:756（`checkDependencies` 经 `probeStatus`）/:772（`tcpStatus` 500ms TCP 退化路径）；`apps/api/internal/httpserver/runtime_state.go:1055`（`probeStatus`：pinger 非 nil 调真实 `Ping`，nil 才退化 TCP）/:212-213（`databaseProbe`/`redisProbe` 字段）；`apps/api/internal/httpserver/server.go:72`（`dependencyPinger` 接口）/:122/:128（`WithDatabasePinger`/`WithRedisPinger`）；`apps/api/internal/app/app.go:174-178`（注入 pinger）/:321（`dbClient.Ping`）/:366-372`（release 模式 Redis ping 失败即退）；`apps/api/cmd/.../main.go`（`-healthcheck` → `/livez`）。
- 资源 / HA（rank16）：`deploy/docker-compose.yml:33`（api 服务无 replicas/restart/limits）/:27-31（redis healthcheck 范式）/:59-63（已有 `DATA_RETENTION_*` env 范式，新增 `REDIS_*` 仿此）；`Makefile`（`backup-postgres`、`deploy-preflight`）；仓库当前无 `deploy/k8s`/helm（待新增骨架）。
- 前端镜像（rank17）：`apps/web/next.config.ts`（无 `output:'standalone'`）；`apps/web/Dockerfile`（全量 COPY + `npm start`）；代理 route handler 运行期读 `SRAPI_API_PROXY_TARGET`。
- 发布流水线（rank24）：`.github/workflows/ci.yml`（仅 `jobs.check`）；`Makefile` 的 `deploy-preflight`（已存在，未被任何 workflow 调用）。
复用的现成原语（仅复用 / 对照，不重复实现、不抄代码）：
- `apps/api/internal/platform/db/migrator.go:67`（`SELECT pg_advisory_lock($1)` 会话级 advisory-lock 范式）——是 **batch16 leader-gate** 的底座，本批不实现 leader-gate，只把它「已就位」当作 rank16 解锁多副本的前置确认点。
- `apps/api/internal/platform/redis`（go-redis 客户端封装，本批扩展其 Options）。
- `dependencyPinger` 接口 + `WithDatabasePinger`/`WithRedisPinger` 注入点（rank18 复用，已存在，确保 release 模式真实注入）。
- batch19 的一线黄金信号告警（`up{job=srapi-api}==0`、连接池 / Redis 健康）与本批 k8s readiness 探针互补（本批就位探针，告警在 batch19）。
sub2api 仅作能力对照不抄代码：
- `Dockerfile.goreleaser` 的单二进制 / 多 arch 发布思路（对照 rank17 镜像瘦身与 rank24 多 arch 发布，借鉴「单可移植产物 + 运行期配置」而非抄构建脚本）；sub2api 的 `go:embed` 单进程模型（前端嵌入 Go 二进制）作为 rank17「减少部署单元」的更激进对照点——本批默认仅做 standalone 瘦身，嵌入式单进程留作可选 stretch（open_decisions「前端 SSR 形态」未定，不擅自激进改形态）。
</references>
