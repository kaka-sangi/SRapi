package httpserver

import (
	"context"
	"net/http"

	errorpassthroughcontract "github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	userattributescontract "github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// configImportRequest is the importable subset of a config snapshot: the
// natural-keyed, self-contained rule config that ports across environments
// without ID remapping. ID-referencing entities (rate limits, providers, models)
// are intentionally export-only.
type configImportRequest struct {
	TLSProfiles              []importTLSProfile    `json:"tls_profiles"`
	UserAttributeDefinitions []importUserAttribute `json:"user_attribute_definitions"`
	ErrorPassthroughRules    []importErrorRule     `json:"error_passthrough_rules"`
}

type importTLSProfile struct {
	Name              string            `json:"name"`
	TLSTemplate       string            `json:"tls_template"`
	HTTPVersionPolicy string            `json:"http_version_policy"`
	UserAgent         string            `json:"user_agent"`
	ExtraHeaders      map[string]string `json:"extra_headers"`
	Enabled           *bool             `json:"enabled"`
}

type importUserAttribute struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	DataType     string   `json:"data_type"`
	Options      []string `json:"options"`
	Required     bool     `json:"required"`
	DisplayOrder int      `json:"display_order"`
	Enabled      *bool    `json:"enabled"`
}

type importErrorRule struct {
	Name        string   `json:"name"`
	Enabled     *bool    `json:"enabled"`
	Priority    int      `json:"priority"`
	Action      string   `json:"action"`
	StatusCodes []int    `json:"status_codes"`
	Classes     []string `json:"classes"`
	Keywords    []string `json:"keywords"`
}

type importSectionResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
}

// handleAdminConfigImport applies the importable sections of a config snapshot by
// natural-key upsert (create when the key is new, update otherwise). With
// ?dry_run=true it reports the create/update counts without writing.
func (s *Server) handleAdminConfigImport(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	dryRunPtr, err := parseBoolQuery(r.URL.Query().Get("dry_run"))
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid dry_run parameter", requestID)
		return
	}
	dryRun := dryRunPtr != nil && *dryRunPtr
	var body configImportRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid config import request", requestID)
		return
	}
	ctx := r.Context()

	tlsResult, err := s.importTLSProfiles(ctx, body.TLSProfiles, dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	attrResult, err := s.importUserAttributeDefinitions(ctx, body.UserAttributeDefinitions, dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}
	ruleResult, err := s.importErrorPassthroughRules(ctx, body.ErrorPassthroughRules, dryRun)
	if err != nil {
		s.writeConfigImportError(w, err, requestID)
		return
	}

	if !dryRun {
		s.runtime.recordAudit(ctx, auditRecordFromRequest(r, session.User.ID, "config_snapshot.import", "config_snapshot", "import", nil, map[string]any{
			"tls_profiles":               tlsResult,
			"user_attribute_definitions": attrResult,
			"error_passthrough_rules":    ruleResult,
		}))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"dry_run":                    dryRun,
			"tls_profiles":               tlsResult,
			"user_attribute_definitions": attrResult,
			"error_passthrough_rules":    ruleResult,
		},
		"request_id": requestID,
	})
}

func (s *Server) importTLSProfiles(ctx context.Context, items []importTLSProfile, dryRun bool) (importSectionResult, error) {
	var result importSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.tlsProfiles.ListProfiles(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]tlsprofilescontract.Profile, len(existing))
	for _, profile := range existing {
		byName[profile.Name] = profile
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.tlsProfiles.UpdateProfile(ctx, current.ID, tlsprofilescontract.UpdateProfile{
				TLSTemplate:       &item.TLSTemplate,
				HTTPVersionPolicy: &item.HTTPVersionPolicy,
				UserAgent:         &item.UserAgent,
				ExtraHeaders:      &item.ExtraHeaders,
				Enabled:           &enabled,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.tlsProfiles.CreateProfile(ctx, tlsprofilescontract.CreateProfile{
			Name:              item.Name,
			TLSTemplate:       item.TLSTemplate,
			HTTPVersionPolicy: item.HTTPVersionPolicy,
			UserAgent:         item.UserAgent,
			ExtraHeaders:      item.ExtraHeaders,
			Enabled:           enabled,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importUserAttributeDefinitions(ctx context.Context, items []importUserAttribute, dryRun bool) (importSectionResult, error) {
	var result importSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.userAttributes.ListDefinitions(ctx)
	if err != nil {
		return result, err
	}
	byKey := make(map[string]userattributescontract.Definition, len(existing))
	for _, def := range existing {
		byKey[def.Key] = def
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		dataType := userattributescontract.DataType(item.DataType)
		current, found := byKey[item.Key]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.userAttributes.UpdateDefinition(ctx, current.ID, userattributescontract.UpdateDefinition{
				Name:         &item.Name,
				DataType:     &dataType,
				Options:      &item.Options,
				Required:     &item.Required,
				DisplayOrder: &item.DisplayOrder,
				Enabled:      &enabled,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.userAttributes.CreateDefinition(ctx, userattributescontract.CreateDefinition{
			Key:          item.Key,
			Name:         item.Name,
			DataType:     dataType,
			Options:      item.Options,
			Required:     item.Required,
			DisplayOrder: item.DisplayOrder,
			Enabled:      enabled,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) importErrorPassthroughRules(ctx context.Context, items []importErrorRule, dryRun bool) (importSectionResult, error) {
	var result importSectionResult
	if len(items) == 0 {
		return result, nil
	}
	existing, err := s.runtime.errorPassthrough.ListRules(ctx)
	if err != nil {
		return result, err
	}
	byName := make(map[string]errorpassthroughcontract.Rule, len(existing))
	for _, rule := range existing {
		byName[rule.Name] = rule
	}
	for _, item := range items {
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		action := errorpassthroughcontract.Action(item.Action)
		current, found := byName[item.Name]
		if found {
			result.Updated++
			if dryRun {
				continue
			}
			if _, err := s.runtime.errorPassthrough.UpdateRule(ctx, current.ID, errorpassthroughcontract.UpdateRule{
				Enabled:     &enabled,
				Priority:    &item.Priority,
				Action:      &action,
				StatusCodes: &item.StatusCodes,
				Classes:     &item.Classes,
				Keywords:    &item.Keywords,
			}); err != nil {
				return result, err
			}
			continue
		}
		result.Created++
		if dryRun {
			continue
		}
		if _, err := s.runtime.errorPassthrough.CreateRule(ctx, errorpassthroughcontract.CreateRule{
			Name:        item.Name,
			Enabled:     enabled,
			Priority:    item.Priority,
			Action:      action,
			StatusCodes: item.StatusCodes,
			Classes:     item.Classes,
			Keywords:    item.Keywords,
		}); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Server) writeConfigImportError(w http.ResponseWriter, err error, requestID string) {
	_ = err
	writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "config import failed", requestID)
}
