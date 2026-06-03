import type {
  CreateAdminUserRequest,
  UpdateAdminUserRequest,
  UpdateUserBalanceRequest,
  User,
  UserStatus,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const USER_STATUSES: UserStatus[] = ["active", "disabled", "pending"];
export const BALANCE_OPERATIONS: UpdateUserBalanceRequest["operation"][] = [
  "set",
  "increment",
  "decrement",
];

export interface UserCreateFormState {
  email: string;
  name: string;
  password: string;
  rolesCsv: string;
  status: UserStatus;
  rpmLimit: string;
}

export interface UserEditFormState {
  name: string;
  rolesCsv: string;
  status: UserStatus;
  rpmLimit: string;
}

export interface UserBalanceFormState {
  amount: string;
  operation: UpdateUserBalanceRequest["operation"];
  currency: string;
  note: string;
}

export function emptyUserCreateForm(): UserCreateFormState {
  return { email: "", name: "", password: "", rolesCsv: "user", status: "active", rpmLimit: "" };
}

export function userEditFormFromUser(user: User): UserEditFormState {
  return {
    name: user.name,
    rolesCsv: user.roles.join(", "),
    status: user.status,
    rpmLimit: user.rpm_limit == null ? "" : String(user.rpm_limit),
  };
}

export function emptyUserBalanceForm(currency = "USD"): UserBalanceFormState {
  return { amount: "0", operation: "increment", currency, note: "" };
}

export function buildCreateUserBody(form: UserCreateFormState): CreateAdminUserRequest {
  const body: CreateAdminUserRequest = {
    email: requiredText(form.email, "Email"),
    name: requiredText(form.name, "Name"),
    password: requiredText(form.password, "Password"),
    status: form.status,
  };
  const roles = parseRoles(form.rolesCsv);
  if (roles) body.roles = roles;
  body.rpm_limit = parseRpmLimit(form.rpmLimit);
  return body;
}

export function buildUpdateUserBody(form: UserEditFormState): UpdateAdminUserRequest {
  const body: UpdateAdminUserRequest = {
    name: requiredText(form.name, "Name"),
    status: form.status,
    rpm_limit: parseRpmLimit(form.rpmLimit),
  };
  const roles = parseRoles(form.rolesCsv);
  if (roles) body.roles = roles;
  return body;
}

export function buildUserBalanceBody(form: UserBalanceFormState): UpdateUserBalanceRequest {
  const body: UpdateUserBalanceRequest = {
    amount: parseDecimalString(form.amount, "Amount"),
    operation: form.operation,
  };
  const currency = form.currency.trim();
  const note = form.note.trim();
  if (currency) body.currency = currency.toUpperCase();
  if (note) body.note = note;
  return body;
}

/** "" → unlimited (null); otherwise a positive integer. */
function parseRpmLimit(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error("RPM limit must be a non-negative integer.");
  }
  return parsed;
}

function parseRoles(value: string): string[] | undefined {
  const roles = value
    .split(",")
    .map((role) => role.trim())
    .filter(Boolean);
  return roles.length ? roles : undefined;
}

function parseDecimalString(value: string, fieldName: string): string {
  const normalized = requiredText(value, fieldName);
  if (!/^[0-9]+(\.[0-9]+)?$/.test(normalized)) {
    throw new Error(`${fieldName} must be a non-negative decimal string.`);
  }
  return normalized;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
