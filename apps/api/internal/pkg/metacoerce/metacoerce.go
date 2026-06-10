package metacoerce

import (
	"encoding/json"
	"strconv"
	"strings"
)

func Bool(metadata map[string]any, key string) bool {
	value, ok := Value(metadata, key)
	if !ok {
		return false
	}
	return BoolValue(value)
}

func BoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func Value(metadata map[string]any, keys ...string) (any, bool) {
	if metadata == nil {
		return nil, false
	}
	for _, key := range keys {
		value, ok := metadata[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func Float(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func Int(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed), true
		}
		parsed, err := typed.Float64()
		return int(parsed), err == nil
	case string:
		raw := strings.TrimSpace(typed)
		if parsed, err := strconv.Atoi(raw); err == nil {
			return parsed, true
		}
		parsed, err := strconv.ParseFloat(raw, 64)
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func OptionalInt(metadata map[string]any, keys ...string) *int {
	value, ok := Value(metadata, keys...)
	if !ok {
		return nil
	}
	parsed, ok := Int(value)
	if !ok {
		return nil
	}
	return &parsed
}

func OptionalFloat(metadata map[string]any, keys ...string) *float64 {
	value, ok := Value(metadata, keys...)
	if !ok {
		return nil
	}
	parsed, ok := Float(value)
	if !ok {
		return nil
	}
	return &parsed
}
