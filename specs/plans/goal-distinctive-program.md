# Goal:业务面完善 + SRapi 特色化(特别是前端)—— Distinctive Program

> 来源:2026-06-12 三路并行审计(sub2api 增量 / CLIProxyAPI 首次系统对比 / SRapi 前端特色机会),全部缺口已逐项在 SRapi 代码反向验证。
> 与 parity program(specs/plans/parity/,batch1-20 ✅)的关系:parity 是追赶,本 program 是 **完善业务面 + 打造 SRapi 自己的身份**(开发者友好的 AI 网关 + 商业化计费平台 + 暖纸编辑风)。
> 铁律不变:摆设只有「真接通」或「诚实下架」;业务面完整 > craft polish;前端改动必须浏览器验证。

## Wave 1 · 前端特色速赢(全部建立在已有后端端点上)

| # | 项 | 规模 | 说明 |
|---|---|---|---|
| W1-1 | **Key 接入向导** | S | 创建 key 成功后给 curl / OpenAI SDK / Anthropic SDK 片段(baseURL+key 内联)+「测试此 key」按钮(打 /v1/models)。补 sub2api UseKeyModal 缺口并超越。 |
| W1-2 | **Playground 三件套** | S-M | system prompt 编辑、温度/参数面板、会话 localStorage 持久化(刷新不丢)。 |
| W1-3 | **充值到账闭环** | S | 下单后轮询 `GET /api/v1/payment/orders`,到账动效 + 余额刷新。钱信任面。 |
| W1-4 | **公开 key 用量自查页** | S | 免登录粘贴 key 查用量(后端 `GET /v1/usage` key 自鉴权已存在)。分发/转售场景。 |
| W1-5 | **custom_menus 摆设修复** | S-M | settings 已有 custom_menus 配置但零消费点 → 导航渲染自定义链接。 |
| W1-6 | **/scheduler-decisions 死路由清理** | S | 用户侧 301 到 admin 撞权限墙 → 移除或修正。 |

## Wave 2 · 网关正确性/账号安全(后端小件)

| # | 项 | 规模 | 说明 |
|---|---|---|---|
| W2-1 | **claude-fable-5 完整支持** | S | 定价种子 + antigravity 白名单 + 模型目录/前端(bedrock 映射已有)。旗舰模型计费正确性。 |
| W2-2 | **Bedrock anthropic-beta 白名单过滤** | S | 现仅去重不过滤 → 不支持的 beta 上游 400。 |
| W2-3 | **previous_response_id 失配保护** | S-M | failover 换账号后剥离失配 id,防 Codex 长会话硬失败。 |
| W2-4 | **token refresh singleflight** | S | per-account 刷新去重,防并发竞态持久化死 token → 封号。 |
| W2-5 | **count_tokens 跨协议兜底** | S | openai 系上游无 TokenCount → Claude Code 在 openai 渠道 count_tokens 报错。 |
| W2-6 | **ModelAlias.FallbackModels 摆设处置** | S-M | 存而不读(仅 scheduler hints,从不降级)→ 真接通或诚实下架。 |

## Wave 3 · 特色大件(M,按价值排序,逐个做)

1. **Playground 对比模式**:两模型并排流式 + 单条消息成本/token 标注 —— 把交界地从玩具变成选型工具(独家资产)。
2. **用量开发者视角**:趋势图 + 日期范围预设 + per-key/per-model 钻取 + CSV —— 计费平台核心信任面。
3. **公开状态页 /status**:模型×渠道可用性时间线(脱敏公开端点小增量)—— 网关公信力名片。
4. **成本模拟器**:pricing rules → 交互计算器(选模型/拖 token/缓存折扣实时算价)—— 把计费深度变成产品卖点。
5. **图片能力级冷却**:图片 429 只冷却 image capability,不殃及文本。
6. **逐请求 payload 捕获+下载**(运维取证)。

## 明确不做/单独决策(记录防遗忘)

- 跨协议增量流式翻译(L,结构性,需单独 goal)
- 集群/插件平台/Vertex/AI Studio relay(架构级决策)
- Web Search 模拟、代理有效期回退、合规门禁、admin 删用户、Kimi/Grok/GeminiCLI OAuth 上游(价值真实,排后续批)
- 系统自更新(与部署方向冲突,跳过)

## 验收口径

每项:focused tests 过 + 前端浏览器验证截图;按域分 commit;最终全量门禁(go test/vet/build/architecture/codequality/migration-check/双端 codegen/web typecheck+lint)。
