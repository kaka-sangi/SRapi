# SRapi 文档地图（docs/）

> `docs/` 回答"**怎么开发 SRapi**"：开发要求、限制与边界、参考项目启示。
> "SRapi 最终要长成什么样、下一步做什么"在 [`../specs/`](../specs/README.md)（最终形态 + 开发计划）。

## 1. 目录结构

| 子目录 | 含义 | 放什么 |
| --- | --- | --- |
| [`requirements/`](#2-requirements--开发要求) | 开发要求 | 改代码必须满足、必须同步的规范：架构、契约、数据、安全、配置、运维、文案、流程门禁。 |
| [`constraints/`](#3-constraints--限制与边界) | 限制与边界 | SRapi 是什么 / 不是什么 / 哪些能力是真实的。 |
| [`insights/`](#4-insights--启示) | 启示 | 从参考项目（sub2api / CLIProxyAPI）学到的决策与迁移认知。 |

## 2. requirements/ — 开发要求

| 文档 | 作用 |
| --- | --- |
| `requirements/ARCHITECTURE.md` | 后端模块边界、依赖方向和调用链。 |
| `requirements/ARCHITECTURE_REQUIREMENTS.md` | 架构要求、启动 harness 和证据映射。 |
| `requirements/MODULE_INTERFACE_CONTRACTS.md` | 模块间 contract、DTO、同步调用和事件边界。 |
| `requirements/DOMAIN_MODEL.md` | 核心业务概念、术语和领域关系。 |
| `requirements/DOMAIN_EVENTS_SPEC.md` | 领域事件、Outbox、Inbox、幂等、重试和补偿规则。 |
| `requirements/DATA_MODEL.md` | PostgreSQL 表、索引、一致性和加密字段。 |
| `requirements/OPENAPI_CONTRACT.md` | HTTP 契约、错误、鉴权、分页和 codegen 规则。 |
| `requirements/SECURITY_MODEL.md` | API Key、Cookie、CSRF、凭证、日志、审计和密钥轮换。 |
| `requirements/CONFIGURATION_SPEC.md` | 环境变量、配置优先级、默认值和生产安全约束。 |
| `requirements/OPERATIONS.md` | 迁移、备份、健康检查、发布、数据生命周期和事故处理。 |
| `requirements/PRODUCT_TONE.md` | 产品定位、中英文语气规则、术语替换表。 |
| `requirements/FRONTEND_ARCHITECTURE.md` | apps/web 工程结构、模块边界、数据流、鉴权链路、质量门、性能预算。 |
| `requirements/FRONTEND_DESIGN_SYSTEM.md` | 控制台视觉、组件、动效和响应式约束。 |
| `requirements/QUALITY_GATES.md` | 不同变更类型必须运行的质量门禁。 |
| `requirements/GOAL_EXECUTION_PROTOCOL.md` | 多轮长期开发 goal 的安全执行规则。 |

## 3. constraints/ — 限制与边界

| 文档 | 作用 |
| --- | --- |
| `constraints/CAPABILITY_BOUNDARIES.md` | 能力边界：SRapi 承诺什么、明确不做什么。 |
| `constraints/2API_REVERSE_PROXY_DEFINITION.md` | SRapi 中"反代/2api"的权威定义：SRapi 模拟官方客户端请求上游，而不是本地 CLI 作为下游入口。 |
| `constraints/PROVIDER_AUTH_MATRIX.md` | Provider × 认证方式真实有效性矩阵（哪些认证真能签发、哪些是摆设）。 |

## 4. insights/ — 启示

| 文档 | 作用 |
| --- | --- |
| `insights/REFERENCE_PROJECT_DECISIONS.md` | 从 `sub2api` / `CLIProxyAPI` 借鉴什么、不复制什么。 |
| `insights/MIGRATION_GUIDE_2API.md` | 从 sub2api / CLIProxyAPI / chatgpt2api 风格部署迁移到 SRapi 的指南。 |

## 5. 最终形态与开发计划（specs/）

子系统的目标设计规格（Gateway、Scheduler、支付、可观测等）在 [`../specs/design/`](../specs/README.md)；
路线图、工作包、进度台账在 [`../specs/plans/`](../specs/README.md)。入口见 [`../specs/README.md`](../specs/README.md)。

## 6. 仓库根目录治理文档

| 文档 | 作用 |
| --- | --- |
| `../README.md` / `../README.zh-CN.md` | 仓库总览、快速开始和能力概览（英文 / 中文）。 |
| `../ARCHITECTURE.md` | 根目录架构入口，指向 `requirements/` 下的 canonical 架构文档。 |
| `../CONTRIBUTING.md` | 贡献流程、本地开发和门禁约定。 |
| `../SECURITY.md` | 安全披露流程与联系方式。 |
| `../LICENSE` | 开源许可证。 |

## 7. 维护规则

代码、接口、配置、部署、运行时或安全策略发生变化时，必须同步更新对应文档：

- 改接口必须同步 `requirements/OPENAPI_CONTRACT.md`。
- 改跨模块调用必须同步 `requirements/MODULE_INTERFACE_CONTRACTS.md`。
- 改领域事件、异步补偿或 Outbox 必须同步 `requirements/DOMAIN_EVENTS_SPEC.md`。
- 改数据表必须同步 `requirements/DATA_MODEL.md`。
- 改配置键必须同步 `requirements/CONFIGURATION_SPEC.md`。
- 改部署、迁移、备份、日志、健康检查必须同步 `requirements/OPERATIONS.md`。
- 改架构要求、启动 harness 和门禁映射必须同步 `requirements/ARCHITECTURE_REQUIREMENTS.md`。
- 新增任何用户可见文案（控制台、错误信息、营销页）必须先核对 `requirements/PRODUCT_TONE.md` 的中英文术语表与语气规则。
- 改 Provider 认证注入或 preset `auth_methods` 必须同步 `constraints/PROVIDER_AUTH_MATRIX.md`。
- 改 Gateway 路由必须同步 `../specs/design/GATEWAY_ROUTE_MATRIX.md`。
- 改 Provider preset 必须同步 `../specs/design/COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`。
- 改模型、Provider、请求或端点能力必须同步 `../specs/design/CAPABILITY_TAXONOMY_SPEC.md`。
- 改反代行为必须同步 `../specs/design/REVERSE_PROXY_SPEC.md`。
- 改调度策略必须同步 `../specs/design/SCHEDULER_V1_SPEC.md`、`../specs/design/SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 和 `../specs/design/SCHEDULING_SCENARIOS.md`。
- 改支付必须同步 `../specs/design/PAYMENT_SPEC.md`。
- 改返利必须同步 `../specs/design/AFFILIATE_REBATE_SPEC.md`。
- 改可观测、告警、运维后台必须同步 `../specs/design/OBSERVABILITY_SPEC.md`。
- 改管理控制面 Dashboard、Ops、设置、公告、兑换码、优惠码或风控 API 必须同步 `../specs/design/ADMIN_CONTROL_PLANE_SPEC.md`。
