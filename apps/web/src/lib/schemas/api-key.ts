import { z } from "zod";

/**
 * SRapi v0.1.0 API key creation schema.
 *
 * Lives in `lib/schemas/` so it can be imported from:
 *   - the create dialog client component (react-hook-form resolver)
 *   - any future Server Action that processes the same payload
 *   - tests (validates the schema in isolation)
 *
 * The constraints intentionally mirror the OpenAPI request body for
 * `POST /api/v1/api-keys`. When the contract changes, update both.
 */
const NAME_MIN = 2;
const NAME_MAX = 80;
const NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9 _.\-/]*$/;
const ALLOWED_MODELS_MAX = 16;
const GROUPS_MAX = 16;

export const createApiKeySchema = z.object({
  name: z
    .string()
    .trim()
    .min(NAME_MIN, { message: "Name must be at least 2 characters." })
    .max(NAME_MAX, { message: "Name must be 80 characters or fewer." })
    .regex(NAME_PATTERN, {
      message: "Use letters, digits, spaces, dots, dashes, slashes or underscores.",
    }),
  allowedModels: z
    .array(z.string().min(1))
    .min(1, { message: "Select at least one model." })
    .max(ALLOWED_MODELS_MAX, { message: "Select at most 16 models." }),
  groupIds: z
    .array(z.string().min(1))
    .max(GROUPS_MAX, { message: "Up to 16 account groups." }),
});

export type CreateApiKeyValues = z.infer<typeof createApiKeySchema>;

/**
 * Helper to parse the comma-separated group IDs the create dialog renders
 * back into a clean array. Used by both the form and any future Server
 * Action that consumes a `FormData`-derived string.
 */
export function parseGroupIdsCsv(input: string): string[] {
  return input
    .split(",")
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
}
