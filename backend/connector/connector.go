// Package connector implements database connection adapters for MySQL, PostgreSQL, and SQLite.
//
// 本包提供统一的数据库连接接口，支持：
// - MySQL (github.com/go-sql-driver/mysql)
// - PostgreSQL (github.com/lib/pq)
// - SQLite (modernc.org/sqlite)
//
// 连接器负责 DSN 拼装、Ping 测试、超时控制和错误归类。
package connector

import (
	"context"
	"database/sql"
	"time"
)

// ErrorCategory 表示连接测试错误的分类，供上层返回结构化错误码。
type ErrorCategory string

const (
	// CategoryNone 表示连接成功，无错误。
	CategoryNone ErrorCategory = ""

	// CategoryAuth 表示认证失败（用户名/密码错误）。
	CategoryAuth ErrorCategory = "AUTH_FAILED"

	// CategoryNetwork 表示网络层不可达或连接拒绝。
	CategoryNetwork ErrorCategory = "NETWORK_ERROR"

	// CategoryTLS 表示 TLS/SSL 协商失败或证书问题。
	CategoryTLS ErrorCategory = "TLS_ERROR"

	// CategoryProtocol 表示数据库协议层错误（版本不兼容、参数错误）。
	CategoryProtocol ErrorCategory = "PROTOCOL_ERROR"

	// CategoryTimeout 表示连接在超时边界内未完成。
	CategoryTimeout ErrorCategory = "TIMEOUT"

	// CategoryUnknown 表示无法归类的其他错误。
	CategoryUnknown ErrorCategory = "UNKNOWN"
)

// ConnectResult 表示连接测试结果。
type ConnectResult struct {
	// Category 为错误分类，成功时为 CategoryNone。
	Category ErrorCategory

	// RawError 为原始驱动错误（用于脱敏处理）。
	RawError error

	// Details 为可选的错误上下文（不含敏感值）。
	Details map[string]string
}

// Connector 定义数据库连接器的统一接口。
type Connector interface {
	// Ping 执行连接可达性测试。
	//
	// 输入：
	// - ctx: 请求上下文，携带超时边界。
	// - params: 连接参数，已注入解析后的凭据。
	//
	// 输出：
	// - ConnectResult: 连接测试结果。
	Ping(ctx context.Context, params ConnectParams) ConnectResult

	// DbType 返回数据库类型标识（mysql/postgres/sqlite）。
	DbType() string
}

// ConnectParams 表示传递给连接器的运行时参数。
type ConnectParams struct {
	// DbType 为数据库类型标识，如 mysql/postgres/sqlite。
	DbType string

	// Host 为数据库主机地址。
	Host string

	// Port 为数据库端口。
	Port int

	// Username 为连接用户名。
	Username string

	// Password 为解析后的明文凭据（运行时使用，不持久化）。
	Password string

	// Database 为目标数据库名。
	Database string

	// Extra 为扩展配置 JSON，可携带 TLS/SSL 等参数。
	Extra string

	// TimeoutSec 为连接超时秒数。
	TimeoutSec int
}

// DriverManager 管理所有已注册的数据库连接器。
type DriverManager struct {
	// connectors 按 db_type 存储连接器实例。
	connectors map[string]Connector
}

// NewDriverManager 创建连接器管理器并注册默认驱动。
func NewDriverManager() *DriverManager {
	m := &DriverManager{
		connectors: make(map[string]Connector),
	}
	m.Register(NewMySQLConnector())
	m.Register(NewPostgresConnector())
	m.Register(NewSQLiteConnector())
	return m
}

// Register 注册指定数据库类型的连接器。
func (m *DriverManager) Register(c Connector) {
	m.connectors[c.DbType()] = c
}

// Get 按数据库类型获取连接器，未注册时返回 nil。
func (m *DriverManager) Get(dbType string) Connector {
	return m.connectors[dbType]
}

// SupportedTypes 返回已注册的数据库类型列表。
func (m *DriverManager) SupportedTypes() []string {
	types := make([]string, 0, len(m.connectors))
	for t := range m.connectors {
		types = append(types, t)
	}
	return types
}

// PingWithTimeout 在指定超时内执行 Ping 测试。
//
// 输入：
// - parent: 父上下文。
// - params: 连接参数。
//
// 输出：
// - ConnectResult: 连接测试结果。
func (m *DriverManager) PingWithTimeout(parent context.Context, params ConnectParams) ConnectResult {
	c := m.Get(params.DbType)
	if c == nil {
		return ConnectResult{
			Category: CategoryProtocol,
			RawError: errUnsupportedDbType,
			Details:  map[string]string{"db_type": params.DbType},
		}
	}

	timeout := params.TimeoutSec
	if timeout <= 0 {
		timeout = 20
	}
	if timeout > 300 {
		timeout = 300
	}

	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()

	result := c.Ping(ctx, params)

	// 若上下文已超时且驱动未返回超时分类，强制设置为超时。
	if ctx.Err() == context.DeadlineExceeded && result.Category != CategoryTimeout {
		result.Category = CategoryTimeout
		result.Details = map[string]string{"timeout_sec": string(rune(timeout))}
	}

	return result
}

var errUnsupportedDbType = &dbTypeError{dbType: "unknown"}

type dbTypeError struct {
	dbType string
}

func (e *dbTypeError) Error() string {
	return "unsupported db_type: " + e.dbType
}

// openAndPing 是通用连接测试辅助函数。
//
// 输入：
// - ctx: 请求上下文。
// - driverName: database/sql 驱动名称。
// - dsn: 数据源名称。
//
// 输出：
// - ConnectResult: 连接测试结果。
func openAndPing(ctx context.Context, driverName, dsn string) ConnectResult {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return ConnectResult{
			Category: CategoryUnknown,
			RawError: err,
		}
	}
	defer db.Close()

	pingErr := db.PingContext(ctx)
	if pingErr == nil {
		return ConnectResult{Category: CategoryNone}
	}

	return classifyError(pingErr)
}

// classifyError 将驱动错误归类为标准错误分类。
func classifyError(err error) ConnectResult {
	if err == nil {
		return ConnectResult{Category: CategoryNone}
	}

	// 超时优先判定（可能来自 context 或驱动超时包装）
	if isTimeoutError(err) {
		return ConnectResult{
			Category: CategoryTimeout,
			RawError: err,
		}
	}

	// TLS 错误判定（优先于认证错误，因为 x509 等关键字可能与 auth 关键字冲突）
	if isTLSError(err) {
		return ConnectResult{
			Category: CategoryTLS,
			RawError: err,
		}
	}

	// 认证错误判定
	if isAuthError(err) {
		return ConnectResult{
			Category: CategoryAuth,
			RawError: err,
		}
	}

	// 网络/连接错误判定
	if isNetworkError(err) {
		return ConnectResult{
			Category: CategoryNetwork,
			RawError: err,
		}
	}

	// 协议错误判定
	if isProtocolError(err) {
		return ConnectResult{
			Category: CategoryProtocol,
			RawError: err,
		}
	}

	// 默认归类为未知
	return ConnectResult{
		Category: CategoryUnknown,
		RawError: err,
	}
}

// isTimeoutError 判断是否为超时相关错误。
func isTimeoutError(err error) bool {
	// 常见超时错误字符串特征
	msg := err.Error()
	return containsAny(msg, "timeout", "deadline exceeded", "context deadline", "i/o timeout")
}

// isAuthError 判断是否为认证失败错误。
func isAuthError(err error) bool {
	msg := err.Error()
	return containsAny(msg, "access denied", "authentication failed", "invalid password",
		"login failed", "auth", "permission denied", "password")
}

// isTLSError 判断是否为 TLS/SSL 相关错误。
func isTLSError(err error) bool {
	msg := err.Error()
	return containsAny(msg, "tls", "ssl", "certificate", "x509", "handshake", "crypto")
}

// isNetworkError 判断是否为网络层错误。
func isNetworkError(err error) bool {
	msg := err.Error()
	return containsAny(msg, "connection refused", "no route to host", "network",
		"dial", "connect", "unreachable", "reset by peer", "broken pipe")
}

// isProtocolError 判断是否为协议层错误。
func isProtocolError(err error) bool {
	msg := err.Error()
	return containsAny(msg, "protocol", "version", "unsupported", "malformed",
		"invalid packet", "handshake", "parameter")
}

// containsAny 检查字符串是否包含任意关键字（大小写不敏感）。
func containsAny(s string, keywords ...string) bool {
	sLower := toLower(s)
	for _, kw := range keywords {
		if contains(sLower, toLower(kw)) {
			return true
		}
	}
	return false
}

// toLower 将字符串转为小写（简化实现，避免导入 strings）。
func toLower(s string) string {
	b := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			b[i] = byte(c + ('a' - 'A'))
		} else {
			b[i] = byte(c)
		}
	}
	return string(b)
}

// contains 检查字符串是否包含子串（简化实现）。
func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}