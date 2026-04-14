package connector

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// MySQL DSN 应正确构建基础参数。
func TestMySQLBuildDSN_Basic(t *testing.T) {
	c := NewMySQLConnector()
	params := ConnectParams{
		Host:     "localhost",
		Port:     3306,
		Username: "testuser",
		Password: "testpass",
		Database: "testdb",
	}
	dsn := c.buildDSN(params)

	if !strings.Contains(dsn, "testuser") {
		t.Errorf("dsn missing username: %s", dsn)
	}
	if !strings.Contains(dsn, "localhost:3306") {
		t.Errorf("dsn missing host:port: %s", dsn)
	}
	if !strings.Contains(dsn, "testdb") {
		t.Errorf("dsn missing database: %s", dsn)
	}
}

// MySQL DSN 应正确处理空参数默认值。
func TestMySQLBuildDSN_Defaults(t *testing.T) {
	c := NewMySQLConnector()
	params := ConnectParams{}
	dsn := c.buildDSN(params)

	// 默认值
	if !strings.Contains(dsn, "root") {
		t.Errorf("dsn should use root as default user: %s", dsn)
	}
	if !strings.Contains(dsn, "127.0.0.1:3306") {
		t.Errorf("dsn should use 127.0.0.1:3306 as default: %s", dsn)
	}
	if !strings.Contains(dsn, "/mysql") {
		t.Errorf("dsn should use mysql as default db: %s", dsn)
	}
}

// PostgreSQL DSN 应正确构建 Key-Value 格式。
func TestPostgresBuildDSN_Basic(t *testing.T) {
	c := NewPostgresConnector()
	params := ConnectParams{
		Host:     "pgserver",
		Port:     5432,
		Username: "pguser",
		Password: "pgpass",
		Database: "pgdb",
	}
	dsn := c.buildDSN(params)

	if !strings.Contains(dsn, "host=pgserver") {
		t.Errorf("dsn missing host: %s", dsn)
	}
	if !strings.Contains(dsn, "port=5432") {
		t.Errorf("dsn missing port: %s", dsn)
	}
	if !strings.Contains(dsn, "user=pguser") {
		t.Errorf("dsn missing user: %s", dsn)
	}
	if !strings.Contains(dsn, "dbname=pgdb") {
		t.Errorf("dsn missing dbname: %s", dsn)
	}
}

// PostgreSQL DSN 应默认禁用 SSL。
func TestPostgresBuildDSN_SSLMode(t *testing.T) {
	c := NewPostgresConnector()
	params := ConnectParams{}
	dsn := c.buildDSN(params)

	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("dsn should have sslmode=disable by default: %s", dsn)
	}
}

// SQLite DSN 应正确处理内存数据库。
func TestSQLiteBuildDSN_Memory(t *testing.T) {
	c := NewSQLiteConnector()
	params := ConnectParams{
		Database: ":memory:",
	}
	dsn := c.buildDSN(params)

	if dsn != ":memory:" {
		t.Errorf("dsn for memory db should be ':memory:', got: %s", dsn)
	}
}

// SQLite DSN 应正确处理空数据库参数默认值。
func TestSQLiteBuildDSN_Default(t *testing.T) {
	c := NewSQLiteConnector()
	params := ConnectParams{}
	dsn := c.buildDSN(params)

	if dsn != ":memory:" {
		t.Errorf("dsn should default to ':memory:', got: %s", dsn)
	}
}

// 错误分类应正确识别超时错误。
func TestClassifyError_Timeout(t *testing.T) {
	err := errors.New("dial tcp 127.0.0.1:3306: i/o timeout")
	result := classifyError(err)

	if result.Category != CategoryTimeout {
		t.Errorf("expected TIMEOUT category, got: %s", result.Category)
	}
}

// 错误分类应正确识别认证错误。
func TestClassifyError_Auth(t *testing.T) {
	err := errors.New("Error 1045: Access denied for user 'root'@'localhost'")
	result := classifyError(err)

	if result.Category != CategoryAuth {
		t.Errorf("expected AUTH_FAILED category, got: %s", result.Category)
	}
}

// 错误分类应正确识别 TLS 错误。
func TestClassifyError_TLS(t *testing.T) {
	err := errors.New("x509: certificate signed by unknown authority")
	result := classifyError(err)

	if result.Category != CategoryTLS {
		t.Errorf("expected TLS_ERROR category, got: %s", result.Category)
	}
}

// 错误分类应正确识别网络错误。
func TestClassifyError_Network(t *testing.T) {
	err := errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")
	result := classifyError(err)

	if result.Category != CategoryNetwork {
		t.Errorf("expected NETWORK_ERROR category, got: %s", result.Category)
	}
}

// DriverManager 应注册所有默认驱动。
func TestDriverManager_Registration(t *testing.T) {
	m := NewDriverManager()

	types := m.SupportedTypes()
	if len(types) != 3 {
		t.Errorf("expected 3 registered types, got: %v", types)
	}

	if m.Get("mysql") == nil {
		t.Error("mysql connector not registered")
	}
	if m.Get("postgres") == nil {
		t.Error("postgres connector not registered")
	}
	if m.Get("sqlite") == nil {
		t.Error("sqlite connector not registered")
	}
}

// DriverManager PingWithTimeout 应对不支持的 db_type 返回协议错误。
func TestDriverManager_UnsupportedDbType(t *testing.T) {
	m := NewDriverManager()
	params := ConnectParams{
		DbType:   "oracle",
		Host:     "localhost",
		Port:     1521,
		Username: "user",
		Password: "pass",
	}

	result := m.PingWithTimeout(context.Background(), params)

	if result.Category != CategoryProtocol {
		t.Errorf("expected PROTOCOL_ERROR for unsupported db_type, got: %s", result.Category)
	}
}

// SQLite 连接测试应成功（内存数据库）。
func TestSQLitePing_Success(t *testing.T) {
	c := NewSQLiteConnector()
	params := ConnectParams{
		Database: ":memory:",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := c.Ping(ctx, params)

	if result.Category != CategoryNone {
		t.Errorf("expected successful ping, got category: %s, error: %v", result.Category, result.RawError)
	}
}

// SQLite 文件路径连接测试应成功。
func TestSQLitePing_FileSuccess(t *testing.T) {
	c := NewSQLiteConnector()
	params := ConnectParams{
		Database: t.TempDir() + "/test.db",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := c.Ping(ctx, params)

	if result.Category != CategoryNone {
		t.Errorf("expected successful ping, got category: %s, error: %v", result.Category, result.RawError)
	}
}

// 超时边界应生效。
func TestTimeoutEnforced(t *testing.T) {
	m := NewDriverManager()
	params := ConnectParams{
		DbType:     "mysql",
		Host:       "10.255.255.1", // 不可达地址
		Port:       3306,
		TimeoutSec: 2, // 2秒超时
	}

	ctx := context.Background()
	start := time.Now()
	result := m.PingWithTimeout(ctx, params)
 elapsed := time.Since(start)

	// 应在超时边界内完成（允许一定误差）
	if elapsed > 3*time.Second {
		t.Errorf("ping took longer than expected timeout: %v", elapsed)
	}

	if result.Category != CategoryTimeout && result.Category != CategoryNetwork {
		t.Errorf("expected TIMEOUT or NETWORK_ERROR for unreachable host, got: %s", result.Category)
	}
}