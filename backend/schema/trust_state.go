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

// TrustBlockingReasonPendingRescan 为进入 pending_rescan 时写入 extra 的稳定原因短码（供 FFI/UI 分支）。
const TrustBlockingReasonPendingRescan = "PENDING_RESCAN"

// TrustBlockingReasonBlockingRisk 为进入 pending_adjustment 时写入 extra 的稳定原因短码（与 BLOCKING_RISK_UNRESOLVED 对齐）。
const TrustBlockingReasonBlockingRisk = "BLOCKING_RISK_UNRESOLVED"

// SchemaTrustAllowsDownstreamExecution 判定是否允许进入下游生成执行流程；仅 trusted 为 true（与 design.md 及 spec-4.3 衔接）。
func SchemaTrustAllowsDownstreamExecution(state SchemaTrustState) bool {
	if state == "" {
		return true
	}
	return state == SchemaTrustTrusted
}

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
