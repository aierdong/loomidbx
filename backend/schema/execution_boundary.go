package schema

import (
	"fmt"
	"strings"
)

const (
	// SchemaBoundaryErrCodeOutOfScope 表示请求超出 schema 扫描子系统职责边界。
	SchemaBoundaryErrCodeOutOfScope = "OUT_OF_SCOPE"
)

// ExecutionGateError 表示下游执行准入校验失败的稳定错误。
type ExecutionGateError struct {
	// Code 为稳定错误码，供 spec-03/spec-04 消费。
	Code string

	// Message 为可读且脱敏的错误说明。
	Message string
}

// Error 实现 error 接口。
func (e *ExecutionGateError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", strings.TrimSpace(e.Code), strings.TrimSpace(e.Message))
}

// EnsureExecutionPrecondition 校验是否允许进入后续生成/执行流程。
//
// 输入参数：
//   - state: 连接当前 schema trust_state。
//
// 返回值：
//   - nil: 允许进入下游执行。
//   - *ExecutionGateError: 不允许执行时返回稳定错误码（如阻断风险未处理）。
func EnsureExecutionPrecondition(state SchemaTrustState) *ExecutionGateError {
	switch state {
	case "", SchemaTrustTrusted:
		return nil
	case SchemaTrustPendingAdjustment:
		return &ExecutionGateError{
			Code:    TrustBlockingReasonBlockingRisk,
			Message: "blocking generator compatibility risks are unresolved; adjust generator configuration before execution",
		}
	case SchemaTrustPendingRescan:
		return &ExecutionGateError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: "schema trust state requires rescan before execution",
		}
	default:
		return &ExecutionGateError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: "schema trust state is invalid for execution",
		}
	}
}
