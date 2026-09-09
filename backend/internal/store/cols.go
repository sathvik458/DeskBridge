package store

import "strings"

func splitCols(cols string) []string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(strings.ReplaceAll(p, "\n", " "))
	}
	return parts
}

func joinCols(parts []string) string {
	return strings.Join(parts, ", ")
}
