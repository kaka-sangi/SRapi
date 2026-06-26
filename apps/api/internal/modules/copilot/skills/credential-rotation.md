---
name: credential-rotation
description: 当用户要求批量更新账号凭证（API key、access token、密码等）时使用。
triggers: 批量更新凭证,轮换凭证,更换密钥,credential rotation,batch credential,更新api key,换key
---
# 批量凭证轮换

## 步骤

1. **确认范围和凭证** — 确认要更新哪些账号的凭证，以及新凭证的来源（用户提供、还是自动生成）。

2. **查询目标账号** — `GET /api/v1/admin/accounts?provider_id={id}&page_size=200`

3. **收集新凭证** — 请用户提供凭证数据。格式为每行一条：`account_id,key=value,key=value`。

4. **执行批量更新** — 使用：
   ```
   POST /api/v1/admin/accounts/batch-update-credentials
   ```
   先 `get_operation_detail("batchUpdateAdminAccountCredentials")` 确认请求体格式。

5. **报告结果** — N 个更新成功，M 个失败及原因。

## 注意事项
- 凭证是敏感数据，操作结果中凭证值会被脱敏显示，这是正常行为。
- 更新凭证后建议执行一次健康检查确认账号可用。
