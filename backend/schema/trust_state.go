package schema

import (
	"fmt"
	"strings"
)

// SchemaTrustState 表示连接维度 schema 可信度状态机取值（持久化于 ldb_connections.extra）。
type SchemaTrustState string

const (
	// SchemaTrustTrusted 表示当前 schema 与生成器配置兼容，可继续下游执行。
	SchemaTrustTrusted SchemaTrustState = "trusted"

	// SchemaTrustPendingRescan 表示连接或环境变化后需重新扫描后方可恢复可信。
	SchemaTrustPendingRescan SchemaTrustState = "pending_rescan"

	// SchemaTrustPendingAdjustment 表示存在阻断级兼容性风险，需先调整生成器配置。
	SchemaTrustPendingAdjustment SchemaTrustState = "pending_adjustment"
)

// ParseSchemaTrustState 将外部字符串解析为 SchemaTrustState。
//
// 输入：
// - s: JSON 或 FFI 传入的枚举字符串。
//
// 输出：
// - SchemaTrustState: 解析成功时的枚举值。
// - error: 未知取值时返回错误。
func ParseSchemaTrustState(s string) (SchemaTrustState, error) {
	switch strings.TrimSpace(s) {
	case string(SchemaTrustTrusted):
		return SchemaTrustTrusted, nil
	case string(SchemaTrustPendingRescan):
		return SchemaTrustPendingRescan, nil
	case string(SchemaTrustPendingAdjustment):
		return SchemaTrustPendingAdjustment, nil
	case "":
		return SchemaTrustTrusted, nil
	default:
		return "", fmt.Errorf("unknown schema trust state: %q", s)
	}
}
