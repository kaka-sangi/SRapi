# SRapi vs sub2api 改进 Goal 总索引

> 来源：2026-06-08，12-agent 对比 SRapi vs sub2api（`/home/senran/Desktop/sub2api`）+ 对抗性核查（47 断言）。
> 诊断：SRapi 管理面比 sub2api 更宽，真正差距集中在 **① 计费引擎"深度"**（漏收/0 元放行/合成配额淹没真实信号）+ **② 信息架构"内聚度"**（同一能力拆成 6+ 散页、信息多处重复），**不是功能广度缺失**。
> 方向：保留 SRapi 规范化数据层（显式表 / AES-GCM / OpenAPI-first）→ 补齐计费维度 → UI 重新收敛。

每个批次都是一份**自包含、可验证、按依赖排序**的 goal 提示词，遵循「角色/背景 → 目标与动机 → 范围/非范围 → 约束 → 成功标准 → 顺序 → 交付物 → 禁止事项 → 进度续作 → 停止条件 → 参考」结构（依据 Anthropic prompt-engineering / Claude Code /goal 文档 / long-running-agents / OKR）。直接把对应文件整段粘给 Codex 即可。

## 推荐执行顺序

| 批次 | 文档 | 覆盖 rank | 主题 | 依赖 |
|---|---|---|---|---|
| 第一批 | [batch1](./goal-sub2api-parity-batch1.md) | 1,2,3,4,11 | 收入安全急修 + 渠道聚合页 + 用户面闭环 | 无 |
| 第二批 | [batch2](./goal-sub2api-parity-batch2.md) | 16,8 | 计费域地基重构（money 公共包 + 定价归位） | 建议接第一批 |
| 第三批 | [batch3](./goal-sub2api-parity-batch3.md) | 5,12 | 计费维度深化（billing_mode/区间 + usage_log 分解） | 强烈建议接第二批 |
| 第四批 | [batch4](./goal-sub2api-parity-batch4.md) | 9,15 | 额度护栏与性能（订阅行物化 + Key USD 配额） | 用 money 包 |
| 第五批 | [batch5](./goal-sub2api-parity-batch5.md) | 6,7,13,17 | 配额真实性 + 账户导入认证 | 第一批配额分离 |
| 第六批 | [batch6](./goal-sub2api-parity-batch6.md) | 10,14,18 | 信息架构收尾 + 用户自助 + 去重 | 可独立 |
| 第七批 | [batch7](./goal-sub2api-parity-batch7.md) | 认证矩阵 | 上游认证矩阵真实性（消灭摆设认证 + 刷新闭环 + CI 守门） | 与 batch5 强相关 |

> 第二、三批有强依赖（重构在前、深化在后）；第四、五、六批相对独立，可按业务优先级调序。
> 第七批源于「认证方式×提供商」逐格核查（见下「认证矩阵基线」），是 batch5「配额真实性」的认证侧根因——摆设认证 = 假配额，建议与 batch5 同期或先行。

## 认证矩阵基线（防回归 · 已逐格 trace 网关签发路径核查）

当前落地矩阵见 [`PROVIDER_AUTH_MATRIX.md`](./PROVIDER_AUTH_MATRIX.md)。第七批后，preset `auth_methods` 只暴露真实可签发集合：

- `service_account_json` 已从 anthropic/bedrock preset 下架，仅作为 legacy enum 保留。
- OpenAI/Gemini preset OAuth 已下架；手工误配会返回 `not_supported`。
- Antigravity 的 `desktop_client_token` / `ide_plugin_token` 已并入 preset `oauth_refresh` 路径。
- `chatgpt-web` preset 已补齐，默认暴露 `web_session_cookie` 并安装为 `reverse-proxy-chatgpt-web`。
- CI 守门为 `TestPresetRuntimeAllowlistsOnlyExposeSignableAuthMethods`。

### 端用户登录（userauthidentity，与上游认证是两张表）

✅ 真有效：password、totp、email(仅验证+找回)、oidc、google、github、linuxdo（一条通用 OAuth2/OIDC 流水线）。
⬜ 摆设：wechat、dingtalk（仅通用 Bearer-userinfo，驱动不了真实非标准协议）。
⬜ 缺失：微信支付 OAuth、邮箱验证码注册登录、microsoft、discord。
横切缺口：OAuth 老用户靠 FindByEmail 匹配、subject 仅 bind 去重 → 每次仍需重输密码（无免密快速通道）。
→ 端用户登录摆设清理列为 batch7 附录的候选 batch8。

### 关系建模（一句话）

SRapi = `runtime_class`(账号锁一种认证) + `Provider.auth_methods`(=preset RuntimeClassAllowlist，仅创建时校验白名单)；**allowlist 是「承诺」、`injectAuth`+`oauthRefreshSettings` 才是「兑现」，脱节即摆设**。
sub2api = `Account.platform`(4常量) + `type`(6常量) + `credentials`JSONB 三件套正交组合，无独立白名单层，codex/geminicli/Vertex 是平台子形态。

## 完整 18 项 backlog（按价值×置信÷成本排序）

| # | 行动 | 类别 | 工作量 | 置信 | 批次 |
|---|---|---|---|---|---|
| 1 | 分离合成 vs 真实配额快照（调度/告警失真） | 缺失 | S | 高 | 1 |
| 2 | family 模糊兜底杜绝 default_zero 0 元放行 | 缺失 | M | 高 | 1 |
| 3 | rate_multiplier + actual_cost | 缺失 | M | 高 | 1 |
| 4 | /admin/channels/[id] 渠道详情聚合页 | 散乱 | L | 高 | 1 |
| 5 | billing_mode（按次/图片）+ token 区间分层 | 缺失 | L | 高 | 3 |
| 6 | Anthropic OAuth 真实额度（5h/7d/7d-Sonnet） | 缺失 | L | 高 | 5 |
| 7 | Preset 增 OAuth → per-platform 一键授权 | 缺失 | M | 高 | 5 |
| 8 | 定价从 subscriptions 抽到 billing 域 | 散乱 | L | 高 | 2 |
| 9 | 订阅行物化 daily/weekly/monthly 用量 | 缺失 | L | 高 | 4 |
| 10 | 用户端 available-channels 自助页 | 缺失 | M | 高 | 6 |
| 11 | dashboard 露余额 + per-user-platform 配额 | 重复 | S | 高 | 1 |
| 12 | usage_log 成本四项分解 + requested/upstream 快照 | 缺失 | M | 高 | 3 |
| 13 | 统一导入入口 + 共用指纹去重 | 散乱 | M | 高 | 5 |
| 14 | ops 监控收进 /admin/ops tab + scheduler 归位 | 散乱 | M | 高 | 6 |
| 15 | API Key USD 成本配额 + 滑窗花费上限 | 缺失 | M | 高 | 4 |
| 16 | 抽 money 公共包 + 统一 USD/0 常量 | 重复 | M | 高 | 2 |
| 17 | 账号封禁/验证状态识别 + 用户配额自助/reset | 缺失 | M | 中 | 5 |
| 18 | 统一 usage 聚合 struct + dashboard/usage 共享 hook | 重复 | S | 高 | 6 |

## 二期（核查中存在但未进 top-18，刻意暂缓）

- service_tier（priority 2x / flex 0.5x）价
- cache 5m / 1h 分档计费
- 长上下文整次会话倍率
- LiteLLM 远程价表定时同步（本方案仅做本地 family 兜底）
- Gemini tier RPD/RPM 完整配额视图
- CRS 整库迁移、明文凭证往返备份

## 已核查纠正（别再当缺口）

- 用户登录 OAuth bind/adopt-by-email：**已有**（bind-current-user + bind-login 端点）。
- 订阅等级/credits 提取：**框架已存在**（config-driven QuotaReport，仅 Codex preset 接了）。
- 计费幂等：**有去重闸门**（(request_id,attempt_no) 唯一索引 + charged_at IS NULL），缺的是内容指纹冲突检测层。
- BillingLedger：**数据已存在**（单行 running-balance，非双分录），缺的是面向用户自己的 /me 端点。

## 必须保留的 SRapi 优势（改造时勿退化）

string + big.Rat 8 位定点（非 float）、cacheWriteRateOrInput 防漏收回退、PricingRule effective 时间窗、AES-GCM 凭证加密、device-code、通用 OAuth 状态机、语义化 NavSection 分组骨架。
