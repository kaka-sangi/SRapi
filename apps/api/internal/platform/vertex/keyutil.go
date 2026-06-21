// Package vertex carries dependency-free helpers shared by every code path
// that talks to Google Vertex AI on behalf of an upstream account. Today
// it ships only the service-account private-key normalization helper
// ported from CLIProxyAPI; future additions (JWT signing → access-token
// exchange, region/project URL templating) will live alongside.
//
// The package intentionally has zero non-stdlib dependencies so any
// SRapi layer — accounts service, provider adapter, admin handler —
// can import it without dragging extra modules into its closure.
package vertex

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
)

// NormalizeServiceAccountJSON sanitizes the private_key inside a service
// account JSON blob so it is safe to consume as a PKCS#1 RSA PEM at runtime.
// Returns the normalized JSON on success or the original bytes plus the
// parsing error so callers can surface a precise message to the operator.
func NormalizeServiceAccountJSON(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw, err
	}
	normalized, err := NormalizeServiceAccountMap(payload)
	if err != nil {
		return raw, err
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return raw, err
	}
	return out, nil
}

// NormalizeServiceAccountMap returns a copy of the given service account
// map with its private_key rewritten as a clean RSA PRIVATE KEY PEM. Other
// fields are preserved verbatim so callers retain project_id, client_email,
// and any vendor-specific keys.
func NormalizeServiceAccountMap(sa map[string]any) (map[string]any, error) {
	if sa == nil {
		return nil, fmt.Errorf("service account payload is empty")
	}
	pk, _ := sa["private_key"].(string)
	if strings.TrimSpace(pk) == "" {
		return nil, fmt.Errorf("service account missing private_key")
	}
	normalized, err := sanitizePrivateKey(pk)
	if err != nil {
		return nil, err
	}
	clone := make(map[string]any, len(sa))
	for k, v := range sa {
		clone[k] = v
	}
	clone["private_key"] = normalized
	return clone, nil
}

func sanitizePrivateKey(raw string) (string, error) {
	pk := strings.ReplaceAll(raw, "\r\n", "\n")
	pk = strings.ReplaceAll(pk, "\r", "\n")
	pk = stripANSIEscape(pk)
	pk = strings.ToValidUTF8(pk, "")
	pk = strings.TrimSpace(pk)

	normalized := pk
	if block, _ := pem.Decode([]byte(pk)); block == nil {
		// The PEM header/footer survived but the inner bytes were
		// mangled — rebuild from the textual payload before failing.
		if reconstructed, err := rebuildPEM(pk); err == nil {
			normalized = reconstructed
		} else {
			return "", fmt.Errorf("private_key is not valid pem: %w", err)
		}
	}

	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return "", fmt.Errorf("private_key pem decode failed")
	}
	rsaBlock, err := ensureRSAPrivateKey(block)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(rsaBlock)), nil
}

func ensureRSAPrivateKey(block *pem.Block) (*pem.Block, error) {
	if block == nil {
		return nil, fmt.Errorf("pem block is nil")
	}
	if block.Type == "RSA PRIVATE KEY" {
		if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
			return nil, fmt.Errorf("private_key invalid rsa: %w", err)
		}
		return block, nil
	}
	if block.Type == "PRIVATE KEY" {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("private_key invalid pkcs8: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private_key is not an RSA key")
		}
		der := x509.MarshalPKCS1PrivateKey(rsaKey)
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}, nil
	}
	// Last-ditch auto-detection — try PKCS#1 first, then PKCS#8.
	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		der := x509.MarshalPKCS1PrivateKey(rsaKey)
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			der := x509.MarshalPKCS1PrivateKey(rsaKey)
			return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}, nil
		}
	}
	return nil, fmt.Errorf("private_key uses unsupported format")
}

func rebuildPEM(raw string) (string, error) {
	kind := "PRIVATE KEY"
	if strings.Contains(raw, "RSA PRIVATE KEY") {
		kind = "RSA PRIVATE KEY"
	}
	header := "-----BEGIN " + kind + "-----"
	footer := "-----END " + kind + "-----"
	start := strings.Index(raw, header)
	end := strings.Index(raw, footer)
	if start < 0 || end <= start {
		return "", fmt.Errorf("missing pem markers")
	}
	body := raw[start+len(header) : end]
	payload := filterBase64(body)
	if payload == "" {
		return "", fmt.Errorf("private_key base64 payload empty")
	}
	der, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("private_key base64 decode failed: %w", err)
	}
	block := &pem.Block{Type: kind, Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

func filterBase64(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '+', r == '/', r == '=':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripANSIEscape(s string) string {
	in := []rune(s)
	out := make([]rune, 0, len(in))
	for i := 0; i < len(in); i++ {
		r := in[i]
		if r != 0x1b {
			out = append(out, r)
			continue
		}
		if i+1 >= len(in) {
			continue
		}
		switch in[i+1] {
		case ']':
			i += 2
			for i < len(in) {
				if in[i] == 0x07 {
					break
				}
				if in[i] == 0x1b && i+1 < len(in) && in[i+1] == '\\' {
					i++
					break
				}
				i++
			}
		case '[':
			i += 2
			for i < len(in) {
				if (in[i] >= 'A' && in[i] <= 'Z') || (in[i] >= 'a' && in[i] <= 'z') {
					break
				}
				i++
			}
		}
	}
	return string(out)
}
