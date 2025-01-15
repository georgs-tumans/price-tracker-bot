package utilities

func GetStringPointerValue(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
