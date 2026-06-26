package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerpreset "github.com/srapi/srapi/apps/api/internal/modules/providers/preset"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// sessionImportClockSkew tolerates minor clock drift when validating token expiry.
const sessionImportClockSkew = 120 * time.Second

// sessionImportDefaultUpstreamClient is the upstream client tag that selects the codex_cli
// reverse-proxy runtime + the refresh-token-only credential path.
const sessionImportDefaultUpstreamClient = "codex_cli"

// sessionImportEntry is one parsed session blob with its 1-based position.
type sessionImportEntry struct {
	index int
	value any
}

// sessionImportAccount is the normalized identity + credential extracted from a
// single session blob, ready to become an upstream account.
type sessionImportAccount struct {
	name           string
	accessToken    string
	refreshToken   string
	idToken        string
	email          string
	accountID      string
	userID         string
	planType       string
	organizationID string
	baseURL        string
	tokenURL       string
	tokenExpiresAt *time.Time
	identityKeys   []string
	warnings       []string
}

// sessionImportJWTClaims captures the subset of the access/id-token JWT payload we read.
type sessionImportJWTClaims struct {
	Sub        string                      `json:"sub"`
	Email      string                      `json:"email"`
	Exp        int64                       `json:"exp"`
	OpenAIAuth *sessionImportJWTAuthClaim  `json:"https://api.openai.com/auth,omitempty"`
}

type sessionImportJWTAuthClaim struct {
	ChatGPTAccountID string                        `json:"chatgpt_account_id"`
	ChatGPTUserID    string                        `json:"chatgpt_user_id"`
	ChatGPTPlanType  string                        `json:"chatgpt_plan_type"`
	UserID           string                        `json:"user_id"`
	POID             string                        `json:"poid"`
	Organizations    []sessionImportJWTOrganization `json:"organizations"`
}

type sessionImportJWTOrganization struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"is_default"`
}

func (s *Server) handleImportAdminSession(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.SessionImportRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid session import request", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil || providerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider_id", requestID)
		return
	}
	provider, err := s.runtime.providers.FindByID(r.Context(), providerID)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider_id not found", requestID)
		return
	}
	entries, err := parseSessionImportEntries(body.Content)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, err.Error(), requestID)
		return
	}
	if len(entries) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "no access token or session content found", requestID)
		return
	}

	upstreamClient := sessionImportUpstreamClientForProvider(provider)
	result := s.importSessions(r.Context(), providerID, sessionImportDefaultBaseURL(provider), upstreamClient, body, entries)

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.session_import", "provider_account", "bulk", nil, map[string]any{
		"provider_id":   providerID,
		"total_count":   result.Total,
		"created_count": result.Created,
		"updated_count": result.Updated,
		"skipped_count": result.Skipped,
		"failed_count":  result.Failed,
		"warning_count": len(result.Warnings),
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.SessionImportResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) importSessions(ctx context.Context, providerID int, defaultBaseURL string, upstreamClient string, body apiopenapi.SessionImportRequest, entries []sessionImportEntry) apiopenapi.SessionImportResult {
	result := apiopenapi.SessionImportResult{
		Total:    len(entries),
		Items:    make([]apiopenapi.SessionImportItem, 0, len(entries)),
		Warnings: make([]apiopenapi.SessionImportMessage, 0),
		Errors:   make([]apiopenapi.SessionImportMessage, 0),
	}

	updateExisting := true
	if body.UpdateExisting != nil {
		updateExisting = *body.UpdateExisting
	}
	existing := s.buildSessionAccountIndex(ctx, providerID)
	groupIDs, _ := apiIDsToInts(body.GroupIds)
	baseName := ""
	if body.Name != nil {
		baseName = strings.TrimSpace(*body.Name)
	}
	requestStatus := toAccountStatusPtr(body.Status)

	provider, providerErr := s.runtime.providers.FindByID(ctx, providerID)
	if providerErr == nil {
		if catalog := sessionImportModelCatalog(provider); len(catalog) > 0 {
			s.quickMapModels(ctx, provider, catalog, nil)
		}
	}

	seen := map[string]int{}
	for _, entry := range entries {
		item, err := normalizeSessionImportEntry(entry)
		if err != nil {
			recordSessionFailure(&result, entry.index, "", err.Error())
			continue
		}
		accountName := buildSessionCreateAccountName(baseName, item, entry.index, len(entries))
		for _, warning := range item.warnings {
			result.Warnings = append(result.Warnings, apiopenapi.SessionImportMessage{
				Index: entry.index, Name: ptrString(accountName), Message: warning,
			})
		}

		if dup, ok := firstSeenImportIdentity(seen, item.identityKeys); ok {
			message := fmt.Sprintf("duplicate of import entry #%d; skipped", dup)
			result.Skipped++
			result.Items = append(result.Items, apiopenapi.SessionImportItem{
				Index: entry.index, Name: ptrString(accountName), Action: apiopenapi.SessionImportItemActionSkipped, Message: ptrString(message),
			})
			result.Warnings = append(result.Warnings, apiopenapi.SessionImportMessage{
				Index: entry.index, Name: ptrString(accountName), Message: message,
			})
			continue
		}
		markImportIdentitySeen(seen, item.identityKeys, entry.index)

		credential, metadata, status, expiryErr := s.resolveSessionImportTarget(item, defaultBaseURL, requestStatus)
		if expiryErr != nil {
			recordSessionFailure(&result, entry.index, accountName, expiryErr.Error())
			continue
		}

		if existingID, ok := existing.find(item.identityKeys); ok {
			if !updateExisting {
				message := "matching account already exists; skipped"
				result.Skipped++
				result.Items = append(result.Items, apiopenapi.SessionImportItem{
					Index: entry.index, Name: ptrString(accountName), Action: apiopenapi.SessionImportItemActionSkipped, AccountId: idPtr(existingID), Message: ptrString(message),
				})
				continue
			}
			s.applySessionUpdate(ctx, &result, entry.index, accountName, existingID, credential, metadata, status, body.ProxyId, upstreamClient)
			continue
		}

		s.applySessionCreate(ctx, &result, entry.index, accountName, providerID, credential, metadata, status, body.ProxyId, groupIDs, existing, item, upstreamClient)
	}

	return result
}

func (s *Server) applySessionCreate(ctx context.Context, result *apiopenapi.SessionImportResult, index int, name string, providerID int, credential, metadata map[string]any, status *accountcontract.Status, proxyID *string, groupIDs []int, existing *sessionAccountIndex, item *sessionImportAccount, upstreamClient string) {
	refreshed, err := s.refreshImportCredential(ctx, accountcontract.RuntimeClassOauthRefresh, ptrString(upstreamClient), metadata, proxyID, credential)
	if err != nil {
		recordSessionFailure(result, index, name, "oauth refresh failed")
		return
	}
	account, err := s.runtime.accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:     providerID,
		Name:           name,
		RuntimeClass:   accountcontract.RuntimeClassOauthRefresh,
		Credential:     refreshed,
		Metadata:       metadata,
		ProxyID:        proxyID,
		Status:         status,
		UpstreamClient: ptrString(upstreamClient),
	})
	if err != nil {
		recordSessionFailure(result, index, name, "create failed")
		return
	}
	existing.add(account.ID, item.identityKeys)
	for _, groupID := range groupIDs {
		if _, err := s.runtime.accounts.AddAccountToGroup(ctx, account.ID, groupID); err != nil {
			result.Warnings = append(result.Warnings, apiopenapi.SessionImportMessage{
				Index: index, Name: ptrString(name), Message: fmt.Sprintf("failed to bind group %d", groupID),
			})
		}
	}
	result.Created++
	result.Items = append(result.Items, apiopenapi.SessionImportItem{
		Index: index, Name: ptrString(name), Action: apiopenapi.SessionImportItemActionCreated, AccountId: idPtr(account.ID),
	})
}

func (s *Server) applySessionUpdate(ctx context.Context, result *apiopenapi.SessionImportResult, index int, name string, accountID int, credential, metadata map[string]any, status *accountcontract.Status, proxyID *string, upstreamClient string) {
	refreshed, err := s.refreshImportCredential(ctx, accountcontract.RuntimeClassOauthRefresh, ptrString(upstreamClient), metadata, proxyID, credential)
	if err != nil {
		recordSessionFailure(result, index, name, "oauth refresh failed")
		return
	}
	credentialCopy := refreshed
	metadataCopy := metadata
	update := accountcontract.UpdateRequest{
		Credential: &credentialCopy,
		Metadata:   &metadataCopy,
		Status:     status,
	}
	if proxyID != nil {
		proxyPtr := proxyID
		update.ProxyID = &proxyPtr
	}
	if _, err := s.runtime.accounts.Update(ctx, accountID, update); err != nil {
		recordSessionFailure(result, index, name, "update failed")
		return
	}
	result.Updated++
	result.Items = append(result.Items, apiopenapi.SessionImportItem{
		Index: index, Name: ptrString(name), Action: apiopenapi.SessionImportItemActionUpdated, AccountId: idPtr(accountID),
	})
}

// resolveSessionImportTarget builds the credential + metadata maps and resolves
// the account status. Refresh-token-less sessions require a valid (non-expired)
// access token and are marked for auto-pause-on-expiry.
func (s *Server) resolveSessionImportTarget(item *sessionImportAccount, defaultBaseURL string, requestStatus *accountcontract.Status) (map[string]any, map[string]any, *accountcontract.Status, error) {
	// Keep the imported access token when present so the import never depends on a
	// blocking/failing OAuth refresh at import time: an unreachable auth
	// endpoint would otherwise make every account fail ("oauth refresh failed").
	// The runtime refreshes lazily via the refresh_token when the access token
	// nears expiry, which is the normal OauthRefresh behavior.
	credential := map[string]any{}
	if item.accessToken != "" {
		credential["access_token"] = item.accessToken
	}
	if item.refreshToken != "" {
		credential["refresh_token"] = item.refreshToken
	}
	if item.idToken != "" {
		credential["id_token"] = item.idToken
	}
	setSessionCredentialIfNotEmpty(credential, "email", item.email)
	setSessionCredentialIfNotEmpty(credential, "chatgpt_account_id", item.accountID)
	setSessionCredentialIfNotEmpty(credential, "chatgpt_user_id", item.userID)
	setSessionCredentialIfNotEmpty(credential, "organization_id", item.organizationID)
	setSessionCredentialIfNotEmpty(credential, "plan_type", item.planType)
	if item.tokenExpiresAt != nil {
		credential["expires_at"] = item.tokenExpiresAt.UTC().Format(time.RFC3339)
	}

	metadata := map[string]any{
		"import_source": "session_import",
		"imported_at":   time.Now().UTC().Format(time.RFC3339),
	}
	setSessionMetadataIfNotEmpty(metadata, "email", item.email)
	setSessionMetadataIfNotEmpty(metadata, "upstream_account_id", item.accountID)
	setSessionMetadataIfNotEmpty(metadata, "upstream_user_id", item.userID)
	setSessionMetadataIfNotEmpty(metadata, "plan_type", item.planType)
	setSessionMetadataIfNotEmpty(metadata, "organization_id", item.organizationID)
	// Upstream-endpoint hints. Prefer a base_url carried by the session blob
	// (e.g. a desktop session pointed at a proxy); otherwise seed the
	// provider/preset default. The reverse-proxy adapter has NO implicit
	// default and hard-fails every request with "reverse proxy upstream base url
	// missing" when this key is absent, so an imported account without it is dead
	// on arrival.
	setSessionMetadataIfNotEmpty(metadata, "base_url", item.baseURL)
	if mapString(metadata, "base_url") == "" {
		setSessionMetadataIfNotEmpty(metadata, "base_url", defaultBaseURL)
	}
	setSessionMetadataIfNotEmpty(metadata, "oauth_token_url", item.tokenURL)

	status := requestStatus
	if item.refreshToken == "" {
		if item.tokenExpiresAt == nil {
			return nil, nil, nil, errors.New("session has no refresh_token and no parseable access_token expiry; cannot import")
		}
		if item.tokenExpiresAt.Add(sessionImportClockSkew).Before(time.Now().UTC()) {
			return nil, nil, nil, fmt.Errorf("access_token already expired at %s", item.tokenExpiresAt.UTC().Format(time.RFC3339))
		}
		metadata["auto_pause_on_expired"] = true
		metadata["token_expires_at"] = item.tokenExpiresAt.UTC().Format(time.RFC3339)
	}
	return credential, metadata, status, nil
}

// --- parsing ---------------------------------------------------------------

func parseSessionImportEntries(content string) ([]sessionImportEntry, error) {
	values, err := parseSessionImportContent(content)
	if err != nil {
		return nil, err
	}
	entries := make([]sessionImportEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, sessionImportEntry{index: len(entries) + 1, value: value})
	}
	return entries, nil
}

func parseSessionImportContent(content string) ([]any, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, nil
	}
	if sessionImportLooksLikeJSON(trimmed) {
		values, err := decodeSessionImportJSONStream(trimmed)
		if err != nil {
			if strings.Contains(trimmed, "\n") {
				if lineValues, lineErr := parseSessionImportLines(trimmed); lineErr == nil {
					return lineValues, nil
				}
			}
			return nil, fmt.Errorf("failed to parse session JSON: %w", err)
		}
		return flattenSessionImportValues(values), nil
	}
	return parseSessionImportLines(trimmed)
}

func parseSessionImportLines(content string) ([]any, error) {
	values := make([]any, 0)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if sessionImportLooksLikeJSON(line) {
			lineValues, err := decodeSessionImportJSONStream(line)
			if err != nil {
				return nil, fmt.Errorf("failed to parse JSON on line %d: %w", len(values)+1, err)
			}
			values = append(values, flattenSessionImportValues(lineValues)...)
			continue
		}
		values = append(values, line)
	}
	return values, nil
}

func decodeSessionImportJSONStream(content string) ([]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	values := make([]any, 0, 1)
	for {
		var value any
		err := decoder.Decode(&value)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("empty JSON content")
	}
	return values, nil
}

func flattenSessionImportValues(values []any) []any {
	out := make([]any, 0, len(values))
	var appendValue func(any)
	appendValue = func(value any) {
		if arr, ok := value.([]any); ok {
			for _, item := range arr {
				appendValue(item)
			}
			return
		}
		if obj, ok := value.(map[string]any); ok {
			if inner, ok := sessionImportEnvelopeArray(obj); ok {
				for _, item := range inner {
					appendValue(item)
				}
				return
			}
		}
		out = append(out, value)
	}
	for _, value := range values {
		appendValue(value)
	}
	return out
}

// sessionImportEnvelopeArray detects an export wrapper such as
// {exported_at, proxies, accounts:[...]} (or sessions/items) and returns the
// inner array so each element becomes its own session entry. It only unwraps
// when the object does NOT itself carry a token field, so a genuine single
// session that merely happens to have an unrelated array key is never split.
func sessionImportEnvelopeArray(obj map[string]any) ([]any, bool) {
	for _, key := range []string{"access_token", "accessToken", "token", "refresh_token", "id_token", "tokens", "credentials"} {
		if _, ok := obj[key]; ok {
			return nil, false
		}
	}
	for _, key := range []string{"accounts", "sessions", "items"} {
		if arr, ok := obj[key].([]any); ok && len(arr) > 0 {
			return arr, true
		}
	}
	return nil, false
}

func sessionImportLooksLikeJSON(content string) bool {
	if content == "" {
		return false
	}
	switch content[0] {
	case '{', '[':
		return true
	default:
		return false
	}
}

// --- normalization ---------------------------------------------------------

func normalizeSessionImportEntry(entry sessionImportEntry) (*sessionImportAccount, error) {
	now := time.Now().UTC()
	item := &sessionImportAccount{}

	switch raw := entry.value.(type) {
	case string:
		item.accessToken = strings.TrimSpace(raw)
	case map[string]any:
		item.accessToken = firstSessionImportString(raw,
			[]string{"tokens", "access_token"}, []string{"tokens", "accessToken"},
			[]string{"credentials", "access_token"}, []string{"credentials", "accessToken"},
			[]string{"access_token"}, []string{"accessToken"}, []string{"token"})
		item.refreshToken = firstSessionImportString(raw,
			[]string{"tokens", "refresh_token"}, []string{"tokens", "refreshToken"},
			[]string{"credentials", "refresh_token"}, []string{"credentials", "refreshToken"},
			[]string{"refresh_token"}, []string{"refreshToken"})
		item.idToken = firstSessionImportString(raw,
			[]string{"tokens", "id_token"}, []string{"tokens", "idToken"},
			[]string{"credentials", "id_token"}, []string{"credentials", "idToken"},
			[]string{"id_token"}, []string{"idToken"})
		item.email = firstSessionImportString(raw, []string{"email"}, []string{"credentials", "email"}, []string{"user", "email"})
		item.accountID = firstSessionImportString(raw,
			[]string{"chatgpt_account_id"}, []string{"chatgptAccountId"},
			[]string{"credentials", "chatgpt_account_id"}, []string{"credentials", "chatgptAccountId"},
			[]string{"account_id"}, []string{"accountId"}, []string{"account", "id"},
			[]string{"account", "account_id"}, []string{"account", "chatgpt_account_id"})
		item.userID = firstSessionImportString(raw,
			[]string{"chatgpt_user_id"}, []string{"chatgptUserId"},
			[]string{"credentials", "chatgpt_user_id"}, []string{"credentials", "chatgptUserId"},
			[]string{"user_id"}, []string{"userId"}, []string{"user", "id"})
		item.planType = firstSessionImportString(raw,
			[]string{"plan_type"}, []string{"planType"},
			[]string{"account", "plan_type"}, []string{"account", "planType"})
		item.organizationID = firstSessionImportString(raw,
			[]string{"organization_id"}, []string{"organizationId"},
			[]string{"org_id"}, []string{"orgId"})
		item.name = firstSessionImportString(raw, []string{"name"}, []string{"user", "name"})
		item.baseURL = firstSessionImportString(raw, []string{"base_url"}, []string{"baseUrl"})
		item.tokenURL = firstSessionImportString(raw, []string{"oauth_token_url"}, []string{"token_url"}, []string{"tokenUrl"})
		if expiresAt, ok := firstSessionImportTime(raw,
			[]string{"tokens", "expires_at"}, []string{"tokens", "expiresAt"},
			[]string{"credentials", "expires_at"}, []string{"credentials", "expiresAt"},
			[]string{"expires_at"}, []string{"expiresAt"}); ok {
			expiresAt := expiresAt.UTC()
			item.tokenExpiresAt = &expiresAt
		}
	default:
		return nil, fmt.Errorf("entry #%d has unsupported format", entry.index)
	}

	if item.accessToken == "" {
		return nil, errors.New("missing access_token")
	}
	if item.idToken != "" {
		_ = enrichSessionImportFromJWT(item, item.idToken, false, now)
	}
	if err := enrichSessionImportFromJWT(item, item.accessToken, true, now); err != nil {
		return nil, err
	}
	if item.tokenExpiresAt == nil {
		item.warnings = append(item.warnings, "could not parse access_token expiry; verify token validity after import")
	}
	if item.refreshToken == "" {
		item.warnings = append(item.warnings, "session has no refresh_token; access_token cannot auto-renew after expiry")
	}

	item.identityKeys = buildSessionIdentityKeys(item.accountID, item.userID, item.email, item.accessToken)
	item.name = buildSessionImportAccountName(item, entry.index)
	return item, nil
}

func enrichSessionImportFromJWT(item *sessionImportAccount, token string, validateExpiry bool, now time.Time) error {
	claims, err := decodeSessionImportJWTClaims(token)
	if err != nil {
		if validateExpiry {
			item.warnings = append(item.warnings, "access_token is not a parseable JWT; cannot validate expiry or identity")
		}
		return nil
	}
	if validateExpiry && claims.Exp > 0 {
		if now.Unix() > claims.Exp+int64(sessionImportClockSkew.Seconds()) {
			return fmt.Errorf("access_token already expired at %s", time.Unix(claims.Exp, 0).UTC().Format(time.RFC3339))
		}
		expiresAt := time.Unix(claims.Exp, 0).UTC()
		item.tokenExpiresAt = &expiresAt
	}
	if item.email == "" {
		item.email = strings.TrimSpace(claims.Email)
	}
	if claims.OpenAIAuth == nil {
		if item.userID == "" {
			item.userID = strings.TrimSpace(claims.Sub)
		}
		return nil
	}
	auth := claims.OpenAIAuth
	if item.accountID == "" {
		item.accountID = strings.TrimSpace(auth.ChatGPTAccountID)
	}
	if item.userID == "" {
		item.userID = strings.TrimSpace(auth.ChatGPTUserID)
	}
	if item.userID == "" {
		item.userID = strings.TrimSpace(auth.UserID)
	}
	if item.planType == "" {
		item.planType = strings.TrimSpace(auth.ChatGPTPlanType)
	}
	if item.organizationID == "" {
		item.organizationID = strings.TrimSpace(auth.POID)
	}
	if item.organizationID == "" {
		for _, org := range auth.Organizations {
			if org.IsDefault {
				item.organizationID = strings.TrimSpace(org.ID)
				break
			}
		}
	}
	if item.organizationID == "" && len(auth.Organizations) > 0 {
		item.organizationID = strings.TrimSpace(auth.Organizations[0].ID)
	}
	if item.userID == "" {
		item.userID = strings.TrimSpace(claims.Sub)
	}
	return nil
}

func decodeSessionImportJWTClaims(token string) (*sessionImportJWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}
	payload, err := decodeSessionImportJWTSegment(parts[1])
	if err != nil {
		return nil, err
	}
	var claims sessionImportJWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func decodeSessionImportJWTSegment(segment string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(segment); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(segment); err == nil {
		return decoded, nil
	}
	padded := segment
	if rem := len(padded) % 4; rem > 0 {
		padded += strings.Repeat("=", 4-rem)
	}
	if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(padded)
}

// --- naming + identity -----------------------------------------------------

func buildSessionImportAccountName(item *sessionImportAccount, index int) string {
	for _, candidate := range []string{item.name, item.email, item.accountID, item.userID} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return fmt.Sprintf("Session import %d", index)
}

func buildSessionCreateAccountName(base string, item *sessionImportAccount, index, total int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return item.name
	}
	if total > 1 {
		return fmt.Sprintf("%s #%d", base, index)
	}
	return base
}

// --- existing-account index (plaintext metadata only) ----------------------

type sessionAccountIndex = importIdentityIndex

func (s *Server) buildSessionAccountIndex(ctx context.Context, providerID int) *sessionAccountIndex {
	index := newImportIdentityIndex()
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return index
	}
	for _, account := range accounts {
		if account.ProviderID != providerID {
			continue
		}
		keys := buildSessionIdentityKeys(
			mapString(account.Metadata, "upstream_account_id"),
			mapString(account.Metadata, "upstream_user_id"),
			mapString(account.Metadata, "email"),
			"",
		)
		index.add(account.ID, keys)
	}
	return index
}

// --- small helpers ---------------------------------------------------------

func recordSessionFailure(result *apiopenapi.SessionImportResult, index int, name, message string) {
	result.Failed++
	var namePtr *string
	if strings.TrimSpace(name) != "" {
		namePtr = ptrString(name)
	}
	result.Items = append(result.Items, apiopenapi.SessionImportItem{
		Index: index, Name: namePtr, Action: apiopenapi.SessionImportItemActionFailed, Message: ptrString(message),
	})
	result.Errors = append(result.Errors, apiopenapi.SessionImportMessage{
		Index: index, Name: namePtr, Message: message,
	})
}

// sessionImportDefaultBaseURL resolves the upstream base URL to seed onto imported
// accounts when the session blob carries none. It prefers the provider's
// own configured base_url (present when the provider was installed from a
// preset), then falls back to the built-in codex-cli preset default. Without
// this seed the reverse-proxy adapter rejects every request with "reverse
// proxy upstream base url missing", so the import would silently create dead
// accounts.
func sessionImportUpstreamClientForProvider(provider providercontract.Provider) string {
	if at, ok := provider.ConfigSchema["account_template"].(map[string]any); ok {
		if uc, ok := at["upstream_client"].(string); ok && strings.TrimSpace(uc) != "" {
			return strings.TrimSpace(uc)
		}
	}
	if strings.Contains(strings.ToLower(provider.AdapterType), "chatgpt-web") {
		return "chatgpt_web"
	}
	return sessionImportDefaultUpstreamClient
}

func sessionImportDefaultBaseURL(provider providercontract.Provider) string {
	if bu := mapString(provider.ConfigSchema, "base_url"); bu != "" {
		return bu
	}
	preset, ok := providerpreset.Default().Lookup("codex-cli")
	if !ok {
		return ""
	}
	if preset.AccountTemplate != nil {
		if bu := mapString(preset.AccountTemplate.DefaultMetadata, "base_url"); bu != "" {
			return bu
		}
	}
	return preset.DefaultBaseURL
}

func sessionImportModelCatalog(provider providercontract.Provider) []string {
	at, ok := provider.ConfigSchema["account_template"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := at["model_catalog"].([]any)
	if !ok {
		return nil
	}
	catalog := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			catalog = append(catalog, strings.TrimSpace(s))
		}
	}
	return catalog
}

func setSessionCredentialIfNotEmpty(credential map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		credential[key] = value
	}
}

func setSessionMetadataIfNotEmpty(metadata map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		metadata[key] = value
	}
}

func idPtr(id int) *apiopenapi.Id {
	value := apiopenapi.Id(strconv.Itoa(id))
	return &value
}

func firstSessionImportString(obj map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if value, ok := sessionImportPathValue(obj, path); ok {
			if str := sessionImportStringValue(value); str != "" {
				return str
			}
		}
	}
	return ""
}

func firstSessionImportTime(obj map[string]any, paths ...[]string) (time.Time, bool) {
	for _, path := range paths {
		if value, ok := sessionImportPathValue(obj, path); ok {
			if parsed, ok := parseSessionImportTimeValue(value); ok {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func sessionImportPathValue(obj map[string]any, path []string) (any, bool) {
	var current any = obj
	for _, key := range path {
		currentObj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := currentObj[key]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func sessionImportStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func parseSessionImportTimeValue(value any) (time.Time, bool) {
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return parsed.UTC(), true
		}
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return sessionImportUnixTime(n), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return sessionImportUnixTime(n), true
		}
		if f, err := v.Float64(); err == nil {
			return sessionImportUnixTime(int64(f)), true
		}
	case float64:
		return sessionImportUnixTime(int64(v)), true
	case int64:
		return sessionImportUnixTime(v), true
	}
	return time.Time{}, false
}

func sessionImportUnixTime(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}
