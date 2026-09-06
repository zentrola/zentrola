package catalog

// ValidModalities 要求显式声明非空集合；未知类型和重复值均视为无效。
func ValidModalities(values []string) bool {
	if len(values) == 0 || len(values) > 4 {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		switch value {
		case "TEXT", "IMAGE", "AUDIO", "VIDEO":
		default:
			return false
		}
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
