import { cn } from "@/lib/cn";

/**
 * Aurora backdrop — 多色径向 blob，慢速漂移。
 *
 * 设计意图：把页面背景从「死寂的奶油纸」升级成「会呼吸的光环境」。Blob 用
 * --aurora-primary / --aurora-warm / --aurora-cool / --aurora-ivory 这四个
 * 全局 token 组合，深浅色模式自动同步。每个 blob 是单独的 absolute 圆，
 * blur(60–100px) + opacity(0.4–0.7) 叠加，得到极光层流的非匀质感。
 *
 * tone: "hero" —— landing/dashboard hero 区域，三色全开 + 大半径
 *       "card" —— 小尺寸卡片背景，单色 + 轻幅
 *       "rail" —— 顶部窄条（admin section hero band）
 */
export function AuroraBackdrop({
  tone = "hero",
  className,
  intensity = 1,
}: {
  tone?: "hero" | "card" | "rail";
  className?: string;
  /** 0.5–1.5 缩放整体亮度，弱光环境用 0.7 */
  intensity?: number;
}) {
  if (tone === "rail") {
    return (
      <div
        aria-hidden
        className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
      >
        <div
          className="aurora-drift absolute -left-20 top-1/2 size-[18rem] -translate-y-1/2 rounded-full blur-3xl"
          style={{
            background:
              "radial-gradient(circle at center, var(--aurora-primary), transparent 70%)",
            opacity: 0.55 * intensity,
          }}
        />
        <div
          className="aurora-drift-slow absolute right-[-8%] top-[-30%] size-[16rem] rounded-full blur-3xl"
          style={{
            background:
              "radial-gradient(circle at center, var(--aurora-warm), transparent 70%)",
            opacity: 0.5 * intensity,
          }}
        />
      </div>
    );
  }

  if (tone === "card") {
    return (
      <div
        aria-hidden
        className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
      >
        <div
          className="aurora-drift absolute -right-16 -top-16 size-[14rem] rounded-full blur-3xl"
          style={{
            background:
              "radial-gradient(circle at center, var(--aurora-primary), transparent 65%)",
            opacity: 0.45 * intensity,
          }}
        />
      </div>
    );
  }

  // hero — full composition
  return (
    <div
      aria-hidden
      className={cn("pointer-events-none absolute inset-0 overflow-hidden", className)}
    >
      <div
        className="aurora-drift absolute -left-32 -top-40 size-[44rem] rounded-full blur-[100px]"
        style={{
          background:
            "radial-gradient(circle at center, var(--aurora-primary), transparent 70%)",
          opacity: 0.65 * intensity,
        }}
      />
      <div
        className="aurora-drift-slow absolute right-[-15%] top-[18%] size-[36rem] rounded-full blur-[90px]"
        style={{
          background:
            "radial-gradient(circle at center, var(--aurora-warm), transparent 70%)",
          opacity: 0.55 * intensity,
        }}
      />
      <div
        className="aurora-drift aurora-pulse absolute left-[8%] bottom-[-25%] size-[38rem] rounded-full blur-[110px]"
        style={{
          background:
            "radial-gradient(circle at center, var(--aurora-cool), transparent 70%)",
          opacity: 0.45 * intensity,
        }}
      />
      <div
        className="aurora-drift-slow absolute right-[20%] bottom-[10%] size-[26rem] rounded-full blur-[80px]"
        style={{
          background:
            "radial-gradient(circle at center, var(--aurora-ivory), transparent 65%)",
          opacity: 0.5 * intensity,
        }}
      />
    </div>
  );
}
