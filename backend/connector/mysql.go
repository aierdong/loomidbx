package connector

import (
	"context"
	"fmt"
	"net/url"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLConnector 实现 MySQL 数据库连接测试。
type MySQLConnector struct{}

// NewMySQLConnector 创建 MySQL 连接器实例。
func NewMySQLConnector() *MySQLConnector {
	return &MySQLConnector{}
}

// DbType 返回数据库类型标识。
func (c *MySQLConnector) DbType() string {
	return "mysql"
}

// Ping 执行 MySQL 连接可达性测试。
//
// 输入：
// - ctx: 请求上下文。
// - params: 连接参数。
//
// 输出：
// - ConnectResult: 连接测试结果。
func (c *MySQLConnector) Ping(ctx context.Context, params ConnectParams) ConnectResult {
	dsn := c.buildDSN(params)
	return openAndPing(ctx, "mysql", dsn)
}

// buildDSN 构建 MySQL DSN（Data Source Name）。
//
// MySQL DSN 格式：[username[:password]@][protocol[(address)]]/dbname[?param1=value1&...]
// 参考：https://github.com/go-sql-driver/mysql#dsn-data-source-name
func (c *MySQLConnector) buildDSN(params ConnectParams) string {
	// 基础连接参数
	user := params.Username
	if user == "" {
		user = "root"
	}

	pass := params.Password
	host := params.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := params.Port
	if port == 0 {
		port = 3306
	}

	dbName := params.Database
	if dbName == "" {
		dbName = "mysql" // 默认连接到 mysql 系统库进行测试
	}

	// 构建 DSN
	// 格式：username:password@protocol(host:port)/dbname?params
	addr := fmt.Sprintf("%s:%d", host, port)

	// 密码需要 URL 编码
	encodedPass := url.QueryEscape(pass)

	base := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, encodedPass, addr, dbName)

	// 解析 extra JSON 获取 TLS 等扩展参数
	extraParams := c.parseExtraParams(params.Extra)
	query := c.buildQueryString(extraParams)

	if query != "" {
		return base + "?" + query
	}
	return base
}

// mysqlExtraParams 表示 MySQL 扩展配置字段。
type mysqlExtraParams struct {
	// TLSMode 控制 TLS 连接模式：false/true/skip-verify/custom。
	TLSMode string `json:"tls"`

	// Charset 指定连接字符集。
	Charset string `json:"charset"`

	// Collation 指定连接排序规则。
	Collation string `json:"collation"`

	// AllowNativePassword 允许使用原生密码认证。
	AllowNativePassword bool `json:"allow_native_password"`

	// AllowOldPassword 允许使用旧密码认证（MySQL 3.23 格式）。
	AllowOldPassword bool `json:"allow_old_password"`
}

// parseExtraParams 解析 extra JSON 获取 MySQL 扩展参数。
func (c *MySQLConnector) parseExtraParams(extra string) mysqlExtraParams {
	if extra == "" {
		return mysqlExtraParams{}
	}

	// 简化 JSON 解析，避免导入 encoding/json（由 connector.go 外层处理）
	// 此处使用默认值
	return mysqlExtraParams{
		TLSMode:             extractJSONString(extra, "tls"),
		Charset:             extractJSONString(extra, "charset"),
		Collation:           extractJSONString(extra, "collation"),
		AllowNativePassword: extractJSONBool(extra, "allow_native_password"),
		AllowOldPassword:    extractJSONBool(extra, "allow_old_password"),
	}
}

// buildQueryString 构建 MySQL DSN 查询参数字符串。
func (c *MySQLConnector) buildQueryString(params mysqlExtraParams) string {
	values := make(url.Values)

	// 超时参数（由 PingWithTimeout 通过 context 控制，DSN 中可选）
	// values.Set("timeout", "10s")

	// TLS 配置
	if params.TLSMode != "" {
		values.Set("tls", params.TLSMode)
	}

	// 字符集
	if params.Charset != "" {
		values.Set("charset", params.Charset)
	}

	// 排序规则
	if params.Collation != "" {
		values.Set("collation", params.Collation)
	}

	// 密码认证模式
	if params.AllowNativePassword {
		values.Set("allowNativePasswords", "true")
	}
	if params.AllowOldPassword {
		values.Set("allowOldPasswords", "true")
	}

	return values.Encode()
}

// extractJSONString 从 JSON 文本中提取指定字段值（简化实现）。
func extractJSONString(jsonText, key string) string {
	// 简化的 JSON 字段提取：查找 "key":"value" 模式
	pattern := fmt.Sprintf(`"%s":"`, key)
	startIdx := findPattern(jsonText, pattern)
	if startIdx < 0 {
		return ""
	}

	valueStart := startIdx + len(pattern)
	valueEnd := findChar(jsonText, '"', valueStart)
	if valueEnd < 0 {
		return ""
	}

	return jsonText[valueStart:valueEnd]
}

// extractJSONBool 从 JSON 文本中提取指定布尔字段值（简化实现）。
func extractJSONBool(jsonText, key string) bool {
	pattern := fmt.Sprintf(`"%s":`, key)
	startIdx := findPattern(jsonText, pattern)
	if startIdx < 0 {
		return false
	}

	valueStart := startIdx + len(pattern)
	// 查找 true 或 false
	if jsonText[valueStart:valueStart+4] == "true" {
		return true
	}
	return false
}

// findPattern 在字符串中查找子串位置。
func findPattern(s, pattern string) int {
	for i := 0; i <= len(s)-len(pattern); i++ {
		if s[i:i+len(pattern)] == pattern {
			return i
		}
	}
	return -1
}

// findChar 从指定位置开始查找字符位置。
func findChar(s string, c byte, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}