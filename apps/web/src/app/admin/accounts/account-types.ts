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
  return value
    .map((item) => (typeof item === "string" ? item.trim() : ""))
    .filter(Boolean);
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

export interface AccountMetadataFact {
  key: string;
  label: string;
  value: string;
}

export function accountMetadataFacts(
  t: Translate,
  account: ProviderAccount,
): AccountMetadataFact[] {
  const metadata = account.metadata;
  const facts: AccountMetadataFact[] = [];

  const email = metadataString(metadata, "email") || metadataString(metadata, "codex_email");
  if (email) {
    facts.push({
      key: "email",
      label: t("adminAccounts.factEmail"),
      value: truncateMetadataValue(email, 34),
    });
  }

  const plan = metadataString(metadata, "plan_type") || metadataString(metadata, "codex_plan_type");
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

  const rpm = metadataNumber(metadata, "rpm_override") ?? metadataNumber(metadata, "rpm_limit");
  if (rpm !== null) {
    facts.push({
      key: "rpm",
      label: t("adminAccounts.factRpm"),
      value: String(rpm),
    });
  }

  const org =
    metadataString(metadata, "organization_id") || metadataString(metadata, "codex_organization_id");
  if (org) {
    facts.push({
      key: "org",
      label: t("adminAccounts.factOrg"),
      value: truncateMetadataValue(org, 18),
    });
  }

  const upstreamID =
    metadataString(metadata, "chatgpt_account_id") ||
    metadataString(metadata, "codex_account_id") ||
    metadataString(metadata, "chatgpt_user_id");
  if (upstreamID) {
    facts.push({
      key: "upstream-id",
      label: t("adminAccounts.factUpstreamId"),
      value: truncateMetadataValue(upstreamID, 18),
    });
  }

  if (account.upstream_client) {
    facts.push({
      key: "client",
      label: t("adminAccounts.factClient"),
      value: truncateMetadataValue(account.upstream_client, 20),
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
