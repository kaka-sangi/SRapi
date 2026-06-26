---
name: usage-audit
description: 当用户要求查看用量统计、账单、消费情况、某个用户或某个模型的使用量时使用。
---
# 查看用量与审计

## 步骤

1. **确认查询范围** — 确认用户要看什么：
   - 全局用量概览？
   - 某个用户的用量？
   - 某个模型的用量？
   - 某个时间段的用量？

2. **查询用量数据**：
   - 全局概览：`GET /api/v1/admin/usage/summary`
   - 按用户：`GET /api/v1/admin/usage?user_id={id}`
   - 按模型：`GET /api/v1/admin/usage?model={model_name}`
   - 今日账号级用量：`GET /api/v1/admin/accounts/usage-today`
   先 `get_operation_detail` 确认可用的查询参数。

3. **查看审计日志**（如需要）：
   - `GET /api/v1/admin/audit-logs?resource_type={type}&page_size=20`
   - 按操作类型：`GET /api/v1/admin/audit-logs?action={action}`

4. **查看账单明细**（如需要）：
   - `GET /api/v1/admin/billing/ledger`

5. **报告** — 用表格和数字清晰展示：
   - 总请求数、总 token 数、总费用
   - 按模型/用户/供应商分组的用量
   - 趋势（与之前对比）

## 注意事项
- 用量查询都是只读操作，可以直接运行。
- 大时间范围的查询可能返回大量数据，使用分页和过滤缩小范围。
