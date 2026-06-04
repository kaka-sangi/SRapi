import type { CreateRoleRequest } from "@/lib/sdk-types";

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

export function buildRoleBody(draft: RoleFormState): CreateRoleRequest {
  return {
    name: draft.name.trim(),
    description: draft.description.trim() || undefined,
    permissions: draft.permissions.map((p) => p.trim()).filter(Boolean),
  };
}
