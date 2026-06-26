---
name: diagnose-account-health
description: 当用户要求检查账号健康状态、排查账号故障、查看哪些账号异常时使用。
---
# 诊断账号健康

## 步骤

1. **获取健康概览** — `GET /api/v1/admin/accounts/health-summary`
   这会返回所有账号的健康摘要（状态、错误、最后使用时间等）。

2. **分析问题** — 从摘要中找出：
   - status 不是 active 的账号
   - 有 last_error 的账号
   - 长时间未使用的账号（last_used_at 很久之前或为空）
   - health_status 异常的账号

3. **深入排查**（如需要）：
   - 查看特定账号详情：`GET /api/v1/admin/accounts/{id}`
   - 查看账号错误日志：`GET /api/v1/admin/ops/error-logs?account_id={id}`
   - 检查账号可用性：`GET /api/v1/admin/accounts/{id}/availability`
   - 测试账号连通性：`POST /api/v1/admin/accounts/{id}/test`（需审批）

4. **报告** — 用表格展示结果：
   - 健康账号数量 / 异常账号数量
   - 每个异常账号的名称、供应商、错误原因
   - 建议的修复操作（刷新令牌、清除错误、禁用等）

5. **提供修复建议** — 根据错误类型：
   - 令牌过期 → 建议使用 `batch-refresh-tokens` skill 批量刷新
   - 账号被封 → 建议禁用并更换
   - 临时错误 → 建议清除错误让调度器重试：`POST /api/v1/admin/accounts/batch-action { "account_ids": [...], "action": "clear_error" }`

## 注意事项
- health-summary 是只读操作，可以直接运行。
- 测试连通性（test）会实际调用上游 API，属于写操作需要审批。
