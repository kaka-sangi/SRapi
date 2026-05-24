import type { ProviderAccount, User } from "../../../../packages/sdk/typescript/src/types.gen";

export type ToggleAction = "enable" | "disable";

export function toggleActionFromStatus(status: string): ToggleAction {
  return status === "disabled" ? "enable" : "disable";
}

export function userToggleIdentifier(user: Pick<User, "email" | "name">): string {
  return user.email || user.name;
}

export function accountToggleIdentifier(account: Pick<ProviderAccount, "name" | "id">): string {
  return account.name || account.id;
}

export function canConfirmToggle(identifier: string, confirmation: string): boolean {
  return identifier.length > 0 && confirmation.trim() === identifier;
}

export function toggleActionLabel(action: ToggleAction, subject: string): string {
  const verb = action === "enable" ? "Enable" : "Disable";
  return `${verb} ${subject}`;
}
