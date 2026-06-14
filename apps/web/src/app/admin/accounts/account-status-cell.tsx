import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
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
        "inline-flex h-7 items-center gap-1.5 rounded-md border border-srapi-border px-2 font-mono text-2xs text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:bg-srapi-card-muted hover:text-srapi-text-primary disabled:pointer-events-none disabled:opacity-50",
      )}
    >
      <span
        aria-hidden
        className={cn(
          "text-[0.7em] leading-none",
          tone === "active"
            ? "text-srapi-success"
            : tone === "limited"
              ? "text-srapi-warning"
              : tone === "error"
                ? "text-srapi-error"
                : "text-srapi-text-tertiary",
        )}
      >
        {tone === "active" ? "●" : tone === "disabled" ? "○" : "■"}
      </span>
      {label}
    </button>
  ) : (
    <QuietBadge status={tone} label={label} />
  );

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {statusBadge}
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
          className="text-2xs text-srapi-primary hover:underline"
        >
          {t("adminAccounts.validationLink")}
        </a>
      ) : null}
    </span>
  );
}
