import type { ReactNode } from "react";
import type { ProviderAccount } from "@/lib/sdk-types";

export type AccountListMode = "cards" | "table";

export interface AccountSelection {
  selected: Set<string>;
  onToggle: (id: string) => void;
  onTogglePage: (ids: string[], checked: boolean) => void;
  bulkActions?: ReactNode;
}

export interface AccountPagination {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}

export function metadataString(metadata: ProviderAccount["metadata"], key: string): string {
  const value = metadata?.[key];
  return typeof value === "string" ? value.trim() : "";
}

export function metadataNumber(metadata: ProviderAccount["metadata"], key: string): number | null {
  const value = metadata?.[key];
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : null;
}

type Translate = (key: string, vars?: Record<string, string | number>) => string;

export function metadataStringList(metadata: ProviderAccount["metadata"], key: string): string[] {
  const value = metadata?.[key];
  if (!Array.isArray(value)) return [];
  return value.map((item) => (typeof item === "string" ? item.trim() : "")).filter(Boolean);
}

function metadataObjectKeyCount(metadata: ProviderAccount["metadata"], key: string): number {
  const value = metadata?.[key];
  if (!value || typeof value !== "object" || Array.isArray(value)) return 0;
  return Object.keys(value).filter(Boolean).length;
}

export function accountModelPolicyLabel(
  t: Translate,
  metadata: ProviderAccount["metadata"],
): string {
  const supported = metadataStringList(metadata, "supported_models").length;
  if (supported > 0) return t("adminAccounts.modelsAllowed", { count: supported });

  const excluded = metadataStringList(metadata, "excluded_models").length;
  if (excluded > 0) return t("adminAccounts.modelsExcluded", { count: excluded });

  const mapped =
    metadataObjectKeyCount(metadata, "model_mapping") +
    metadataObjectKeyCount(metadata, "compact_model_mapping");
  if (mapped > 0) return t("adminAccounts.modelsMapped", { count: mapped });

  return t("adminAccounts.modelsAll");
}

function truncateMetadataValue(value: string, max = 28): string {
  if (value.length <= max) return value;
  return `${value.slice(0, Math.max(0, max - 1))}…`;
}

function isLikelyURL(value: string): boolean {
  return /^https?:\/\//i.test(value.trim());
}

function metadataBoolean(metadata: ProviderAccount["metadata"], key: string): boolean | null {
  const value = metadata?.[key];
  return typeof value === "boolean" ? value : null;
}

export interface AccountMetadataFact {
  key: string;
  label: string;
  value: string;
  tone?: "default" | "enabled" | "disabled";
}

export function accountMetadataFacts(
  t: Translate,
  account: ProviderAccount,
): AccountMetadataFact[] {
  const metadata = account.metadata;
  const facts: AccountMetadataFact[] = [];

  // Backend canonicalizes metadata at write time (see
  // apps/api/internal/modules/accounts/service/metadata_canonical.go); the
  // 000056 backfill migrated existing rows. We read canonical keys only and
  // do NOT chase alias chains — if the canonical key is missing, the data
  // genuinely is missing.
  const email = metadataString(metadata, "email");
  if (email) {
    facts.push({
      key: "email",
      label: t("adminAccounts.factEmail"),
      value: truncateMetadataValue(email, 34),
    });
  }

  const plan = metadataString(metadata, "plan_type");
  if (plan) {
    facts.push({
      key: "plan",
      label: t("adminAccounts.factPlan"),
      value: truncateMetadataValue(plan, 18),
    });
  }

  const maxConcurrency = metadataNumber(metadata, "max_concurrency");
  if (maxConcurrency !== null) {
    facts.push({
      key: "max-concurrency",
      label: t("adminAccounts.factMaxConcurrency"),
      value: String(maxConcurrency),
    });
  }

  const maxSessions = metadataNumber(metadata, "max_sessions");
  if (maxSessions !== null) {
    facts.push({
      key: "max-sessions",
      label: t("adminAccounts.factMaxSessions"),
      value: String(maxSessions),
    });
  }

  const rpm = metadataNumber(metadata, "rpm_limit");
  if (rpm !== null) {
    facts.push({
      key: "rpm",
      label: t("adminAccounts.factRpm"),
      value: String(rpm),
    });
  }

  const org = metadataString(metadata, "organization_id");
  if (org) {
    facts.push({
      key: "org",
      label: t("adminAccounts.factOrg"),
      value: truncateMetadataValue(org, 18),
    });
  }

  const upstreamID = metadataString(metadata, "upstream_account_id");
  if (upstreamID) {
    facts.push({
      key: "upstream-id",
      label: t("adminAccounts.factUpstreamId"),
      value: truncateMetadataValue(upstreamID, 18),
    });
  }

  if (account.upstream_client && !isLikelyURL(account.upstream_client)) {
    facts.push({
      key: "client",
      label: t("adminAccounts.factClient"),
      value: truncateMetadataValue(account.upstream_client, 20),
    });
  }

  return facts;
}

export function accountProfileFacts(t: Translate, account: ProviderAccount): AccountMetadataFact[] {
  return accountMetadataFacts(t, account).filter(
    (fact) => !["email", "max-concurrency", "max-sessions", "rpm"].includes(fact.key),
  );
}

export function accountCapacityFacts(
  t: Translate,
  account: ProviderAccount,
): AccountMetadataFact[] {
  const metadata = account.metadata;
  const facts: AccountMetadataFact[] = [];

  const maxConcurrency = metadataNumber(metadata, "max_concurrency");
  if (maxConcurrency !== null) {
    const currentConcurrency = metadataNumber(metadata, "current_concurrency");
    facts.push({
      key: "max-concurrency",
      label: t("adminAccounts.factMaxConcurrency"),
      value:
        currentConcurrency !== null
          ? `${currentConcurrency}/${maxConcurrency}`
          : String(maxConcurrency),
    });
  }

  const maxSessions = metadataNumber(metadata, "max_sessions");
  if (maxSessions !== null) {
    const activeSessions =
      metadataNumber(metadata, "active_sessions") ?? metadataNumber(metadata, "current_sessions");
    facts.push({
      key: "max-sessions",
      label: t("adminAccounts.factMaxSessions"),
      value: activeSessions !== null ? `${activeSessions}/${maxSessions}` : String(maxSessions),
    });
  }

  const rpm = metadataNumber(metadata, "rpm_limit");
  if (rpm !== null) {
    const used = metadataNumber(metadata, "rpm_used");
    facts.push({
      key: "rpm",
      label: t("adminAccounts.factRpm"),
      value: used !== null ? `${used}/${rpm}` : String(rpm),
    });
  }

  return facts;
}

const accountEndpointOverrides = [
  { key: "chat_completions", metadataKey: "capability_chat_completions" },
  { key: "responses", metadataKey: "capability_responses" },
  { key: "messages", metadataKey: "capability_messages" },
] as const;

export function accountEndpointCapabilityFacts(
  t: Translate,
  account: ProviderAccount,
): AccountMetadataFact[] {
  const facts: AccountMetadataFact[] = [];
  for (const item of accountEndpointOverrides) {
    const value = metadataBoolean(account.metadata, item.metadataKey);
    if (value === null) continue;
    facts.push({
      key: item.key,
      label: t(`adminGatewayResources.endpointShort.${item.key}`),
      value: value ? t("adminAccounts.capabilityForcedOn") : t("adminAccounts.capabilityForcedOff"),
      tone: value ? "enabled" : "disabled",
    });
  }
  return facts;
}

export interface AccountIdentitySummary {
  primary: string;
  secondary: string[];
}

export function accountIdentitySummary(
  t: Translate,
  account: ProviderAccount,
): AccountIdentitySummary {
  const facts = accountMetadataFacts(t, account);
  const email = facts.find((fact) => fact.key === "email")?.value;
  const plan = facts.find((fact) => fact.key === "plan")?.value;
  const upstreamID = facts.find((fact) => fact.key === "upstream-id")?.value;
  const client = facts.find((fact) => fact.key === "client")?.value;
  return {
    primary: email || upstreamID || account.name,
    secondary: [plan, client, upstreamID && email ? upstreamID : ""].filter(
      (value): value is string => Boolean(value),
    ),
  };
}
