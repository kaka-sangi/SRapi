"use client";

import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
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

  if (account.needs_reauth_at) {
    return <QuietBadge status="error" label={t("adminAccounts.tokenNeedsReauth")} />;
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
  if (deltaMs < 0) {
    return (
      <QuietBadge
        status="limited"
        label={t("adminAccounts.tokenExpiredAgo", { duration })}
      />
    );
  }
  return (
    <QuietBadge
      status="disabled"
      label={t("adminAccounts.tokenRefreshesIn", { duration })}
    />
  );
}
