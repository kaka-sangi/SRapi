package httpserver

import (
	"math"

	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func positiveIntFromInt64(value int64) (int, bool) {
	if value <= 0 || value > int64(math.MaxInt) {
		return 0, false
	}
	return int(value), true
}

func nonNegativeIntFromInt64Ptr(value *int64) (int, bool) {
	if value == nil {
		return 0, true
	}
	if *value < 0 || *value > int64(math.MaxInt) {
		return 0, false
	}
	return int(*value), true
}

func intPtrFromInt64Ptr(value *int64) (*int, bool) {
	if value == nil {
		return nil, true
	}
	if *value < int64(math.MinInt) || *value > int64(math.MaxInt) {
		return nil, false
	}
	converted := int(*value)
	return &converted, true
}

func nonNegativeIntSliceFromInt64Ptr(value *[]int64) ([]int, bool) {
	if value == nil {
		return nil, true
	}
	out := make([]int, 0, len(*value))
	for _, item := range *value {
		if item < 0 || item > int64(math.MaxInt) {
			return nil, false
		}
		out = append(out, int(item))
	}
	return out, true
}

func firstInt64PtrAsInt(values ...*int64) *int {
	for _, value := range values {
		if value == nil || *value > int64(math.MaxInt) || *value < int64(math.MinInt) {
			continue
		}
		converted := int(*value)
		return &converted
	}
	return nil
}

func int64PtrFromIntPtr(value *int) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}

func int64Ptr(value int) *int64 {
	converted := int64(value)
	return &converted
}

func stringPtr(value string) *string {
	return &value
}

func optionalNonEmptyStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalSlice[T any](value *[]T) []T {
	if value == nil {
		return nil
	}
	return *value
}

func openapiOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func openapiOptionalStringPtr(value *string) *string {
	resolved := openapiOptionalString(value)
	return &resolved
}

func openapiOptionalStringMap(value *map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	return *value
}

func openapiOptionalStringMapPtr(value *map[string]string) *map[string]string {
	resolved := openapiOptionalStringMap(value)
	return &resolved
}

func openapiOptionalStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func openapiOptionalBool(value *bool) bool {
	return value != nil && *value
}

func ptrAPIAdminSettings(value apiopenapi.AdminSettings) *apiopenapi.AdminSettings {
	return &value
}
