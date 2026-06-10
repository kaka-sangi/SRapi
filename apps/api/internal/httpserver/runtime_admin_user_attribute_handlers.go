package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	userattributescontract "github.com/srapi/srapi/apps/api/internal/modules/userattributes/contract"
	userattributesservice "github.com/srapi/srapi/apps/api/internal/modules/userattributes/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// userAttributeDefinitionPayload is the JSON shape returned for an attribute
// definition. The EAV admin surface predates the OpenAPI catalog, so it uses
// local DTOs encoded via writeJSONAny rather than generated types.
type userAttributeDefinitionPayload struct {
	ID           int       `json:"id"`
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	DataType     string    `json:"data_type"`
	Options      []string  `json:"options"`
	Required     bool      `json:"required"`
	DisplayOrder int       `json:"display_order"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type userAttributeValuePayload struct {
	DefinitionID int        `json:"definition_id"`
	Key          string     `json:"key"`
	Name         string     `json:"name"`
	DataType     string     `json:"data_type"`
	Options      []string   `json:"options"`
	Required     bool       `json:"required"`
	Value        string     `json:"value"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

type createUserAttributeDefinitionRequest struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	DataType     string   `json:"data_type"`
	Options      []string `json:"options"`
	Required     bool     `json:"required"`
	DisplayOrder int      `json:"display_order"`
	Enabled      *bool    `json:"enabled"`
}

type updateUserAttributeDefinitionRequest struct {
	Name         *string   `json:"name"`
	DataType     *string   `json:"data_type"`
	Options      *[]string `json:"options"`
	Required     *bool     `json:"required"`
	DisplayOrder *int      `json:"display_order"`
	Enabled      *bool     `json:"enabled"`
}

type setUserAttributeValueRequest struct {
	Value string `json:"value"`
}

func toUserAttributeDefinitionPayload(def userattributescontract.Definition) userAttributeDefinitionPayload {
	options := def.Options
	if options == nil {
		options = []string{}
	}
	return userAttributeDefinitionPayload{
		ID:           def.ID,
		Key:          def.Key,
		Name:         def.Name,
		DataType:     string(def.DataType),
		Options:      options,
		Required:     def.Required,
		DisplayOrder: def.DisplayOrder,
		Enabled:      def.Enabled,
		CreatedAt:    def.CreatedAt.UTC(),
		UpdatedAt:    def.UpdatedAt.UTC(),
	}
}

func (s *Server) handleListAdminUserAttributeDefinitions(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	defs, err := s.runtime.userAttributes.ListDefinitions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list user attribute definitions", requestID)
		return
	}
	data := make([]userAttributeDefinitionPayload, 0, len(defs))
	for _, def := range defs {
		data = append(data, toUserAttributeDefinitionPayload(def))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleCreateAdminUserAttributeDefinition(w http.ResponseWriter, r *http.Request) {
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
	var body createUserAttributeDefinitionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user attribute definition request", requestID)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	def, err := s.runtime.userAttributes.CreateDefinition(r.Context(), userattributescontract.CreateDefinition{
		Key:          body.Key,
		Name:         body.Name,
		DataType:     userattributescontract.DataType(body.DataType),
		Options:      body.Options,
		Required:     body.Required,
		DisplayOrder: body.DisplayOrder,
		Enabled:      enabled,
	})
	if err != nil {
		s.writeUserAttributeError(w, err, requestID, "invalid user attribute definition request")
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_attribute_definition.create", "user_attribute_definition", strconv.Itoa(def.ID), nil, map[string]any{
		"key":       def.Key,
		"name":      def.Name,
		"data_type": def.DataType,
		"required":  def.Required,
		"enabled":   def.Enabled,
	}))
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toUserAttributeDefinitionPayload(def),
		"request_id": requestID,
	})
}

func (s *Server) handleUpdateAdminUserAttributeDefinition(w http.ResponseWriter, r *http.Request) {
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
	definitionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || definitionID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user attribute definition id", requestID)
		return
	}
	var body updateUserAttributeDefinitionRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user attribute definition request", requestID)
		return
	}
	input := userattributescontract.UpdateDefinition{
		Name:         body.Name,
		Required:     body.Required,
		DisplayOrder: body.DisplayOrder,
		Enabled:      body.Enabled,
		Options:      body.Options,
	}
	if body.DataType != nil {
		dt := userattributescontract.DataType(*body.DataType)
		input.DataType = &dt
	}
	def, err := s.runtime.userAttributes.UpdateDefinition(r.Context(), definitionID, input)
	if err != nil {
		s.writeUserAttributeError(w, err, requestID, "invalid user attribute definition request")
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_attribute_definition.update", "user_attribute_definition", strconv.Itoa(def.ID), nil, map[string]any{
		"name":     def.Name,
		"enabled":  def.Enabled,
		"required": def.Required,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toUserAttributeDefinitionPayload(def),
		"request_id": requestID,
	})
}

func (s *Server) handleDeleteAdminUserAttributeDefinition(w http.ResponseWriter, r *http.Request) {
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
	definitionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || definitionID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user attribute definition id", requestID)
		return
	}
	if err := s.runtime.userAttributes.DeleteDefinition(r.Context(), definitionID); err != nil {
		s.writeUserAttributeError(w, err, requestID, "failed to delete user attribute definition")
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_attribute_definition.delete", "user_attribute_definition", strconv.Itoa(definitionID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": definitionID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminUserAttributeValues(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	defs, err := s.runtime.userAttributes.ListDefinitions(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list user attributes", requestID)
		return
	}
	values, err := s.runtime.userAttributes.ListUserValues(r.Context(), userID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list user attributes", requestID)
		return
	}
	valueByDefinition := make(map[int]userattributescontract.Value, len(values))
	for _, value := range values {
		valueByDefinition[value.DefinitionID] = value
	}
	data := make([]userAttributeValuePayload, 0, len(defs))
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		payload := userAttributeValuePayload{
			DefinitionID: def.ID,
			Key:          def.Key,
			Name:         def.Name,
			DataType:     string(def.DataType),
			Options:      attributeOptions(def.Options),
			Required:     def.Required,
		}
		if value, ok := valueByDefinition[def.ID]; ok {
			payload.Value = value.Value
			updatedAt := value.UpdatedAt.UTC()
			payload.UpdatedAt = &updatedAt
		}
		data = append(data, payload)
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleSetAdminUserAttributeValue(w http.ResponseWriter, r *http.Request) {
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
	userID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || userID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user id", requestID)
		return
	}
	definitionID, err := strconv.Atoi(r.PathValue("definitionId"))
	if err != nil || definitionID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid user attribute definition id", requestID)
		return
	}
	if _, err := s.runtime.users.FindByID(r.Context(), userID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "user not found", requestID)
		return
	}
	var body setUserAttributeValueRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid user attribute value request", requestID)
		return
	}
	value, err := s.runtime.userAttributes.SetUserValue(r.Context(), userattributescontract.SetValue{
		UserID:       userID,
		DefinitionID: definitionID,
		Value:        body.Value,
	})
	if err != nil {
		s.writeUserAttributeError(w, err, requestID, "invalid user attribute value request")
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "user_attribute_value.set", "user", strconv.Itoa(userID), nil, map[string]any{
		"definition_id": definitionID,
		"value":         value.Value,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"definition_id": value.DefinitionID,
			"value":         value.Value,
			"updated_at":    value.UpdatedAt.UTC(),
		},
		"request_id": requestID,
	})
}

func (s *Server) writeUserAttributeError(w http.ResponseWriter, err error, requestID, invalidMessage string) {
	switch {
	case errors.Is(err, userattributescontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "user attribute not found", requestID)
	case errors.Is(err, userattributescontract.ErrDuplicateKey):
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "user attribute key already exists", requestID)
	case errors.Is(err, userattributesservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, invalidMessage, requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process user attribute request", requestID)
	}
}
