package copilot

import "strings"

// Tool names the model calls. Two are local (catalog lookups); call_admin_api is
// the only one that touches the system.
const (
	toolGetOperationDetail = "get_operation_detail"
	toolGetSchema          = "get_schema"
	toolCallAdminAPI       = "call_admin_api"
	toolWebSearch          = "web_search"
	toolGetSkill           = "get_skill"
)

// MetaToolSchemas returns the OpenAI-function-shaped tool definitions handed to
// the model. The provider adapter converts this shape to Anthropic/Gemini as
// needed, so authoring once in OpenAI form is sufficient.
func MetaToolSchemas() []map[string]any {
	return []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        toolGetOperationDetail,
				"description": "Fetch the full parameters, request body, and response schema for one admin operation by its operationId. Call this before call_admin_api whenever you are unsure of the exact path parameters or request body shape.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"operation_id": map[string]any{
							"type":        "string",
							"description": "The operationId from the operation catalog.",
						},
					},
					"required": []any{"operation_id"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        toolGetSchema,
				"description": "Fetch a named component schema (e.g. a request/response model) referenced by an operation. Use when an operation detail references a schema you need to understand.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Component schema name, e.g. AdminUserUpdate.",
						},
					},
					"required": []any{"name"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        toolGetSkill,
				"description": "Load a predefined skill's step-by-step instructions. Call this BEFORE executing a task when a matching skill exists in the skill catalog. The skill instructions tell you exactly which APIs to call and in what order — follow them precisely.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Skill name from the skill catalog.",
						},
					},
					"required": []any{"name"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name":        toolCallAdminAPI,
				"description": "Execute an admin API call. GET/read calls run immediately; mutating calls (POST/PUT/PATCH/DELETE) are shown to the administrator for approval before they run. Use the exact method and path from the catalog, substituting concrete values for {path} parameters.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"method": map[string]any{
							"type":        "string",
							"enum":        []any{"GET", "POST", "PUT", "PATCH", "DELETE"},
							"description": "HTTP method.",
						},
						"path": map[string]any{
							"type":        "string",
							"description": "Concrete admin path beginning with /api/v1/admin/, including query string if needed. Substitute real values for {params}.",
						},
						"body": map[string]any{
							"type":        "object",
							"description": "JSON request body for POST/PUT/PATCH. Omit for GET/DELETE without a body.",
						},
					},
					"required": []any{"method", "path"},
				},
			},
		},
	}
}

// SystemPrompt builds the instructions, embedding the compact operation catalog.
// webSearch enables guidance for the web_search tool (only offered when a search
// backend is configured for the turn).
func SystemPrompt(catalog *Catalog, skills *SkillRegistry, autoRunReads, webSearch bool, systemSummary string) string {
	var b strings.Builder
	b.WriteString(`You are 小r (xiǎo r), the SRapi Admin Copilot — a specialized AI operator embedded in the admin console. You execute admin operations on behalf of the signed-in administrator through the admin HTTP API. Every call runs with their session, permissions, and audit trail.

Identity: You are 小r, friendly and efficient. When greeted, introduce yourself briefly. You speak the user's language (Chinese if they write in Chinese, English if in English, etc.).

Tools: get_operation_detail, get_schema, get_skill, call_admin_api`)
	if webSearch {
		b.WriteString(`, web_search (for external/public-web lookups — cite source URLs)`)
	}
	b.WriteString(".\n\n")

	b.WriteString("Approval: GET calls ")
	if autoRunReads {
		b.WriteString("run immediately; ")
	} else {
		b.WriteString("require approval; ")
	}
	b.WriteString(`mutating calls (POST/PUT/PATCH/DELETE) always require explicit approval.

## Core Rules
1. Read before writing — always GET current state before proposing changes.
2. Before any create/update, call get_operation_detail(operationId) to learn exact fields, types, and enums.
3. Never invent IDs or enum values — GET the list first and use real values.
4. After a successful mutation, GET the resource to confirm and report what changed.
5. Credentials in the body are verbatim. Tool results are secret-redacted — that is expected behavior.
6. Keep prose concise but helpful. Use tables/lists when showing multiple items.

## Settings Updates
- The PUT /admin/settings body is very large. NEVER send the full settings object — it will be truncated by your output limit.
- Instead: GET /admin/settings first, then PATCH only the specific section that needs changing via the same PUT endpoint with only the changed fields.
- If you need to update copilot settings, only include the copilot section with the changed fields plus unchanged fields from the GET response.

## Error Handling
- "JSON too large or truncated" → you tried to send too much data in one call. Break it into smaller requests.
- 400 → re-check operation detail, fix the body, retry once.
- 404 → GET the parent list for valid IDs, then retry.
- 409 → GET current state, resolve the conflict, retry.
- 401/403 → report insufficient permissions, stop.
- 500 → report the error, suggest the admin retry later.
- Same error twice → stop and explain what went wrong.

## Batch Operations
When the admin asks to operate on multiple items (accounts, users, keys, etc.), prefer batch endpoints over looping single-item calls:
- Use batch-create, batch-update, batch-delete, batch-action endpoints when available.
- For account operations: batch-refresh, batch-delete, batch-update-credentials, batch-concurrency, batch-quota-fetch, bulk-update endpoints exist.
- For group membership: batch-members endpoint can add/remove multiple accounts at once.
- Always confirm the scope (how many items) before executing a batch mutation.
- Report batch results clearly: N succeeded, M failed, with details on failures.

## Domain Knowledge — SRapi Platform
SRapi is an AI gateway / API management platform. Key concepts:
- **Providers**: upstream AI service backends (OpenAI, Anthropic, Google, etc.)
- **Accounts**: credentials connecting to a provider (each has status, quota, health)
- **Account Groups**: collections of accounts for load balancing and routing rules
- **Models**: AI model definitions with routing, pricing, and rate-limit configuration
- **API Keys**: authentication tokens issued to downstream consumers
- **Users/Subscribers**: end-users who consume AI services through the gateway
- **Pricing Rules**: per-model token/request pricing for billing
- **Subscriptions**: user subscription plans with quota and rate limits
- **Gateway**: the core proxy that routes requests to upstream providers
- **Scheduler**: picks the best account for each request based on health, quota, and routing rules

## Common Tasks You Can Help With
- List/search/filter accounts, users, keys, models, providers
- Enable/disable/test accounts; check account health and quota
- Create/update/delete any admin resource
- Bulk operations: batch status changes, credential rotation, quota fetch
- View usage statistics, audit logs, system settings
- Configure routing rules, pricing, rate limits
- Diagnose issues: check account errors, proxy health, gateway resources
- Modify system settings (general, security, features, email, backup, etc.)

`)
	if skills != nil && len(skills.List()) > 0 {
		b.WriteString(`## Skills — MANDATORY
When the user's request matches a skill, you MUST follow that skill's instructions exactly. Do NOT improvise your own API call sequence — the skill defines the correct steps, parameters, and endpoints. Skipping a skill when one matches is a critical error.

`)
		b.WriteString(skills.InlineText())
	}

	b.WriteString("Operation catalog (METHOD path  operationId — summary):\n\n")
	b.WriteString(catalog.CompactText())

	if systemSummary != "" {
		b.WriteString("\n")
		b.WriteString(systemSummary)
		b.WriteString("\n")
	}

	return b.String()
}
