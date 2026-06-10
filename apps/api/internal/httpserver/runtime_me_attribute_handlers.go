package httpserver

import (
	"net/http"
	"strconv"

	userattributesservice "github.com/srapi/srapi/apps/api/internal/modules/userattributes/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type currentUserAttributeValueRequest struct {
	Values []currentUserAttributeValueInput `json:"values"`
}

type currentUserAttributeValueInput struct {
	DefinitionID int    `json:"definition_id"`
	Value        string `json:"value"`
}

func (s *Server) handleRegistrationAttributes(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	defs, err := s.runtime.userAttributes.ListDefinitions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list registration attributes", requestID)
		return
	}
	data := make([]userAttributeValuePayload, 0, len(defs))
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		data = append(data, userAttributeValuePayload{
			DefinitionID: def.ID,
			Key:          def.Key,
			Name:         def.Name,
			DataType:     string(def.DataType),
			Options:      attributeOptions(def.Options),
			Required:     def.Required,
		})
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleCurrentUserAttributes(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	items, err := s.runtime.userAttributes.ListVisibleUserAttributes(r.Context(), session.User.ID)
	if err != nil {
		s.writeUserAttributeError(w, err, requestID, "invalid user attribute request")
		return
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toCurrentUserAttributePayloads(items),
		"pagination": pagination(len(items)),
		"request_id": requestID,
	})
}

func (s *Server) handleUpdateCurrentUserAttributes(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var body currentUserAttributeValueRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user attribute request", requestID)
		return
	}
	values := make(map[int]string, len(body.Values))
	for _, item := range body.Values {
		values[item.DefinitionID] = item.Value
	}
	if err := s.runtime.userAttributes.ValidateRequiredValues(r.Context(), values); err != nil {
		s.writeUserAttributeError(w, err, requestID, "invalid user attribute request")
		return
	}
	items, err := s.runtime.userAttributes.SetUserValues(r.Context(), session.User.ID, values)
	if err != nil {
		s.writeUserAttributeError(w, err, requestID, "invalid user attribute request")
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_attribute_value.self_update", "user", strconv.Itoa(session.User.ID), nil, map[string]any{
		"count": len(values),
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toCurrentUserAttributePayloads(items),
		"pagination": pagination(len(items)),
		"request_id": requestID,
	})
}

func toCurrentUserAttributePayloads(items []userattributesservice.UserAttribute) []userAttributeValuePayload {
	data := make([]userAttributeValuePayload, 0, len(items))
	for _, item := range items {
		payload := userAttributeValuePayload{
			DefinitionID: item.Definition.ID,
			Key:          item.Definition.Key,
			Name:         item.Definition.Name,
			DataType:     string(item.Definition.DataType),
			Options:      attributeOptions(item.Definition.Options),
			Required:     item.Definition.Required,
		}
		if item.Value != nil {
			payload.Value = item.Value.Value
			updatedAt := item.Value.UpdatedAt.UTC()
			payload.UpdatedAt = &updatedAt
		}
		data = append(data, payload)
	}
	return data
}

func attributeOptions(options []string) []string {
	if options == nil {
		return []string{}
	}
	return options
}
