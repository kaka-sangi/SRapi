---
name: batch-refresh-tokens
description: 当用户要求批量刷新 OAuth 令牌、批量续期账号时使用。
---
# 批量刷新 OAuth 令牌

## 步骤

1. **确认范围** — 确认要刷新哪些账号。通常按供应商筛选，且只有 runtime_class 为 oauth_refresh 或 oauth_device_code 的账号才支持刷新。

2. **查询 OAuth 账号** — `GET /api/v1/admin/accounts?provider_id={id}&page_size=200`
   从结果中筛选 runtime_class 为 `oauth_refresh` 或 `oauth_device_code` 的账号。

3. **报告数量** — 告诉用户将刷新 N 个 OAuth 账号。

4. **执行批量刷新** — 使用：
   ```
   POST /api/v1/admin/accounts/batch-refresh
   { "account_ids": ["id1", "id2", ...] }
   ```
   先 `get_operation_detail("batchRefreshAdminAccounts")` 确认格式。

5. **报告结果** — N 个刷新成功，M 个失败及原因（令牌过期、provider 拒绝等）。

## 注意事项
- 只能刷新 OAuth 类账号，API key 类账号不需要也不支持刷新。
- 刷新失败的账号可能需要重新授权。
