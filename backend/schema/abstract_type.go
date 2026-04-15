package schema

import "strings"

const (
	// AbstractTypeInt 表示整型抽象类型。
	AbstractTypeInt = "int"
	// AbstractTypeString 表示字符串抽象类型。
	AbstractTypeString = "string"
	// AbstractTypeDecimal 表示小数/浮点抽象类型。
	AbstractTypeDecimal = "decimal"
	// AbstractTypeDatetime 表示日期时间抽象类型。
	AbstractTypeDatetime = "datetime"
	// AbstractTypeBoolean 表示布尔抽象类型。
	AbstractTypeBoolean = "boolean"
)

// ResolveAbstractType 将方言相关的原始类型字符串映射为统一抽象类型。
//
// 约束：未知类型默认回退为 string，避免下游因新类型崩溃。
func ResolveAbstractType(rawType string) string {
	base := strings.ToLower(strings.TrimSpace(rawType))
	if base == "" {
		return AbstractTypeString
	}

	// 去掉括号长度等修饰：varchar(255) -> varchar
	if idx := strings.IndexByte(base, '('); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}

	switch base {
	case "int", "integer", "tinyint", "smallint", "mediumint", "bigint", "serial", "bigserial":
		return AbstractTypeInt
	case "decimal", "numeric", "float", "double", "real", "money":
		return AbstractTypeDecimal
	case "date", "time", "datetime", "timestamp", "timestamptz", "year":
		return AbstractTypeDatetime
	case "bool", "boolean", "bit":
		return AbstractTypeBoolean
	case "char", "varchar", "text", "nchar", "nvarchar", "clob", "string":
		return AbstractTypeString
	default:
		// SQLite 类型亲和性：INTEGER/TEXT/REAL/BLOB/NUMERIC
		if strings.Contains(base, "int") {
			return AbstractTypeInt
		}
		if strings.Contains(base, "char") || strings.Contains(base, "text") || strings.Contains(base, "clob") {
			return AbstractTypeString
		}
		if strings.Contains(base, "real") || strings.Contains(base, "floa") || strings.Contains(base, "doub") || strings.Contains(base, "dec") || strings.Contains(base, "num") {
			return AbstractTypeDecimal
		}
		if strings.Contains(base, "date") || strings.Contains(base, "time") {
			return AbstractTypeDatetime
		}
		if strings.Contains(base, "bool") {
			return AbstractTypeBoolean
		}
		return AbstractTypeString
	}
}

