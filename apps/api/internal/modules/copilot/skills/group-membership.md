---
name: group-membership
description: 当用户要求批量添加或移除账号分组成员时使用。
---
# 批量管理分组成员

## 步骤

1. **确认分组** — 确认目标分组。如果用户说的是分组名称：
   - `GET /api/v1/admin/account-groups` 查出分组列表，找到 ID

2. **确认账号** — 确认要添加/移除哪些账号：
   - 按供应商：`GET /api/v1/admin/accounts?provider_id={id}&page_size=200`
   - 按名称：`GET /api/v1/admin/accounts?q={query}`
   - 查看当前成员：`GET /api/v1/admin/account-groups/{group_id}/members`

3. **报告并确认** — 告诉用户将要添加/移除 N 个账号到分组 X。

4. **执行批量操作**：
   - 批量添加：
     ```
     POST /api/v1/admin/account-groups/{group_id}/members/batch
     { "account_ids": ["id1", "id2", ...] }
     ```
   - 批量移除：
     ```
     DELETE /api/v1/admin/account-groups/{group_id}/members/batch
     { "account_ids": ["id1", "id2", ...] }
     ```
   先 `get_operation_detail("batchAddAdminAccountGroupMembers")` 或 `get_operation_detail("batchRemoveAdminAccountGroupMembers")` 确认格式。

5. **报告结果** — N 个成功，M 个失败。语义是幂等的：已在组内的添加和不在组内的移除都算成功。

## 注意事项
- 幂等语义：重复添加已有成员不会报错。
- 一次最多操作的数量由服务端限制，超大批量需要分批。
