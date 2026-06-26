---
name: batch-account-status
description: 当用户要求批量启用、禁用、或变更多个上游账号状态时使用。包括按供应商、按分组、按状态筛选后批量操作。
---
# 批量变更账号状态

## 步骤

1. **确认范围** — 向用户确认：要操作哪些账号？按什么筛选（供应商、分组、当前状态）？目标状态是什么（active / disabled）？

2. **查询账号列表** — 根据筛选条件获取账号：
   - 按供应商：`GET /api/v1/admin/accounts?provider_id={id}&status={current_status}&page_size=200`
   - 按分组：先 `GET /api/v1/admin/account-groups` 找到分组 ID，再 `GET /api/v1/admin/account-groups/{id}/members`
   - 不确定供应商 ID 时：先 `GET /api/v1/admin/providers` 查出供应商列表

3. **报告数量并确认** — 告诉用户将要操作 N 个账号，列出前几个的名称，请用户确认。

4. **执行批量更新** — 使用批量端点：
   ```
   POST /api/v1/admin/accounts/batch
   {
     "account_ids": ["id1", "id2", ...],
     "status": "active"  // 或 "disabled"
   }
   ```
   注意：使用 `batchUpdateAdminAccounts` 操作，先 `get_operation_detail("batchUpdateAdminAccounts")` 确认请求体格式。

5. **报告结果** — 告诉用户：N 个成功，M 个失败，以及失败原因。

## 注意事项
- 不要逐个调用单条更新 API，必须使用 batch 端点。
- 每次批量最多 500 个，超过需分批。
- 操作前必须先 GET 确认范围，不要凭猜测操作。
