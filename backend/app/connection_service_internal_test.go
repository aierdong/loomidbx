package app

import (
	"errors"
	"strings"
	"testing"
)

// TestSanitizeErrorRedactsSecrets 验证错误输出不会泄露敏感值。
func TestSanitizeErrorRedactsSecrets(t *testing.T) {
	raw := errors.New("dial failed password=super-secret token=abc123")
	got := sanitizeError(raw, "super-secret", "abc123")
	if strings.Contains(got, "super-secret") || strings.Contains(got, "abc123") {
		t.Fatalf("sanitized error leaks secret: %q", got)
	}
}
