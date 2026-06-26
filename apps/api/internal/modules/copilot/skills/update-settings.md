---
name: update-settings
description: 当用户要求修改系统设置（通用设置、安全设置、功能开关、邮件配置等）时使用。
triggers: 修改设置,更新设置,系统设置,update settings,change settings,功能开关,邮件配置,安全设置
---
# 安全更新系统设置

## 关键警告
PUT /admin/settings 的请求体非常大。**绝对不要**发送完整的设置对象——会被输出长度限制截断，导致数据丢失。

## 步骤

1. **获取当前设置** — `GET /api/v1/admin/settings`

2. **定位目标字段** — 从返回的 JSON 中找到用户要修改的具体字段所在的 section（如 `general`、`security`、`features`、`email` 等）。

3. **构造最小更新体** — 只包含要修改的 section，且该 section 内保留所有原有字段 + 修改的字段。例如：
   ```json
   {
     "general": {
       "site_name": "新名称",
       "... 保留该 section 内的其他原有字段 ..."
     }
   }
   ```

4. **执行更新** — `PUT /api/v1/admin/settings`（带上面的最小体）

5. **验证** — `GET /api/v1/admin/settings` 确认修改生效。

## 注意事项
- **只发送修改的 section**，不要发送完整的 settings 对象。
- 修改 copilot 设置时同理：只发送 `copilot` section。
- 安全相关设置（JWT secret、master key 等）修改后可能导致服务中断，操作前必须警告用户。
