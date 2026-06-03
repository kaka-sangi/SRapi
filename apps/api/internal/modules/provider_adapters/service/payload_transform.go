package service

import (
	"encoding/json"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// applyPayloadTransforms mutates the marshaled upstream request body per the
// operator-configured transforms carried on the request. It works on the final
// JSON object body (after protocol translation), so paths target the upstream
// shape, e.g. "reasoning.effort" or "generationConfig.thinkingConfig.thinkingBudget".
//
// v1 supports dotted object paths (no array indices/wildcards); a body that is
// not a JSON object is returned untouched.
func applyPayloadTransforms(raw []byte, transforms []contract.PayloadTransform) ([]byte, error) {
	if len(transforms) == 0 || len(raw) == 0 {
		return raw, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw, nil
	}
	if doc == nil {
		doc = map[string]any{}
	}
	changed := false
	for _, transform := range transforms {
		path := strings.TrimSpace(transform.Path)
		if path == "" {
			continue
		}
		segments := strings.Split(path, ".")
		switch transform.Action {
		case "override":
			setPayloadPath(doc, segments, transform.Value)
			changed = true
		case "default":
			if !payloadPathExists(doc, segments) {
				setPayloadPath(doc, segments, transform.Value)
				changed = true
			}
		case "filter":
			if deletePayloadPath(doc, segments) {
				changed = true
			}
		}
	}
	if !changed {
		return raw, nil
	}
	return json.Marshal(doc)
}

// setPayloadPath sets a dotted path, creating intermediate objects as needed.
func setPayloadPath(doc map[string]any, segments []string, value any) {
	cur := doc
	for i := 0; i < len(segments)-1; i++ {
		seg := segments[i]
		next, ok := cur[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[seg] = next
		}
		cur = next
	}
	cur[segments[len(segments)-1]] = value
}

// payloadPathExists reports whether a dotted path resolves to a non-null value.
func payloadPathExists(doc map[string]any, segments []string) bool {
	cur := doc
	for i := 0; i < len(segments)-1; i++ {
		next, ok := cur[segments[i]].(map[string]any)
		if !ok {
			return false
		}
		cur = next
	}
	value, ok := cur[segments[len(segments)-1]]
	return ok && value != nil
}

// deletePayloadPath removes a dotted path; returns true if anything was removed.
func deletePayloadPath(doc map[string]any, segments []string) bool {
	cur := doc
	for i := 0; i < len(segments)-1; i++ {
		next, ok := cur[segments[i]].(map[string]any)
		if !ok {
			return false
		}
		cur = next
	}
	leaf := segments[len(segments)-1]
	if _, ok := cur[leaf]; !ok {
		return false
	}
	delete(cur, leaf)
	return true
}
