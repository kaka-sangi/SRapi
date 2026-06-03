import type {
  CreateUserAttributeDefinitionRequest,
  UserAttributeDefinition,
} from "../../../../packages/sdk/typescript/src/types.gen";

export type UserAttributeDataType = UserAttributeDefinition["data_type"];

export const USER_ATTRIBUTE_DATA_TYPES: UserAttributeDataType[] = [
  "string",
  "number",
  "boolean",
  "select",
];

export interface UserAttributeFormState {
  key: string;
  name: string;
  data_type: UserAttributeDataType;
  options: string[];
  required: boolean;
  display_order: string;
  enabled: boolean;
}

export function emptyUserAttributeForm(): UserAttributeFormState {
  return {
    key: "",
    name: "",
    data_type: "string",
    options: [],
    required: false,
    display_order: "0",
    enabled: true,
  };
}

export function userAttributeFormFromDefinition(
  definition: UserAttributeDefinition,
): UserAttributeFormState {
  return {
    key: definition.key,
    name: definition.name,
    data_type: definition.data_type,
    options: definition.options ?? [],
    required: definition.required,
    display_order: String(definition.display_order ?? 0),
    enabled: definition.enabled,
  };
}

export function buildUserAttributeBody(
  form: UserAttributeFormState,
): CreateUserAttributeDefinitionRequest {
  const key = form.key.trim();
  if (!key) {
    throw new Error("Key is required.");
  }
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  const options = form.options.map((option) => option.trim()).filter(Boolean);
  if (form.data_type === "select" && options.length === 0) {
    throw new Error("Select attributes need at least one option.");
  }
  const displayOrder = form.display_order.trim();
  return {
    key,
    name,
    data_type: form.data_type,
    // Options only mean anything for a select; drop them otherwise so the stored
    // definition matches what the form actually offers.
    options: form.data_type === "select" ? options : [],
    required: form.required,
    display_order: displayOrder === "" ? 0 : Number(displayOrder),
    enabled: form.enabled,
  };
}
