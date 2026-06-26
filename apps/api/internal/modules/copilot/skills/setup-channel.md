---
name: setup-channel
description: 当用户要求创建渠道、设置倍率、创建不同价格的 API Key 分组时使用。
triggers: 创建渠道,渠道,倍率,channel,rate multiplier,定价分组,计费分组,不同价格
---
# 创建渠道（定价分组）

渠道 = Account Group + rate_multiplier。API Key 绑定分组后，通过该 Key 的请求只会路由到分组内的账号，并按分组倍率计费。

## 步骤

1. **确认需求** — 问用户：
   - 渠道名称？（如 "标准渠道"、"溢价渠道"）
   - 倍率？（1.0 = 原价，1.5 = 加价 50%，0.8 = 八折）
   - 要加哪些账号进去？（按供应商/名称）

2. **创建分组** — 先 `get_operation_detail("createAdminAccountGroup")` 确认格式：
   ```
   POST /api/v1/admin/account-groups
   {
     "name": "标准渠道",
     "description": "1倍率标准渠道",
     "rate_multiplier": "1.00000000",
     "status": "active"
   }
   ```

3. **添加账号到分组** — 用批量端点：
   - 先查账号列表：`GET /api/v1/admin/accounts?provider_id={id}&page_size=200`
   - 批量添加：
     ```
     POST /api/v1/admin/account-groups/{group_id}/members/batch
     { "account_ids": ["1", "2", "3"] }
     ```
   先 `get_operation_detail("batchAddAdminAccountGroupMembers")` 确认格式。

4. **创建 API Key 绑定分组** — 告诉用户在"API 密钥"页面创建 Key 时选择该分组。或者用 API：
   - `GET /api/v1/admin/api-keys` 查看已有 Key
   - 管理员在前端 /api-keys 页面创建时可选择分组

5. **验证** — 确认分组创建成功：
   ```
   GET /api/v1/admin/account-groups/{group_id}
   GET /api/v1/admin/account-groups/{group_id}/members
   ```

## 示例：创建三个渠道

| 渠道 | 倍率 | 说明 |
|------|------|------|
| 经济渠道 | 0.80 | 八折，使用低成本账号 |
| 标准渠道 | 1.00 | 原价 |
| 溢价渠道 | 1.50 | 加价 50%，使用高质量账号 |

## 注意事项
- 一个账号不要同时放进多个不同倍率的分组——倍率会产生歧义
- API Key 必须绑定分组才会受倍率影响，不绑分组的 Key 按 1.0x 计费
- 不绑分组的 Key 可以用所有账号（不受分组限制）
- rate_multiplier 格式为字符串，保留 8 位小数（如 "1.50000000"）
