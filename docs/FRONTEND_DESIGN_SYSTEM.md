# SRapi 前端设计系统与视觉工程规范

> 本文件只规定**视觉**。文案语气与中英文术语表统一在 [`docs/PRODUCT_TONE.md`](./PRODUCT_TONE.md)。任何 UI 改动同时遵守这两份规范。

## 1. 设计哲学 (Academic Editorial & Cards)

SRapi 的视觉风格定义为 **“学术期刊/社论感 (Anthropic Claude)” 与 “结构化沉浸式流 (ChatGPT)” 的融合**。它不是一个充斥着渐变蓝色、荧光发光、或炫目呼吸灯的流水线 AI 模板网站，而是一个充满墨香、高可读性、带有浓厚“学院派 (Calm & Intellectual)”质感的技术控制台。

### 1.1 核心原则

- **暖调纸张 (Warm Alabaster Paper)**：摒弃刺眼的冷灰 (`#F3F4F6`) 或纯黑白，主色调采用温暖的象牙纸张色、干墨炭黑与陶土泥土红，营造极佳的护眼阅读环境。
- **排版压倒一切 (Typography Sovereign)**：主标题、核心段落和关键状态采用高雅的衬线体（Cormorant Garamond），与无衬线功能性文本 (Inter) 产生精美张力。
- **1px 极细微刻 (1px Industrial Lines)**：杜绝任何粗重的彩色进度条。所有的额度指示、边界划分均使用 1px 的超轻砂岩线与垂直 notch，呈现出物理仪表的精细感。
- **拒绝 AI 视觉套路 (No AI Tech-Slop)**：
  - **严禁**：任何闪烁的 Ping 绿点、彩色渐变背景、浮夸的卡片发光边框。
  - **倡导**：静默的文字状态标记、大面积的高级留白（Negative Space）、极轻量的阴影与高度拟真的物理质感。

---

## 2. 颜色系统 (Color Tokens)

设计系统提供两套核心主题：**温润纸张 (Warm Light)** 与 **深邃墨水 (Ink Dark)**。

> **唯一事实来源：[`apps/web/src/app/globals.css`](../apps/web/src/app/globals.css) 的 `@theme` / `.dark` 块。** 下表与该文件 1:1 同步（截至 0.1.0）；若两者出现差异，以 `globals.css` 为准并回填本表。组件中不得散落硬编码颜色，一律引用 `--color-srapi-*` 变量（见 §9.5）。

### 2.1 温润纸张 (Warm Light)

适用于日间长时间管理运维。

| Token 名 | CSS 变量值 | Tailwind 示例 | 视觉职责 |
| :--- | :--- | :--- | :--- |
| `background` | `#F9F6F0` | `bg-srapi-bg` | 温暖的象牙白棉纸色整页背景 |
| `card` | `#FFFFFF` | `bg-srapi-card` | 纯白纸张卡片，用于核心内容容器 |
| `card-muted` | `#F1EBE4` | `bg-srapi-card-muted` | 稍深微温的沙色，用于侧边栏和次要区 |
| `text-primary` | `#1C1A17` | `text-srapi-text-primary` | 烟炱炭黑，温和有温度，用于主文本 |
| `text-secondary`| `#6E6A5F` | `text-srapi-text-secondary` | 软砂岩灰，用于辅助说明和标签 |
| `text-tertiary` | `#9A9384` | `text-srapi-text-tertiary` | 更淡的砂岩灰，用于第三级提示文字 |
| `border` | `#E7DDCB` | `border-srapi-border` | 极细低对比度砂岩边框线 |
| `border-strong` | `#D8CCB4` | `border-srapi-border-strong` | 稍深的砂岩线，用于轨道底色、滚动条 |
| `primary` | `#BF5638` | `bg-srapi-primary` | 陶土红 (Terracotta)，标志性核心强调色 |
| `primary-hover` | `#A2452B` | `bg-srapi-primary-hover` | 陶土深红，用于 Hover |
| `success` | `#3F6F3A` | `text-srapi-success` | 软罗勒绿，用于健康状态 |
| `warning` | `#B07A28` | `text-srapi-warning` | 软琥珀，用于警示/降级态 |
| `error` | `#B3432F` | `text-srapi-error` | 软砖红，用于故障或异常 |
| `invert` / `invert-fg` | `#1C1A17` / `#F9F6F0` | — | 反相主按钮（炭黑底 + 纸白字），见 §8 |

### 2.2 深邃墨水 (Ink Dark)

适用于夜间长时间运维。

| Token 名 | CSS 变量值 | Tailwind 示例 | 视觉职责 |
| :--- | :--- | :--- | :--- |
| `background` | `#100F0D` | `bg-srapi-bg` | 枯墨黑，极其沉静的整页背景 |
| `card` | `#1A1916` | `bg-srapi-card` | 石板黑纸张，用于卡片容器 |
| `card-muted` | `#232019` | `bg-srapi-card-muted` | 次级深炭色区 |
| `text-primary` | `#F1EFE9` | `text-srapi-text-primary` | 羊皮纸白，温润的主文本 |
| `text-secondary`| `#A09A8D` | `text-srapi-text-secondary` | 枯苇灰，用于次要说明 |
| `text-tertiary` | `#6F6A5F` | `text-srapi-text-tertiary` | 更深的枯苇灰，用于第三级提示文字 |
| `border` | `#2C2A23` | `border-srapi-border` | 枯墨边框线 |
| `border-strong` | `#3A372E` | `border-srapi-border-strong` | 稍亮的枯墨线，用于轨道底色、滚动条 |
| `primary` | `#E07A5F` | `bg-srapi-primary` | 陶土金/暖金，用于夜间强调 |
| `primary-hover` | `#C96243` | `bg-srapi-primary-hover` | 暖金 Hover |
| `success` | `#5FA564` | `text-srapi-success` | 夜间罗勒绿 |
| `warning` | `#D6A45A` | `text-srapi-warning` | 夜间琥珀 |
| `error` | `#E2705A` | `text-srapi-error` | 夜间砖红 |
| `invert` / `invert-fg` | `#F1EFE9` / `#100F0D` | — | 反相主按钮（纸白底 + 枯墨字），见 §8 |

---

## 3. 字体与排版 (Typography)

### 3.1 跨语言字族 (Font Family)

SRapi 前端视觉由三种截然不同的字族构成（均经 `next/font/google` 注入 CSS 变量，见 `apps/web/src/app/layout.tsx`）：

- **文学/社论字族 (The Editorial Serif)**：`Cormorant Garamond`（高对比展示衬线，回退 `Georgia, serif`；变量 `--font-serif-display`）。
  - **应用范围**：大标题、栏目主入口名、数值大指标、调度器 Selected 节点名。Cormorant 纤细，仅在大尺寸使用并配中重字重 + 光学字距。
- **高精度无衬线 (Functional UI Sans)**：`Inter`（变量 `--font-inter`）。
  - **应用范围**：控制台常规 UI、表单、二级列表、按钮。
- **技术等宽 (The Technical Mono)**：`JetBrains Mono`（变量 `--font-jetbrains`）。
  - **应用范围**：API Keys 掩码、Token 计数、用量、耗时数值、调度原始 JSON、流式终端输出。

### 3.2 字体排版分级 (字阶)

```css
/* 页面大标题（示例字阶；实际请用 globals.css 的 --text-* token） */
h1 {
  font-family: var(--font-serif-display), 'Cormorant Garamond', Georgia, serif;
  font-size: 2.125rem; /* --text-3xl ≈ 34px */
  line-height: 1.14;
  font-weight: 600;
  letter-spacing: -0.022em;
  color: var(--color-srapi-text-primary);
}

/* 栏目/次级标题 */
h2.editorial {
  font-family: var(--font-serif-display), 'Cormorant Garamond', Georgia, serif;
  font-size: 1.125rem; /* --text-lg ≈ 18px */
  font-style: italic;
  font-weight: 500;
  color: var(--color-srapi-text-secondary);
}

/* 常规 UI 标签 */
span.tag {
  font-family: 'Inter', sans-serif;
  font-size: 0.875rem; /* 14px */
  font-weight: 500;
}

/* 精密数据指标 */
span.mono-metric {
  font-family: 'JetBrains Mono', monospace;
  font-size: 1.875rem; /* 30px */
  font-weight: 600;
}
```

---

## 4. 反模板化组件规范 (Non-Generic Components)

### 4.1 棉纸物理微杂色纹理 (Cotton Paper Texture)

为了在电子屏幕上模拟真实的纤维棉麻纸张，系统通过超低不透明度的 SVG 噪点层，消除无意义的数码平滑色块。

- **SVG 分形噪声滤镜公式**：
```css
.paper-grain::before {
    content: "";
    position: absolute;
    top: 0; left: 0; right: 0; bottom: 0;
    width: 100%; height: 100%;
    opacity: 0.04; /* 必须锁定在 3.5% - 4% 之间，过高会产生脏感 */
    pointer-events: none;
    z-index: 1;
    background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)'/%3E%3C/svg%3E");
}
```

### 4.2 物理压凹内高光技术 (Tactile Letterpress Depth)

所有的卡片和组件通过多层阴影（双层阴影 + 1px 白色内高光环），模拟出物理世界中凸版印刷（Letterpress）深嵌在重磅棉纸中的触觉。

- **高保真 CSS 阴影公式**：
```css
/* 温润纸张模式 (Light) */
.tactile-card {
    box-shadow:
        0 1px 2px rgba(25, 25, 25, 0.02),               /* 超软微阴影 */
        0 4px 20px -2px rgba(25, 25, 25, 0.015),          /* 远端重力渐衰 */
        inset 0 1px 0 0 rgba(255, 255, 255, 0.7);        /* 1px 物理防漫反射内高光，营造物理厚度 */
}

/* 深邃墨水模式 (Dark) */
.dark .tactile-card {
    box-shadow:
        0 1px 2px rgba(0, 0, 0, 0.2),
        0 4px 20px -2px rgba(0, 0, 0, 0.15),
        inset 0 1px 0 0 rgba(255, 255, 255, 0.04);       /* 暗夜微高光 */
}
```

### 4.3 物理仪表型 Quota 轨道

不再使用 HTML5 风格的厚重圆角进度条。SRapi 的额度和健康度采用**超细刻度线仪表**：

- **结构**：一条极细的 1px 水平背景线，通过绝对定位在其上方投射一个 13px 高、1px 宽的垂直 notch（指示针）。
- **Light 颜色**：轨道背景 `--color-srapi-border-strong` (`#D8CCB4`)，指示针 `--color-srapi-primary` (`#BF5638`)。
- **Dark 颜色**：轨道背景 `--color-srapi-border-strong` (`#3A372E`)，指示针 `--color-srapi-primary` (`#E07A5F`)。
- **保护拦截态**：当 Quota 进入警示阈值，指示针切换为 `--color-srapi-warning`（`data-level="warn"`）；进入危急阈值切换为 `--color-srapi-error`（`data-level="crit"`）。已实现于 `globals.css` 的 `.quota-rail .quota-notch` 与 `components/ui/quota-notch-rail.tsx`。

```txt
轨道：━━━━━━━━━━━━━━━━━━━━┿━━━━━━━━━━━━
                           ↑
                       (Notch 针)
```

### 4.4 静默状态标记 (Quiet Badges)

取消传统 Bootstrap 的高对比度、大背景圆角 Badge。全部状态改为**静默文字标记**：

- **Active**：`● Active`（11px font-mono，没有绿色亮底背景，依靠极细 1px 砂岩边框包裹）。
- **Protected/Limited**：`■ Protected`（11px font-mono，橙色实心正方形，而非圆形）。
- **Disabled**：直接对父卡片应用 `opacity-40`，将文字变为砂岩灰，不做多余视觉标记。

---

## 5. 打字机流式输出与骨架过渡 (Streaming & Transitions)

在展示调度决策日志或 API 调用调试时，SRapi 不直接刷出整块数据。它必须采用 **ChatGPT/Claude 同款的逐字流式打字输出 (Typing Stream)**。

### 5.1 骨架屏脉冲过渡 (Skeleton Dispatch Pulse)
为消除网络抖动带来的空白焦虑，在向流式引擎分发数据之前，展示框会以 **`Skeleton Loader`** 率先过渡。
- **加载态行为**：三行长短不一的圆角线段，应用 `1.2s`、`ease-in-out` 的微光呼吸效果 (`opacity: 0.6` 到 `1.0` 往复）。
- **时序衔接**：骨架脉冲维持 `600ms` 的硬件并发模拟时延，随后平滑淡出，流式打字输出引擎立刻以阻尼弹跳形态淡入接管。

### 5.2 流式打字输出交互
- **增量渲染**：单行文本以随机 `5ms - 15ms` 的间隔进行字符增量渲染。
- **微光游标**：在正在打字的行末，投射一个 `4px` 宽、`14px` 高的流式游标 (`.stream-cursor`)。
- **游标呼吸**：游标应用 `800ms` 的微光呼吸效果。打字结束时，游标静默移除。
- **分步缩进**：每一行输出左侧伴随一条 `1.5px` 宽的暖色细分割线作为空间定位（`.stream-line`，色值取 `--color-srapi-primary` 的 45% 透明叠加）。

---

## 6. 动效工程与回弹规范 (Dynamic Motion System)

设计系统的所有动效不应有工业机器的死板和滞后感。必须遵循**阻尼物理回弹**。

### 6.1 黄金阻尼进场曲线 (The Editorial Bloom Curve)
所有主面板、控制卡片进场淡入时，统一应用：
- **进场曲线**：`cubic-bezier(0.16, 1, 0.3, 1)`（代表在 16% 时间内迅速拉起，并在剩余 84% 时间内极其平滑地刹车、微弱回弹）。
- **位移差**：进场时向下偏移 `20px`。
- **交错阶梯延迟 (Stagger Delay Ratio)**：
  - Header: 延迟 `0ms`
  - Title/Breadcrumbs: 延迟 `80ms`
  - Left Panel: 延迟 `160ms`
  - Right Panel: 延迟 `240ms`
  - 这种阶梯感使整页在刷新时呈现柔和、生动的波浪般“盛放”开来的视觉奇迹。

```css
/* 动效缓动 CSS 类范本 */
.animate-bloom {
    opacity: 0;
    animation: editorialBloom 700ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
}
```

---

## 7. 多端自适应设计规范 (Desktop vs. Mobile Responsiveness)

网关控制台不仅需要在桌面端提供强大的深度排版，也需要在移动端保证极佳的单手操作、可读性和不撑破容器的流式交互。

### 7.1 桌面端大屏：非对称多栏布局 (Asymmetric Multicolumn)

在大屏视口（`lg:grid-cols-12`）下，系统采用**左重右轻、不对称发布台（Publishing Desk）布局**：

- **左侧核心配置（占 7-8 列）**：用于长时间监控和精细操作（如账号池、API Key 表单）。左侧排版舒缓，有大量优雅的留白和细分隔线。
- **右侧流式监视器（占 4-5 列，且保持 `lg:sticky lg:top-28` 悬浮锁死）**：当管理员在左侧操作、测试或观察时，右侧的流式控制台（如打字机调度决策）实时同步输出，双眼无需在页面间频繁切换，提高运维效率。

### 7.2 移动端小屏：视口降维与手势化 (Mobile Consolidation)

当视口缩窄至移动端（`max-width: 1024px`）时，布局自动重构：

- **单列垂直瀑布流**：所有的多栏并列强制拆解、降维成单列流。
  - Tailwind：`grid-cols-1 lg:grid-cols-12`。
- **横向宽数据表的“侧滑防护罩”**：由于移动端屏幕极窄，横向数据表（如 Token Key 列表）如果不做适配，会强制将整页向右撑破，产生灾难性的横向滚动条。
  - **解决方案**：外层必须套用 `overflow-x-auto min-w-[500px] scrollbar-none` 容器。在移动端上，页面主体保持居中对齐，而宽表格可以在其卡片内单独进行平滑的手势侧滑。
- **调度决策抽屉化 (Bottom Drawers)**：
  - 在移动端上，右侧悬浮的调度决策日志应重构为自适应。它会随流堆叠在资源台下方，保持长图文阅读体验。在交互设计中，管理员点击“Simulate”后，应提供可选的 **Framer Motion 底部抽屉弹窗 (Bottom Sheet/Drawer)**，单手轻扫即可展开或收起日志，完美复制原生 App 质感。

---

## 8. 前端视觉一致性约束 (Visual Guardrails)

- **绝对禁止冷灰色**：Tailwind 的 `bg-gray-100`、`text-slate-900` 属于被禁用的“ generic AI slop ”。所有颜色必须严格映射至 `warm-bg`、`ink-bg` 变量。
- **禁止使用第三方图标包发光**：只允许使用极轻量的 `lucide-react`，尺寸统一为 `w-4 h-4` 且不可设置发光特效。
- **对焦高亮暖色化**：键盘聚焦（`:focus-visible`）时统一投射一圈暖色对焦环（`--color-srapi-primary` 的 32% 透明叠加，见 `globals.css` 的 `*:focus-visible`），禁止出现系统默认的亮蓝色对焦环；文本输入框 / 文本域 / 下拉触发器改用「边框变深」表达聚焦，不叠加暖色环（否则看起来像红框报错）。
- **极简大圆角按钮**：
  - 主操作按钮必须采用饱满优雅的 **`rounded-full`**（药丸钮）或 **`rounded-xl`**，严禁使用直角或生硬小圆角。
  - **按键色彩（完美对标 Anthropic Research）**，统一走反相 token `--color-srapi-invert` / `--color-srapi-invert-fg`：
    - **Light Mode**：背景为炭黑 `#1C1A17`，文字为纸白 `#F9F6F0`，Hover 时微弱抬升（`.btn-raise`）。
    - **Dark Mode**：背景为羊皮白 `#F1EFE9`，文字为枯墨黑 `#100F0D`，Hover 时同样轻微抬升。
  - 按钮文字进行微弱的大写字母间距加宽（`tracking-widest uppercase`），呈现出优雅的学术定制感。

## 9. 可访问性与工程约束

视觉风格不得压过可访问性和可操作性。

### 9.1 对比度

- 正文文本与背景必须满足 WCAG AA。
- 关键操作按钮和错误状态必须满足 WCAG AA。
- 低对比度砂岩线只能用于装饰和分隔，不能作为唯一状态表达。

### 9.2 Reduced Motion

如果用户系统启用 `prefers-reduced-motion: reduce`：

- 禁用逐字随机打字动画，改为分块显示。
- 禁用 `Skeleton Dispatch Pulse` 的循环脉冲，改为静态骨架。
- 禁用大幅位移的 `editorialBloom`，只保留轻量淡入或直接展示。
- 底部抽屉动画时间不得超过 `150ms`。

### 9.3 键盘与焦点

- 所有按钮、菜单、表格行操作、抽屉关闭按钮必须可键盘访问。
- Focus ring 必须可见，颜色使用暖色体系，但不得完全移除。
- 表格横向滚动区域必须可通过键盘聚焦并滚动。

### 9.4 组件映射（as-built 清单）

控制台已成熟落地，下列为承载本设计系统的实有组件。命名以 `apps/web/src/components/` 实际文件为准；其中部分概念（卡片质感、API Key 列表、调度概览卡片、用量列表）由 **CSS 工具类 + 通用原语**实现，而非同名独立组件——这一点在表中明确标注，避免误把 CSS 类当成组件查找。

| 视觉概念 | 实际承载 | 路径 / 备注 |
| :--- | :--- | :--- |
| 物理压凹卡片 | **CSS 类 `.tactile-card`**（非组件） + `Card` 原语 | `globals.css` §4.2；`components/ui/card.tsx` |
| 静默状态标记 | `QuietBadge` | `components/ui/quiet-badge.tsx` |
| 物理仪表 Quota 轨道 | `QuotaNotchRail` | `components/ui/quota-notch-rail.tsx`（配 `.quota-rail`） |
| 调度决策流式输出 | `SchedulerDecisionStream` | `components/ui/scheduler-decision-stream.tsx` |
| 指标卡片（概览） | `StatCard` | `components/ui/stat-card.tsx`（取代设想中的 `SchedulerOverviewCards`） |
| API Key 创建 | `ApiKeyCreateDialog` | `components/features/api-key-create-dialog.tsx` |
| API Key 用量 | `ApiKeyUsageDialog` | `components/features/api-key-usage-dialog.tsx` |
| API Key / 用量列表 | `Table` 原语 + 页面组合 | `components/ui/table.tsx`；`app/api-keys/page.tsx`、`app/usage/...`（无 `ApiKeyTable` / `UsageLogTable` 同名组件） |
| Provider 账号管理 | 一组 admin 组件 | `components/admin/account-{detail-sheet,form-dialog,import-dialog}.tsx`、`components/features/account-test-dialog.tsx`（无 `ProviderAccountTable` / `ProviderAccountHealthCard` 同名组件） |

> 通用原语完整目录见 `components/ui/`（button / dialog / sheet / select / multi-select / tabs / popover / tooltip / toast / pagination / empty-state / skeleton 等）；features / admin / charts / chat / playground / layout 等业务组件目录见 `apps/web/src/components/`。本表只覆盖与设计系统直接相关的视觉原语，并非组件总清单——以 `components/` 目录为权威来源。

### 9.5 Tailwind Token 落地

颜色已映射到 Tailwind v4 的 `@theme` token（`globals.css`），业务组件统一引用，不散落硬编码颜色。已落地的颜色 token（以 `globals.css` 为准）：

```txt
--color-srapi-bg
--color-srapi-card
--color-srapi-card-muted
--color-srapi-text-primary
--color-srapi-text-secondary
--color-srapi-text-tertiary
--color-srapi-border
--color-srapi-border-strong
--color-srapi-primary
--color-srapi-primary-hover
--color-srapi-success
--color-srapi-warning
--color-srapi-error
--color-srapi-invert
--color-srapi-invert-fg
```

> 除颜色外，`globals.css` 的 `@theme` 还成套定义了字阶（`--text-2xs..--text-6xl` 含 line-height / letter-spacing 三件套与流体 `--text-hero`）、圆角语言（`--radius-sm..--radius-pill`）、缓动（`--ease-bloom` / `--ease-out-quint` / `--ease-spring` / `--ease-spring-bounce`）与高度阶梯（`--shadow-e1..e3`）。新增组件优先复用这些 token，不要一次性硬编码字号或阴影。

