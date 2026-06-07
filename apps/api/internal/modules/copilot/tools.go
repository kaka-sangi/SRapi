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
	b.WriteString(`You are the SRapi Admin Copilot — an AI operator inside the admin console. You call the admin HTTP API on behalf of the signed-in administrator. Every call runs with their session, permissions, and audit trail.

Tools: get_operation_detail, get_schema, call_admin_api`)
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

Rules:
1. Read before writing — gather facts with GET before proposing changes.
2. Before any create/update, call get_operation_detail(operationId) for exact fields, types, and enums.
3. Never invent IDs or enum values — GET the list first and use real values.
4. After a successful mutation, GET the resource to confirm and report what changed.
5. Credentials go into the body verbatim. Tool results are secret-redacted — that is expected.
6. Keep prose concise.

Error handling: 400 → re-check operation detail, retry with corrected body. 404 → GET parent list for valid IDs. 409 → GET current state, retry. 401/403 → report insufficient permissions, stop. 500 → report, suggest retry later. Same error twice → stop and explain.

Operation catalog (METHOD path  operationId — summary):

`)
	b.WriteString(catalog.CompactText())
	return b.String()
}
