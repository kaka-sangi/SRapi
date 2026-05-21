# SRapi 调度场景矩阵

## 1. 目的

本文档把调度内核设计转化为可测试、可验证的具体场景。

这些场景应直接转化为：

- Scheduler 单元测试。
- Integration test。
- 调度模拟器用例。
- 管理后台策略验证样例。

## 2. 测试维度

调度测试至少覆盖：

```txt
健康度
额度水位
延迟
成本
会话粘度
缓存亲和
用户等级
并发限制
RPM/TPM
Provider 故障
账号故障
流式失败
重试与 fallback
```

## 3. 基础术语

候选账号：

```txt
A, B, C
```

用户等级：

```txt
free
standard
pro
admin
```

策略：

```txt
balanced
quality_first
cost_saver
cache_affinity
low_latency
```

## 4. 场景 A：健康优先

### 条件

```txt
账号 A：健康 0.95，额度 50%，成本中等
账号 B：健康 0.60，额度 90%，成本低
策略：balanced
```

### 预期

选择账号 A。

### 原因

Balanced 策略下健康度权重最高，B 虽额度更高且成本更低，但健康明显较差。

### 测试断言

- selected_account = A
- score.health 对结果贡献最高
- B 未被过滤，只是评分较低

## 5. 场景 B：额度保护

### 条件

```txt
账号 A：健康 0.95，额度 8%，高质量账号
账号 B：健康 0.90，额度 70%，普通账号
用户：free
策略：cost_saver
```

### 预期

选择账号 B。

### 原因

A 进入 emergency_only 水位，免费用户不应消耗高质量低水位账号。

### 测试断言

- selected_account = B
- A reject 或 quota_score 显著降低
- decision 中包含 quota_protection reason

## 6. 场景 C：高级用户使用保护水位账号

### 条件

```txt
账号 A：健康 0.98，额度 15%，高质量账号
账号 B：健康 0.70，额度 80%，普通账号
用户：pro
策略：quality_first
```

### 预期

可选择账号 A。

### 原因

Pro 用户质量优先，A 虽额度偏低但仍在 protected，不是 0 或 emergency-only。

### 测试断言

- selected_account = A
- quota_score 降低但未过滤
- priority_score 提高

## 7. 场景 D：会话粘度命中

### 条件

```txt
conversation_hash 已绑定账号 A
账号 A：健康 0.90，额度 60%
账号 B：健康 0.92，额度 60%
策略：balanced
sticky_strength：soft
```

### 预期

选择账号 A。

### 原因

A 和 B 健康接近，soft sticky 使 A 得分更高。

### 测试断言

- selected_account = A
- sticky_hit = true
- sticky_score > 0

## 8. 场景 E：粘性账号故障后切换

### 条件

```txt
conversation_hash 已绑定账号 A
账号 A：状态 rate_limited 或 circuit_open
账号 B：健康 0.88，额度 70%
sticky_strength：soft
```

### 预期

选择账号 B。

### 原因

Soft sticky 不能压过硬过滤和可用性。

### 测试断言

- A 被 PolicyFilter 排除
- selected_account = B
- decision 包含 sticky_broken_reason

## 9. 场景 F：Hard Sticky 账号轻微降级

### 条件

```txt
session_hash 绑定账号 A
账号 A：状态 degraded，健康 0.65，未限流，额度足够
账号 B：健康 0.95
sticky_strength：hard
```

### 预期

选择账号 A。

### 原因

Hard sticky 要求尽量保持账号，除非不可用。

### 测试断言

- selected_account = A
- hard_sticky_hit = true
- A 未被过滤

## 10. 场景 G：Hard Sticky 账号不可用

### 条件

```txt
session_hash 绑定账号 A
账号 A：auth_failed / disabled / dead
账号 B：健康 0.90
sticky_strength：hard
```

### 预期

选择账号 B 或返回无可用账号。

### 原因

Hard sticky 不是使用已失效账号的理由。

### 测试断言

- A 被硬过滤
- 如果 B 可用，selected_account = B
- decision 包含 hard_sticky_unavailable

## 11. 场景 H：缓存亲和优先

### 条件

```txt
长上下文请求：input_tokens = 80000
账号 A：存在 prompt_prefix_hash 缓存，健康 0.88
账号 B：无缓存，健康 0.95
策略：cache_affinity
```

### 预期

选择账号 A。

### 原因

长上下文缓存收益显著，且 A 健康可接受。

### 测试断言

- selected_account = A
- cache_affinity_hit = true
- estimated_cost_saving > 0

## 12. 场景 I：缓存账号健康过差

### 条件

```txt
账号 A：有缓存，健康 0.30，状态 degraded
账号 B：无缓存，健康 0.95
策略：cache_affinity
```

### 预期

选择账号 B。

### 原因

缓存收益不能压过严重健康风险。

### 测试断言

- selected_account = B
- A health penalty 显著
- decision 包含 cache_overridden_by_health

## 13. 场景 J：成本优先

### 条件

```txt
账号 A：成本高，健康 0.95，额度 90%
账号 B：成本低，健康 0.85，额度 90%
用户：free
策略：cost_saver
```

### 预期

选择账号 B。

### 原因

Cost Saver 策略下，B 的健康满足最低阈值且成本更低。

### 测试断言

- selected_account = B
- cost_score 主导

## 14. 场景 K：低延迟策略

### 条件

```txt
账号 A：p95 latency 3000ms，健康 0.95
账号 B：p95 latency 800ms，健康 0.90
策略：low_latency
```

### 预期

选择账号 B。

### 原因

Low Latency 策略下延迟权重最高。

### 测试断言

- selected_account = B
- latency_score 主导

## 15. 场景 L：并发 Lease 限制

### 条件

```txt
账号 A：max_concurrency = 10，current_concurrency = 10
账号 B：max_concurrency = 10，current_concurrency = 5
策略：balanced
```

### 预期

选择账号 B。

### 原因

A 并发已满，被硬过滤或饱和惩罚降到不可选。

### 测试断言

- A rejected reason = concurrency_full
- Lease 创建在 B 上

## 16. 场景 M：RPM 限制

### 条件

```txt
账号 A：RPM 已满
账号 B：RPM 未满
```

### 预期

选择账号 B。

### 测试断言

- A rejected reason = rpm_limit_exceeded
- selected_account = B

## 17. 场景 N：无可用账号

### 条件

```txt
所有候选账号均 disabled / quota_exceeded / circuit_open
```

### 预期

返回 `NO_AVAILABLE_ACCOUNT`。

### 测试断言

- selected_account = null
- error_code = NO_AVAILABLE_ACCOUNT
- decision 记录所有 reject reasons

## 18. 场景 O：Provider 级故障

### 条件

```txt
Provider OpenAI 最近 5xx 激增
Provider Anthropic 正常
模型有 fallback mapping
```

### 预期

降低 OpenAI Provider 候选权重或选择 Anthropic fallback。

### 测试断言

- provider_health_penalty 生效
- fallback_provider_used = true

## 19. 场景 P：模型 fallback

### 条件

```txt
请求 model = premium-model
主 Provider 无可用账号
fallback_models = [compatible-model-a, compatible-model-b]
```

### 预期

按 fallback 顺序和策略尝试候选模型。

### 测试断言

- selected_model in fallback_models
- decision 记录 original_model 和 selected_model

## 20. 场景 Q：用户余额不足

### 条件

```txt
用户余额不足以覆盖 estimated_cost
```

### 预期

请求在调度前或 PolicyFilter 阶段被拒绝。

### 测试断言

- error_code = USER_BALANCE_INSUFFICIENT
- 不创建 Provider request
- 不创建 account lease

## 21. 场景 R：API Key 模型限制

### 条件

```txt
API Key allowed_models = [gpt-4o-mini]
请求 model = gpt-4o
```

### 预期

拒绝请求。

### 测试断言

- error_code = MODEL_NOT_ALLOWED
- Scheduler 不进入账号评分

## 22. 场景 S：流式请求开始前失败

### 条件

```txt
stream = true
选中账号 A
上游连接超时，尚未返回任何 chunk
账号 B 可用
```

### 预期

允许 fallback 到账号 B。

### 测试断言

- retry_attempt = 1
- original_account = A
- selected_account_after_retry = B
- client 未收到重复 chunk

## 23. 场景 T：流式请求中途失败

### 条件

```txt
stream = true
上游已返回部分 chunk 后断开
```

### 预期

默认不重试，记录 partial failure。

### 测试断言

- no_retry_after_first_chunk = true
- usage 记录 partial
- feedback error_class = stream_interrupted

## 24. 场景 U：账号 auth_failed

### 条件

```txt
账号 A 返回 401 auth failed
```

### 预期

账号 A 标记 needs_reauth 或 disabled，后续调度排除。

### 测试断言

- account_status updated
- error_class = auth_failed
- should_disable_account = true 或 needs_reauth = true

## 25. 场景 V：无 usage 返回

### 条件

```txt
Provider 响应成功但没有 usage
```

### 预期

使用估算 usage，标记 estimated。

### 测试断言

- usage_estimated = true
- billing uses estimated usage
- observability 标记 provider_missing_usage

## 26. 场景 W：Top N 加权随机

### 条件

```txt
账号 A/B/C 分数接近
```

### 预期

多次请求分布在 A/B/C，不全部集中到最高分账号。

### 测试断言

- distribution roughly follows weight
- no single account overloaded

## 27. 场景 X：账号冷启动

### 条件

```txt
新账号 A 无历史健康数据
账号 B 健康 0.90
```

### 预期

A 可以获得少量探测流量，但不会承载大量请求。

### 测试断言

- A status = warmup
- A has limited probability
- warmup cap 生效

## 28. 场景 Y：调度模拟器

### 条件

管理员输入请求模型、用户组、策略和账号池状态。

### 预期

返回模拟调度结果，不真实调用 Provider。

### 测试断言

- no lease committed
- no provider request sent
- decision explainable

## 29. 场景 Z：策略权重变更

### 条件

管理员调整 balanced 策略权重。

### 预期

后续调度使用新权重，旧决策仍保留当时权重快照。

### 测试断言

- new decisions use new weights
- old decisions immutable
- strategy_version recorded

## 30. 最小测试集合

Scheduler v1 必须先覆盖：

```txt
A 健康优先
B 额度保护
D soft sticky
E sticky failure fallback
J cost first
L lease concurrency
M rpm limit
N no available account
Q balance insufficient
S stream retry before first chunk
T no retry after first chunk
```

H/I cache affinity 深度落库场景可在 MVP 中作为 pending，MVP 只要求实现 cache-only 分数和对应单元测试。

## 31. 测试数据构造建议

测试中构造统一 fixtures：

```txt
users: free_user, standard_user, pro_user
models: cheap_model, premium_model, long_context_model
providers: openai_compatible, anthropic_compatible
accounts: healthy_high_quota, healthy_low_quota, degraded_cached, rate_limited, disabled
strategies: balanced, cost_saver, quality_first, cache_affinity
```

## 32. 成功标准

每个场景必须验证：

- 是否选中正确账号。
- 未选中账号的原因。
- 分数明细是否符合预期。
- 是否创建或拒绝 Lease。
- 是否生成 Scheduler Decision。
- 是否产生正确 Feedback。
- 是否影响账号健康或额度状态。
