package core

import "strings"

// containsSubstring reports whether s contains substr.
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}
