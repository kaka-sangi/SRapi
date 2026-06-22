import { cn } from "@/lib/cn";

/**
 * BrandMark —— SRapi 的抽象品牌符号。
 *
 * 形状寓意：三道交叠的光弧 + 一个收束的核 —— 「三家供应商汇入一个网关」。
 * 不直接画 "S" 字母，而是用「同心椭圆 + 切线」勾勒，既有几何感又有动势。
 * 配 aurora 配色（陶土主调 + 暖金 + 极光绿），与新视觉语汇同源。
 *
 * 用法：
 *   <BrandMark size={64} />          —— 默认（landing hero、empty state 主图）
 *   <BrandMark size={28} muted />    —— 灰阶（侧栏 brand、loading 占位）
 *   <BrandMark size={120} animated />—— 大型（onboarding 首屏、首次设置完成态）
 */
export function BrandMark({
  size = 64,
  muted = false,
  animated = false,
  className,
}: {
  size?: number;
  muted?: boolean;
  animated?: boolean;
  className?: string;
}) {
  const id = React.useId();
  const gradPrimary = `bm-grad-primary-${id}`;
  const gradWarm = `bm-grad-warm-${id}`;
  const gradCool = `bm-grad-cool-${id}`;

  return (
    <svg
      role="img"
      aria-label="SRapi brand mark"
      viewBox="0 0 64 64"
      width={size}
      height={size}
      className={cn("shrink-0", animated && "aurora-pulse", className)}
    >
      <defs>
        <linearGradient id={gradPrimary} x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor={muted ? "currentColor" : "var(--color-srapi-primary)"} />
          <stop offset="100%" stopColor={muted ? "currentColor" : "var(--color-srapi-primary-hover)"} />
        </linearGradient>
        <linearGradient id={gradWarm} x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor={muted ? "currentColor" : "var(--color-srapi-warning)"} stopOpacity="0.9" />
          <stop offset="100%" stopColor={muted ? "currentColor" : "var(--color-srapi-primary)"} stopOpacity="0.6" />
        </linearGradient>
        <linearGradient id={gradCool} x1="0%" y1="100%" x2="100%" y2="0%">
          <stop offset="0%" stopColor={muted ? "currentColor" : "var(--color-srapi-success)"} stopOpacity="0.7" />
          <stop offset="100%" stopColor={muted ? "currentColor" : "var(--color-srapi-primary)"} stopOpacity="0.4" />
        </linearGradient>
      </defs>

      {/* outer arc — 极光绿 → 陶土，从左下到右上 */}
      <path
        d="M 8 44 Q 32 8, 56 24"
        fill="none"
        stroke={`url(#${gradCool})`}
        strokeWidth="3.5"
        strokeLinecap="round"
        opacity={muted ? 0.5 : 1}
      />
      {/* mid arc — 暖金，反向 */}
      <path
        d="M 12 22 Q 36 56, 56 40"
        fill="none"
        stroke={`url(#${gradWarm})`}
        strokeWidth="3.5"
        strokeLinecap="round"
        opacity={muted ? 0.6 : 1}
      />
      {/* inner converging chord */}
      <path
        d="M 18 32 Q 32 24, 48 36"
        fill="none"
        stroke={`url(#${gradPrimary})`}
        strokeWidth="4.5"
        strokeLinecap="round"
      />
      {/* core dot — 收束点 */}
      <circle
        cx="32"
        cy="32"
        r="3.2"
        fill={muted ? "currentColor" : "var(--color-srapi-primary)"}
      />
    </svg>
  );
}

// React 类型在使用 useId 时需要导入；放在末尾避免污染上方注释展示
import * as React from "react";
