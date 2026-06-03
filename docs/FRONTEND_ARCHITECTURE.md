# SRapi 前端架构（v0.1.0）

> 本文件描述 `apps/web` 的工程结构、模块边界、数据流和质量门。视觉与文案分别在 `FRONTEND_DESIGN_SYSTEM.md` 和 `PRODUCT_TONE.md`。

## 1. 技术栈

| 层 | 技术 | 版本 / 备注 |
| --- | --- | --- |
| 框架 | Next.js | 16 (App Router, RSC + Edge Proxy) |
| UI 运行时 | React | 19 |
| 类型 | TypeScript | 5（`strict: true`） |
| 样式 | Tailwind CSS | 4（`@theme` design tokens） |
| 组件原语 | Radix UI | Dialog / Label / Select / Switch / Toast / Tooltip / Slot |
| 设计系统 | 自建 `@/components/ui` | `cva` + `tailwind-merge`，Anthropic + Stripe 风格 |
| 数据获取 | `@tanstack/react-query` | v5，所有页面通过 `@/hooks/queries` 拿数据 |
| 表单 | `react-hook-form` + `zod` + `@hookform/resolvers/zod` | schemas 在 `@/lib/schemas/`，可被客户端表单与未来的 Server Action 共享 |
| API 类型 | `packages/sdk/typescript` | 由 `make openapi-ts-codegen` 从 OpenAPI 自动生成 |
| 主题 | `next-themes` | 解决 SSR 闪屏 |
| i18n | 自建（`useSyncExternalStore`） | 命名空间分文件，可平滑迁到 `next-intl` |
| 动效 | `framer-motion` | 仅用于抽屉、阻尼回弹 |
| 图标 | `lucide-react` | 锁定 16/14px |
| 单测 | Vitest + Testing Library | happy-dom 环境 |
| a11y | `axe-core` (单测) + `@axe-core/playwright` (e2e) | 阻断级别：`critical`、`serious` |
| e2e | Playwright | Chromium，演示模式默认无后端依赖 |
| 性能预算 | `@next/bundle-analyzer` + `tools/bundle-budget.mjs` | `npm run analyze` 看图；`bundle-budget` 是 web-check 中的硬门槛 |
| 遥测 | `web-vitals` + 自托管 beacon hook | 可选 DSN，不内置第三方 SaaS |

## 2. 目录布局

```txt
apps/web/
├─ src/
│  ├─ app/                       # Next.js App Router
│  │  ├─ layout.tsx             # 根布局：providers + WebVitalsReporter
│  │  ├─ page.tsx               # / 落地 + 登录
│  │  ├─ error.tsx              # 路由级错误边界
│  │  ├─ not-found.tsx          # 404 页面
│  │  ├─ loading.tsx            # 路由级 Suspense fallback
│  │  ├─ global-error.tsx       # 应用级硬错误兜底
│  │  ├─ globals.css            # @theme 设计令牌 + paper-grain + tactile-card
│  │  ├─ dashboard/ api-keys/ usage/                # 用户工作区
│  │  ├─ account/ billing/ redeem/ affiliate/        # 用户自助
│  │  ├─ provider-accounts/ scheduler-decisions/     # 网关只读视图
│  │  ├─ admin/                                      # 管理后台（按域分组导航）
│  │  │  ├─ dashboard/ users/ usage/                       # 概览
│  │  │  ├─ providers/ models/ accounts/ groups/ proxies/  # 网关资源
│  │  │  ├─ subscriptions/ orders(/plans) channels/pricing/
│  │  │  │  payment-providers/ promo-codes/ redeem/        # 商业化
│  │  │  ├─ affiliates/{invites,rebates,transfers}/        # 推广联盟
│  │  │  ├─ ops(/strategy) risk-control/ announcements/    # 运营
│  │  │  └─ settings/                                      # 系统
│  │  └─ srapi-health/route.ts  # 健康代理
│  │
│  ├─ proxy.ts                  # Next 16 边缘 proxy（替代旧 middleware.ts）
│  │
│  ├─ providers/
│  │  ├─ index.tsx              # 组合：Theme + Query + Language
│  │  ├─ query-provider.tsx     # 单例 QueryClient
│  │  └─ theme-provider.tsx     # next-themes 包装
│  │
│  ├─ components/
│  │  ├─ ui/                    # 设计系统原语（Button / Card / Dialog / Table / Sheet / Tabs ...）
│  │  ├─ admin/                 # AdminListView / ResourceFormDialog / ConfirmDialog /
│  │  │                         #   AccountFormDialog / BindProxyDialog / RowActionsMenu / ListToolbar
│  │  ├─ layout/                # AppShell / AdminShell / SidebarNav(+nav-items) /
│  │  │                         #   TopNav / PageHeader / PageQueryState / AuthGate
│  │  ├─ features/              # GatewayOverview / ApiKeyCreateDialog
│  │  ├─ charts/                # Sparkline / BarSeries
│  │  └─ auth/ visual/
│  │
│  ├─ hooks/
│  │  ├─ queries.ts             # 用户侧（apiService / meApi）数据 + mutation hooks
│  │  ├─ admin-queries.ts       # 管理侧（adminApi）数据 + mutation hooks
│  │  ├─ use-admin-list.ts      # 列表 search/filter/sort/分页/多选 状态
│  │  └─ use-debounced-value.ts
│  │
│  ├─ context/
│  │  ├─ LanguageContext.tsx    # i18n hook，基于 useSyncExternalStore
│  │  └─ ToastContext.tsx       # 全局 toast
│  │
│  ├─ i18n/messages/
│  │  ├─ index.ts               # flatLookup + applyVariables
│  │  ├─ en.ts                  # 命名空间字典：common/login/admin/...
│  │  └─ zh.ts
│  │
│  └─ lib/
│     ├─ api.ts / me-api.ts / admin-api.ts  # apiService / meApi / adminApi 门面：只走生成 SDK
│     ├─ admin-*-form.ts        # 各 admin 资源的表单构造/校验（zod 风格纯函数）
│     ├─ cn.ts                  # tailwind-merge + clsx
│     ├─ query-keys.ts          # 集中式 query 键
│     ├─ routes.ts              # ADMIN_ROUTES / USER_ROUTES 路径常量
│     ├─ status-badge.ts / admin-format.ts  # 状态色映射 / 金额·日期格式化
│     ├─ schemas/               # zod schemas（client + 未来 Server Action 共用）
│     │   └─ api-key.ts         # createApiKeySchema + parseGroupIdsCsv
│     ├─ session-cookie.ts      # 给 proxy.ts 使用的非凭据存在标记
│     └─ telemetry.ts           # web-vitals + redacted exception collector
│
├─ tests/
│  ├─ unit/                     # Vitest 单测 + axe 单测
│  └─ e2e/                      # Playwright + axe e2e
│
├─ next.config.ts               # CSP / 安全头 / bundle analyzer
├─ vitest.config.ts             # 单测配置
├─ vitest.setup.ts              # @testing-library/jest-dom + 浏览器 shim
├─ playwright.config.ts
├─ eslint.config.mjs            # next + 严格基线
├─ .prettierrc.json             # prettier-plugin-tailwindcss
└─ package.json                 # engines.node >= 20.10
```

## 3. 数据流

```txt
Page (Client Component)
   │  useXxx (TanStack Query)
   ▼
hooks/queries.ts   ──> apiService (lib/api.ts) ──> packages/sdk (生成自 OpenAPI)

Page (Login)
   │  apiService.login → setSessionPresenceCookie
   ▼
proxy.ts (edge)        ──> 守卫 /admin、/dashboard 等受保护路由
AuthGate (client)      ──> 兜底守卫 + 注入 user / runtimeStatus 到子组件
```

- 所有页面通过 hooks 拿数据：用户侧用 `@/hooks/queries`（内部调 `apiService` / `meApi`），管理侧用 `@/hooks/admin-queries`（内部调 `adminApi`）；三个门面（`lib/api.ts` / `lib/me-api.ts` / `lib/admin-api.ts`）内部都走生成的 `packages/sdk`。**禁止**在页面里手写 `useEffect+fetch` 或 `useState+setLoading`。
- 管理列表统一走 `useAdminList`（search/filter/sort/分页/多选状态）+ `AdminListView`（渲染表格/工具栏/分页/批量条 + loading/empty/error）+ `PageQueryState`（loading/error/空态包装）；写操作走 `ResourceFormDialog` / `ConfirmDialog`，成功后由 `admin-queries` 的 mutation 统一失效 `["admin", <resource>]` 前缀。
- 管理后台侧边导航在 `components/layout/nav-items.ts` 按域分组（概览/网关资源/商业化/推广联盟/运营/系统），**每个 admin 路由都必须在此登记**，避免出现只能靠 URL 访问的孤儿页。
- 前端主体不提供演示业务数据回退；后端不可用或接口失败时显示明确错误态/空态。
- 凭据从未进入前端：`packages/sdk` 用浏览器 cookie + CSRF，`session-cookie.ts` 只写一个非敏感的 "presence" 标记给 proxy 用。
- AuthGate 的当前用户由 `useSyncExternalStore(localStorage)` 派生；缓存由 raw JSON 字符串校验，避免 React error #185。同 tab 内 `apiService.login/logout` 触发 `srapi:user-change` 事件即时刷新。

## 4. 鉴权链路

| 阶段 | 位置 | 作用 |
| --- | --- | --- |
| 1 | `apiService.login` | 写 `localStorage` + 写非敏感 cookie（`srapi_session_present`、`srapi_session_role`） |
| 2 | `proxy.ts` (edge) | 看到 cookie，决定是否 SSR 重定向到 `/`、`/admin`、`/dashboard` |
| 3 | `AuthGate` (client) | `useSyncExternalStore(localStorage)`；与 proxy 一致；防止 cookie 与 localStorage 漂移 |
| 4 | `apiService.logout` | 清 localStorage + 清 cookie，跳回 `/` |

## 5. 质量门

`make web-check` 串行执行：

1. `npm run typecheck` — `tsc --noEmit`
2. `npm run lint` — eslint（next + 严格规则）
3. `npm run test` — vitest run（单测 + axe 单测，目前 98 例）
4. `npm run build` — `next build`（同时验证 CSP 头）
5. `node tools/bundle-budget.mjs` — 读 `apps/web/bundle-budget.json` 校验 chunk 大小

`make web-check-e2e` 单独跑（成本高）：

1. API preflight：检查 `SRAPI_WEB_E2E_API_URL`（默认 `http://127.0.0.1:8080`）的 `/livez` 与 `/readyz`；该目标同时传给 `SRAPI_API_PROXY_TARGET`，浏览器默认继续通过 Next 同源代理访问后端。只有在目标 API 已配置浏览器 CORS/cookie 凭据时，才用 `SRAPI_WEB_E2E_DIRECT_BROWSER_API=1` 让 SDK 直连 API。
2. `next build`
3. 安装 Playwright Chromium（可通过 `SRAPI_WEB_E2E_SKIP_INSTALL=1` 跳过；OS 依赖用 `npm run test:e2e:install-deps` 显式安装）
4. `playwright test`（含 `@axe-core/playwright` 全页 a11y）

`make check` 已把 `web-check` 加在最后一步，前后端任意一侧出问题都拦截。

## 6. 性能预算

| 指标 | 目标 | 工具 |
| --- | --- | --- |
| LCP | ≤ 2.5s @ p75 | `web-vitals` reporter + `NEXT_PUBLIC_SRAPI_TELEMETRY_URL` |
| INP | ≤ 200ms @ p75 | 同上 |
| CLS | ≤ 0.1 @ p75 | 同上 |
| 全部 JS chunk 总和 | ≤ 2.20 MB（`bundle-budget.json/all-js-chunks`） | `tools/bundle-budget.mjs`（阻断级） |
| 单个最大 chunk | ≤ 500 KB（`bundle-budget.json/largest-chunk`） | 同上 |
| 首屏 JS | bundle analyzer 看图 | `npm run analyze` |
| 主路由 RSC | 静态预渲染优先 | `next build` 输出表 |

`NEXT_PUBLIC_SRAPI_TELEMETRY_URL` 同时接收 Core Web Vitals 与前端异常摘要。生产 CSP 会把该 URL 的 origin 加入 `connect-src`，但不会放开通配域。异常上报只包含低基数字段、当前页面、错误名称/消息/栈摘要和脱敏后的上下文；不得上传 API key、Authorization、Cookie、CSRF、session、provider credential、prompt、messages 或请求体。

## 7. 安全

- `next.config.ts` 在生产环境注入：CSP（`default-src 'self'`）、HSTS、`X-Frame-Options: DENY`、`Referrer-Policy: strict-origin-when-cross-origin`、`Permissions-Policy`（禁用相机/麦克风/定位）。
- 不写明文凭据到 `localStorage`；明文 API key 仅在创建一次性显示。
- `apiService` 通过浏览器 cookie + `X-CSRF-Token`（来自 `localStorage`）调后端。
- proxy 只读非敏感 cookie，**不参与**真实鉴权决策。

## 8. 可访问性

- 所有原语（Button / Dialog / Label）继承 Radix 语义。
- `text-[9px]` / `text-[10px]` 是历史遗留，**新代码禁止**；最小字号 12px (`--text-xs`)，11px (`--text-2xs`) 仅限 `font-mono` 大写小标签。
- 单测：`tests/unit/a11y.test.tsx` 用 `axe-core` 跑核心原语。
- e2e：`tests/e2e/a11y.spec.ts` 用 `@axe-core/playwright` 跑落地与工作台，`critical/serious` 级别任何一条违规都阻断 PR。

## 9. 国际化

- 字典分文件：`src/i18n/messages/{en,zh}.ts`，按命名空间组织：`common` / `adminCommon` / `feedback` / `nav` / `login` / `dashboard` / `apiKeys` / `usage` / `providers` / `scheduler` / `account` / `billing` / `redeem` / `affiliate`，以及管理域 `adminUsers` / `adminAccounts` / `adminGroups` / `adminProviders` / `adminModels` / `adminProxies` / `adminSubscriptions` / `adminOrders` / `adminPromos` / `adminAffiliates` / `adminAnnouncements` / `adminOps` / `adminRisk` / `adminUsage` / `adminSettings` / `adminPayments` / `adminPricing`。
- `LanguageContext` 通过 `useSyncExternalStore(localStorage + custom event)` 跨组件实时同步语言切换。
- 添加新文案：先改 `en.ts`，再改 `zh.ts`，再在页面里 `t('newKey')`。`tests/unit/messages.test.ts` 会断言两边 key 完全对齐。

## 10. 演进路线

- **WP-160a**（已完成）：P0–P3 + 前端 harness，组件库、TanStack Query 全量接入、AuthGate 修复 React #185、6 页全部走 hooks、API key 表单走 react-hook-form + zod、bundle 预算守门。98 单测 + 5 e2e 通过。
- **WP-1310**（业务面补全，进行中）：在 `apps/web` 重写之上，把后端已具备但 UI 未暴露/未正确操作的能力补齐。slice 1 已落地：修复账号启停反向（P0）；admin 侧边栏改为分组导航使全部页面可达；账号域死能力接线（清错/恢复/发现模型/绑代理/导出 + allSettled 批量）；admin 仪表盘改用 `useAdminDashboard` 快照；新增支付渠道页；兑换码统计 + 安全批量停用；用量维度聚合；ops 告警/SLO 健康修正；API Key/订单取消反馈；错误/404 页 i18n；`PageQueryState` disabled query 修正。详见 `specs/STATUS.md` 的 WP-1310 条目与待办（SLO 增改 UI、模型分页、退款上限、策略回放字段、定价批量导入、账号详情抽屉、分组成员管理[受后端缺 list-members 端点阻塞]、状态徽章 i18n、浏览器验证）。
- **WP-160b**：把 `LanguageContext` 替换成 `next-intl`，启用 ICU plural、按路由代码分割。
- **WP-160c**：把 admin 总览、用量页改成 RSC + Server Action（form action 用现成的 `@/lib/schemas/api-key.ts` 直接复用）。
- **WP-160d**：Storybook + Chromatic 视觉回归（如果团队真的需要设计评审产物）。
- **WP-160e**：Lighthouse CI 接入 `make web-check-e2e`（性能 / a11y / SEO 综合分阻断）。
