---
name: setup-provider
description: 当用户要求新建供应商（provider）并配置账号时使用，包括从零开始接入一个新的 AI 平台。
triggers: 新建供应商,添加供应商,接入供应商,setup provider,add provider,new provider,接入平台
---
# 新建供应商并配置

## 步骤

1. **收集信息** — 确认以下内容：
   - 供应商名称（slug，如 `openai`、`anthropic`、`deepseek`）
   - 显示名称
   - 适配器类型（通常 `openai_compatible`、`anthropic`、`reverse_proxy` 等）
   - 协议（通常 `chat_completions`、`messages`）

2. **检查是否已存在** — `GET /api/v1/admin/providers?q={name}`

3. **创建供应商** — 先 `get_operation_detail("createAdminProvider")` 确认请求体格式，然后：
   ```
   POST /api/v1/admin/providers
   { "name": "deepseek", "display_name": "DeepSeek", "adapter_type": "openai_compatible", "protocol": "chat_completions", "status": "active" }
   ```

4. **创建账号** — 确认用户是否有 API key 或凭证，然后：
   先 `get_operation_detail("createAdminAccount")` 确认格式。
   ```
   POST /api/v1/admin/accounts
   { "provider_id": {新建供应商的ID}, "name": "deepseek-main", "credentials": {"api_key": "sk-..."}, "status": "active" }
   ```

5. **确认模型映射** — 询问用户是否需要为已有模型添加此供应商的映射。如果需要，使用 `setup-model-routing` skill。

6. **验证** — `GET /api/v1/admin/providers/{id}` 确认创建成功，报告给用户。

## 注意事项
- 供应商 name（slug）创建后不可更改。
- 如果不确定 adapter_type 或 protocol，先 `get_operation_detail("createAdminProvider")` 查看枚举值。
