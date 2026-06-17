// Claude thinking signature validation and stripping.
//
// Ported from CLIProxyAPI internal/signature/claude.go +
// claude_validation.go (the "shouldStripClaudeThinkingBlock" /
// "StripInvalidClaudeThinkingBlocks" / "InspectClaudeSignaturePayload"
// trio that protects the upstream from forged thinking blocks).
//
// CLIProxyAPI walks raw JSON bytes via gjson/sjson. srapi has no
// tidwall dependency and threads messages as []map[string]any all the
// way through the provider adapter, so the API surface here mirrors
// that in-memory shape. The validation rules are identical:
//
//   - Cache prefix (everything up to '#') is stripped.
//   - The signature must start with 'E' (single-layer base64) or 'R'
//     (double-layer base64).
//   - Both layers must decode and the innermost payload must start
//     with the Claude protobuf marker 0x12.
//   - The protobuf must carry the top-level field 2 -> field 1 channel
//     block with a varint channel_id (strict mode).
//
// Forged or malformed thinking blocks are silently dropped so the
// upstream never sees them; this is the documented Claude bypass
// behaviour from sub2api / CLIProxyAPI.
package signature

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// MaxClaudeThinkingSignatureLen caps the inspected signature length to
// prevent pathological inputs from burning CPU. Mirrors CLIProxyAPI.
const MaxClaudeThinkingSignatureLen = 32 * 1024 * 1024

// ClaudeSignatureValidationOptions controls how far Claude thinking
// signatures are inspected. Default (zero value) does full validation
// including base64 layer decode + 0x12 marker check. Strict additionally
// walks the protobuf tree.
type ClaudeSignatureValidationOptions struct {
	// PrefixOnly only checks for an optional cache prefix followed by
	// an E/R Claude signature prefix.
	PrefixOnly bool
	// Base64Only checks the optional cache prefix, E/R Claude signature
	// prefix, and base64 layers without validating the decoded Claude
	// marker or protobuf tree.
	Base64Only bool
	// AllowEmptySignatureWithEmptyText preserves empty thinking
	// placeholders with no signature and no thinking/text payload
	// during strip operations.
	AllowEmptySignatureWithEmptyText bool
	// Strict turns on full protobuf-tree validation.
	Strict bool
}

// ClaudeSignatureTree describes the protobuf fields currently used for
// Claude thinking signature routing.
type ClaudeSignatureTree struct {
	EncodingLayers      int
	ChannelID           uint64
	Field2              *uint64
	RoutingClass        string
	InfrastructureClass string
	SchemaFeatures      string
	ModelText           string
	LegacyRouteHint     string
	HasField7           bool
}

func claudeSignatureValidationOptions(opts []ClaudeSignatureValidationOptions) ClaudeSignatureValidationOptions {
	if len(opts) == 0 {
		return ClaudeSignatureValidationOptions{}
	}
	return opts[0]
}

// IsValidClaudeThinkingSignature returns whether rawSignature is a
// valid Claude thinking signature under the requested validation
// options.
func IsValidClaudeThinkingSignature(rawSignature string, opts ...ClaudeSignatureValidationOptions) bool {
	opt := claudeSignatureValidationOptions(opts)
	if opt.PrefixOnly {
		return HasClaudeThinkingSignaturePrefix(rawSignature)
	}
	if opt.Base64Only {
		return HasDecodableClaudeThinkingSignature(rawSignature)
	}
	_, err := NormalizeClaudeThinkingSignature(rawSignature, opts...)
	return err == nil
}

// HasDecodableClaudeThinkingSignature reports whether rawSignature has
// the Claude E/R shape and its expected base64 layer(s) can be decoded.
func HasDecodableClaudeThinkingSignature(rawSignature string) bool {
	sig := stripClaudeSignaturePrefix(rawSignature)
	if sig == "" || len(sig) > MaxClaudeThinkingSignatureLen {
		return false
	}
	switch sig[0] {
	case 'E':
		decoded, err := base64.StdEncoding.DecodeString(sig)
		return err == nil && len(decoded) > 0
	case 'R':
		decoded, err := base64.StdEncoding.DecodeString(sig)
		if err != nil || len(decoded) == 0 || decoded[0] != 'E' {
			return false
		}
		innerDecoded, err := base64.StdEncoding.DecodeString(string(decoded))
		return err == nil && len(innerDecoded) > 0
	default:
		return false
	}
}

// HasClaudeThinkingSignaturePrefix reports whether rawSignature has the
// Claude E/R signature prefix after stripping an optional cache prefix.
func HasClaudeThinkingSignaturePrefix(rawSignature string) bool {
	sig := stripClaudeSignaturePrefix(rawSignature)
	if sig == "" {
		return false
	}
	return sig[0] == 'E' || sig[0] == 'R'
}

func stripClaudeSignaturePrefix(rawSignature string) string {
	sig := strings.TrimSpace(rawSignature)
	if sig == "" {
		return ""
	}
	if idx := strings.IndexByte(sig, '#'); idx >= 0 {
		sig = strings.TrimSpace(sig[idx+1:])
	}
	return sig
}

// NormalizeClaudeThinkingSignature strips any cache prefix, validates
// the signature, and returns the double-layer R-form expected by
// Antigravity bypass mode.
func NormalizeClaudeThinkingSignature(rawSignature string, opts ...ClaudeSignatureValidationOptions) (string, error) {
	opt := claudeSignatureValidationOptions(opts)
	sig := stripClaudeSignaturePrefix(rawSignature)
	if sig == "" {
		return "", fmt.Errorf("empty signature")
	}
	if len(sig) > MaxClaudeThinkingSignatureLen {
		return "", fmt.Errorf("signature exceeds maximum length (%d bytes)", MaxClaudeThinkingSignatureLen)
	}
	switch sig[0] {
	case 'R':
		if err := validateClaudeDoubleLayerSignature(sig, opt); err != nil {
			return "", err
		}
		return sig, nil
	case 'E':
		if err := validateClaudeSingleLayerSignature(sig, opt); err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString([]byte(sig)), nil
	default:
		return "", fmt.Errorf("invalid signature: expected 'E' or 'R' prefix, got %q", string(sig[0]))
	}
}

func validateClaudeDoubleLayerSignature(sig string, opt ClaudeSignatureValidationOptions) error {
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("invalid double-layer signature: base64 decode failed: %w", err)
	}
	if len(decoded) == 0 {
		return fmt.Errorf("invalid double-layer signature: empty after decode")
	}
	if decoded[0] != 'E' {
		return fmt.Errorf("invalid double-layer signature: inner does not start with 'E', got 0x%02x", decoded[0])
	}
	return validateClaudeSingleLayerSignatureContent(string(decoded), 2, opt)
}

func validateClaudeSingleLayerSignature(sig string, opt ClaudeSignatureValidationOptions) error {
	return validateClaudeSingleLayerSignatureContent(sig, 1, opt)
}

func validateClaudeSingleLayerSignatureContent(sig string, encodingLayers int, opt ClaudeSignatureValidationOptions) error {
	decoded, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("invalid single-layer signature: base64 decode failed: %w", err)
	}
	if len(decoded) == 0 {
		return fmt.Errorf("invalid single-layer signature: empty after decode")
	}
	if decoded[0] != 0x12 {
		return fmt.Errorf("invalid Claude signature: expected first byte 0x12, got 0x%02x", decoded[0])
	}
	if !opt.Strict {
		return nil
	}
	_, err = InspectClaudeSignaturePayload(decoded, encodingLayers)
	return err
}

// InspectClaudeSignaturePayload inspects the decoded Claude thinking
// signature protobuf payload.
func InspectClaudeSignaturePayload(payload []byte, encodingLayers int) (*ClaudeSignatureTree, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("invalid Claude signature: empty payload")
	}
	if payload[0] != 0x12 {
		return nil, fmt.Errorf("invalid Claude signature: expected first byte 0x12, got 0x%02x", payload[0])
	}
	container, err := extractClaudeBytesField(payload, 2, "top-level protobuf")
	if err != nil {
		return nil, err
	}
	channelBlock, err := extractClaudeBytesField(container, 1, "Claude Field 2 container")
	if err != nil {
		return nil, err
	}
	return inspectClaudeChannelBlock(channelBlock, encodingLayers)
}

func inspectClaudeChannelBlock(channelBlock []byte, encodingLayers int) (*ClaudeSignatureTree, error) {
	tree := &ClaudeSignatureTree{
		EncodingLayers:      encodingLayers,
		RoutingClass:        "unknown",
		InfrastructureClass: "infra_unknown",
		SchemaFeatures:      "unknown_schema_features",
	}
	haveChannelID := false
	hasField6 := false
	hasField7 := false

	err := walkClaudeProtobufFields(channelBlock, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.1 channel_id must be varint")
			}
			channelID, err := decodeClaudeVarintField(raw, "Field 2.1.1 channel_id")
			if err != nil {
				return err
			}
			tree.ChannelID = channelID
			haveChannelID = true
		case 2:
			if typ != protowire.VarintType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.2 field2 must be varint")
			}
			field2, err := decodeClaudeVarintField(raw, "Field 2.1.2 field2")
			if err != nil {
				return err
			}
			tree.Field2 = &field2
		case 6:
			if typ != protowire.BytesType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.6 model_text must be bytes")
			}
			modelBytes, err := decodeClaudeBytesField(raw, "Field 2.1.6 model_text")
			if err != nil {
				return err
			}
			if !utf8.Valid(modelBytes) {
				return fmt.Errorf("invalid Claude signature: Field 2.1.6 model_text is not valid UTF-8")
			}
			tree.ModelText = string(modelBytes)
			hasField6 = true
		case 7:
			if typ != protowire.VarintType {
				return fmt.Errorf("invalid Claude signature: Field 2.1.7 must be varint")
			}
			if _, err := decodeClaudeVarintField(raw, "Field 2.1.7"); err != nil {
				return err
			}
			hasField7 = true
			tree.HasField7 = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !haveChannelID {
		return nil, fmt.Errorf("invalid Claude signature: missing Field 2.1.1 channel_id")
	}

	switch tree.ChannelID {
	case 11:
		tree.RoutingClass = "routing_class_11"
	case 12:
		tree.RoutingClass = "routing_class_12"
	}
	if tree.Field2 == nil {
		tree.InfrastructureClass = "infra_default"
	} else {
		switch *tree.Field2 {
		case 1:
			tree.InfrastructureClass = "infra_aws"
		case 2:
			tree.InfrastructureClass = "infra_google"
		default:
			tree.InfrastructureClass = "infra_unknown"
		}
	}
	switch {
	case hasField6:
		tree.SchemaFeatures = "extended_model_tagged_schema"
	case !hasField6 && !hasField7 && len(channelBlock) >= 70 && len(channelBlock) <= 72:
		tree.SchemaFeatures = "compact_schema"
	}
	return tree, nil
}

func extractClaudeBytesField(msg []byte, fieldNum protowire.Number, scope string) ([]byte, error) {
	var value []byte
	err := walkClaudeProtobufFields(msg, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		if num != fieldNum {
			return nil
		}
		if typ != protowire.BytesType {
			return fmt.Errorf("invalid Claude signature: %s field %d must be bytes", scope, fieldNum)
		}
		bytesValue, err := decodeClaudeBytesField(raw, fmt.Sprintf("%s field %d", scope, fieldNum))
		if err != nil {
			return err
		}
		value = bytesValue
		return nil
	})
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("invalid Claude signature: missing %s field %d", scope, fieldNum)
	}
	return value, nil
}

func walkClaudeProtobufFields(msg []byte, visit func(num protowire.Number, typ protowire.Type, raw []byte) error) error {
	for offset := 0; offset < len(msg); {
		num, typ, n := protowire.ConsumeTag(msg[offset:])
		if n < 0 {
			return fmt.Errorf("invalid Claude signature: malformed protobuf tag: %w", protowire.ParseError(n))
		}
		offset += n
		valueLen := protowire.ConsumeFieldValue(num, typ, msg[offset:])
		if valueLen < 0 {
			return fmt.Errorf("invalid Claude signature: malformed protobuf field %d: %w", num, protowire.ParseError(valueLen))
		}
		fieldRaw := msg[offset : offset+valueLen]
		if err := visit(num, typ, fieldRaw); err != nil {
			return err
		}
		offset += valueLen
	}
	return nil
}

func decodeClaudeVarintField(raw []byte, label string) (uint64, error) {
	value, n := protowire.ConsumeVarint(raw)
	if n < 0 {
		return 0, fmt.Errorf("invalid Claude signature: failed to decode %s: %w", label, protowire.ParseError(n))
	}
	return value, nil
}

func decodeClaudeBytesField(raw []byte, label string) ([]byte, error) {
	value, n := protowire.ConsumeBytes(raw)
	if n < 0 {
		return nil, fmt.Errorf("invalid Claude signature: failed to decode %s: %w", label, protowire.ParseError(n))
	}
	return value, nil
}

// StripInvalidClaudeThinkingBlocks removes thinking content blocks
// whose signatures are missing or fail Claude signature validation,
// preserving every other block intact. Ported from CLIProxyAPI's
// internal/signature/claude.go StripInvalidClaudeThinkingBlocks, which
// uses gjson/sjson over raw JSON. srapi already has messages in
// []map[string]any form by the time the adapter hits the outbound
// build, so the input type is adapted accordingly. The validation
// rules and the "drop missing/forged signatures, keep everything
// else" behaviour are identical.
//
// Each message map is expected to optionally carry a "content" key
// whose value is a []any (or []map[string]any) of content parts.
// Parts whose "type" is "thinking" are validated; parts of any other
// type are passed through unmodified. The slice is mutated in place
// AND the modified slice is returned for convenience.
func StripInvalidClaudeThinkingBlocks(messages []map[string]any, opts ...ClaudeSignatureValidationOptions) []map[string]any {
	if len(messages) == 0 {
		return messages
	}
	opt := claudeSignatureValidationOptions(opts)
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content, ok := msg["content"]
		if !ok {
			continue
		}
		stripped, changed := stripClaudeThinkingFromContent(content, opt)
		if changed {
			msg["content"] = stripped
		}
	}
	return messages
}

// StripInvalidClaudeThinkingBlocksAndEmptyMessages applies the same
// strip and then drops any message whose content is empty after the
// strip. Mirrors the CLIProxyAPI helper of the same name.
func StripInvalidClaudeThinkingBlocksAndEmptyMessages(messages []map[string]any, opts ...ClaudeSignatureValidationOptions) []map[string]any {
	stripped := StripInvalidClaudeThinkingBlocks(messages, opts...)
	if len(stripped) == 0 {
		return stripped
	}
	out := make([]map[string]any, 0, len(stripped))
	for _, msg := range stripped {
		if msg == nil {
			continue
		}
		content, ok := msg["content"]
		if !ok {
			out = append(out, msg)
			continue
		}
		switch typed := content.(type) {
		case []map[string]any:
			if len(typed) == 0 {
				continue
			}
		case []any:
			if len(typed) == 0 {
				continue
			}
		}
		out = append(out, msg)
	}
	return out
}

func stripClaudeThinkingFromContent(content any, opt ClaudeSignatureValidationOptions) (any, bool) {
	switch typed := content.(type) {
	case []map[string]any:
		kept := make([]map[string]any, 0, len(typed))
		stripped := false
		for _, part := range typed {
			if part == nil {
				kept = append(kept, part)
				continue
			}
			if isClaudeThinkingPart(part) && shouldStripClaudeThinkingPart(part, opt) {
				stripped = true
				continue
			}
			kept = append(kept, part)
		}
		if stripped {
			return kept, true
		}
		return typed, false
	case []any:
		kept := make([]any, 0, len(typed))
		stripped := false
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				kept = append(kept, item)
				continue
			}
			if isClaudeThinkingPart(part) && shouldStripClaudeThinkingPart(part, opt) {
				stripped = true
				continue
			}
			kept = append(kept, item)
		}
		if stripped {
			return kept, true
		}
		return typed, false
	default:
		return content, false
	}
}

func isClaudeThinkingPart(part map[string]any) bool {
	t, _ := part["type"].(string)
	return t == "thinking"
}

func shouldStripClaudeThinkingPart(part map[string]any, opt ClaudeSignatureValidationOptions) bool {
	if opt.AllowEmptySignatureWithEmptyText && isEmptyClaudeThinkingPlaceholder(part) {
		return false
	}
	sig, _ := part["signature"].(string)
	return !IsValidClaudeThinkingSignature(sig, opt)
}

func isEmptyClaudeThinkingPlaceholder(part map[string]any) bool {
	sig, _ := part["signature"].(string)
	if strings.TrimSpace(sig) != "" {
		return false
	}
	return strings.TrimSpace(claudeThinkingPartText(part)) == ""
}

func claudeThinkingPartText(part map[string]any) string {
	if text, ok := part["text"].(string); ok && text != "" {
		return text
	}
	thinkingField, exists := part["thinking"]
	if !exists {
		return ""
	}
	switch typed := thinkingField.(type) {
	case string:
		return typed
	case map[string]any:
		if inner, ok := typed["text"].(string); ok {
			return inner
		}
		if inner, ok := typed["thinking"].(string); ok {
			return inner
		}
	}
	return ""
}
