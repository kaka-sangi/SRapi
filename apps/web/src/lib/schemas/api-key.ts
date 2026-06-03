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
const IP_LIST_MAX = 64;

const ipWindowLimit = z
  .number()
  .int({ message: "Limit must be a whole number." })
  .min(0, { message: "Limit cannot be negative." })
  .optional();

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
  // IP allow/deny entries (IP or CIDR). Format is validated authoritatively by
  // the API; the client only bounds the count and rejects blanks.
  allowedIps: z
    .array(z.string().trim().min(1))
    .max(IP_LIST_MAX, { message: "Up to 64 allowed IPs/CIDRs." }),
  deniedIps: z
    .array(z.string().trim().min(1))
    .max(IP_LIST_MAX, { message: "Up to 64 denied IPs/CIDRs." }),
  requestLimit5h: ipWindowLimit,
  requestLimit1d: ipWindowLimit,
  requestLimit7d: ipWindowLimit,
  // Per-key throughput ceilings (0/empty = unlimited) and an optional expiry.
  rpmLimit: ipWindowLimit,
  tpmLimit: ipWindowLimit,
  concurrencyLimit: ipWindowLimit,
  expiresAt: z.string().trim().optional(),
});

export type CreateApiKeyValues = z.infer<typeof createApiKeySchema>;
