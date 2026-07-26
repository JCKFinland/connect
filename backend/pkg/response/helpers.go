package response

func toString(value interface{}) string {

	if value == nil {
		return ""
	}

	if s, ok := value.(string); ok {
		return s
	}

	return ""
}