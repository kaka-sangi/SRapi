---
name: batch-delete-accounts
description: 当用户要求批量删除上游账号时使用。
---
# 批量删除账号

## 步骤

1. **确认范围** — 向用户确认要删除哪些账号（按供应商、按状态、按名称等）。删除是危险操作，必须明确确认。

2. **查询账号列表** — `GET /api/v1/admin/accounts?provider_id={id}&status={status}&page_size=200`

3. **报告并确认** — 列出将要删除的账号名称和数量，警告用户此操作不可撤销。

4. **执行批量删除** — 使用：
   ```
   POST /api/v1/admin/accounts/batch-delete
   { "account_ids": ["id1", "id2", ...] }
   ```
   先 `get_operation_detail("batchDeleteAdminAccounts")` 确认请求体格式。

5. **报告结果** — N 个成功删除，M 个失败及原因。

## 注意事项
- 这是不可逆操作，必须先列出具体账号让用户确认。
- 如果用户只是想暂停账号，建议使用禁用而不是删除。
