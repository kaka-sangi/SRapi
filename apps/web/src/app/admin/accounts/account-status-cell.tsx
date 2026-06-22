import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DataTooltip, type DataTooltipRow } from "@/components/ui/data-tooltip";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { ProviderAccount } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { metadataString } from "./account-types";

export function AccountStatusCell({
  account,
  busy = false,
  onToggle,
}: {
  account: ProviderAccount;
  busy?: boolean;
  onToggle?: () => void;
}) {
  const { t } = useLanguage();
  const quotaClass = metadataString(account.metadata, "last_quota_error_class");
  const validationURL = metadataString(account.metadata, "validation_url");
  const tone = quietStatusFor(account.status);
  const label = statusLabel(t, account.status);
  const actionLabel = account.status === "disabled" ? t("common.enable") : t("common.disable");

  const statusBadge = onToggle ? (
    <button
      type="button"
      disabled={busy}
      aria-label={actionLabel}
      title={actionLabel}
      onClick={onToggle}
      className={cn(
        "inline-flex h-7 items-center gap-1.5 rounded-full border border-srapi-border bg-srapi-card px-2.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:border-srapi-border-strong hover:bg-srapi-card-muted hover:text-srapi-text-primary disabled:pointer-events-none disabled:opacity-50",
      )}
    >
      <span
        aria-hidden
        className={cn(
          "inline-block size-1.5 rounded-full",
          tone === "active"
            ? "bg-srapi-success"
            : tone === "limited"
              ? "bg-srapi-warning"
              : tone === "error"
                ? "bg-srapi-error"
                : "bg-srapi-text-tertiary/60",
        )}
      />
      {label}
    </button>
  ) : (
    <QuietBadge status={tone} label={label} />
  );

  // Build a hover-reveal data card for the status chip: priority/weight/risk
  // are routing-level numbers that operators triage with — surface them on
  // hover so the chip doubles as a routing summary.
  const rows: DataTooltipRow[] = [
    { label: t("adminCommon.status"), value: label, tone: tone === "active" ? "success" : tone === "error" ? "error" : tone === "limited" ? "warning" : "muted" },
    { label: t("adminAccounts.priority"), value: String(account.priority ?? 0) },
    { label: t("adminAccounts.weight"), value: String(account.weight ?? 1) },
  ];
  if (account.risk_level) {
    rows.push({ label: t("adminAccounts.riskLevel"), value: account.risk_level });
  }
  if (quotaClass) {
    rows.push({
      label: t("adminAccounts.lastError"),
      value: quotaClass === "validation_required" ? t("adminAccounts.validationRequired") : quotaClass,
      tone: quotaClass === "validation_required" ? "warning" : "error",
    });
  }

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <DataTooltip
        title={t("adminCommon.status")}
        primary={label}
        rows={rows}
        footer={account.upstream_client || undefined}
      >
        {statusBadge}
      </DataTooltip>
      {quotaClass ? (
        <QuietBadge
          status={quotaClass === "validation_required" ? "limited" : "error"}
          label={quotaClass === "validation_required" ? t("adminAccounts.validationRequired") : quotaClass}
        />
      ) : null}
      {validationURL ? (
        <a
          href={validationURL}
          target="_blank"
          rel="noreferrer"
          className="text-[11px] font-medium text-srapi-primary hover:underline"
        >
          {t("adminAccounts.validationLink")}
        </a>
      ) : null}
    </span>
  );
}
