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
