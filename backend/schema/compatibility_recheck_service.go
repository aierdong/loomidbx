package schema

import (
	"context"
	"strings"
	"time"
)

// CompatibilityRecheckService 表示 schema 同步成功后的全量重判定服务。
//
// 该服务用于在 ApplySchemaSync 成功落库后，对连接范围内的既有字段配置进行一次全量兼容性重判定，
// 以便生成不兼容报告供 UI/后续流程消费（对齐 spec-03 design.md 的 CompatibilityRecheckService/RevalidateAllConfigs）。
type CompatibilityRecheckService interface {
	// RevalidateAllConfigs 对指定连接执行全量重判定。
	//
	// 输入：
	// - ctx: 调用上下文。
	// - connectionID: 连接标识。
	//
	// 输出：
	// - CompatibilityReportSnapshot: 重判定报告快照（用于落库与返回契约）。
	// - error: 仅在内部依赖不可用/不可恢复时返回；调用方可将其映射为 failed 状态而非中断主流程。
	RevalidateAllConfigs(ctx context.Context, connectionID string) (CompatibilityReportSnapshot, error)
}

// NoopCompatibilityRecheckService 为当前实现未接入 spec-03 存储/报告链路时提供安全默认值。
type NoopCompatibilityRecheckService struct{}

// RevalidateAllConfigs 为 no-op 实现，返回“无配置跳过”的空报告。
func (s NoopCompatibilityRecheckService) RevalidateAllConfigs(_ context.Context, connectionID string) (CompatibilityReportSnapshot, error) {
	_ = strings.TrimSpace(connectionID)
	return CompatibilityReportSnapshot{
		Status:          CompatibilityRecheckStatusSkippedNoGeneratorConfig,
		GeneratedAtUnix: time.Now().Unix(),
		Summary: CompatibilityReportSummary{
			Mode:          GeneratorCompatibilityModeNoGeneratorConfig,
			TotalRisks:    0,
			BlockingRisks: 0,
		},
		Risks: []GeneratorCompatibilityRisk{},
	}, nil
}

