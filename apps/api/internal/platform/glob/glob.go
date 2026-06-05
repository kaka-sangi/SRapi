// Package glob provides a small, case-insensitive "*" wildcard matcher shared
// across modules that filter model names (payload-rule model matching and
// per-account model inclusion/exclusion). It is deliberately minimal: only the
// "*" wildcard is special, so patterns are easy to reason about for operators.
package glob

import "strings"

// Match reports whether value satisfies pattern. The "*" character is a
// wildcard matching any run of characters. Supported forms:
//
//	""          -> matches anything (no constraint)
//	"*"         -> matches anything
//	"exact"     -> matches only "exact"
//	"prefix*"   -> matches values starting with "prefix"
//	"*suffix"   -> matches values ending with "suffix"
//	"*mid*"     -> matches values containing "mid"
//	"a*b*c"     -> matches values with those anchored segments in order
//
// Matching is case-insensitive and ignores surrounding whitespace on both
// pattern and value.
func Match(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" || pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(value[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && idx != 0 {
			return false // anchored prefix (pattern does not start with '*')
		}
		pos += idx + len(part)
	}
	if last := parts[len(parts)-1]; last != "" && !strings.HasSuffix(value, last) {
		return false // anchored suffix (pattern does not end with '*')
	}
	return true
}

// MatchAny reports whether value matches at least one of the patterns. Blank
// patterns are skipped so an empty/whitespace entry never matches everything by
// accident; with no usable patterns it returns false.
func MatchAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		if Match(pattern, value) {
			return true
		}
	}
	return false
}
