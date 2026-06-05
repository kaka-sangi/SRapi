import type { CreateRoleRequest, Role, UpdateRoleRequest } from "@/lib/sdk-types";

/** SRapi's bootstrap roles — immutable, so the UI hides edit/delete for them
 * (the backend also rejects modifying them). */
export const BUILT_IN_ROLES = ["owner", "admin", "operator", "user"];

export function isBuiltInRole(name: string): boolean {
  return BUILT_IN_ROLES.includes(name);
}

/** Flat draft for the create-role dialog. Permissions are free-form
 * `resource:action` keys (e.g. `usage:read`) — there is no enumeration endpoint,
 * so the form collects them as tags. */
export interface RoleFormState {
  name: string;
  description: string;
  permissions: string[];
}

export function emptyRoleForm(): RoleFormState {
  return { name: "", description: "", permissions: [] };
}

export function roleFormFromRole(role: Role): RoleFormState {
  return {
    name: role.name,
    description: role.description ?? "",
    permissions: role.permissions ?? [],
  };
}

export function buildRoleBody(draft: RoleFormState): CreateRoleRequest {
  return {
    name: draft.name.trim(),
    description: draft.description.trim() || undefined,
    permissions: draft.permissions.map((p) => p.trim()).filter(Boolean),
  };
}

/** The role name is immutable, so updates only carry description + permissions. */
export function buildRoleUpdateBody(draft: RoleFormState): UpdateRoleRequest {
  return {
    description: draft.description.trim() || undefined,
    permissions: draft.permissions.map((p) => p.trim()).filter(Boolean),
  };
}
