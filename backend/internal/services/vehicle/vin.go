package vehicle

import "strings"

func normalizeVIN(value string) *string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return nil
	}

	return &value
}

func vinValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
