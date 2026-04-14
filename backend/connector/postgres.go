package connector

import (
	"context"
	"fmt"

	_ "github.com/lib/pq"
)

// PostgresConnector 实现 PostgreSQL 数据库连接测试。
type PostgresConnector struct{}

// NewPostgresConnector 创建 PostgreSQL 连接器实例。
func NewPostgresConnector() *PostgresConnector {
	return &PostgresConnector{}
}

// DbType 返回数据库类型标识。
func (c *PostgresConnector) DbType() string {
	return "postgres"
}

// Ping 执行 PostgreSQL 连接可达性测试。
//
// 输入：
// - ctx: 请求上下文。
// - params: 连接参数。
//
// 输出：
// - ConnectResult: 连接测试结果。
func (c *PostgresConnector) Ping(ctx context.Context, params ConnectParams) ConnectResult {
	dsn := c.buildDSN(params)
	return openAndPing(ctx, "postgres", dsn)
}

// buildDSN 构建 PostgreSQL DSN（连接字符串）。
//
// PostgreSQL DSN 格式（lib/pq）：postgres://user:password@host:port/dbname?param1=value1&...
// 或 Key-Value 格式：host=x port=y user=z password=w dbname=n
// 参考：https://github.com/lib/pq#connection-string-parameters
func (c *PostgresConnector) buildDSN(params ConnectParams) string {
	// 基础连接参数
	user := params.Username
	if user == "" {
		user = "postgres"
	}

	pass := params.Password
	host := params.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := params.Port
	if port == 0 {
		port = 5432
	}

	dbName := params.Database
	if dbName == "" {
		dbName = "postgres" // 默认连接到 postgres 系统库进行测试
	}

	// 使用 Key-Value 格式构建 DSN
	// 格式：host=x port=y user=z password=w dbname=n sslmode=disable
	parts := []string{
		fmt.Sprintf("host=%s", c.escapeValue(host)),
		fmt.Sprintf("port=%d", port),
		fmt.Sprintf("user=%s", c.escapeValue(user)),
		fmt.Sprintf("dbname=%s", c.escapeValue(dbName)),
	}

	// 密码处理
	if pass != "" {
		parts = append(parts, fmt.Sprintf("password=%s", c.escapeValue(pass)))
	}

	// 解析 extra JSON 获取 SSL 等扩展参数
	extraParams := c.parseExtraParams(params.Extra)

	// SSL 模式
	sslMode := extraParams.SSLMode
	if sslMode == "" {
		sslMode = "disable" // 默认禁用 SSL，测试时使用简单连接
	}
	parts = append(parts, fmt.Sprintf("sslmode=%s", sslMode))

	// SSL 证书配置
	if extraParams.SSLCert != "" {
		parts = append(parts, fmt.Sprintf("sslcert=%s", c.escapeValue(extraParams.SSLCert)))
	}
	if extraParams.SSLKey != "" {
		parts = append(parts, fmt.Sprintf("sslkey=%s", c.escapeValue(extraParams.SSLKey)))
	}
	if extraParams.SSLRootCert != "" {
		parts = append(parts, fmt.Sprintf("sslrootcert=%s", c.escapeValue(extraParams.SSLRootCert)))
	}

	// 连接超时（可选，由 context 控制）
	// connect_timeout 单位为秒
	// timeoutSec := params.TimeoutSec
	// if timeoutSec > 0 {
	// 	parts = append(parts, fmt.Sprintf("connect_timeout=%d", timeoutSec))
	// }

	return joinParts(parts, " ")
}

// postgresExtraParams 表示 PostgreSQL 扩展配置字段。
type postgresExtraParams struct {
	// SSLMode 控制 SSL 连接模式：disable/allow/prefer/require/verify-ca/verify-full。
	SSLMode string `json:"sslmode"`

	// SSLCert 指定客户端证书文件路径。
	SSLCert string `json:"sslcert"`

	// SSLKey 指定客户端私钥文件路径。
	SSLKey string `json:"sslkey"`

	// SSLRootCert 指定 CA 证书文件路径。
	SSLRootCert string `json:"sslrootcert"`

	// TimeZone 指定连接时区。
	TimeZone string `json:"timezone"`

	// ApplicationName 指定应用名称（用于服务器端日志）。
	ApplicationName string `json:"application_name"`
}

// parseExtraParams 解析 extra JSON 获取 PostgreSQL 扩展参数。
func (c *PostgresConnector) parseExtraParams(extra string) postgresExtraParams {
	if extra == "" {
		return postgresExtraParams{}
	}

	return postgresExtraParams{
		SSLMode:         extractJSONString(extra, "sslmode"),
		SSLCert:         extractJSONString(extra, "sslcert"),
		SSLKey:          extractJSONString(extra, "sslkey"),
		SSLRootCert:     extractJSONString(extra, "sslrootcert"),
		TimeZone:        extractJSONString(extra, "timezone"),
		ApplicationName: extractJSONString(extra, "application_name"),
	}
}

// escapeValue 对 DSN 值进行转义处理（lib/pq 要求）。
//
// 如果值包含空格或特殊字符，需要用单引号包裹。
func (c *PostgresConnector) escapeValue(v string) string {
	if v == "" {
		return ""
	}

	// 检查是否需要转义
	needEscape := false
	for _, ch := range v {
		if ch == ' ' || ch == '\'' || ch == '\\' || ch == '=' || ch == '&' {
			needEscape = true
			break
		}
	}

	if !needEscape {
		return v
	}

	// 单引号包裹并转义内部单引号
	result := "'"
	for _, ch := range v {
		if ch == '\'' {
			result += "\\'"
		} else if ch == '\\' {
			result += "\\\\"
		} else {
			result += string(ch)
		}
	}
	result += "'"
	return result
}

// joinParts 用分隔符连接字符串列表。
func joinParts(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}