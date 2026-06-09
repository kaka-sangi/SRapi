package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

type importIdentityIndex struct {
	byKey map[string]int
}

func newImportIdentityIndex() *importIdentityIndex {
	return &importIdentityIndex{byKey: map[string]int{}}
}

func buildImportIdentityKeys(providerID int, name string, runtimeClass accountcontract.RuntimeClass, upstreamClient *string, metadata map[string]any, credential map[string]any) []string {
	prefix := fmt.Sprintf("provider:%d:", providerID)
	keys := make([]string, 0, 6)
	if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
		keys = append(keys, prefix+"name:"+name)
	}
	for _, field := range []string{"codex_account_id", "account_id", "chatgpt_account_id"} {
		if value := mapString(metadata, field); value != "" {
			keys = append(keys, prefix+"account:"+strings.ToLower(value))
			break
		}
	}
	for _, field := range []string{"codex_user_id", "user_id", "chatgpt_user_id"} {
		if value := mapString(metadata, field); value != "" {
			keys = append(keys, prefix+"user:"+strings.ToLower(value))
			break
		}
	}
	for _, field := range []string{"codex_email", "email"} {
		if value := mapString(metadata, field); value != "" {
			keys = append(keys, prefix+"email:"+strings.ToLower(value))
			break
		}
	}
	if upstreamClient != nil {
		if value := strings.ToLower(strings.TrimSpace(*upstreamClient)); value != "" {
			keys = append(keys, prefix+"runtime:"+strings.ToLower(string(runtimeClass))+":client:"+value)
		}
	}
	if fingerprint := importCredentialFingerprint(credential); fingerprint != "" {
		keys = append(keys, prefix+"credential:"+fingerprint)
	}
	return keys
}

func buildCodexIdentityKeys(accountID, userID, email, accessToken string) []string {
	metadata := map[string]any{
		"codex_account_id": strings.TrimSpace(accountID),
		"codex_user_id":    strings.TrimSpace(userID),
		"codex_email":      strings.TrimSpace(email),
	}
	credential := map[string]any{}
	if accessToken = strings.TrimSpace(accessToken); accessToken != "" {
		credential["access_token"] = accessToken
	}
	return stripImportIdentityProviderPrefix(buildImportIdentityKeys(0, "", "", nil, metadata, credential))
}

func stripImportIdentityProviderPrefix(keys []string) []string {
	const prefix = "provider:0:"
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, strings.TrimPrefix(key, prefix))
	}
	return out
}

func importCredentialFingerprint(credential map[string]any) string {
	for _, key := range []string{"access_token", "refresh_token", "api_key", "cookie", "id_token"} {
		value, ok := credential[key].(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		sum := sha256.Sum256([]byte(value))
		return key + ":" + hex.EncodeToString(sum[:])
	}
	return ""
}

func (i *importIdentityIndex) add(accountID int, keys []string) {
	if i == nil || i.byKey == nil {
		return
	}
	for _, key := range keys {
		if key == "" || strings.Contains(key, "credential:") || strings.HasPrefix(key, "access:") {
			continue
		}
		i.byKey[key] = accountID
	}
}

func (i *importIdentityIndex) find(keys []string) (int, bool) {
	if i == nil {
		return 0, false
	}
	for _, key := range keys {
		if key == "" || strings.Contains(key, "credential:") || strings.HasPrefix(key, "access:") {
			continue
		}
		if id, ok := i.byKey[key]; ok {
			return id, true
		}
	}
	return 0, false
}

func firstSeenImportIdentity(seen map[string]int, keys []string) (int, bool) {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if index, ok := seen[key]; ok {
			return index, true
		}
	}
	return 0, false
}

func markImportIdentitySeen(seen map[string]int, keys []string, index int) {
	for _, key := range keys {
		if key != "" {
			seen[key] = index
		}
	}
}
