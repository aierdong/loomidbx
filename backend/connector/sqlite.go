package connector

import (
	"context"

	_ "modernc.org/sqlite"
)

// SQLiteConnector 实现 SQLite 数据库连接测试。
type SQLiteConnector struct{}

// NewSQLiteConnector 创建 SQLite 连接器实例。
func NewSQLiteConnector() *SQLiteConnector {
	return &SQLiteConnector{}
}

// DbType 返回数据库类型标识。
func (c *SQLiteConnector) DbType() string {
	return "sqlite"
}

// Ping 执行 SQLite 连接可达性测试。
//
// SQLite 为嵌入式数据库，不需要网络探测。
// 测试逻辑：
// - 若 Database 为空或 ":memory:"，视为可达（临时库）。
// - 若 Database 为文件路径，尝试打开并 Ping。
//
// 输入：
// - ctx: 请求上下文。
// - params: 连接参数，Database 字段为文件路径或 ":memory:"。
//
// 输出：
// - ConnectResult: 连接测试结果。
func (c *SQLiteConnector) Ping(ctx context.Context, params ConnectParams) ConnectResult {
	dsn := c.buildDSN(params)
	return openAndPing(ctx, "sqlite", dsn)
}

// buildDSN 构建 SQLite DSN。
//
// SQLite DSN 格式：文件路径或 ":memory:"，可选查询参数。
// 参考：https://pkg.go.dev/modernc.org/sqlite
func (c *SQLiteConnector) buildDSN(params ConnectParams) string {
	dbName := params.Database
	if dbName == "" {
		dbName = ":memory:" // 默认使用内存数据库进行测试
	}

	// 解析 extra JSON 获取扩展参数
	extraParams := c.parseExtraParams(params.Extra)
	query := c.buildQueryString(extraParams)

	if query != "" {
		return dbName + "?" + query
	}
	return dbName
}

// sqliteExtraParams 表示 SQLite 扩展配置字段。
type sqliteExtraParams struct {
	// Mode 控制数据库打开模式：ro/rw/rwc/memory。
	Mode string `json:"mode"`

	// Cache 控制缓存模式：shared/private。
	Cache string `json:"cache"`

	// BusyTimeout 设置锁等待超时（毫秒）。
	BusyTimeout int `json:"busy_timeout"`

	// JournalMode 设置日志模式：delete/truncate/persist/memory/wal/off.
	JournalMode string `json:"journal_mode"`

	// ForeignKeys 启用外键约束。
	ForeignKeys bool `json:"foreign_keys"`
}

// parseExtraParams 解析 extra JSON 获取 SQLite 扩展参数。
func (c *SQLiteConnector) parseExtraParams(extra string) sqliteExtraParams {
	if extra == "" {
		return sqliteExtraParams{}
	}

	return sqliteExtraParams{
		Mode:         extractJSONString(extra, "mode"),
		Cache:        extractJSONString(extra, "cache"),
		BusyTimeout:  extractJSONInt(extra, "busy_timeout"),
		JournalMode:  extractJSONString(extra, "journal_mode"),
		ForeignKeys:  extractJSONBool(extra, "foreign_keys"),
	}
}

// buildQueryString 构建 SQLite DSN 查询参数字符串。
func (c *SQLiteConnector) buildQueryString(params sqliteExtraParams) string {
	parts := []string{}

	if params.Mode != "" {
		parts = append(parts, "mode="+params.Mode)
	}

	if params.Cache != "" {
		parts = append(parts, "cache="+params.Cache)
	}

	if params.BusyTimeout > 0 {
		parts = append(parts, "_busy_timeout="+itoa(params.BusyTimeout))
	}

	if params.JournalMode != "" {
		parts = append(parts, "_journal_mode="+params.JournalMode)
	}

	if params.ForeignKeys {
		parts = append(parts, "_foreign_keys=1")
	}

	return joinParts(parts, "&")
}

// extractJSONInt 从 JSON 文本中提取指定整数字段值（简化实现）。
func extractJSONInt(jsonText, key string) int {
	pattern := `"` + key + `":`
	startIdx := findPattern(jsonText, pattern)
	if startIdx < 0 {
		return 0
	}

	valueStart := startIdx + len(pattern)

	// 提取数字部分
	numStart := -1
	for i := valueStart; i < len(jsonText); i++ {
		ch := jsonText[i]
		if ch >= '0' && ch <= '9' {
			if numStart < 0 {
				numStart = i
			}
		} else if numStart >= 0 {
			// 数字结束
			return atoi(jsonText[numStart:i])
		}
	}

	if numStart >= 0 {
		return atoi(jsonText[numStart:])
	}

	return 0
}

// itoa 将整数转为字符串（简化实现）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	sign := false
	if n < 0 {
		sign = true
		n = -n
	}

	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if sign {
		return "-" + string(digits)
	}
	return string(digits)
}

// atoi 将字符串转为整数（简化实现）。
func atoi(s string) int {
	result := 0
	sign := 1

	for i, ch := range s {
		if i == 0 && ch == '-' {
			sign = -1
			continue
		}
		if ch >= '0' && ch <= '9' {
			result = result*10 + int(ch-'0')
		}
	}

	return result * sign
}