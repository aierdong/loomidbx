package app

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"loomidbx/backend/storage"
)

func TestNewPlatformKeyringAccessor(t *testing.T) {
	accessor, err := NewPlatformKeyringAccessor()

	// 检查平台支持矩阵
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		if err != nil {
			t.Errorf("expected no error on %s, got: %v", runtime.GOOS, err)
		}
		if accessor == nil {
			t.Error("expected non-nil accessor on supported platform")
		}
		if accessor.service != keyringServiceName {
			t.Errorf("expected service=%s, got=%s", keyringServiceName, accessor.service)
		}
	default:
		if err != ErrKeyringUnavailable {
			t.Errorf("expected ErrKeyringUnavailable on %s, got: %v", runtime.GOOS, err)
		}
		if accessor != nil {
			t.Error("expected nil accessor on unsupported platform")
		}
	}
}

func TestBuildKeyringRef(t *testing.T) {
	tests := []struct {
		connectionID string
		expected     string
	}{
		{"abc123", "connection:abc123"},
		{"", "connection:"},
		{"test-connection-id", "connection:test-connection-id"},
	}

	for _, tt := range tests {
		t.Run(tt.connectionID, func(t *testing.T) {
			ref := BuildKeyringRef(tt.connectionID)
			if ref != tt.expected {
				t.Errorf("expected=%s, got=%s", tt.expected, ref)
			}
		})
	}
}

func TestContainsAccessDeniedKeywords(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"access denied", true},
		{"ACCESS DENIED", true},
		{"permission denied", true},
		{"Permission Denied", true},
		{"not authorized", true},
		{"unauthorized", true},
		{"authentication failed", true},
		{"some random error", false},
		{"keyring unavailable", false},
		{"secret not found in keyring", false},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			result := containsAccessDeniedKeywords(tt.msg)
			if result != tt.expected {
				t.Errorf("msg=%s, expected=%v, got=%v", tt.msg, tt.expected, result)
			}
		})
	}
}

func TestWrapKeyringError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{"nil error", nil, nil},
		{"access denied", errors.New("access denied for user"), ErrKeyringAccessDenied},
		{"permission denied", errors.New("permission denied"), ErrKeyringAccessDenied},
		{"not authorized", errors.New("not authorized to access"), ErrKeyringAccessDenied},
		{"other error", errors.New("some other error"), ErrKeyringUnavailable},
		{"unavailable", errors.New("keyring service unavailable"), ErrKeyringUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapKeyringError(tt.err)
			if result != tt.expected {
				t.Errorf("expected=%v, got=%v", tt.expected, result)
			}
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"Access Denied", "access denied", true},
		{"access denied", "ACCESS DENIED", true},
		{"ACCESS DENIED", "access denied", true},
		{"some access denied error", "access denied", true},
		{"access denied", "denied", true},
		{"no match here", "access denied", false},
		{"", "anything", false},
		{"anything", "", true}, // empty substring always matches
	}

	for _, tt := range tests {
		t.Run(tt.s+"/"+tt.substr, func(t *testing.T) {
			result := containsIgnoreCase(tt.s, tt.substr)
			if result != tt.want {
				t.Errorf("s=%s, substr=%s, want=%v, got=%v", tt.s, tt.substr, tt.want, result)
			}
		})
	}
}

func TestMakeLower(t *testing.T) {
	tests := []struct {
		s    string
		want string
	}{
		{"ABC", "abc"},
		{"abc", "abc"},
		{"AbC", "abc"},
		{"Access Denied", "access denied"},
		{"123", "123"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			result := makeLower(tt.s)
			if result != tt.want {
				t.Errorf("expected=%s, got=%s", tt.want, result)
			}
		})
	}
}

// TestKeyringPurger tests the KeyringPurger implementation.
func TestKeyringPurger(t *testing.T) {
	t.Run("nil accessor returns nil", func(t *testing.T) {
		purger := NewKeyringPurger(nil)
		ref := storage.CredentialReference{Provider: "keyring", CredentialRef: "test-ref"}
		err := purger.PurgeCredentialReference(context.Background(), ref)
		if err != nil {
			t.Errorf("expected nil error with nil accessor, got: %v", err)
		}
	})

	t.Run("non-keyring provider returns nil", func(t *testing.T) {
		// 在支持平台上创建 accessor
		accessor, err := NewPlatformKeyringAccessor()
		if err != nil {
			// 非支持平台跳过此测试
			t.Skipf("skipping on unsupported platform: %s", runtime.GOOS)
		}

		purger := NewKeyringPurger(accessor)
		ref := storage.CredentialReference{Provider: "env", CredentialRef: "test-ref"}
		err = purger.PurgeCredentialReference(context.Background(), ref)
		if err != nil {
			t.Errorf("expected nil error for non-keyring provider, got: %v", err)
		}
	})
}

// TestPlatformKeyringAccessorInterface verifies the KeyringAccessor interface compliance.
func TestPlatformKeyringAccessorInterface(t *testing.T) {
	// 编译时检查接口合规性
	var _ KeyringAccessor = (*PlatformKeyringAccessor)(nil)
}

// TestKeyringErrorCodes verifies error code constants are defined.
func TestKeyringErrorCodes(t *testing.T) {
	if CodeKeyringUnavailable != "KEYRING_UNAVAILABLE" {
		t.Errorf("expected KEYRING_UNAVAILABLE, got: %s", CodeKeyringUnavailable)
	}
	if CodeKeyringAccessDenied != "KEYRING_ACCESS_DENIED" {
		t.Errorf("expected KEYRING_ACCESS_DENIED, got: %s", CodeKeyringAccessDenied)
	}
}

// TestErrKeyringConstants verifies error constants are properly defined.
func TestErrKeyringConstants(t *testing.T) {
	if ErrKeyringUnavailable == nil {
		t.Error("ErrKeyringUnavailable should not be nil")
	}
	if ErrKeyringAccessDenied == nil {
		t.Error("ErrKeyringAccessDenied should not be nil")
	}

	// 检查错误消息
	if ErrKeyringUnavailable.Error() != "keyring unavailable" {
		t.Errorf("unexpected error message: %s", ErrKeyringUnavailable.Error())
	}
	if ErrKeyringAccessDenied.Error() != "keyring access denied" {
		t.Errorf("unexpected error message: %s", ErrKeyringAccessDenied.Error())
	}
}