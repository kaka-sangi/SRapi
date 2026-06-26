---
name: setup-model-routing
description: 当用户要求新建模型、添加模型别名、或配置模型到供应商的映射时使用。
---
# 配置模型路由

## 步骤

1. **确认目标** — 确认用户要做什么：
   - 新建模型？需要：canonical_name、display_name
   - 给已有模型加别名？需要：模型 ID、别名
   - 添加供应商映射？需要：模型 ID、供应商 ID、上游模型名

2. **新建模型**（如需要）：
   - 先 `get_operation_detail("createAdminModel")` 确认格式
   - `GET /api/v1/admin/models?q={name}` 检查是否已存在
   - ```
     POST /api/v1/admin/models
     { "canonical_name": "deepseek-r1", "display_name": "DeepSeek R1", "family": "deepseek", "status": "active" }
     ```

3. **添加别名**（如需要）：
   - 先 `get_operation_detail("createAdminModelAlias")` 确认格式
   - ```
     POST /api/v1/admin/models/{model_id}/aliases
     { "alias": "deepseek", "status": "active" }
     ```

4. **添加供应商映射**（如需要）：
   - 先查供应商 ID：`GET /api/v1/admin/providers?q={provider_name}`
   - 先 `get_operation_detail("createAdminModelMapping")` 确认格式
   - ```
     POST /api/v1/admin/models/{model_id}/mappings
     { "provider_id": 123, "upstream_model_name": "deepseek-reasoner", "status": "active" }
     ```

5. **验证** — `GET /api/v1/admin/models/{id}` 确认配置，并检查别名和映射。

## 注意事项
- canonical_name 创建后不可更改。
- upstream_model_name 是发给供应商 API 的实际模型名，不一定与 canonical_name 相同。
- 一个模型可以有多个供应商映射，调度器会根据健康度和策略选择。
