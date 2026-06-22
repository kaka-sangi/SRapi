"use client";

import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DataTooltip, type DataTooltipRow } from "@/components/ui/data-tooltip";
import type { ProviderAccount } from "@/lib/sdk-types";

// Compact duration formatter shared by the three render branches. Mirrors the
// admin-bell style ("5m" / "3h" / "2d") but uses i18n strings so the surrounding
// "Refreshes in {duration}" / "Expired {duration} ago" copy reads naturally
// in both en and zh.
function formatDurationMs(deltaMs: number): string {
  const seconds = Math.max(1, Math.floor(Math.abs(deltaMs) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

function isOAuthRuntimeClass(value: ProviderAccount["runtime_class"]): boolean {
  return value === "oauth_refresh" || value === "oauth_device_code";
}

/** Best-effort ISO/timestamp -> short local datetime string. Falls back to raw. */
function formatLocalShort(value: string | null | undefined): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export interface TokenExpiryChipProps {
  account: ProviderAccount;
  /** Override the "now" reference for tests. */
  now?: Date;
}

/**
 * TokenExpiryChip renders one of three states next to the account status:
 *   - "Needs reauth" (error tone) when needs_reauth_at is set — the proactive
 *     refresh worker has given up and an operator must re-bind the account.
 *   - "Expired Xm ago" (warning tone) when token_expires_at is in the past
 *     but needs_reauth_at is not yet set — usually a transient state between
 *     a missed refresh and the next worker tick.
 *   - "Refreshes in Xh" (neutral tone) when token_expires_at is upcoming.
 *
 * Renders nothing for non-OAuth accounts or when no expiry has been
 * snapshotted yet (the gateway's on-demand refresh path covers those).
 */
export function TokenExpiryChip({ account, now }: TokenExpiryChipProps) {
  const { t } = useLanguage();

  if (!isOAuthRuntimeClass(account.runtime_class)) {
    return null;
  }

  const lastRefresh = formatLocalShort(account.last_refreshed_at);
  const expiresAtLocal = formatLocalShort(account.token_expires_at);
  const needsReauthAt = formatLocalShort(account.needs_reauth_at);

  if (account.needs_reauth_at) {
    // Pull the (already truncated to 500 chars by the backend) last refresh
    // error so operators can tell why the refresh worker gave up — was it
    // invalid_grant (revoked token), an upstream 5xx (transient that just
    // crossed the attempts threshold), or a misconfigured client id? The
    // backend snapshots this on every failed refresh path, so it's the
    // single most informative signal for triaging needs_reauth accounts.
    const reason = account.refresh_last_error?.trim();
    const attempts = account.refresh_attempts ?? 0;
    const rows: DataTooltipRow[] = [];
    if (attempts > 0) {
      rows.push({
        label: t("adminAccounts.tokenReauthAttemptsLabel", { count: attempts }),
        value: String(attempts),
        tone: "error",
      });
    }
    if (lastRefresh) {
      rows.push({ label: t("adminAccounts.lastRefreshedAt"), value: lastRefresh });
    }
    if (needsReauthAt) {
      rows.push({ label: t("adminAccounts.tokenNeedsReauth"), value: needsReauthAt, tone: "error" });
    }
    return (
      <DataTooltip
        title={t("adminAccounts.tokenNeedsReauth")}
        primary={
          reason ? (
            <span className="block max-w-[18rem] break-words font-mono text-xs leading-snug">
              {reason}
            </span>
          ) : (
            t("adminAccounts.tokenReauthNoReason")
          )
        }
        rows={rows.length > 0 ? rows : undefined}
      >
        <QuietBadge status="error" label={t("adminAccounts.tokenNeedsReauth")} />
      </DataTooltip>
    );
  }

  if (!account.token_expires_at) {
    return null;
  }

  const reference = now ?? new Date();
  const expiry = new Date(account.token_expires_at);
  if (Number.isNaN(expiry.getTime())) {
    return null;
  }
  const deltaMs = expiry.getTime() - reference.getTime();
  const duration = formatDurationMs(deltaMs);
  const expired = deltaMs < 0;

  const rows: DataTooltipRow[] = [];
  if (expiresAtLocal) {
    rows.push({
      label: t("adminAccounts.tokenExpiresAt"),
      value: expiresAtLocal,
      tone: expired ? "warning" : "default",
    });
  }
  if (lastRefresh) {
    rows.push({ label: t("adminAccounts.lastRefreshedAt"), value: lastRefresh });
  }
  const attempts = account.refresh_attempts ?? 0;
  if (attempts > 0) {
    rows.push({
      label: t("adminAccounts.tokenReauthAttemptsLabel", { count: attempts }),
      value: String(attempts),
      tone: "warning",
    });
  }

  const label = expired
    ? t("adminAccounts.tokenExpiredAgo", { duration })
    : t("adminAccounts.tokenRefreshesIn", { duration });

  // Skip the tooltip wrapper when we have nothing extra to reveal — keeps the
  // chip cheap for accounts with no refresh history snapshotted yet.
  if (rows.length === 0) {
    return <QuietBadge status={expired ? "limited" : "disabled"} label={label} />;
  }

  return (
    <DataTooltip
      title={expired ? t("adminAccounts.tokenExpiredAgo", { duration }) : t("adminAccounts.tokenRefreshesIn", { duration })}
      primary={
        <span className="tabular text-srapi-text-primary">
          {expired ? `-${duration}` : duration}
        </span>
      }
      rows={rows}
    >
      <QuietBadge status={expired ? "limited" : "disabled"} label={label} />
    </DataTooltip>
  );
}
