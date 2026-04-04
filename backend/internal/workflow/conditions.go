package workflow

import "strings"

func evaluateCondition(output, condition string) bool {
	switch strings.ToLower(strings.TrimSpace(condition)) {
	case "always", "":
		return true
	case "approved":
		return strings.Contains(strings.ToUpper(output), "APPROVED")
	case "rejected":
		return strings.Contains(strings.ToUpper(output), "REJECTED")
	default:
		return strings.Contains(output, condition)
	}
}
