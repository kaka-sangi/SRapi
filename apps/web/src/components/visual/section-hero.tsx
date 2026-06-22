import { cn } from "@/lib/cn";
import { AuroraBackdrop } from "./aurora-backdrop";
import { BrandMark } from "./brand-mark";

/**
 * SectionHero —— admin/user 大区入口的迷你 hero band。
 *
 * 破除「上来就是表格 / 表单」的平凡感：在 PageHeader 之上叠一道 aurora rail，
 * 配 BrandMark 缩略 + 关键 KPI（不强制），把每个大区都打造成「有标识的目的地」
 * 而非「一张孤独的表」。
 *
 * 用法：
 *   <SectionHero
 *     eyebrow="Gateway · Accounts"
 *     title="账户池"
 *     description="管理上游 provider 的 OAuth/API key 凭证、限流、配额。"
 *     metrics={[
 *       { label: "在用", value: "12" },
 *       { label: "异常", value: "2", tone: "warning" },
 *     ]}
 *   />
 */
export interface SectionHeroMetric {
  label: string;
  value: React.ReactNode;
  tone?: "default" | "success" | "warning" | "error";
}

export function SectionHero({
  eyebrow,
  title,
  description,
  metrics,
  actions,
  className,
  showMark = true,
}: {
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  description?: React.ReactNode;
  metrics?: SectionHeroMetric[];
  actions?: React.ReactNode;
  className?: string;
  showMark?: boolean;
}) {
  return (
    <div
      className={cn(
        "relative overflow-hidden rounded-2xl border border-srapi-border bg-srapi-card",
        "px-6 py-7 sm:px-8 sm:py-8",
        className,
      )}
    >
      <AuroraBackdrop tone="rail" intensity={0.85} />
      <div className="dot-grid-overlay pointer-events-none absolute right-0 top-0 h-32 w-40 opacity-60" aria-hidden />

      <div className="relative flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-4">
          {showMark ? (
            <BrandMark size={44} className="mt-1 hidden sm:block" />
          ) : null}
          <div className="min-w-0">
            {eyebrow ? (
              <div className="mb-2 inline-flex items-center gap-1.5 rounded-full bg-srapi-accent-soft px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-primary">
                {eyebrow}
              </div>
            ) : null}
            <h1 className="text-3xl font-semibold leading-tight tracking-tight text-srapi-text-primary sm:text-[2.125rem]">
              {title}
            </h1>
            {description ? (
              <p className="mt-2 max-w-2xl text-sm leading-relaxed text-srapi-text-secondary">
                {description}
              </p>
            ) : null}
          </div>
        </div>

        {(metrics && metrics.length > 0) || actions ? (
          <div className="flex shrink-0 flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:gap-5">
            {metrics && metrics.length > 0 ? (
              <div className="flex items-center gap-5 rounded-2xl border border-srapi-border bg-srapi-card/80 px-4 py-3 backdrop-blur-sm">
                {metrics.map((m, i) => (
                  <div key={i} className="min-w-0">
                    <div className="text-[11px] font-semibold uppercase tracking-[0.1em] text-srapi-text-tertiary">
                      {m.label}
                    </div>
                    <div
                      className={cn(
                        "mt-0.5 text-xl font-semibold tabular tracking-tight",
                        m.tone === "success" && "text-srapi-success",
                        m.tone === "warning" && "text-srapi-warning",
                        m.tone === "error" && "text-srapi-error",
                        (!m.tone || m.tone === "default") && "text-srapi-text-primary",
                      )}
                    >
                      {m.value}
                    </div>
                  </div>
                ))}
              </div>
            ) : null}
            {actions ? (
              <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
