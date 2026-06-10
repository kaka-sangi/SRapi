# SRapi Specs（specs/）

> `specs/` 回答"**SRapi 最终要长成什么样、按什么计划走过去**"：最终完美形态（总纲 + 各子系统目标设计）和开发计划（路线图、工作包、进度台账、对齐批次）。
> "开发时必须遵守什么"（要求、限制、启示）在 [`../docs/`](../docs/README.md)。

## 1. 目录结构

| 位置 | 含义 |
| --- | --- |
| [`FINAL_STATE.md`](FINAL_STATE.md) | 最终产品与平台目标形态（总纲）。 |
| [`design/`](#2-design--子系统最终形态) | 各子系统的目标设计规格。 |
| [`plans/`](#3-plans--开发计划与进度) | 路线图、工作包、进度台账、历史计划。 |
| [`plans/parity/`](#4-plansparity--sub2api-对齐批次) | sub2api / CLIProxyAPI 对齐计划的分批执行文档。 |

## 2. design/ — 子系统最终形态

| 文档 | 作用 |
| --- | --- |
| `design/AI_ENDPOINT_COMPATIBILITY.md` | Chat Completions、Responses、Messages、Gemini 等端点互转与 Canonical AI IR。 |
| `design/GATEWAY_ROUTE_MATRIX.md` | Gateway 路由族、Provider alias、passthrough、WebSocket 和阶段规划。 |
| `design/PROVIDER_ADAPTER_SPEC.md` | Provider Adapter 扩展、错误分类、usage 和流式解析规范。 |
| `design/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` | OpenAI-compatible / Anthropic-compatible preset、默认 base URL、auth mode、模型目录和 route alias。 |
| `design/CAPABILITY_TAXONOMY_SPEC.md` | Request / Model / Provider / Endpoint capability 命名、版本、降级和匹配规则。 |
| `design/REVERSE_PROXY_SPEC.md` | 2api 反代、TLS / HTTP/2 / Header 指纹、cookie / OAuth 凭证和反封号策略。 |
| `design/SCHEDULING_KERNEL_DESIGN.md` | 调度内核总体设计和长期演进模型。 |
| `design/SCHEDULER_V1_SPEC.md` | 调度过滤、打分、Lease、Decision 和 Feedback 规则。 |
| `design/SCHEDULER_STRATEGY_EXTENSION_SPEC.md` | 调度策略注册、版本、灰度、dry-run、shadow decision 和回滚规则。 |
| `design/SCHEDULING_SCENARIOS.md` | Scheduler 单元测试、集成测试和模拟器场景。 |
| `design/PAYMENT_SPEC.md` | 支付渠道、多实例、订单状态机、Webhook、退款和幂等。 |
| `design/AFFILIATE_REBATE_SPEC.md` | 邀请关系、返利规则、返利账本、退款补偿和转余额。 |
| `design/OBSERVABILITY_SPEC.md` | AI-native 指标、Ops Dashboard、SLO、Burn-rate 告警和 Provider 健康矩阵。 |
| `design/ADMIN_CONTROL_PLANE_SPEC.md` | 管理控制面的 Dashboard、Ops、设置、公告、兑换码、优惠码和风控 API 边界。 |

## 3. plans/ — 开发计划与进度

| 文档 | 作用 |
| --- | --- |
| `plans/STATUS.md` | 跨 goal 的当前进度、下一推荐工作包和最近门禁记录（**进度权威来源**）。 |
| `plans/ROADMAP.md` | 通往最终平台形态的阶段计划。 |
| `plans/WORK_PACKAGES.md` | 按阶段拆分的可执行工作包、责任范围、完成定义和门禁。 |
| `plans/PROJECT_DEVELOPMENT_PLAN.md` | 项目定位、阶段路线图和技术方向。 |
| `plans/COMMERCIALIZATION_PLAN.md` | （历史 / 已完成）从骨架走向生产级网关的工程计划；进度以 `plans/STATUS.md` 为准。 |
| `plans/MVP_SPEC.md` | （历史 / 已被取代）最初 MVP 需求与验收条件；现状以 `plans/STATUS.md` 为准。 |
| `plans/MVP_IMPLEMENTATION_PLAN.md` | （历史 / 已被取代）最初 MVP 里程碑拆解；现状以 `plans/WORK_PACKAGES.md` 与 `plans/STATUS.md` 为准。 |

## 4. plans/parity/ — sub2api 对齐批次

| 文档 | 作用 |
| --- | --- |
| `plans/parity/goal-sub2api-parity-README.md` | 对齐计划总览：批次划分、已落地与剩余差距。 |
| `plans/parity/goal-sub2api-parity-batch1.md` … `batch19.md` | 每批次的目标、实现方案与验收门禁（batch1–7 已落地；batch8–19 为剩余计划）。 |
| `plans/parity/batch14-end-user-platform-parity.md` | batch14 端用户平台对齐的拆解细化。 |

## 5. 实现 goal 的强制阅读顺序

执行任何实现类 goal 前，按此顺序阅读：

1. `plans/STATUS.md`
2. [`../docs/requirements/GOAL_EXECUTION_PROTOCOL.md`](../docs/requirements/GOAL_EXECUTION_PROTOCOL.md)
3. `plans/WORK_PACKAGES.md`（或对应 `plans/parity/` 批次文档）
4. [`../docs/requirements/QUALITY_GATES.md`](../docs/requirements/QUALITY_GATES.md)
5. 所选工作包引用的 `design/` 与 `../docs/` 文档

Goal prompt 模板：

```txt
Create a goal for SRapi: read specs/README.md, select the next pending work package from specs/plans/STATUS.md, implement it end to end, run its quality gates, update specs/plans/STATUS.md, and stop only when the selected work package is complete or genuinely blocked by the protocol.
```

## 6. Source of Truth 与冲突优先级

最终产品形态由以下文档定义：`FINAL_STATE.md`、`design/` 各子系统规格、`plans/PROJECT_DEVELOPMENT_PLAN.md`。

发生冲突时：

1. 安全问题以 `../docs/requirements/SECURITY_MODEL.md` 为准。
2. HTTP 契约与生成 SDK 以 `../docs/requirements/OPENAPI_CONTRACT.md` 为准。
3. 持久化数据以 `../docs/requirements/DATA_MODEL.md` 为准。
4. 依赖方向以 `../docs/requirements/MODULE_INTERFACE_CONTRACTS.md` 为准。
5. `plans/WORK_PACKAGES.md` 只在实现顺序上有最终发言权，不决定架构。

## 7. 维护规则

- 完成任何工作包必须更新 `plans/STATUS.md`，并把下一个工作包移入 `next_recommended`；不要因为单个工作包完成就把全局 goal 标记为完成。
- 子系统行为变化必须同步对应 `design/` 规格（映射表见 [`../docs/README.md`](../docs/README.md) §7）。
- 对齐批次的目标或验收变化必须同步 `plans/parity/` 对应批次文档。
