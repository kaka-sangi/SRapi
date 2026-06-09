# Goal：计费维度深化（billing_mode/区间分层 + usage_log 成本分解）（第三批）

> 用途：整段（或 `<role_and_context>`…`<stop_clause>`）粘贴给编码 agent（Codex / Claude Code）。
> 背景：覆盖 ranked backlog rank 5 + rank 12。补齐 SRapi 相对 sub2api 缺失的计费维度，堵住「图片模型 token=0 收 0 元」与「长上下文阶梯收不到」的漏收，并让费用可对账。
> 前置：强烈建议第二批（定价已归位 billing 域、money 公共包）先落地。

---

<role_and_context>
你是 SRapi 仓库的资深全栈工程师。SRapi 是一个 AI 网关/计费平台（对标 sub2api）。
技术栈：后端 Go + ent ORM（apps/api，OpenAPI-first，定价计算应位于 billing 域，schema 在 apps/api/ent/schema/，网关在 apps/api/internal/httpserver/）；前端 Next.js + TypeScript（apps/web）。
本地开发：make dev-up + npm run dev；登录 admin@srapi.local / Admin1234。所有改动须过 go build/vet + 前端 tsc/lint，遵循 OpenAPI-first。绝不抄 sub2api 源码。
</role_and_context>

<objective>
目标（可衡量最终态）：(a) PricingRule 支持多计费模式（token / per_request / image）与 token 区间分层定价，EstimatePrice 按模式分发，图片/按次模型不再因 token=0 而收 0 元、长上下文阶梯能正确计价；(b) UsageLog 物化成本四项分解（input/output/cache_read/cache_write_cost）+ requested_model/upstream_model + billing_mode 快照，使费用构成可向用户展示、可对账申诉。完成后有针对性测试佐证每种模式与区间命中。
动机：纯图片/Veo/按次模型按 token 估算 = 收 0 元（收入漏洞）；>200K 长上下文翻倍这类阶梯收不到（亏钱）；usage_log 只有单 cost 列，无法解释费用来源、对账困难。SRapi 已支持图片/音频网关，定价层必须跟上。
</objective>

<scope>
本次包含（按 sequencing 一次推进一个单元）：
1. schema：PricingRule 增 billing_mode 字段（token/per_request/image）+ per_request_price；新增 pricing_interval 子表（min_tokens/max_tokens + 该区间四项分档价 + 可选 tier_label / 分辨率档）。UsageLog 增 input_cost/output_cost/cache_read_cost/cache_write_cost + requested_model/upstream_model + billing_mode 快照列。
2. compute：EstimatePrice 按 billing_mode 分发到 token / per_request / image 三条算法；token 模式按 input+cacheRead 总 token 落区间，无区间则回退 flat 价（与现状兼容）；PricingResult 返回四项分项明细供写入 usage_log。
3. 网关接线：media/audio handler 把 image_count / size 传入计费上下文；recordGatewayUsage 写入四项分解 + requested/upstream + billing_mode 快照。
4. 前端：定价管理页支持选择 billing_mode 与编辑区间；usage 明细页展示费用四项构成（input/output/cacheRead/cacheWrite）。

明确不做（out-of-scope）：
- 不做 service_tier(priority/flex) 价、不做 cache 5m/1h 分档、不做长上下文整次会话倍率、不做 LiteLLM 远程价表同步（这些是更后续的「二期定价维度」，本批不碰，也不要假装做）。
- 不改 effective 时间窗 / provider_id=0 通配优先级。
- 不动 rate_multiplier 逻辑（第一批已落地，本批沿用）。
</scope>

<constraints>
途中不得改变：
- 金额 string + big.Rat 8 位定点，绝不改回 float；用第二批的 internal/pkg/money。
- token 模式无区间时的行为必须与现状 flat 价逐位一致（回归测试佐证）。
- 现有 OpenAPI 响应结构兼容：usage_log 新增字段为可选/附加，不破坏现有用量端点结构（破坏需先说明）。
- ent schema 改动后 go generate ./ent/...，同步 store/mock（memory store DeletedAt 过滤、Store-mock codegen 已知坑）。
- 不得修改无关 *_test.go；新增能力必须配新测试。
其他约束：前端遵循 warm-paper 视觉与既有组件，不引入新设计语言。
</constraints>

<success_criteria>
完成需同时满足（每条给出可观察证据）：
1. cd apps/api && go build ./... && go vet ./...，退出码 0（贴尾部）。
2. per_request 可证：测试证明 billing_mode=per_request 时，token=0 的请求仍按 per_request_price 计费（非 0）；贴出测试通过输出。
3. image 可证：测试证明 image 模式按 image_count×档位价计费；贴出输出。
4. 区间可证：测试证明 token 落入某区间时用该区间价、无区间回退 flat 价与旧值一致；贴出输出。
5. usage_log 分解可证：测试证明一次计费写入的 input/output/cache_read/cache_write_cost 之和 = total cost，且 requested/upstream/billing_mode 已快照；贴出输出。
6. 前端：cd apps/web && npm run lint && npx tsc --noEmit 退出码 0（贴尾部）。
7. 浏览器验证（chrome-devtools）：截图证明定价页可选 billing_mode/编辑区间，usage 明细页显示费用四项构成。
8. go test ./... 全绿 + git status 干净，git diff --stat 贴出。
或 30 turns 后停止并汇总剩余阻塞项。
</success_criteria>

<sequencing>
每单元自带测试并单独 commit：
1. 阶段一：schema 改动（PricingRule.billing_mode/per_request_price、pricing_interval 子表、UsageLog 四项+快照列）→ go generate → 同步 store/mock。
2. 阶段二：EstimatePrice 按模式分发 + 区间命中 + PricingResult 分项；配单测（per_request/image/区间/flat 回退/分项求和）。
3. 阶段三：网关接线（media/audio 传 image_count/size；recordGatewayUsage 写分解+快照）。
4. 阶段四：前端定价页模式/区间编辑 + usage 明细四项展示。
5. 阶段五：全量 go test + 前端构建 + 浏览器验证。
每阶段一个 commit，message 末尾加 Co-Authored-By 行。
</sequencing>

<artifact>
交付物：feat 分支若干语义化 commit + schema/compute/网关/前端改动 + 新增测试（全 passing）+ progress.txt 收尾（已完成阶段、各成功标准证据位置、未做的 out-of-scope 项）。
</artifact>

<guardrails>
绝不：删/改既有测试让其通过；hardcode 过测试；--no-verify 或 git push --force；把金额改回 float；伪造未接入的能力（service_tier/cache 分档/长上下文倍率本批不做就不要假装做）。
先问我：破坏现有 OpenAPI 响应结构、删除现有顶层路由文件、不可逆数据迁移。
正常措辞执行即可。
</guardrails>

<progress_and_resume>
每完成一阶段更新 progress.txt + git commit。
新 context 开始：先 pwd、git log --oneline -10、读 progress.txt，再 go build ./... 确认编译态，从下一步继续。
</progress_and_resume>

<stop_clause>
满足全部 success_criteria 即停；若 30 turns 仍未达成，停下汇总：已完成阶段、编译/测试状态、剩余阻塞项及原因。
</stop_clause>

<references>
- 定价计算（第二批后应在 billing 域，否则仍在）：apps/api/internal/modules/subscriptions/service/service.go（EstimatePrice:515 / priceFromRule:695）。
- 定价 schema：apps/api/ent/schema/pricingrule.go（现仅 input/output/cache_read/cache_write per-million + effective 时间窗）。
- 用量 schema：apps/api/ent/schema/usagelog.go:38-44（现仅 cost+billable_cost，需加四项+快照）。
- 网关计费：apps/api/internal/httpserver/runtime_gateway_core.go（gatewayPricing:554，PricingSource 标记）；media/audio handler：apps/api/internal/httpserver/runtime_gateway_media*/audio_handlers.go。
- 定价页：apps/web/src/app/admin/channels/pricing/page.tsx；用量页：apps/web/src/app/usage/page.tsx。
sub2api 仅作能力对照不抄代码：channel.go BillingMode(token/per_request/image)、FindMatchingInterval 区间匹配、CalculateImageCost 按 1K/2K/4K、usage_log 四项 cost+快照、computeTokenBreakdown 算法思路。
</references>
