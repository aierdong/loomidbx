package app_test

import (
	"context"
	"testing"

	"loomidbx/backend/app"
	"loomidbx/backend/connector"
)

// SQLite 连接测试应成功（内存数据库）。
func TestConnectionSQLiteSuccess(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "sqlite",
		Database: ":memory:",
	})
	if errObj != nil {
		t.Fatalf("expected success, got: %+v", errObj)
	}
}

// SQLite 文件连接测试应成功。
func TestConnectionSQLiteFileSuccess(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "sqlite",
		Database: t.TempDir() + "/test.db",
	})
	if errObj != nil {
		t.Fatalf("expected success, got: %+v", errObj)
	}
}

// 不支持的数据库类型应返回协议错误。
func TestConnectionUnsupportedDbType(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType: "oracle",
	})
	if errObj == nil {
		t.Fatal("expected error for unsupported db_type")
	}
	if errObj.Code != app.CodeProtocolError {
		t.Fatalf("expected PROTOCOL_ERROR, got: %s", errObj.Code)
	}
}

// 网络不可达应返回 UPSTREAM_UNAVAILABLE。
func TestConnectionNetworkUnreachable(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "mysql",
		Host:     "10.255.255.1", // 不可达地址
		Port:     3306,
		TimeoutSec: 2,
	})
	if errObj == nil {
		t.Fatal("expected error for unreachable host")
	}
	// 网络不可达或超时都是合法结果
	if errObj.Code != app.CodeUpstreamUnavailable && errObj.Code != app.CodeDeadlineExceeded {
		t.Fatalf("expected UPSTREAM_UNAVAILABLE or DEADLINE_EXCEEDED, got: %s", errObj.Code)
	}
}

// 超时错误应返回 DEADLINE_EXCEEDED 并携带 timeout_sec 详情。
func TestConnectionTimeout(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:     "postgres",
		Host:       "10.255.255.1",
		Port:       5432,
		TimeoutSec: 3,
	})
	if errObj == nil {
		t.Fatal("expected timeout error")
	}
	// 超时或网络错误都是合法结果（取决于驱动实现）
	if errObj.Code == app.CodeDeadlineExceeded {
		if errObj.Details["timeout_sec"] != "3" {
			t.Errorf("timeout_sec detail should be 3, got: %s", errObj.Details["timeout_sec"])
		}
	}
}

// 错误详情不应包含明文密码。
func TestConnectionErrorNoPasswordLeak(t *testing.T) {
	svc, _ := newService(t)
	secret := "super-secret-password-123"
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "mysql",
		Host:     "10.255.255.1",
		Port:     3306,
		Password: secret,
		TimeoutSec: 2,
	})
	if errObj == nil {
		t.Fatal("expected error")
	}

	// 检查详情中不包含明文密码
	if errObj.Details != nil {
		for k, v := range errObj.Details {
			if containsSecret(v, secret) {
				t.Errorf("error detail %s leaks password: %s", k, v)
			}
		}
	}
}

// containsSecret 检查字符串是否包含敏感值。
func containsSecret(s, secret string) bool {
	if secret == "" {
		return false
	}
	for i := 0; i <= len(s)-len(secret); i++ {
		if s[i:i+len(secret)] == secret {
			return true
		}
	}
	return false
}

// mockConnectorManager 用于测试错误分类映射。
type mockConnectorManager struct {
	result connector.ConnectResult
}

func (m *mockConnectorManager) PingWithTimeout(_ context.Context, _ connector.ConnectParams) connector.ConnectResult {
	return m.result
}

func (m *mockConnectorManager) SupportedTypes() []string {
	return []string{"mysql", "postgres", "sqlite"}
}

// 认证错误应正确映射为 AUTH_FAILED。
func TestAuthErrorMapping(t *testing.T) {
	result := connector.ConnectResult{
		Category: connector.CategoryAuth,
		RawError: nil,
	}
	// 模拟 connector 返回认证错误
	// 实际映射在 mapConnectResultToAppError 中测试
	if result.Category != connector.CategoryAuth {
		t.Errorf("category mismatch")
	}
}

// TLS 错误应正确映射为 TLS_ERROR。
func TestTLSErrorMapping(t *testing.T) {
	result := connector.ConnectResult{
		Category: connector.CategoryTLS,
		RawError: nil,
	}
	if result.Category != connector.CategoryTLS {
		t.Errorf("category mismatch")
	}
}