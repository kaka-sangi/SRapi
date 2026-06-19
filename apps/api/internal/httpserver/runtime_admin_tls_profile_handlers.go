package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	tlsprofilescontract "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/contract"
	tlsprofilesservice "github.com/srapi/srapi/apps/api/internal/modules/tls_profiles/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type tlsProfilePayload struct {
	ID                int               `json:"id"`
	Name              string            `json:"name"`
	TLSTemplate       string            `json:"tls_template"`
	HTTPVersionPolicy string            `json:"http_version_policy"`
	UserAgent         string            `json:"user_agent"`
	ExtraHeaders      map[string]string `json:"extra_headers"`
	Enabled           bool              `json:"enabled"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type createTLSProfileRequest struct {
	Name              string            `json:"name"`
	TLSTemplate       string            `json:"tls_template"`
	HTTPVersionPolicy string            `json:"http_version_policy"`
	UserAgent         string            `json:"user_agent"`
	ExtraHeaders      map[string]string `json:"extra_headers"`
	Enabled           *bool             `json:"enabled"`
}

type updateTLSProfileRequest struct {
	Name              *string            `json:"name"`
	TLSTemplate       *string            `json:"tls_template"`
	HTTPVersionPolicy *string            `json:"http_version_policy"`
	UserAgent         *string            `json:"user_agent"`
	ExtraHeaders      *map[string]string `json:"extra_headers"`
	Enabled           *bool              `json:"enabled"`
}

func toTLSProfilePayload(profile tlsprofilescontract.Profile) tlsProfilePayload {
	headers := profile.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	return tlsProfilePayload{
		ID:                profile.ID,
		Name:              profile.Name,
		TLSTemplate:       profile.TLSTemplate,
		HTTPVersionPolicy: profile.HTTPVersionPolicy,
		UserAgent:         profile.UserAgent,
		ExtraHeaders:      headers,
		Enabled:           profile.Enabled,
		CreatedAt:         profile.CreatedAt.UTC(),
		UpdatedAt:         profile.UpdatedAt.UTC(),
	}
}

func toAPITLSProfile(profile tlsprofilescontract.Profile) apiopenapi.TLSProfile {
	headers := profile.ExtraHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	return apiopenapi.TLSProfile{
		Id:                int64(profile.ID),
		Name:              profile.Name,
		TlsTemplate:       profile.TLSTemplate,
		HttpVersionPolicy: profile.HTTPVersionPolicy,
		UserAgent:         profile.UserAgent,
		ExtraHeaders:      headers,
		Enabled:           profile.Enabled,
		CreatedAt:         profile.CreatedAt.UTC(),
		UpdatedAt:         profile.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminTLSProfiles(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	profiles, err := s.runtime.tlsProfiles.ListProfiles(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list tls profiles", requestID)
		return
	}
	data := make([]tlsProfilePayload, 0, len(profiles))
	for _, profile := range profiles {
		data = append(data, toTLSProfilePayload(profile))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pg,
		"request_id": requestID,
	})
}

func (s *Server) handleCreateAdminTLSProfile(w http.ResponseWriter, r *http.Request) {
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
	var body createTLSProfileRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid tls profile request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	profile, err := s.runtime.tlsProfiles.CreateProfile(r.Context(), tlsprofilescontract.CreateProfile{
		Name:              body.Name,
		TLSTemplate:       body.TLSTemplate,
		HTTPVersionPolicy: body.HTTPVersionPolicy,
		UserAgent:         body.UserAgent,
		ExtraHeaders:      body.ExtraHeaders,
		Enabled:           enabled,
	})
	if err != nil {
		s.writeTLSProfileError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "tls_profile.create", "tls_profile", strconv.Itoa(profile.ID), nil, map[string]any{
		"name":         profile.Name,
		"tls_template": profile.TLSTemplate,
		"enabled":      profile.Enabled,
	}))
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toTLSProfilePayload(profile),
		"request_id": requestID,
	})
}

func (s *Server) handleUpdateAdminTLSProfile(w http.ResponseWriter, r *http.Request) {
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
	profileID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || profileID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid tls profile id", requestID)
		return
	}
	var body updateTLSProfileRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid tls profile request", requestID)
		return
	}
	profile, err := s.runtime.tlsProfiles.UpdateProfile(r.Context(), profileID, tlsprofilescontract.UpdateProfile{
		Name:              body.Name,
		TLSTemplate:       body.TLSTemplate,
		HTTPVersionPolicy: body.HTTPVersionPolicy,
		UserAgent:         body.UserAgent,
		ExtraHeaders:      body.ExtraHeaders,
		Enabled:           body.Enabled,
	})
	if err != nil {
		s.writeTLSProfileError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "tls_profile.update", "tls_profile", strconv.Itoa(profile.ID), nil, map[string]any{
		"name":         profile.Name,
		"tls_template": profile.TLSTemplate,
		"enabled":      profile.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toTLSProfilePayload(profile),
		"request_id": requestID,
	})
}

func (s *Server) handleDeleteAdminTLSProfile(w http.ResponseWriter, r *http.Request) {
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
	profileID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || profileID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid tls profile id", requestID)
		return
	}
	if err := s.runtime.tlsProfiles.DeleteProfile(r.Context(), profileID); err != nil {
		s.writeTLSProfileError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "tls_profile.delete", "tls_profile", strconv.Itoa(profileID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": profileID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) writeTLSProfileError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, tlsprofilescontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "tls profile not found", requestID)
	case errors.Is(err, tlsprofilescontract.ErrDuplicateName):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "tls profile name already exists", requestID)
	case errors.Is(err, tlsprofilesservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid tls profile request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process tls profile request", requestID)
	}
}

// expandEgressProfileMetadata resolves a named TLS fingerprint profile reference
// in account metadata into concrete egress_profile fields. It fills only keys the
// account left unset so account-provided egress values always win. Wired into the
// reverse_proxy egress resolver via SetNamedProfileExpander.
func (rt *runtimeState) expandEgressProfileMetadata(metadata map[string]any) (map[string]any, bool) {
	if rt == nil || rt.tlsProfiles == nil || metadata == nil {
		return nil, false
	}
	ref := tlsProfileReference(metadata)
	if ref == "" {
		return nil, false
	}
	snapshot := rt.tlsProfiles.Snapshot(context.Background())
	profile, ok := snapshot[strings.ToLower(ref)]
	if !ok {
		return nil, false
	}
	nested := cloneAnyMap(anyMapValue(metadata["egress_profile"]))
	if nested == nil {
		nested = map[string]any{}
	}
	setEgressDefault(nested, metadata, "tls_template", profile.TLSTemplate)
	setEgressDefault(nested, metadata, "http_version_policy", profile.HTTPVersionPolicy)
	setEgressDefault(nested, metadata, "user_agent", profile.UserAgent)
	if len(profile.ExtraHeaders) > 0 && egressKeyAbsent(nested, metadata, "extra_static_headers") {
		headers := make(map[string]any, len(profile.ExtraHeaders))
		for key, value := range profile.ExtraHeaders {
			headers[key] = value
		}
		nested["extra_static_headers"] = headers
	}
	out := cloneAnyMap(metadata)
	if out == nil {
		out = map[string]any{}
	}
	out["egress_profile"] = nested
	return out, true
}

func tlsProfileReference(metadata map[string]any) string {
	if ref := stringFromAny(metadata["tls_profile"]); ref != "" {
		return ref
	}
	if nested := anyMapValue(metadata["egress_profile"]); nested != nil {
		if ref := stringFromAny(nested["profile"]); ref != "" {
			return ref
		}
		if ref := stringFromAny(nested["tls_profile"]); ref != "" {
			return ref
		}
	}
	return ""
}

func setEgressDefault(nested, metadata map[string]any, key, value string) {
	if value == "" {
		return
	}
	if egressKeyAbsent(nested, metadata, key) {
		nested[key] = value
	}
}

func egressKeyAbsent(nested, metadata map[string]any, key string) bool {
	if nested != nil {
		if stringFromAny(nested[key]) != "" {
			return false
		}
	}
	if stringFromAny(metadata[key]) != "" {
		return false
	}
	if stringFromAny(metadata["egress_"+key]) != "" {
		return false
	}
	return true
}

func anyMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return nil
	}
}

func stringFromAny(value any) string {
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return ""
}
