# SRapi vs sub2api 改进 Goal 总索引（完整 program · 零 deferral）

> 来源：2026-06-08-09，多轮 28-agent 对比 SRapi vs sub2api（`/home/senran/Desktop/sub2api`）+ 对抗性核查（130+ 断言），全部基于**当前代码**（含未提交 WIP）。
> 目标：全力对标 sub2api，**消灭 SRapi 全部摆设**（后端建好但前端无入口/不消费、或字段定义但无强制点）、补全功能缺失、清零技术债、达成架构验收口径。**所有发现都已编入批次，无"后续/二期"搁置项。**

## 现状全景（已核查）

**batch1-7 已落地且扎实**（commit `302d26b`/`d75c142`/`53f05e3`/`c7b1c5b` + 在途 WIP + `PROVIDER_AUTH_MATRIX.md`+CI）：支付下单→回调→验签→入账→退款扣回主链端到端、payload 转换引擎真改请求体、error-passthrough 真匹配、TLS 指纹真做 uTLS 出站、会话粘度、故障转移+冷却+调度反馈、分组费率倍率真计费、Anthropic/Codex 真实配额+告警、认证矩阵真实性+CI 守门、配额合成/真实分离、family 定价兜底、渠道聚合页、用户配额自助。

**剩余差距三类（batch8-15 覆盖）：**
1. **摆设（31 项，最高优先）**——最严重的整片摆设：affiliate 邀请链从源头断裂（邀请码/关系/规则三写入零入口，返佣引擎被饿死）、scheduler 7 策略硬编码 seed 表恒空（运营无法调权重）、RBAC 215 处仅 owner/admin 二档（前端权限框是自由文本）、风控页全套表单零强制、内容安全只脱敏不拦截、channel-monitor 三字段无 worker 全惰性、~12 个 settings 开关存而不读、TLS profile 无账号绑定 UI、多个孤儿后端端点。
2. **功能缺失（26 项）**——支付真实退款/对账/手续费/通道负载均衡/Airwallex、SOCKS5+uTLS 组合、Antigravity 配额、Gemini RPD、service_tier/cache 分档/长上下文倍率/图片 token、daily-weekly 配额键、端用户登录 wechat/dingtalk/微信支付/邮箱验证码/OAuth 免密。
3. **技术债（9 项）**——5 个核心文件逼近 2200 行红线（`runtime_gateway_core.go` 仅余 ~44 行）、2 个函数逼近 210 行、metadata/quota 持久化多处重复、死代码、命名误导、热路径全表扫。

每个批次都是**自包含、可验证、按依赖排序**的 goal 提示词，遵循「角色/背景 → 目标与动机 → 范围/非范围 → 约束 → 成功标准 → 顺序 → 交付物 → 禁止事项 → 进度续作 → 停止条件 → 参考」结构（依据 Anthropic prompt-engineering / Claude Code /goal / long-running-agents / OKR）。直接把对应文件整段粘给 Codex 即可。**铁律：摆设的合法处置只有两种——真接通 或 诚实下架，绝不伪造未实现能力。**

## 推荐执行顺序

| 批次 | 文档 | 主题 | 状态/依赖 |
|---|---|---|---|
| 第一批 | [batch1](./goal-sub2api-parity-batch1.md) | 收入安全急修 + 渠道聚合页 + 用户面闭环 | ✅ 大体已落地 |
| 第二批 | [batch2](./goal-sub2api-parity-batch2.md) | 计费域地基重构（money 公共包 + 定价归位） | 进行中/部分 |
| 第三批 | [batch3](./goal-sub2api-parity-batch3.md) | 计费维度深化（billing_mode/区间 + usage_log 分解） | WIP 在途 |
| 第四批 | [batch4](./goal-sub2api-parity-batch4.md) | 额度护栏与性能（订阅行物化 + Key USD 配额） | WIP 在途 |
| 第五批 | [batch5](./goal-sub2api-parity-batch5.md) | 配额真实性 + 账户导入认证 | 部分已落地 |
| 第六批 | [batch6](./goal-sub2api-parity-batch6.md) | 信息架构收尾 + 用户自助 + 去重 | 部分已落地 |
| 第七批 | [batch7](./goal-sub2api-parity-batch7.md) | 上游认证矩阵真实性（消灭摆设认证 + CI 守门） | ✅ 已落地 |
| **第八批** | [batch8](./goal-sub2api-parity-batch8.md) | **分销与促销资金链闭环（消灭最大整片摆设）** | 无强依赖·可先行 |
| **第九批** | [batch9](./goal-sub2api-parity-batch9.md) | **RBAC 真实化 + 风控/内容安全接强制点** | 接 batch8 后 |
| **第十批** | [batch10](./goal-sub2api-parity-batch10.md) | **调度策略 CRUD + 探测真实性 + 孤儿端点接线** | 独立 |
| **第十一批** | [batch11](./goal-sub2api-parity-batch11.md) | **账号认证/配额/TLS 绑定摆设清除 + 上游配额补全** | 独立 |
| **第十二批** | [batch12](./goal-sub2api-parity-batch12.md) | **支付上游真实性（退款/对账/手续费/负载均衡）** | 独立 |
| **第十三批** | [batch13](./goal-sub2api-parity-batch13.md) | **计费维度深化（service_tier/cache/长上下文/图片token）+ daily-weekly 配额** | 接 batch12 后 |
| **第十四批** | [batch14](./goal-sub2api-parity-batch14.md) | **端用户登录摆设清除 + 平台自助/配置补全** | 接 batch12/13 后 |
| **第十五批** | [batch15](./goal-sub2api-parity-batch15.md) | **架构与技术债清理（拆巨文件/消重/死码/补测/性能）** | 穿插进行（见下） |
| **第十六批** | [batch16](./goal-sub2api-parity-batch16.md) | **【NFR·并发】worker leader-gate + balance_charger/outbox/idempotency 竞态收口** | 开多副本前置 |
| **第十七批** | [batch17](./goal-sub2api-parity-batch17.md) | **【NFR·部署】Redis 连接护栏 + 真实 readiness + 资源/HA 基线 + standalone 镜像** | 依赖 batch16 |
| **第十八批** | [batch18](./goal-sub2api-parity-batch18.md) | **【NFR·性能】热路径去全表扫（网关主链不再每请求扫 usage_logs/accounts）** | 单副本扩容前置 |
| **第十九批** | [batch19](./goal-sub2api-parity-batch19.md) | **【NFR·容量】可观测/读路径去全表扫 + 全表 retention + 分批删除 + 索引** | 接 batch18 |

> **batch15 拆文件须穿插：** `runtime_gateway_core.go` 仅余 ~44 行就触 2200 红线。任何会给核心文件增行的功能批次（8-14、18）开工前，应先做 batch15 对应的拆文件子项，否则架构测试会红。建议：batch8/9 先行（不大动核心文件）→ 穿插 batch15 拆文件 → 再做 batch10-14。

## NFR（性能/容量/并发/部署）治理 · batch16-19

> 来源：2026-06-09 NFR 专项审计（9-agent，4 维 + 对抗核查）。结论详见各批文档；要点如下。

**生产就绪判定（已核查）：当前不能直接多副本部署，但正确性地基扎实。**
- ✅ **不是问题（勿动）**：计费 `ChargeUsage` 用 Serializable + `charged_at IS NULL` 两端条件 claim + 行数校验，**多副本不会重复扣费**（`billing/store.go:133/220`）；限流/并发槽/调度租约全 Redis 原子 Lua；到期/退款/返佣条件更新+ReferenceID 幂等；outbox `(event_id,consumer)` 去重；DB 迁移 `pg_advisory_lock`（→ batch16 leader-gate 可复用此范式）；连接池有界 + 优雅关停 + OTel 齐备。
- ❌ **多副本硬阻塞**：① 14 个 worker 无 leader-gate（`app.go:613` 无条件全启）→ 多副本 N 倍打上游探测触发风控封号（最高优先）；② `/metrics` 多副本各报全表聚合 + counter 倒退，`rate()` 语义错乱。
- ❌ **单副本容量天花板 = usage_logs 行数**：每个成功请求同步全表扫 usage_logs 生成账号快照（`recordGatewayUsage→recordGatewayAccountSnapshots`），~50-100 万行起 p99 秒级、连接池打满。**确定性劣化**。
- 对照 sub2api：它有 `SchedulerCache`（Redis 快照桶）+ `ops_repo_preagg` 预聚合 + outbox 异步记账 —— SRapi 这三处缺，是主要性能差距。

**已确认的执行决策（2026-06-09，用户拍板）：**
1. **部署形态 = 先单副本上线、后续要能扩** → 执行顺序：**先 batch18 → batch19**（拆掉 usage_logs 全表扫炸弹，让单副本能扩容），**再 batch16 → batch17**（为水平扩展铺路）。
2. **计费记账 = 持久队列异步**（秒级最终一致）→ batch18 用 outbox 式持久队列把记账/快照移出热路径，保留已有计费正确性。
3. **HA/RPO = 暂不确定，先留扩展点** → batch17 把 Postgres/Redis 连接做成可指向托管/HA 实例的配置，但不强制 HA 拓扑；PITR/哨兵待 RPO 明确后再上。

> ⚠️ **部署护栏（落地前必须遵守）：在 batch16 leader-gate 就位前，生产部署必须显式锁定 `replicas=1`**（compose/k8s 不得 `--scale`），否则立即触发 14 worker N 倍打上游 → 账号被封。batch17 解锁多副本的前提是确认 batch16 已就位。

### NFR backlog（24 项 · 3 critical + worker 阻塞）

| 批 | # | 行动 | 严重度 | 类别 |
|---|---|---|---|---|
| 18 | 2 | 网关每请求同步全表扫 usage_logs 生成账号快照（O(请求×行数)炸弹） | critical | 性能 |
| 19 | 3 | /v1/usage 与管理用量/账单/审计列表把整张表读进内存再 Go 过滤 | critical | 性能 |
| 18 | 5 | 调度候选每请求全表 List accounts + 逐候选 N+1，failover 每 attempt 重跑 | critical | 性能 |
| 16 | 1 | 14 worker 无 leader-gate，多副本 N 倍打上游触发风控 | high | 并发 |
| 19 | 4 | /metrics 全表重算 + counter 倒退 + N+1 健康快照 | high | 性能 |
| 18 | 6 | recordGatewayUsage 同步阻塞响应，串联二次计价+多次写库 | high | 性能 |
| 18 | 10 | 准入+配额每请求多次窗口扫描，逐行 big.Rat 求和而非 SQL SUM | high | 性能 |
| 18 | 11 | EstimatePrice 每请求全量 ListPricingRules + 子查询 ×2-3 | high | 性能 |
| 18 | 12 | apiKeyByID 走 ListByUser 全量映射 + 线性扫，每请求多次 | high | 性能 |
| 19 | 13 | slo_evaluator 每 60s 把窗口内全部 usage_logs 读内存逐行评估 | high | 性能 |
| 19 | 7 | 多张高频写表无 retention（磁盘必然耗尽） | high | 容量 |
| 17 | 9 | Redis 客户端裸默认 Options（无 PoolSize/超时） | high | 部署 |
| 16 | 14 | balance_charger 多副本 Serializable 序列化风暴 | med | 并发 |
| 16 | 15 | outbox 双副本重复投递：邮件类副作用无幂等护栏 | med | 并发 |
| 16 | 23 | idempotency Reacquire 锁过期双副本同时接管竞态 | low | 并发 |
| 17 | 16 | 部署无资源 limits/restart/HA，PG/Redis 单点 SPOF | med | 部署 |
| 17 | 17 | Next.js 未用 standalone，镜像臃肿 + 代理地址 build 期烤入 | med | 部署 |
| 17 | 18 | /readyz 部分真实（nil 才退化 TCP）；release Redis 瞬断即退进程 | low | 部署 |
| 17 | 24 | CI 仅 make check，无镜像构建/发布/SBOM | med | 部署 |
| 19 | 8 | retention 单条无界 DELETE（无分批），大表清理锁表 | med | 容量 |
| 19 | 21 | usage_logs 缺 (provider_id,created_at) 复合索引 | med | 容量 |
| 19 | 22 | 快照请求路径与 worker 双写；availability rollup 无 worker | med | 容量 |
| 19 | 19 | 告警仅覆盖 ops_alert_events，无热路径黄金信号 | low | 部署 |
| 18/19 | 20 | 流式 meter + raw body 双份缓冲分配 | low | 性能 |

### NFR 待你后续拍板（不阻塞动工，影响 retention 天数/HA 选型）

各高频写表的合规保留期（billing_ledger 财务监管期 / scheduler_request_snapshots 排障期 / outbox 已发布事件 / account_quota_snapshots）；可观测新鲜度容忍度（预聚合分钟级延迟是否可接受）；前端是否真需 SSR（可否 standalone/静态/嵌入 Go 二进制对齐 sub2api 单进程）。

## batch8-15 完整 backlog（65 项 · 摆设优先）

> 类别：摆设 / 功能缺失 / 技术债 / WIP收尾。工作量 S/M/L。

### batch8 分销与促销资金链闭环
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 1 | affiliate 邀请闭环零接线（返佣链从源头无法启动） | 摆设 | L |
| 3 | scheduler 策略 CRUD 全摆设（7 策略硬编码 seed，表恒空） | 摆设 | L |
| 15 | affiliate ledger settle/withdraw/manual_adjustment 无生产者 | 摆设 | M |
| 17 | promo_code 缺 per-user 限领与最低订单额 | 摆设 | M |
| 64 | promo 名额回滚 + subscriptions 死代码（与 batch15 协同） | 技术债 | M |

### batch9 RBAC 真实化 + 风控/内容安全
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 2 | RBAC 215 处仅二档，前端权限框自由文本永不校验 | 摆设 | L |
| 25 | RoleOperator 角色无任何可达能力 | 摆设 | S |
| 4 | 风控规则配置整页摆设（黑名单/限速/enforce 零强制） | 摆设 | M |
| 5 | 内容安全只脱敏不拦截，信用卡正则无 Luhn 误伤 | 摆设 | L |

### batch10 调度策略 CRUD + 探测真实性 + 孤儿端点
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 3 | scheduler 策略 CRUD（权重可调） | 摆设 | L |
| 21 | scope_type/scope_id 作用域策略建模但永不加载 | 摆设 | M |
| 22 | premium_quality 'priority' 权重键抽象泄漏（改名 quality） | 摆设 | S |
| 19 | scheduled-test cron_expression 永不解析 | 摆设 | S |
| 20 | scheduled-test 无探测模型选择器（可静默全空转） | 摆设 | M |
| 18 | 调度器孤儿端点 simulate/overview 零前端调用 | 摆设 | M |

### batch11 账号认证/配额/TLS 绑定 + 上游配额
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 7 | TLS 指纹 profile 无账号绑定 UI（唯一途径手敲 metadata） | 摆设 | M |
| 11 | risk_level 不可设置，scheduler riskPenalty 分支死代码 | 摆设 | S |
| 27 | runtime_class service_account_json 无签名器（必失败） | 摆设 | S |
| 28 | desktop_client_token/ide_plugin_token 冗余别名 | 摆设 | S |
| 13 | QuotaReport Plan/Credits 解析返回但从不持久化 | 摆设 | M |
| 38 | Antigravity 账号配额获取缺失 | 功能缺失 | M |
| 6 | channel monitor interval/enabled/trigger 三字段无 worker | 摆设 | M |

### batch12 支付上游真实性
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 31 | 上游真实退款缺失（RequestRefund 纯本地账务） | 功能缺失 | L |
| 35 | 退款状态机缺中间态 refunding/refund_failed | 功能缺失 | M |
| 32 | 主动对账/订单状态查询 QueryOrder 缺失 | 功能缺失 | M |
| 33 | 支付手续费率缺失 | 功能缺失 | M |
| 34 | 支付通道负载均衡策略缺失 | 功能缺失 | M |
| 23 | OrderStatus 'failed' 枚举从不被设置 | 摆设 | S |
| 16 | PaymentAuditLog 对账/取证表只写不读 | 摆设 | M |
| 24 | WeChat payer_openid/client_ip 无类型化下单入口 | 摆设 | S |
| 36 | Airwallex 支付提供商缺失 | 功能缺失 | M |

### batch13 计费维度深化 + daily/weekly 配额 + usage 聚合
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 12 | 订阅 daily/weekly 物化用量无配额键（UI 进度条恒零宽） | 摆设 | M |
| 30 | usagelog 成本分解不进聚合 | 功能缺失 | M |
| 14 | 模型注册表 quality_tier 选路/计费零消费（UI 文案误导） | 摆设 | M |
| 43 | service_tier 价（priority 2x/flex 0.5x） | 功能缺失 | M |
| 44 | cache 创建 5m/1h 分档计费 | 功能缺失 | M |
| 45 | 长上下文整次会话倍率 | 功能缺失 | M |
| 47 | 图片输出 token 独立费率/尺寸倍率表 | 功能缺失 | M |
| 46 | BillingModelSource 计费模型名开关 | 功能缺失 | M |
| 48 | LiteLLM 远程价表定时同步 | 功能缺失 | M |

### batch14 端用户登录摆设清除 + 平台自助/配置
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 51 | 端用户登录 wechat/dingtalk 摆设（仅通用 Bearer-userinfo） | 摆设 | L |
| 53 | OAuth 返回用户免密快速通道缺失 | 摆设 | M |
| 52 | 邮箱验证码注册/登录闭环缺失 | 功能缺失 | M |
| 54 | 微信支付 OAuth(取 openid)缺失 | 功能缺失 | M |
| 8 | 约 12 个 settings 功能开关存而不读 | 摆设 | L |
| 9 | 站点品牌/协议设置无公开端点，前台不渲染 | 摆设 | M |
| 10 | 用户自定义属性无 /me 入口，Required 从不强制 | 摆设 | M |
| 55 | console 写操作缺幂等保护 | 功能缺失 | M |
| 56 | captcha 无 admin UI 配置 | 功能缺失 | M |
| 57 | auditlog 保留与清理缺失 | 功能缺失 | S |
| 58 | SecuritySecret 集中密钥保险库 | 功能缺失 | M |

### batch15 架构与技术债清理
| # | 行动 | 类别 | 量 |
|---|---|---|---|
| 59 | 5 个核心文件逼近 2200 行红线 → 拆到 <1800 | 技术债 | L |
| 60 | parseAnthropic/OpenAICompatibleStream 逼近 210 行 → <150 | 技术债 | M |
| 61 | metadata 强制转换 helper 跨包重复 → 抽 internal/pkg | 技术债 | M |
| 62 | quota 持久化 worker/handler 重复 → 共享 helper | 技术债 | S |
| 63 | pricing_override 双 key 别名历史债 → 迁移删旧 | 技术债 | S |
| 64 | subscriptions 死代码 + codex_quota 命名整理 | 技术债 | M |
| 65 | cost-usage/usage domain 缺测 + SummarizeUserWindow 缺测 | 技术债 | M |
| 66 | EstimatePrice/usage 全表扫性能债 → 谓词查询/缓存 | 技术债 | M |
| 37 | SOCKS5 + TLS 指纹组合被硬拒 → 隧道拨号器 | 功能缺失 | M |
| 49 | 完整自定义 ClientHello/HTTP2 指纹（或文档化边界） | 功能缺失 | L |
| 50 | 网关级服务端 Web Search 执行器（或文档化取舍） | 功能缺失 | M |
| 42 | 账号调度状态字段 metadata_json 未索引（扩展性） | 技术债 | L |
| 67 | availability rollups 无后台 worker | WIP收尾 | S |
| 68 | 通用账号导入 update-existing 未实现 | WIP收尾 | S |

## 架构验收口径（"架构完美/零技术债"按本仓库自有强制衡量）

`apps/api/internal/architecture/architecture_test.go` + `internal/codequality/code_quality_test.go`：① 模块生产码只能 import 别模块的 **contract**（白名单：auth→users、models→capabilities、operations→usage、provider_adapters→{accounts,models,providers}、scheduler→{accounts,capabilities,models,providers}）；② 单文件 ≤2200 行、函数 ≤210 行；③ gofmt；④ Go 1.26.3；⑤ runtime HTTP 兼容层 ≤120 行。batch15 的验收 = 这些测试全绿且核心文件留出余量（<1800 行 / 函数 <150 行）。

## 认证矩阵基线（防回归 · 已逐格 trace 网关签发路径核查）

当前落地矩阵见 [`PROVIDER_AUTH_MATRIX.md`](../../../docs/constraints/PROVIDER_AUTH_MATRIX.md)。第七批后，preset `auth_methods` 只暴露真实可签发集合：

- `service_account_json` 已从 anthropic/bedrock preset 下架，仅作为 legacy enum 保留（batch11 rank27 收尾删枚举或实接 Vertex）。
- OpenAI/Gemini preset OAuth 已下架；手工误配会返回 `not_supported`。
- Antigravity 的 `desktop_client_token` / `ide_plugin_token` 已并入 preset `oauth_refresh` 路径（batch11 rank28 收尾）。
- `chatgpt-web` preset 已补齐，默认暴露 `web_session_cookie` 并安装为 `reverse-proxy-chatgpt-web`。
- CI 守门为 `TestPresetRuntimeAllowlistsOnlyExposeSignableAuthMethods`。

### 端用户登录（userauthidentity，与上游认证是两张表）

✅ 真有效：password、totp、email(仅验证+找回)、oidc、google、github、linuxdo（一条通用 OAuth2/OIDC 流水线）。
⬜ 摆设/缺失：wechat、dingtalk（仅通用 Bearer-userinfo）、微信支付 OAuth、邮箱验证码注册登录、OAuth 免密快速通道 → **已全部编入 batch14（不再是后续）**。

### 关系建模（一句话）

SRapi = `runtime_class`(账号锁一种认证) + `Provider.auth_methods`(=preset RuntimeClassAllowlist，仅创建时校验白名单)；**allowlist 是「承诺」、`injectAuth`+`oauthRefreshSettings` 才是「兑现」，脱节即摆设**。
sub2api = `Account.platform`(4常量) + `type`(6常量) + `credentials`JSONB 三件套正交组合，无独立白名单层，codex/geminicli/Vertex 是平台子形态。

## batch1-7 涵盖项（前轮 18 项 backlog · 已大体落地）

分离合成/真实配额快照、family 兜底杜绝 0 元放行、rate_multiplier+actual_cost、渠道详情聚合页、billing_mode/区间分层、Anthropic OAuth 真实额度、Preset 一键授权、定价抽到 billing 域、订阅行物化用量、available-channels 自助页、dashboard 余额+配额、usage_log 四项分解、统一导入入口、ops tab 收纳、API Key USD 配额、money 公共包、封禁状态识别、统一 usage 聚合 + 认证矩阵真实性（batch7）。

## 已核查纠正（别再当缺口）

- 用户登录 OAuth bind/adopt-by-email：**已有**（bind-current-user + bind-login）。
- 订阅等级/credits 提取：**框架已存在**（config-driven QuotaReport）。
- 计费幂等：**有去重闸门**（(request_id,attempt_no) 唯一索引 + charged_at IS NULL）。
- BillingLedger：**数据已存在**（单行 running-balance）。
- 支付主链/退款扣回 balance/payload 引擎/error-passthrough/TLS uTLS/会话粘度/故障转移：**经核查均真实闭环**，勿当摆设重做。

## 必须保留的 SRapi 优势（改造时勿退化）

string + big.Rat 8 位定点（非 float）、cacheWriteRateOrInput 防漏收回退、PricingRule effective 时间窗、AES-GCM 凭证加密、device-code、通用 OAuth 状态机、语义化 NavSection 分组骨架、模块 contract-only 边界、认证矩阵 CI 守门。

---

> **零 deferral 声明：** 曾被推迟的全部"二期"项（service_tier/cache 5m-1h/长上下文倍率/图片 token/LiteLLM 同步 → batch13；自定义 ClientHello/HTTP2 指纹/网关级 web search → batch15）与全部端用户登录摆设（→ batch14）均已编入上述批次，本 program 无未规划的剩余工作。
