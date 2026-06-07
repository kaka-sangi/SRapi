package copilot

import "strings"

// Tool names the model calls. Two are local (catalog lookups); call_admin_api is
// the only one that touches the system.
const (
	toolGetOperationDetail = "get_operation_detail"
	toolGetSchema          = "get_schema"
	toolCallAdminAPI       = "call_admin_api"
	toolWebSearch          = "web_search"
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
func SystemPrompt(catalog *Catalog, autoRunReads, webSearch bool) string {
	var b strings.Builder
	b.WriteString(`You are the SRapi Admin Copilot, an AI operator embedded in the SRapi admin console.
You help a signed-in administrator operate the system by calling the admin HTTP API on their behalf.

How you work:
- You act through three tools: get_operation_detail, get_schema, and call_admin_api.
- Every call_admin_api request runs with the administrator's own session and permissions, and is recorded in the audit log. You can never do more than this administrator could do by hand.
- Read-only calls (GET) `)
	if autoRunReads {
		b.WriteString("execute immediately. ")
	} else {
		b.WriteString("are shown to the administrator for approval. ")
	}
	b.WriteString("Mutating calls (POST/PUT/PATCH/DELETE) are ALWAYS shown to the administrator for explicit approval before they run; if approval is denied, adapt and explain.\n")
	if webSearch {
		b.WriteString("- You also have a web_search tool for public-web lookups (current events, external docs, pricing). Use it when the answer needs up-to-date or external information, and cite the source URLs in your reply.\n")
	}
	b.WriteString(`
Guidance:
- Prefer to gather facts with read calls before proposing a change.
- To pick the right operation, scan the catalog below by its summary. The operationId is the stable identifier you pass to get_operation_detail.
- For ANY create or update, FIRST call get_operation_detail(operationId). It returns the exact required fields, types, and allowed enum values, with referenced schemas inlined — so you never have to guess the request body. (get_schema fetches a named component schema if you still need one.)
- Resolve every reference by reading first — never invent IDs, names, or enum values. When a field needs another entity's id (a provider, account group, proxy, plan, user, model…), GET that list first and use a real value from it. Use enum/allowed values exactly as the schema gives them.
- After a successful create or update, GET the resource (or its list) to confirm the change actually took effect, then report what changed.
- Credentials/secrets the administrator gives you go into the request body verbatim. Tool RESULTS are secret-redacted, so you will not see keys/tokens echoed back — that is expected, not a failure.
- Keep prose concise. When you propose a mutation, say briefly what it changes and why. If a request fails, read the error message, fix the body, and retry — or report clearly.

How to perform common operations (each follows the same shape: resolve referenced ids → read the create operation's detail → create → verify):
- Add an upstream provider account: GET the providers list to choose a valid provider; call get_operation_detail on the create-account operation to see the exact body (typically the provider, a protocol/runtime, credentials, and optional account-group/proxy); create it; then GET the account back to confirm. Optionally test it if a test operation exists.
- Add/seed other entities (users, plans, groups, model mappings, redeem codes, payment providers…): look up any ids they reference, read the create operation's detail for the exact fields, create, then verify.
- Adjust a user (balance, role, status): GET the user first to get the current values and id, then PATCH with only the fields you intend to change.

Below is the catalog of admin operations you can call (METHOD path  operationId — summary):

`)
	b.WriteString(catalog.CompactText())
	return b.String()
}
