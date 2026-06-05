package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// codexImportClockSkew tolerates minor clock drift when validating token expiry.
const codexImportClockSkew = 120 * time.Second

// codexImportUpstreamClient is the upstream client tag that selects the codex_cli
// reverse-proxy runtime + the refresh-token-only credential path.
const codexImportUpstreamClient = "codex_cli"

// codexImportEntry is one parsed session blob with its 1-based position.
type codexImportEntry struct {
	index int
	value any
}

// codexImportAccount is the normalized identity + credential extracted from a
// single session blob, ready to become a codex_cli account.
type codexImportAccount struct {
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

// codexJWTClaims captures the subset of the access/id-token JWT payload we read.
type codexJWTClaims struct {
	Sub        string             `json:"sub"`
	Email      string             `json:"email"`
	Exp        int64              `json:"exp"`
	OpenAIAuth *codexJWTAuthClaim `json:"https://api.openai.com/auth,omitempty"`
}

type codexJWTAuthClaim struct {
	ChatGPTAccountID string                 `json:"chatgpt_account_id"`
	ChatGPTUserID    string                 `json:"chatgpt_user_id"`
	ChatGPTPlanType  string                 `json:"chatgpt_plan_type"`
	UserID           string                 `json:"user_id"`
	POID             string                 `json:"poid"`
	Organizations    []codexJWTOrganization `json:"organizations"`
}

type codexJWTOrganization struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"is_default"`
}

func (s *Server) handleImportAdminCodexSession(w http.ResponseWriter, r *http.Request) {
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
	var body apiopenapi.CodexSessionImportRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid codex session import request", requestID)
		return
	}
	providerID, err := strconv.Atoi(string(body.ProviderId))
	if err != nil || providerID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid provider_id", requestID)
		return
	}
	if _, err := s.runtime.providers.FindByID(r.Context(), providerID); err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "provider_id not found", requestID)
		return
	}
	entries, err := parseCodexSessionImportEntries(body.Content)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, err.Error(), requestID)
		return
	}
	if len(entries) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "no access token or codex session content found", requestID)
		return
	}

	result := s.importCodexSessions(r.Context(), providerID, body, entries)

	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "provider_account.import_codex_session", "provider_account", "bulk", nil, map[string]any{
		"provider_id":   providerID,
		"total_count":   result.Total,
		"created_count": result.Created,
		"updated_count": result.Updated,
		"skipped_count": result.Skipped,
		"failed_count":  result.Failed,
		"warning_count": len(result.Warnings),
	}))
	writeJSONAny(w, http.StatusOK, apiopenapi.CodexSessionImportResponse{
		Data:      result,
		RequestId: requestID,
	})
}

func (s *Server) importCodexSessions(ctx context.Context, providerID int, body apiopenapi.CodexSessionImportRequest, entries []codexImportEntry) apiopenapi.CodexSessionImportResult {
	result := apiopenapi.CodexSessionImportResult{
		Total:    len(entries),
		Items:    make([]apiopenapi.CodexSessionImportItem, 0, len(entries)),
		Warnings: make([]apiopenapi.CodexSessionImportMessage, 0),
		Errors:   make([]apiopenapi.CodexSessionImportMessage, 0),
	}

	updateExisting := true
	if body.UpdateExisting != nil {
		updateExisting = *body.UpdateExisting
	}
	existing := s.buildCodexAccountIndex(ctx, providerID)
	groupIDs, _ := apiIDsToInts(body.GroupIds)
	baseName := ""
	if body.Name != nil {
		baseName = strings.TrimSpace(*body.Name)
	}
	requestStatus := toAccountStatusPtr(body.Status)

	seen := map[string]int{}
	for _, entry := range entries {
		item, err := normalizeCodexImportEntry(entry)
		if err != nil {
			recordCodexFailure(&result, entry.index, "", err.Error())
			continue
		}
		accountName := buildCodexCreateAccountName(baseName, item, entry.index, len(entries))
		for _, warning := range item.warnings {
			result.Warnings = append(result.Warnings, apiopenapi.CodexSessionImportMessage{
				Index: entry.index, Name: ptrString(accountName), Message: warning,
			})
		}

		if dup, ok := firstSeenCodexIdentity(seen, item.identityKeys); ok {
			message := fmt.Sprintf("duplicate of import entry #%d; skipped", dup)
			result.Skipped++
			result.Items = append(result.Items, apiopenapi.CodexSessionImportItem{
				Index: entry.index, Name: ptrString(accountName), Action: apiopenapi.CodexSessionImportItemActionSkipped, Message: ptrString(message),
			})
			result.Warnings = append(result.Warnings, apiopenapi.CodexSessionImportMessage{
				Index: entry.index, Name: ptrString(accountName), Message: message,
			})
			continue
		}
		markCodexIdentitySeen(seen, item.identityKeys, entry.index)

		credential, metadata, status, expiryErr := s.resolveCodexImportTarget(item, requestStatus)
		if expiryErr != nil {
			recordCodexFailure(&result, entry.index, accountName, expiryErr.Error())
			continue
		}

		if existingID, ok := existing.find(item.identityKeys); ok {
			if !updateExisting {
				message := "matching account already exists; skipped"
				result.Skipped++
				result.Items = append(result.Items, apiopenapi.CodexSessionImportItem{
					Index: entry.index, Name: ptrString(accountName), Action: apiopenapi.CodexSessionImportItemActionSkipped, AccountId: idPtr(existingID), Message: ptrString(message),
				})
				continue
			}
			s.applyCodexUpdate(ctx, &result, entry.index, accountName, existingID, credential, metadata, status, body.ProxyId)
			continue
		}

		s.applyCodexCreate(ctx, &result, entry.index, accountName, providerID, credential, metadata, status, body.ProxyId, groupIDs, existing, item)
	}

	return result
}

func (s *Server) applyCodexCreate(ctx context.Context, result *apiopenapi.CodexSessionImportResult, index int, name string, providerID int, credential, metadata map[string]any, status *accountcontract.Status, proxyID *string, groupIDs []int, existing *codexAccountIndex, item *codexImportAccount) {
	refreshed, err := s.refreshImportCredential(ctx, accountcontract.RuntimeClassOauthRefresh, ptrString(codexImportUpstreamClient), metadata, proxyID, credential)
	if err != nil {
		recordCodexFailure(result, index, name, "oauth refresh failed")
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
		UpstreamClient: ptrString(codexImportUpstreamClient),
	})
	if err != nil {
		recordCodexFailure(result, index, name, "create failed")
		return
	}
	existing.add(account.ID, item.identityKeys)
	for _, groupID := range groupIDs {
		if _, err := s.runtime.accounts.AddAccountToGroup(ctx, account.ID, groupID); err != nil {
			result.Warnings = append(result.Warnings, apiopenapi.CodexSessionImportMessage{
				Index: index, Name: ptrString(name), Message: fmt.Sprintf("failed to bind group %d", groupID),
			})
		}
	}
	result.Created++
	result.Items = append(result.Items, apiopenapi.CodexSessionImportItem{
		Index: index, Name: ptrString(name), Action: apiopenapi.CodexSessionImportItemActionCreated, AccountId: idPtr(account.ID),
	})
}

func (s *Server) applyCodexUpdate(ctx context.Context, result *apiopenapi.CodexSessionImportResult, index int, name string, accountID int, credential, metadata map[string]any, status *accountcontract.Status, proxyID *string) {
	refreshed, err := s.refreshImportCredential(ctx, accountcontract.RuntimeClassOauthRefresh, ptrString(codexImportUpstreamClient), metadata, proxyID, credential)
	if err != nil {
		recordCodexFailure(result, index, name, "oauth refresh failed")
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
		recordCodexFailure(result, index, name, "update failed")
		return
	}
	result.Updated++
	result.Items = append(result.Items, apiopenapi.CodexSessionImportItem{
		Index: index, Name: ptrString(name), Action: apiopenapi.CodexSessionImportItemActionUpdated, AccountId: idPtr(accountID),
	})
}

// resolveCodexImportTarget builds the credential + metadata maps and resolves
// the account status. Refresh-token-less sessions require a valid (non-expired)
// access token and are marked for auto-pause-on-expiry.
func (s *Server) resolveCodexImportTarget(item *codexImportAccount, requestStatus *accountcontract.Status) (map[string]any, map[string]any, *accountcontract.Status, error) {
	credential := map[string]any{"access_token": item.accessToken}
	if item.refreshToken != "" {
		credential["refresh_token"] = item.refreshToken
		// Send refresh-token-only so refreshImportCredential mints a fresh
		// access token via the codex_cli reverse-proxy.
		delete(credential, "access_token")
	}
	if item.idToken != "" {
		credential["id_token"] = item.idToken
	}
	setCodexCredentialIfNotEmpty(credential, "email", item.email)
	setCodexCredentialIfNotEmpty(credential, "chatgpt_account_id", item.accountID)
	setCodexCredentialIfNotEmpty(credential, "chatgpt_user_id", item.userID)
	setCodexCredentialIfNotEmpty(credential, "organization_id", item.organizationID)
	setCodexCredentialIfNotEmpty(credential, "plan_type", item.planType)
	if item.tokenExpiresAt != nil {
		credential["expires_at"] = item.tokenExpiresAt.UTC().Format(time.RFC3339)
	}

	metadata := map[string]any{
		"import_source": "codex_session",
		"imported_at":   time.Now().UTC().Format(time.RFC3339),
	}
	setCodexMetadataIfNotEmpty(metadata, "codex_email", item.email)
	setCodexMetadataIfNotEmpty(metadata, "codex_account_id", item.accountID)
	setCodexMetadataIfNotEmpty(metadata, "codex_user_id", item.userID)
	setCodexMetadataIfNotEmpty(metadata, "codex_plan_type", item.planType)
	setCodexMetadataIfNotEmpty(metadata, "codex_organization_id", item.organizationID)
	// Optional upstream-endpoint hints (e.g. a desktop session pointed at a
	// proxy). When absent, the codex_cli runtime falls back to its defaults.
	setCodexMetadataIfNotEmpty(metadata, "base_url", item.baseURL)
	setCodexMetadataIfNotEmpty(metadata, "oauth_token_url", item.tokenURL)

	status := requestStatus
	if item.refreshToken == "" {
		if item.tokenExpiresAt == nil {
			return nil, nil, nil, errors.New("session has no refresh_token and no parseable access_token expiry; cannot import")
		}
		if item.tokenExpiresAt.Add(codexImportClockSkew).Before(time.Now().UTC()) {
			return nil, nil, nil, fmt.Errorf("access_token already expired at %s", item.tokenExpiresAt.UTC().Format(time.RFC3339))
		}
		metadata["auto_pause_on_expired"] = true
		metadata["token_expires_at"] = item.tokenExpiresAt.UTC().Format(time.RFC3339)
	}
	return credential, metadata, status, nil
}

// --- parsing ---------------------------------------------------------------

func parseCodexSessionImportEntries(content string) ([]codexImportEntry, error) {
	values, err := parseCodexSessionImportContent(content)
	if err != nil {
		return nil, err
	}
	entries := make([]codexImportEntry, 0, len(values))
	for _, value := range values {
		entries = append(entries, codexImportEntry{index: len(entries) + 1, value: value})
	}
	return entries, nil
}

func parseCodexSessionImportContent(content string) ([]any, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, nil
	}
	if codexLooksLikeJSON(trimmed) {
		values, err := decodeCodexJSONStream(trimmed)
		if err != nil {
			if strings.Contains(trimmed, "\n") {
				if lineValues, lineErr := parseCodexSessionImportLines(trimmed); lineErr == nil {
					return lineValues, nil
				}
			}
			return nil, fmt.Errorf("failed to parse session JSON: %w", err)
		}
		return flattenCodexImportValues(values), nil
	}
	return parseCodexSessionImportLines(trimmed)
}

func parseCodexSessionImportLines(content string) ([]any, error) {
	values := make([]any, 0)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if codexLooksLikeJSON(line) {
			lineValues, err := decodeCodexJSONStream(line)
			if err != nil {
				return nil, fmt.Errorf("failed to parse JSON on line %d: %w", len(values)+1, err)
			}
			values = append(values, flattenCodexImportValues(lineValues)...)
			continue
		}
		values = append(values, line)
	}
	return values, nil
}

func decodeCodexJSONStream(content string) ([]any, error) {
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

func flattenCodexImportValues(values []any) []any {
	out := make([]any, 0, len(values))
	var appendValue func(any)
	appendValue = func(value any) {
		if arr, ok := value.([]any); ok {
			for _, item := range arr {
				appendValue(item)
			}
			return
		}
		out = append(out, value)
	}
	for _, value := range values {
		appendValue(value)
	}
	return out
}

func codexLooksLikeJSON(content string) bool {
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

func normalizeCodexImportEntry(entry codexImportEntry) (*codexImportAccount, error) {
	now := time.Now().UTC()
	item := &codexImportAccount{}

	switch raw := entry.value.(type) {
	case string:
		item.accessToken = strings.TrimSpace(raw)
	case map[string]any:
		item.accessToken = firstCodexString(raw,
			[]string{"tokens", "access_token"}, []string{"tokens", "accessToken"},
			[]string{"access_token"}, []string{"accessToken"}, []string{"token"})
		item.refreshToken = firstCodexString(raw,
			[]string{"tokens", "refresh_token"}, []string{"tokens", "refreshToken"},
			[]string{"refresh_token"}, []string{"refreshToken"})
		item.idToken = firstCodexString(raw,
			[]string{"tokens", "id_token"}, []string{"tokens", "idToken"},
			[]string{"id_token"}, []string{"idToken"})
		item.email = firstCodexString(raw, []string{"email"}, []string{"user", "email"})
		item.accountID = firstCodexString(raw,
			[]string{"chatgpt_account_id"}, []string{"chatgptAccountId"},
			[]string{"account_id"}, []string{"accountId"}, []string{"account", "id"},
			[]string{"account", "account_id"}, []string{"account", "chatgpt_account_id"})
		item.userID = firstCodexString(raw,
			[]string{"chatgpt_user_id"}, []string{"chatgptUserId"},
			[]string{"user_id"}, []string{"userId"}, []string{"user", "id"})
		item.planType = firstCodexString(raw,
			[]string{"plan_type"}, []string{"planType"},
			[]string{"account", "plan_type"}, []string{"account", "planType"})
		item.organizationID = firstCodexString(raw,
			[]string{"organization_id"}, []string{"organizationId"},
			[]string{"org_id"}, []string{"orgId"})
		item.name = firstCodexString(raw, []string{"name"}, []string{"user", "name"})
		item.baseURL = firstCodexString(raw, []string{"base_url"}, []string{"baseUrl"})
		item.tokenURL = firstCodexString(raw, []string{"oauth_token_url"}, []string{"token_url"}, []string{"tokenUrl"})
		if expiresAt, ok := firstCodexTime(raw,
			[]string{"tokens", "expires_at"}, []string{"tokens", "expiresAt"},
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
		_ = enrichCodexImportFromJWT(item, item.idToken, false, now)
	}
	if err := enrichCodexImportFromJWT(item, item.accessToken, true, now); err != nil {
		return nil, err
	}
	if item.tokenExpiresAt == nil {
		item.warnings = append(item.warnings, "could not parse access_token expiry; verify token validity after import")
	}
	if item.refreshToken == "" {
		item.warnings = append(item.warnings, "session has no refresh_token; access_token cannot auto-renew after expiry")
	}

	item.identityKeys = buildCodexIdentityKeys(item.accountID, item.userID, item.email, item.accessToken)
	item.name = buildCodexImportAccountName(item, entry.index)
	return item, nil
}

func enrichCodexImportFromJWT(item *codexImportAccount, token string, validateExpiry bool, now time.Time) error {
	claims, err := decodeCodexJWTClaims(token)
	if err != nil {
		if validateExpiry {
			item.warnings = append(item.warnings, "access_token is not a parseable JWT; cannot validate expiry or identity")
		}
		return nil
	}
	if validateExpiry && claims.Exp > 0 {
		if now.Unix() > claims.Exp+int64(codexImportClockSkew.Seconds()) {
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

func decodeCodexJWTClaims(token string) (*codexJWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}
	payload, err := decodeCodexJWTSegment(parts[1])
	if err != nil {
		return nil, err
	}
	var claims codexJWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func decodeCodexJWTSegment(segment string) ([]byte, error) {
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

func buildCodexImportAccountName(item *codexImportAccount, index int) string {
	for _, candidate := range []string{item.name, item.email, item.accountID, item.userID} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return fmt.Sprintf("Codex import %d", index)
}

func buildCodexCreateAccountName(base string, item *codexImportAccount, index, total int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return item.name
	}
	if total > 1 {
		return fmt.Sprintf("%s #%d", base, index)
	}
	return base
}

func buildCodexIdentityKeys(accountID, userID, email, accessToken string) []string {
	keys := make([]string, 0, 4)
	accountID = strings.TrimSpace(accountID)
	userID = strings.TrimSpace(userID)
	if accountID != "" {
		keys = append(keys, "account:"+accountID)
	}
	if userID != "" {
		keys = append(keys, "user:"+userID)
	}
	if accountID == "" && userID == "" {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			keys = append(keys, "email:"+email)
		}
	}
	if accessToken = strings.TrimSpace(accessToken); accessToken != "" {
		sum := sha256.Sum256([]byte(accessToken))
		keys = append(keys, "access:"+hex.EncodeToString(sum[:]))
	}
	return keys
}

func firstSeenCodexIdentity(seen map[string]int, keys []string) (int, bool) {
	for _, key := range keys {
		if index, ok := seen[key]; ok {
			return index, true
		}
	}
	return 0, false
}

func markCodexIdentitySeen(seen map[string]int, keys []string, index int) {
	for _, key := range keys {
		seen[key] = index
	}
}

// --- existing-account index (plaintext metadata only) ----------------------

type codexAccountIndex struct {
	byKey map[string]int
}

func (s *Server) buildCodexAccountIndex(ctx context.Context, providerID int) *codexAccountIndex {
	index := &codexAccountIndex{byKey: map[string]int{}}
	accounts, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return index
	}
	for _, account := range accounts {
		if account.ProviderID != providerID {
			continue
		}
		keys := buildCodexIdentityKeys(
			mapString(account.Metadata, "codex_account_id"),
			mapString(account.Metadata, "codex_user_id"),
			mapString(account.Metadata, "codex_email"),
			"",
		)
		index.add(account.ID, keys)
	}
	return index
}

func (i *codexAccountIndex) add(accountID int, keys []string) {
	if i == nil || i.byKey == nil {
		return
	}
	for _, key := range keys {
		// Access-token fingerprint keys are not derivable from existing
		// (encrypted) credentials, so they never match a stored account.
		if strings.HasPrefix(key, "access:") {
			continue
		}
		i.byKey[key] = accountID
	}
}

func (i *codexAccountIndex) find(keys []string) (int, bool) {
	if i == nil {
		return 0, false
	}
	for _, key := range keys {
		if strings.HasPrefix(key, "access:") {
			continue
		}
		if id, ok := i.byKey[key]; ok {
			return id, true
		}
	}
	return 0, false
}

// --- small helpers ---------------------------------------------------------

func recordCodexFailure(result *apiopenapi.CodexSessionImportResult, index int, name, message string) {
	result.Failed++
	var namePtr *string
	if strings.TrimSpace(name) != "" {
		namePtr = ptrString(name)
	}
	result.Items = append(result.Items, apiopenapi.CodexSessionImportItem{
		Index: index, Name: namePtr, Action: apiopenapi.CodexSessionImportItemActionFailed, Message: ptrString(message),
	})
	result.Errors = append(result.Errors, apiopenapi.CodexSessionImportMessage{
		Index: index, Name: namePtr, Message: message,
	})
}

func setCodexCredentialIfNotEmpty(credential map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		credential[key] = value
	}
}

func setCodexMetadataIfNotEmpty(metadata map[string]any, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		metadata[key] = value
	}
}

func idPtr(id int) *apiopenapi.Id {
	value := apiopenapi.Id(strconv.Itoa(id))
	return &value
}

func firstCodexString(obj map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if value, ok := codexPathValue(obj, path); ok {
			if str := codexStringValue(value); str != "" {
				return str
			}
		}
	}
	return ""
}

func firstCodexTime(obj map[string]any, paths ...[]string) (time.Time, bool) {
	for _, path := range paths {
		if value, ok := codexPathValue(obj, path); ok {
			if parsed, ok := parseCodexTimeValue(value); ok {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func codexPathValue(obj map[string]any, path []string) (any, bool) {
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

func codexStringValue(value any) string {
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

func parseCodexTimeValue(value any) (time.Time, bool) {
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
			return codexUnixTime(n), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return codexUnixTime(n), true
		}
		if f, err := v.Float64(); err == nil {
			return codexUnixTime(int64(f)), true
		}
	case float64:
		return codexUnixTime(int64(v)), true
	case int64:
		return codexUnixTime(v), true
	}
	return time.Time{}, false
}

func codexUnixTime(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}
