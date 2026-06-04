package copilot

import "strings"

// Tool names the model calls. Two are local (catalog lookups); call_admin_api is
// the only one that touches the system.
const (
	toolGetOperationDetail = "get_operation_detail"
	toolGetSchema          = "get_schema"
	toolCallAdminAPI       = "call_admin_api"
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
func SystemPrompt(catalog *Catalog, autoRunReads bool) string {
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
	b.WriteString(`Mutating calls (POST/PUT/PATCH/DELETE) are ALWAYS shown to the administrator for explicit approval before they run; if approval is denied, adapt and explain.

Guidance:
- Prefer to gather facts with read calls before proposing a change.
- When unsure of an operation's path parameters or body shape, call get_operation_detail first; use the exact method and path from the catalog and substitute concrete values for {params}.
- Keep prose concise. Explain what you did and what you found. When you propose a mutation, briefly say what it will change and why.
- Never invent IDs or values; look them up. If a request fails, read the error and try to recover or report clearly.

Below is the catalog of admin operations you can call (METHOD path  operationId — summary):

`)
	b.WriteString(catalog.CompactText())
	return b.String()
}
