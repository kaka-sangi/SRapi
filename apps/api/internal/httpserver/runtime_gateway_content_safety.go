package httpserver

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	contentsafetycontract "github.com/srapi/srapi/apps/api/internal/modules/content_safety/contract"
	contentsafetyservice "github.com/srapi/srapi/apps/api/internal/modules/content_safety/service"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

// moderationSecretVersion tags ciphertexts produced by the content safety
// moderation key path. Bumping this string forces a re-encrypt cycle and
// makes mistaken cross-namespace pastes fail loudly instead of silently
// decrypting under the wrong domain.
const moderationSecretVersion = "moderationv1"

func contentSafetyConfigFromAdminControl(config admincontrolcontract.ContentSafetyConfig) contentsafetycontract.Config {
	return contentsafetycontract.Config{
		Enabled:              config.Enabled,
		Mode:                 contentsafetycontract.Mode(config.Mode),
		RedactPII:            config.RedactPII,
		BlockPII:             config.BlockPII,
		BlockPromptInjection: config.BlockPromptInjection,
		BlockCustomKeywords:  config.BlockCustomKeywords,
		CustomKeywords:       append([]string(nil), config.CustomKeywords...),
		ModelScopes:          append([]string(nil), config.ModelScopes...),
		Moderation: contentsafetycontract.ModerationOptions{
			Enabled:     config.Moderation.Enabled,
			BlockOnFlag: config.Moderation.BlockOnFlag,
			Thresholds:  cloneFloatMap(config.Moderation.Categories),
		},
	}
}

func contentSafetyFindingsAudit(findings []contentsafetycontract.Finding) []map[string]any {
	out := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		entry := map[string]any{
			"kind":     string(finding.Kind),
			"severity": string(finding.Severity),
			"count":    finding.Count,
			"redacted": finding.Redacted,
		}
		if finding.Category != "" {
			entry["category"] = finding.Category
		}
		if finding.Score > 0 {
			entry["score"] = finding.Score
		}
		out = append(out, entry)
	}
	return out
}

// moderationClientCache memoizes an OpenAIModerationClient so its in-memory
// response cache and outbound connection pool survive across requests. The
// signature key collapses (apiKey, baseURL, model, timeout, ttl) into a
// single string — any operator change rebuilds the client lazily on the
// next request rather than at admin-PUT time so the swap is consistent
// regardless of which process saw the settings update first.
type moderationClientCache struct {
	mu        sync.Mutex
	signature string
	client    *contentsafetyservice.OpenAIModerationClient
}

func (c *moderationClientCache) get(config admincontrolcontract.ContentSafetyModerationConfig, apiKey string) (*contentsafetyservice.OpenAIModerationClient, error) {
	signature := moderationSignature(config, apiKey)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil && c.signature == signature {
		return c.client, nil
	}
	client, err := contentsafetyservice.NewOpenAIModerationClient(contentsafetyservice.OpenAIModerationOptions{
		APIKey:    apiKey,
		BaseURL:   config.BaseURL,
		Model:     config.Model,
		Timeout:   time.Duration(config.TimeoutMS) * time.Millisecond,
		CacheSize: 256,
		CacheTTL:  time.Duration(config.CacheTTLSeconds) * time.Second,
	})
	if err != nil {
		c.client = nil
		c.signature = ""
		return nil, err
	}
	c.client = client
	c.signature = signature
	return client, nil
}

func moderationSignature(config admincontrolcontract.ContentSafetyModerationConfig, apiKey string) string {
	// API keys must not appear in plaintext anywhere they could leak (logs,
	// snapshots); the cache only needs to detect change, so hash the key
	// via a fingerprint that uses the master version tag as a salt.
	return fmt.Sprintf("%s|%s|%s|%d|%d|%s", config.Provider, config.BaseURL, config.Model, config.TimeoutMS, config.CacheTTLSeconds, fingerprintAPIKey(apiKey))
}

func fingerprintAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return fmt.Sprintf("%s…%s", key[:4], key[len(key)-4:])
}

func cloneFloatMap(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// buildModerationProvider decrypts the persisted API key and asks the
// runtime cache for a ready-to-call OpenAI moderation client. Returns nil
// (and a typed sentinel) when no credentials are configured so callers can
// fail-open silently rather than logging at every request.
func (rt *runtimeState) buildModerationProvider(config admincontrolcontract.ContentSafetyModerationConfig) (contentsafetycontract.ModerationProvider, error) {
	if !config.Enabled || strings.TrimSpace(config.APIKeyCiphertext) == "" {
		return nil, contentsafetyservice.ErrModerationNotConfigured
	}
	apiKey, err := decryptModerationSecret(rt.cfg.Security.MasterKey, config.APIKeyCiphertext)
	if err != nil {
		return nil, err
	}
	return rt.moderationClients.get(config, apiKey)
}

// decryptModerationSecret mirrors decryptMasterSecret on *Server but lives
// on the runtime side so the gateway hot path can decrypt without bouncing
// back through a Server method (the runtime owns no *Server reference).
// The version label MUST stay in sync with moderationSecretVersion.
func decryptModerationSecret(masterKey string, ciphertextValue string) (string, error) {
	parts := strings.Split(ciphertextValue, ":")
	if len(parts) != 3 || parts[0] != moderationSecretVersion {
		return "", errors.New("invalid encrypted moderation secret")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	key, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, []byte(moderationSecretVersion))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
