package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type crsPreviewRequest struct {
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type crsPreviewAccount struct {
	CrsAccountID string `json:"crs_account_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	Type         string `json:"type"`
}

type crsPreviewResponse struct {
	NewAccounts      []crsPreviewAccount `json:"new_accounts"`
	ExistingAccounts []crsPreviewAccount `json:"existing_accounts"`
}

type crsSyncRequest struct {
	BaseURL            string   `json:"base_url"`
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	SyncProxies        bool     `json:"sync_proxies,omitempty"`
	SelectedAccountIDs []string `json:"selected_account_ids,omitempty"`
}

type crsSyncResultItem struct {
	CrsAccountID string `json:"crs_account_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Action       string `json:"action"`
	Error        string `json:"error,omitempty"`
}

type crsSyncResponse struct {
	Created int                 `json:"created"`
	Updated int                 `json:"updated"`
	Skipped int                 `json:"skipped"`
	Failed  int                 `json:"failed"`
	Items   []crsSyncResultItem `json:"items"`
}

func (s *Server) handleAdminCRSPreview(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}

	var body crsPreviewRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request", requestID)
		return
	}
	if body.BaseURL == "" || body.Username == "" || body.Password == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "base_url, username, and password are required", requestID)
		return
	}

	remoteAccounts, err := fetchCRSAccounts(body.BaseURL, body.Username, body.Password)
	if err != nil {
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "failed to fetch from CRS: "+err.Error(), requestID)
		return
	}

	existingAccounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	existingNames := make(map[string]bool, len(existingAccounts))
	for _, a := range existingAccounts {
		existingNames[a.Name] = true
	}

	result := crsPreviewResponse{
		NewAccounts:      []crsPreviewAccount{},
		ExistingAccounts: []crsPreviewAccount{},
	}
	for _, ra := range remoteAccounts {
		preview := crsPreviewAccount{
			CrsAccountID: ra.ID,
			Kind:         ra.Kind,
			Name:         ra.Name,
			Platform:     ra.Platform,
			Type:         ra.Type,
		}
		if existingNames[ra.Name] {
			result.ExistingAccounts = append(result.ExistingAccounts, preview)
		} else {
			result.NewAccounts = append(result.NewAccounts, preview)
		}
	}

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       result,
		"request_id": requestID,
	})
}

func (s *Server) handleAdminCRSSync(w http.ResponseWriter, r *http.Request) {
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

	var body crsSyncRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request", requestID)
		return
	}
	if body.BaseURL == "" || body.Username == "" || body.Password == "" {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "base_url, username, and password are required", requestID)
		return
	}

	remoteAccounts, err := fetchCRSAccounts(body.BaseURL, body.Username, body.Password)
	if err != nil {
		writeStandardError(w, http.StatusBadGateway, apiopenapi.INTERNALERROR, "failed to fetch from CRS: "+err.Error(), requestID)
		return
	}

	selected := make(map[string]bool, len(body.SelectedAccountIDs))
	for _, id := range body.SelectedAccountIDs {
		selected[id] = true
	}

	existingAccounts, _ := s.runtime.accounts.List(r.Context())
	existingByName := make(map[string]bool, len(existingAccounts))
	for _, a := range existingAccounts {
		existingByName[a.Name] = true
	}

	providers, _ := s.runtime.providers.List(r.Context())
	providerByKey := make(map[string]int, len(providers))
	for _, p := range providers {
		providerByKey[p.Name] = p.ID
	}

	result := crsSyncResponse{Items: []crsSyncResultItem{}}
	for _, ra := range remoteAccounts {
		if len(body.SelectedAccountIDs) > 0 && !selected[ra.ID] {
			result.Skipped++
			result.Items = append(result.Items, crsSyncResultItem{
				CrsAccountID: ra.ID, Kind: ra.Kind, Name: ra.Name,
				Action: "skipped",
			})
			continue
		}

		if existingByName[ra.Name] {
			result.Updated++
			result.Items = append(result.Items, crsSyncResultItem{
				CrsAccountID: ra.ID, Kind: ra.Kind, Name: ra.Name,
				Action: "updated",
			})
			continue
		}

		providerID := providerByKey[ra.Platform]
		if providerID == 0 {
			providerID = providerByKey["openai-compatible"]
		}
		if providerID == 0 && len(providers) > 0 {
			providerID = providers[0].ID
		}

		runtimeClass := accountcontract.RuntimeClassAPIKey
		credential := ra.Credential

		_, err := s.runtime.accounts.Create(r.Context(), accountcontract.CreateRequest{
			ProviderID:   providerID,
			Name:         ra.Name,
			RuntimeClass: runtimeClass,
			Credential:   credential,
			Metadata:     ra.Metadata,
		})
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, crsSyncResultItem{
				CrsAccountID: ra.ID, Kind: ra.Kind, Name: ra.Name,
				Action: "failed", Error: err.Error(),
			})
			continue
		}
		result.Created++
		result.Items = append(result.Items, crsSyncResultItem{
			CrsAccountID: ra.ID, Kind: ra.Kind, Name: ra.Name,
			Action: "created",
		})
	}

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "crs_sync", "provider_account", "bulk", nil, map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"skipped": result.Skipped,
		"failed":  result.Failed,
	}))

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       result,
		"request_id": requestID,
	})
}

type crsRemoteAccount struct {
	ID         string
	Kind       string
	Name       string
	Platform   string
	Type       string
	Credential map[string]any
	Metadata   map[string]any
}

func fetchCRSAccounts(baseURL, username, password string) ([]crsRemoteAccount, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	url := baseURL + "/api/accounts"

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to CRS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("CRS returned %d: %s", resp.StatusCode, string(body))
	}

	var rawAccounts []struct {
		ID         string         `json:"id"`
		Kind       string         `json:"kind"`
		Name       string         `json:"name"`
		Platform   string         `json:"platform"`
		Type       string         `json:"type"`
		Credential map[string]any `json:"credential"`
		Metadata   map[string]any `json:"metadata"`
		BaseURL    string         `json:"base_url"`
		APIKey     string         `json:"api_key"`
		Token      string         `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&rawAccounts); err != nil {
		return nil, fmt.Errorf("decode CRS response: %w", err)
	}

	accounts := make([]crsRemoteAccount, 0, len(rawAccounts))
	for _, ra := range rawAccounts {
		credential := ra.Credential
		if credential == nil {
			credential = make(map[string]any)
		}
		if ra.APIKey != "" {
			credential["api_key"] = ra.APIKey
		}
		if ra.Token != "" {
			credential["access_token"] = ra.Token
		}

		metadata := ra.Metadata
		if metadata == nil {
			metadata = make(map[string]any)
		}
		if ra.BaseURL != "" {
			metadata["base_url"] = ra.BaseURL
		}

		name := ra.Name
		if name == "" {
			name = fmt.Sprintf("crs-%s-%s", ra.Platform, ra.ID)
		}

		accounts = append(accounts, crsRemoteAccount{
			ID:         ra.ID,
			Kind:       ra.Kind,
			Name:       name,
			Platform:   ra.Platform,
			Type:       ra.Type,
			Credential: credential,
			Metadata:   metadata,
		})
	}
	return accounts, nil
}
