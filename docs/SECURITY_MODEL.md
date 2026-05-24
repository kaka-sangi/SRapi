# SRapi 安全模型

## 1. 目标

本文档定义 SRapi MVP 和后续阶段必须遵守的安全边界，覆盖控制台登录、API Key、Provider 凭证、Cookie、CSRF、日志脱敏、审计、密钥轮换和 AI Gateway 特有风险。

安全原则：

```txt
默认最小权限、敏感数据不落明文、日志默认脱敏、所有高风险操作可审计、外部输入不可信。
```

## 2. 信任边界

```txt
Browser Console
  -> Console API (/api/v1/*)
  -> Application Service
  -> PostgreSQL / Redis

API Client
  -> Gateway API (/v1/*)
  -> API Key Auth
  -> Scheduler
  -> Provider Adapter
  -> Upstream Provider
```

边界规则：

- Browser Console 使用 Cookie 登录态，不使用 Gateway API Key。
- API Client 使用 Bearer API Key，不使用控制台 Cookie。
- Provider Adapter 不得接触用户登录 Session。
- Scheduler 不得解密 Provider 凭证。
- Provider 凭证只在 Adapter 发起请求前由受控服务解密。

## 3. 控制台认证

控制台 API 推荐：

```txt
HttpOnly Cookie + CSRF Token
```

Cookie 必须设置：

```txt
HttpOnly = true
Secure = true in non-local environments
SameSite = Lax by default
Path = /
```

Session 要求：

- Access session 必须有过期时间。
- Refresh session 如果引入，必须支持 rotation。
- 登出必须使当前 session 失效。
- Session ID 持久化时只能保存 hash，不得保存 Cookie 原文。
- 高风险操作可以要求二次确认或重新认证。

CSRF 要求：

- 所有控制台写操作必须校验 `X-CSRF-Token`。
- 只读 GET 不应产生状态变更。
- CORS 默认只允许控制台来源。

CSRF Token 实现建议：

```txt
Synchronizer Token Pattern
```

流程：

- 登录后服务端为 session 生成 CSRF token。
- 前端通过 `/api/v1/auth/csrf` 或 session bootstrap 响应获取 token。
- 所有控制台写操作通过 `X-CSRF-Token` 发送 token。
- 服务端必须校验 token 与当前 session 绑定。
- CSRF token 持久化时只能保存 hash，登录响应只返回本次新建 token 的明文值。

可选方案：

```txt
Signed Double-Submit Cookie
```

要求：

- CSRF cookie 可以被前端读取，但必须签名。
- Session cookie 必须保持 HttpOnly。
- CSRF token 必须绑定 session 或 session id 派生值。
- 禁止使用未签名的 naive double-submit cookie。

Defense in depth：

- 写操作校验 `Origin`。
- `Origin` 缺失时校验 `Referer`。
- 可启用 Fetch Metadata headers 拦截跨站写请求。
- SameSite 只是纵深防御，不得替代 CSRF token。

### 3.1 控制台密码安全

控制台用户密码不得明文或可逆加密存储。

推荐哈希：

```txt
Argon2id
```

MVP 推荐参数起点：

```txt
memory = 19456 KiB
iterations = 2
parallelism = 1
```

如果运行环境暂不支持 Argon2id，可以使用：

```txt
bcrypt cost >= 12
```

要求：

- 每个密码必须使用独立 salt。
- 密码哈希参数必须随 hash 一起保存。
- 登录失败必须有速率限制。
- 管理员密码重置必须写 audit log。

## 4. API Key 安全

API Key 用于 Gateway API。

### 4.1 格式

建议格式：

```txt
sk_live_<prefix>_<secret>
sk_test_<prefix>_<secret>
```

MVP 可简化为：

```txt
sk_<random>
```

要求：

- 原文只展示一次。
- `prefix` 用于快速定位，不用于认证。
- `secret` 必须使用 CSPRNG 生成。
- API Key 必须支持禁用、过期、模型范围、RPM/TPM 限制。

### 4.2 存储

数据库字段：

```txt
prefix
hash
status
scopes_json
allowed_models_json
rpm_limit
tpm_limit
expires_at
last_used_at
```

哈希建议：

```txt
hash = HMAC-SHA256(server_pepper, full_api_key)
```

原因：

- API Key 是高熵随机值，不需要像用户密码一样使用慢哈希。
- HMAC pepper 可以防止数据库泄漏后离线验证 key。
- `server_pepper` 必须来自环境变量或密钥管理系统。

禁止：

- 持久化完整 API Key。
- 在日志、错误、audit before/after 中输出完整 API Key。
- 使用 prefix 作为认证依据。

## 5. Provider 凭证安全

Provider Account 的凭证包括：

```txt
api_key
oauth_access_token
oauth_refresh_token
oauth_device_code
web_session_cookie
desktop_client_token
cli_device_token
ide_plugin_token
service_account_json
custom_headers
custom_reverse_proxy_payload
```

必须加密字段：

```txt
provider_accounts.credential_ciphertext
provider_accounts.cookie_jar_ciphertext
provider_accounts.device_fingerprint_ciphertext
proxies.url_ciphertext
payment_provider_instances.config_ciphertext
settings.value_ciphertext when is_secret = true
```

### 5.1 加密方案

MVP 推荐：

```txt
AES-256-GCM
```

每条密文必须包含或可关联：

```txt
key_version
nonce
ciphertext
auth_tag
aad_version
created_at
```

主密钥要求：

- 不得写入仓库。
- 本地开发通过 `.env` 提供。
- 生产环境应来自 KMS 或密钥管理系统。
- 支持 key version，为后续轮换做准备。

AES-GCM 要求：

- 同一 `key_version` 下 nonce 不得重复。
- 推荐使用 96-bit CSPRNG nonce。
- 必须使用认证附加数据 AAD 绑定资源上下文。
- AAD 至少包含 `resource_type`、`resource_id`、`field_name`、`key_version`。
- 解密失败必须返回内部错误，不得回退到明文或忽略认证失败。
- 生产环境优先使用 KMS 或 envelope encryption 管理主密钥。

### 5.2 解密边界

- 只有 Provider 调用前的受控路径可以解密。
- 解密后的凭证只存在内存中。
- Adapter 不得自行持久化明文凭证。
- 错误 details 不得包含凭证。

### 5.3 反代凭证额外要求

`runtime_class != api_key` 的反代凭证（cookie、OAuth token、device code、desktop / CLI / IDE token、device fingerprint 等）必须遵守：

- 必须经由 Reverse Proxy Runtime 注入，不得通过日志、metrics、scheduler scores_json、usage metadata 或 audit before/after 泄漏。
- 必须每账号独立 cookie jar、device fingerprint 与连接池，不得跨账号共享。
- 必须支持 OAuth 自动 refresh 与失败保护，详见 `REVERSE_PROXY_SPEC.md`。
- 备份导出必须二次加密或屏蔽。
- 反代请求向上游发出时，不得包含任何 SRapi 自身标识（`X-Request-ID`、`X-Forwarded-*`、`Via`、`X-SRapi-*`、`User-Agent: SRapi/*` 等）。
- 解题 / 验证码 / Cloudflare challenge 的中间 token 必须按 cookie jar 绑定，且不得跨账号复用。

### 5.4 账号导入导出安全边界

Provider Account 的 import/export 是运维便利接口，不是明文凭证备份通道。

- 导出接口不得返回 `credential`、`credential_ciphertext`、API Key、OAuth token、refresh token、Cookie、Authorization header、password、secret 或其他可复用凭证。
- 导出 metadata 必须递归移除敏感键，只保留非敏感操作字段，例如 `base_url`、策略标签、分组和代理绑定。
- 导出响应必须显式标记 `credential_exported: false`。
- 导入接口可以接收 write-only `credential` payload，但服务端必须立即进入 Provider Account 加密写入路径。
- 导入失败的错误消息、audit、日志和响应不得回显导入凭证内容。
- import 属于控制台写操作，必须要求登录态和 CSRF；export 属于管理读操作，仍必须要求登录态和 RBAC。

## 6. RBAC 与权限

MVP 角色：

```txt
owner
admin
operator
user
```

建议权限边界：

| 能力 | owner | admin | operator | user |
| --- | --- | --- | --- | --- |
| 用户管理 | yes | yes | no | self only |
| Provider 管理 | yes | yes | read/test only | no |
| Provider Account 凭证写入 | yes | yes | no | no |
| Scheduler 策略修改 | yes | yes | no | no |
| Scheduler 查看 | yes | yes | yes | no |
| API Key 管理 | yes | yes | no | self only |
| Usage 查看 | yes | yes | yes | self only |
| Billing 调整 | yes | no by default | no | no |
| Audit 查看 | yes | yes | no | no |

所有管理端写操作必须写 audit log。

## 7. Prompt 与日志安全

默认不得记录完整用户 prompt、messages、tool arguments。

允许记录：

```txt
request_id
user_id
api_key_id
model
provider
account_id
input_token_estimate
output_tokens
latency_ms
error_class
status_code
```

禁止默认记录：

```txt
full prompt
full messages
API Key
Provider credential
OAuth token
Cookie
Authorization header
PII raw value
```

调试模式要求：

- 必须显式开启。
- 必须限制管理员权限。
- 必须脱敏 Authorization、Cookie、email、phone、token-like 字段。
- 必须有保留时间。
- 必须写入 audit log。

## 8. Gateway 安全

Gateway 必须执行：

- API Key 鉴权。
- API Key 状态、过期时间、模型范围检查。
- RPM/TPM 限制。
- 用户余额或 entitlement 检查。
- 请求体大小限制。
- 上游超时限制。
- Provider 错误脱敏。

OpenAI-compatible 错误响应不得泄漏：

- 上游完整错误体中的 secret。
- 内部账号 id 以外的敏感配置。
- 数据库错误细节。
- 堆栈信息。

## 9. Provider Adapter 安全

Adapter 禁止：

- 打印请求 header 中的 Authorization。
- 打印 Provider 凭证。
- 自行选择或切换账号。
- 自行持久化凭证。
- 对业务错误进行无限重试。

Adapter 必须：

- 使用平台层 HTTP client。
- 尊重请求 timeout。
- 对上游错误分类。
- 对日志字段脱敏。
- 将 usage 和 error feedback 回传。

## 10. AI / LLM 特有安全

MVP 至少遵守：

- 用户 prompt 不作为系统指令执行。
- 管理后台展示 prompt 片段时默认隐藏或脱敏。
- Tool call 参数如果后续支持，必须验证 schema。
- Provider 返回内容不得直接注入管理后台 HTML。
- 前端不得使用 `dangerouslySetInnerHTML` 渲染模型内容，除非经过可信 sanitizer。

后续如果支持 MCP、工具调用或文件处理，需要新增专项威胁模型。

## 11. 审计日志

必须审计：

- 登录失败次数异常。
- API Key 创建、禁用、删除。
- Provider 创建、更新、删除。
- Provider Account 创建、更新、禁用、凭证轮换。
- Scheduler 策略修改。
- Billing 手动调整。
- Settings secret 更新。
- 调试日志开关变更。

审计字段：

```txt
actor_user_id
action
resource_type
resource_id
before_json
after_json
ip
user_agent
trace_id
created_at
```

审计日志不得包含完整 secret。

## 12. 密钥轮换

MVP 必须预留：

```txt
credential_version
settings key version
provider account credential version
```

轮换流程：

1. 新增新版本主密钥。
2. 新写入使用新版本。
3. 后台任务逐步重加密旧密文。
4. 验证所有旧版本迁移完成。
5. 禁用旧密钥。

轮换失败不能覆盖原密文。

## 13. 本地开发安全

`.env.example` 可以包含占位符，不得包含真实 secret。

必须忽略：

```txt
.env
.env.local
*.key
*.pem
```

本地默认管理员密码如果存在，必须只用于开发环境，并在 README 中标注不可用于生产。

## 14. 安全测试清单

MVP 必须覆盖：

- API Key 原文不落库测试。
- API Key hash 校验测试。
- 禁用 API Key 后不可调用 Gateway。
- Provider 凭证加密和解密失败处理测试。
- Cookie 安全属性测试。
- CSRF 写操作拦截测试。
- RBAC 管理接口权限测试。
- 反代请求不向上游泄漏 SRapi 自有 Header 的回归测试。
- 反代凭证（cookie / OAuth token / device fingerprint）跨账号隔离测试。
- 反代账号封号信号识别与 needs_reauth / disabled 自动切换测试。
- 日志不包含 Authorization / Cookie / Provider credential 测试。
- Provider 错误脱敏测试。
- Audit log 高风险操作测试。

## 15. Ship Gate 安全门禁

上线前必须确认：

- 没有真实 secret 提交到仓库。
- 没有 API Key 原文持久化。
- 没有 Provider 凭证明文持久化。
- 管理写操作有 RBAC 和 Audit。
- Gateway 有 rate limit。
- Cookie 使用安全属性。
- CSRF 已启用。
- 日志脱敏策略已验证。
- OpenAPI securitySchemes 与实际 middleware 一致。
