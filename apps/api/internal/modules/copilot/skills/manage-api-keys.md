---
name: manage-api-keys
description: 当用户要求查看、吊销、启用 API 密钥，或检查某个用户的密钥时使用。
triggers: api密钥,api key,吊销密钥,revoke key,查看密钥,密钥管理,key管理
---
# 管理 API 密钥

## 步骤

1. **确认操作** — 用户要做什么：查看所有密钥？查某个用户的？批量吊销？

2. **查询密钥**：
   - 所有密钥：`GET /api/v1/admin/api-keys?page_size=50`
   - 按用户：`GET /api/v1/admin/api-keys?user_id={id}`
   - 按状态：`GET /api/v1/admin/api-keys?status=active`

3. **执行操作**：
   - 吊销（禁用）：
     ```
     PATCH /api/v1/admin/api-keys/{id}
     { "status": "disabled" }
     ```
   - 重新启用：
     ```
     PATCH /api/v1/admin/api-keys/{id}
     { "status": "active" }
     ```
   - 重置用量计数：
     ```
     POST /api/v1/admin/api-keys/{id}/reset-usage
     ```
   先 `get_operation_detail` 确认操作 ID 和请求格式。

4. **报告** — 展示密钥列表用表格：名称、前缀、所属用户、状态、最后使用时间。

## 注意事项
- 吊销密钥会立即使使用该密钥的请求失败。
- 密钥不支持硬删除，只能禁用（保留审计记录）。
- 过期密钥（expired）是终态，无法重新启用。
