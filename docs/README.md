# SRapi 文档地图

## 1. 目的

`docs/` 保存 SRapi 架构、实现规格、运行治理和产品能力边界。代码、接口、配置、部署、运行时或安全策略发生变化时，必须同步更新对应文档。

## 2. 核心必读文档

| 文档 | 作用 |
| --- | --- |
| `PROJECT_DEVELOPMENT_PLAN.md` | 项目定位、阶段路线图和技术方向。 |
| `MVP_SPEC.md` | MVP 功能需求、非功能需求、验收条件和测试映射。 |
| `MVP_IMPLEMENTATION_PLAN.md` | MVP 里程碑拆解和实现顺序。 |
| `ARCHITECTURE.md` | 后端模块边界、依赖方向和调用链。 |
| `ARCHITECTURE_REQUIREMENTS.md` | MVP 架构要求、启动 harness 和证据映射。 |
| `MODULE_INTERFACE_CONTRACTS.md` | 模块间 contract、DTO、同步调用和事件边界。 |
| `DOMAIN_EVENTS_SPEC.md` | 领域事件、Outbox、Inbox、幂等、重试和补偿规则。 |
| `DOMAIN_MODEL.md` | 核心业务概念、术语和领域关系。 |
| `DATA_MODEL.md` | PostgreSQL 表、索引、一致性和加密字段。 |
| `OPENAPI_CONTRACT.md` | HTTP 契约、错误、鉴权、分页和 codegen 规则。 |
| `SECURITY_MODEL.md` | API Key、Cookie、CSRF、凭证、日志、审计和密钥轮换。 |
| `CONFIGURATION_SPEC.md` | 环境变量、配置优先级、默认值和生产安全约束。 |
| `OPERATIONS.md` | 迁移、备份、健康检查、发布、数据生命周期和事故处理。 |

## 3. Gateway 与 Provider 文档

| 文档 | 作用 |
| --- | --- |
| `AI_ENDPOINT_COMPATIBILITY.md` | Chat Completions、Responses、Messages、Gemini 等端点互转与 Canonical AI IR。 |
| `GATEWAY_ROUTE_MATRIX.md` | Gateway 路由族、Provider alias、passthrough、WebSocket 和阶段规划。 |
| `PROVIDER_ADAPTER_SPEC.md` | Provider Adapter 扩展、错误分类、usage 和流式解析规范。 |
| `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md` | OpenAI-compatible / Anthropic-compatible preset、默认 base URL、auth mode、模型目录和 route alias。 |
| `CAPABILITY_TAXONOMY_SPEC.md` | Request / Model / Provider / Endpoint capability 命名、版本、降级和匹配规则。 |
| `REVERSE_PROXY_SPEC.md` | 2api 反代、TLS / HTTP/2 / Header 指纹、cookie / OAuth 凭证和反封号策略。 |

## 4. Scheduler 文档

| 文档 | 作用 |
| --- | --- |
| `SCHEDULING_KERNEL_DESIGN.md` | 调度内核总体设计和长期演进模型。 |
| `SCHEDULER_V1_SPEC.md` | MVP 调度过滤、打分、Lease、Decision 和 Feedback 规则。 |
| `SCHEDULER_STRATEGY_EXTENSION_SPEC.md` | 调度策略注册、版本、灰度、dry-run、shadow decision 和回滚规则。 |
| `SCHEDULING_SCENARIOS.md` | Scheduler 单元测试、集成测试和模拟器场景。 |

## 5. 商业化与运营文档

| 文档 | 作用 |
| --- | --- |
| `PAYMENT_SPEC.md` | 支付渠道、多实例、订单状态机、Webhook、退款和幂等。 |
| `AFFILIATE_REBATE_SPEC.md` | 邀请关系、返利规则、返利账本、退款补偿和转余额。 |
| `OBSERVABILITY_SPEC.md` | AI-native 指标、Ops Dashboard、SLO、Burn-rate 告警和 Provider 健康矩阵。 |

## 6. 前端文档

| 文档 | 作用 |
| --- | --- |
| `FRONTEND_DESIGN_SYSTEM.md` | 控制台视觉、组件、动效和响应式约束。 |

## 7. Codex 执行规格

| 文档 | 作用 |
| --- | --- |
| `../specs/README.md` | 长期 Codex goal 的入口、阅读顺序和恢复提示。 |
| `../specs/WORK_PACKAGES.md` | 按阶段拆分的可执行工作包、责任范围和完成定义。 |
| `../specs/QUALITY_GATES.md` | 不同变更类型必须运行的质量门禁。 |
| `../specs/STATUS.md` | 跨 goal 的当前进度、下一推荐工作包和最近门禁记录。 |

## 8. 维护规则

- 改接口必须同步 `OPENAPI_CONTRACT.md`。
- 改跨模块调用必须同步 `MODULE_INTERFACE_CONTRACTS.md`。
- 改领域事件、异步补偿或 Outbox 必须同步 `DOMAIN_EVENTS_SPEC.md`。
- 改数据表必须同步 `DATA_MODEL.md`。
- 改 Gateway 路由必须同步 `GATEWAY_ROUTE_MATRIX.md`。
- 改 Provider preset 必须同步 `COMPATIBLE_PROVIDER_REGISTRY_SPEC.md`。
- 改模型、Provider、请求或端点能力必须同步 `CAPABILITY_TAXONOMY_SPEC.md`。
- 改反代行为必须同步 `REVERSE_PROXY_SPEC.md`。
- 改调度策略必须同步 `SCHEDULER_V1_SPEC.md`、`SCHEDULER_STRATEGY_EXTENSION_SPEC.md` 和 `SCHEDULING_SCENARIOS.md`。
- 改配置键必须同步 `CONFIGURATION_SPEC.md`。
- 改部署、迁移、备份、日志、健康检查必须同步 `OPERATIONS.md`。
- 改支付必须同步 `PAYMENT_SPEC.md`。
- 改返利必须同步 `AFFILIATE_REBATE_SPEC.md`。
- 改可观测、告警、运维后台必须同步 `OBSERVABILITY_SPEC.md`。
- 改架构要求、启动 harness 和门禁映射必须同步 `ARCHITECTURE_REQUIREMENTS.md`。
