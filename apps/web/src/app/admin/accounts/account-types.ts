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
