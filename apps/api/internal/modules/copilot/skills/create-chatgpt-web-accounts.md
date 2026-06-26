---
name: create-chatgpt-web-accounts
description: 当用户要求创建 ChatGPT Web 账号、导入 ChatGPT 账号、添加 chatgpt_web 类型的账号时使用。
triggers: chatgpt,chatgpt web,chatgpt-web,chatgpt_web,gpt账号,chatgpt账号,导入chatgpt
---
# 创建 ChatGPT Web 账号

## 关键警告
ChatGPT Web 账号使用**一次性 refresh_token**。如果创建时配置不完整，系统可能自动尝试刷新并消耗旧 RT，导致 RT 永久丢失。**必须**一次性提供完整凭证。

## 前置检查

1. **确认供应商存在且 adapter_type 正确** — `GET /api/v1/admin/providers?q=chatgpt`
   - 如果没有 chatgpt-web 供应商，创建时**必须**使用 `adapter_type: "reverse-proxy-chatgpt-web"`（不是 `reverse_proxy`）
   - 如果已有供应商但 adapter_type 不是 `reverse-proxy-chatgpt-web`，需要 PATCH 修正，否则 discover-models 会报 "unsupported"
   - 创建示例：
     ```
     POST /api/v1/admin/providers
     { "name": "chatgpt-web", "display_name": "ChatGPT Web", "adapter_type": "reverse-proxy-chatgpt-web", "protocol": "chat_completions", "status": "active" }
     ```

2. **确认模型映射存在** — `GET /api/v1/admin/models` 并检查是否有映射到 chatgpt-web 供应商的模型
   - 如果没有，先用 `setup-model-routing` skill 创建映射
   - 常见模型：gpt-5.5, gpt-5.4, gpt-5.4-mini, gpt-image-2

## 创建步骤

1. **收集信息** — 向用户索要以下**全部**字段（缺一不可）：
   - `refresh_token`（必须）
   - `client_id`（必须，通常为固定值）
   - GPT 密码 / 邮箱密码（可选，存入 notes）

2. **确认 operationId 格式** — `get_operation_detail("createAdminAccount")` 确认请求体字段

3. **逐个创建账号** — refresh_token 很长，批量接口可能超出大小限制，使用单条创建：
   ```
   POST /api/v1/admin/accounts
   {
     "provider_id": {chatgpt-web供应商ID},
     "name": "{用户给的名称}",
     "runtime_class": "oauth_refresh",
     "upstream_client": "chatgpt_web",
     "status": "active",
     "credentials": {
       "refresh_token": "{用户提供的RT}",
       "client_id": "{用户提供的client_id}",
       "token_url": "https://auth.openai.com/oauth/token",
       "redirect_uri": "com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback"
     }
   }
   ```

4. **创建后立即刷新** — 确认凭证正确：
   ```
   POST /api/v1/admin/accounts/{新账号ID}/refresh
   ```

5. **发现并注册模型** — 刷新成功后，执行模型发现以自动注册该供应商的可用模型映射：
   ```
   POST /api/v1/admin/accounts/{新账号ID}/discover-models
   { "persist": true }
   ```
   这会查询 ChatGPT 后端获取该账号可用的模型列表，并自动创建缺失的 Model 和 ModelProviderMapping 记录。

6. **验证** — `GET /api/v1/admin/accounts/{id}` 确认 status=active 且无错误。检查模型映射：`GET /api/v1/admin/models` 确认模型已注册。

## 注意事项
- **不要分步创建**（先建空账号再 PATCH 凭证）——创建和凭证必须一次到位
- `upstream_client: "chatgpt_web"` 是必填字段，缺少会导致 missing_requirements 错误
- `token_url` 和 `redirect_uri` 是 OAuth 刷新必需的，缺少会导致 "oauth refresh configuration missing"
- refresh_token 很长（200+ 字符），一次只创建一个账号避免请求体超限
- 如果用户提供了 GPT 密码或邮箱密码，写入 `notes` 字段保存
- **必须**在刷新成功后执行 `discover-models`（persist=true），否则供应商下不会有模型映射，请求会报 "no model mapped"
- 供应商的 `adapter_type` 必须是 `reverse-proxy-chatgpt-web`（不是 `reverse_proxy`），否则 discover-models 会报 "unsupported"
- 每个账号创建完整流程：创建 → 刷新 → 发现模型，三步缺一不可
