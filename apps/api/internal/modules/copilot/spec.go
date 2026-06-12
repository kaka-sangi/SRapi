package copilot

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.spec.yaml
var specYAML []byte

// adminPathPrefix scopes the copilot to the admin control plane. Public,
// gateway, and current-user surfaces are intentionally out of reach.
const adminPathPrefix = "/api/v1/admin/"

// copilotPathPrefix covers the copilot's own routes (chat, config, conversation
// history); all are excluded from the catalog so the model can't recurse into
// itself or manage its own saved conversations.
const (
	copilotChatPath   = "/api/v1/admin/copilot/chat"
	copilotConfigPath = "/api/v1/admin/copilot/config"
	copilotPathPrefix = "/api/v1/admin/copilot/"
)

var httpMethods = []string{"get", "post", "put", "patch", "delete"}

var (
	catalogOnce sync.Once
	catalog     *Catalog
	catalogErr  error
)

// CatalogEntry is a compact description of one admin operation, shown to the
// model so it knows what it can call.
type CatalogEntry struct {
	OperationID string
	Method      string // upper-case
	Path        string
	Summary     string
	Mutation    bool
}

// Catalog is the parsed, admin-scoped view of the OpenAPI contract.
type Catalog struct {
	entries    []CatalogEntry
	byID       map[string]CatalogEntry
	operations map[string]map[string]any // operationID -> raw operation node
	schemas    map[string]any            // component schemas
}

// LoadCatalog parses the embedded spec and builds the admin operation catalog.
func LoadCatalog() (*Catalog, error) {
	catalogOnce.Do(func() {
		catalog, catalogErr = buildCatalog()
	})
	return catalog, catalogErr
}

func buildCatalog() (*Catalog, error) {
	var root map[string]any
	if err := yaml.Unmarshal(specYAML, &root); err != nil {
		return nil, fmt.Errorf("copilot: parse embedded spec: %w", err)
	}
	cat := &Catalog{
		byID:       map[string]CatalogEntry{},
		operations: map[string]map[string]any{},
		schemas:    map[string]any{},
	}
	if components, ok := root["components"].(map[string]any); ok {
		if schemas, ok := components["schemas"].(map[string]any); ok {
			cat.schemas = schemas
		}
	}
	paths, _ := root["paths"].(map[string]any)
	for rawPath, rawItem := range paths {
		path, ok := rawPath, true
		if !ok || !strings.HasPrefix(path, adminPathPrefix) {
			continue
		}
		if strings.HasPrefix(path, copilotPathPrefix) {
			continue
		}
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		for _, method := range httpMethods {
			opNode, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			opID := strings.TrimSpace(stringField(opNode, "operationId"))
			if opID == "" {
				continue
			}
			entry := CatalogEntry{
				OperationID: opID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Summary:     strings.TrimSpace(stringField(opNode, "summary")),
				Mutation:    isMutationMethod(method),
			}
			cat.entries = append(cat.entries, entry)
			cat.byID[opID] = entry
			cat.operations[opID] = opNode
		}
	}
	sort.Slice(cat.entries, func(i, j int) bool {
		if cat.entries[i].Path == cat.entries[j].Path {
			return cat.entries[i].Method < cat.entries[j].Method
		}
		return cat.entries[i].Path < cat.entries[j].Path
	})
	if len(cat.entries) == 0 {
		return nil, fmt.Errorf("copilot: no admin operations found in spec")
	}
	return cat, nil
}

// Entries returns the catalog entries (admin operations).
func (c *Catalog) Entries() []CatalogEntry { return c.entries }

// CompactText renders the catalog as one line per operation for the system
// prompt: "METHOD /path  operationId — summary".
func (c *Catalog) CompactText() string {
	var b strings.Builder
	for _, e := range c.entries {
		b.WriteString(e.Method)
		b.WriteByte(' ')
		b.WriteString(e.Path)
		b.WriteString("  ")
		b.WriteString(e.OperationID)
		if e.Summary != "" {
			b.WriteString(" — ")
			b.WriteString(e.Summary)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// OperationDetail returns a model-friendly description of one operation:
// method, path, summary, parameters, requestBody, and 2xx response — with one
// level of #/components/schemas/* refs inlined so the model sees concrete shapes.
func (c *Catalog) OperationDetail(operationID string) (map[string]any, bool) {
	entry, ok := c.byID[operationID]
	if !ok {
		return nil, false
	}
	node := c.operations[operationID]
	detail := map[string]any{
		"operation_id": entry.OperationID,
		"method":       entry.Method,
		"path":         entry.Path,
	}
	if entry.Summary != "" {
		detail["summary"] = entry.Summary
	}
	if desc := strings.TrimSpace(stringField(node, "description")); desc != "" {
		detail["description"] = desc
	}
	if params, ok := node["parameters"]; ok {
		detail["parameters"] = c.inline(params, 0)
	}
	if body, ok := node["requestBody"]; ok {
		detail["request_body"] = c.inline(body, 0)
	}
	if responses, ok := node["responses"].(map[string]any); ok {
		for _, code := range []string{"200", "201", "202", "204"} {
			if resp, ok := responses[code]; ok {
				detail["response"] = map[string]any{code: c.inline(resp, 0)}
				break
			}
		}
	}
	return detail, true
}

// Schema returns a named component schema with one level of refs inlined.
func (c *Catalog) Schema(name string) (map[string]any, bool) {
	schema, ok := c.schemas[name]
	if !ok {
		return nil, false
	}
	resolved, _ := c.inline(schema, 0).(map[string]any)
	if resolved == nil {
		return map[string]any{"schema": c.inline(schema, 0)}, true
	}
	return resolved, true
}

// Lookup resolves a method + path to a catalog entry. Matching tolerates path
// parameters: the spec template "/users/{id}" matches a concrete "/users/42".
func (c *Catalog) Lookup(method, path string) (CatalogEntry, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = cleanPath(path)
	for _, e := range c.entries {
		if e.Method == method && pathMatches(e.Path, path) {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// inline walks a node and replaces each `$ref: "#/components/schemas/X"` with the
// referenced schema, up to maxInlineDepth levels deep (cycle/blowup guard).
const maxInlineDepth = 2

func (c *Catalog) inline(node any, depth int) any {
	switch typed := node.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok && len(typed) == 1 {
			if depth >= maxInlineDepth {
				return map[string]any{"$ref": ref}
			}
			if name, ok := componentSchemaName(ref); ok {
				if schema, ok := c.schemas[name]; ok {
					return c.inline(schema, depth+1)
				}
			}
			return map[string]any{"$ref": ref}
		}
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = c.inline(v, depth)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = c.inline(v, depth)
		}
		return out
	default:
		return node
	}
}

func componentSchemaName(ref string) (string, bool) {
	const prefix = "#/components/schemas/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix), true
	}
	return "", false
}

func isMutationMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func stringField(node map[string]any, key string) string {
	if v, ok := node[key].(string); ok {
		return v
	}
	return ""
}

func cleanPath(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return strings.TrimRight(path, "/")
}

// pathMatches compares a spec path template (with {params}) to a concrete path.
func pathMatches(template, concrete string) bool {
	template = strings.TrimRight(template, "/")
	tParts := strings.Split(strings.TrimPrefix(template, "/"), "/")
	cParts := strings.Split(strings.TrimPrefix(concrete, "/"), "/")
	if len(tParts) != len(cParts) {
		return false
	}
	for i := range tParts {
		if strings.HasPrefix(tParts[i], "{") && strings.HasSuffix(tParts[i], "}") {
			if cParts[i] == "" {
				return false
			}
			continue
		}
		if tParts[i] != cParts[i] {
			return false
		}
	}
	return true
}
