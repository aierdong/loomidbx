package schema

import (
	"context"
	"fmt"
)

// trustGateImpl 实现 SchemaTrustGate，将状态迁移持久化到连接 extra。
type trustGateImpl struct {
	// repo 提供连接 schema 子域元数据的加载与合并写入。
	repo TrustConnectionMetaRepository
}

// NewSchemaTrustGate 构造基于仓储的 SchemaTrustGate 实现。
//
// 输入参数：
//   - repo: 连接 extra 中 schema 元数据读写接口。
//
// 返回值：SchemaTrustGate 实例。
func NewSchemaTrustGate(repo TrustConnectionMetaRepository) SchemaTrustGate {
	return &trustGateImpl{repo: repo}
}

// GetSchemaTrustState 返回连接当前可信度与 extra 中的扫描/同步元数据摘要。
//
// 输入参数：
//   - ctx: 请求上下文。
//   - connectionID: 连接 ID。
//
// 返回值：
//   - TrustStateView: 读模型。
//   - error: 连接不存在或解析失败时返回错误。
func (g *trustGateImpl) GetSchemaTrustState(ctx context.Context, connectionID string) (TrustStateView, error) {
	meta, err := g.repo.LoadConnectionSchemaMeta(ctx, connectionID)
	if err != nil {
		return TrustStateView{}, err
	}
	st := meta.SchemaTrustState
	if st == "" {
		st = SchemaTrustTrusted
	}
	return TrustStateView{
		State:                st,
		LastBlockingReason:   meta.SchemaLastBlockingReason,
		LastSchemaScanUnix:   meta.LastSchemaScanUnix,
		LastSchemaSyncUnix:   meta.LastSchemaSyncUnix,
		CompatibilityReport:  meta.CompatibilityReport,
	}, nil
}

// UpdateTrustState 基于扫描/Diff/风险结果迁移状态并持久化到连接 extra。
//
// 输入参数：
//   - ctx: 请求上下文。
//   - connectionID: 连接 ID。
//   - in: 状态迁移输入信号。
//
// 返回值：
//   - SchemaTrustState: 迁移后的状态（与持久化一致）。
//   - error: 加载或写入失败时返回错误。
func (g *trustGateImpl) UpdateTrustState(ctx context.Context, connectionID string, in TrustStateUpdateInput) (SchemaTrustState, error) {
	meta, err := g.repo.LoadConnectionSchemaMeta(ctx, connectionID)
	if err != nil {
		return "", err
	}
	current := meta.SchemaTrustState
	if current == "" {
		current = SchemaTrustTrusted
	}
	next := computeNextTrustState(current, in)
	if next == current {
		return next, nil
	}
	reason := blockingReasonForTrustState(next)
	patch := ConnectionSchemaMetaPatch{
		TrustState:         &next,
		LastBlockingReason: &reason,
	}
	if err := g.repo.PatchConnectionSchemaMeta(ctx, connectionID, patch); err != nil {
		return "", err
	}
	return next, nil
}

// CheckBlockingRisksHandled 在 ApplySchemaSync 前校验 pending_adjustment 下是否已确认阻断风险。
//
// 输入参数：
//   - ctx: 请求上下文。
//   - connectionID: 连接 ID。
//   - acknowledgedRiskIDs: 用户已确认的阻断风险 ID 列表。
//
// 返回值：
//   - error: 处于 pending_adjustment 且未提供任何确认时返回 TrustGatePreconditionError（BLOCKING_RISK_UNRESOLVED）；其它状态为 nil。
func (g *trustGateImpl) CheckBlockingRisksHandled(ctx context.Context, connectionID string, acknowledgedRiskIDs []string) error {
	view, err := g.GetSchemaTrustState(ctx, connectionID)
	if err != nil {
		return err
	}
	if view.State != SchemaTrustPendingAdjustment {
		return nil
	}
	if len(acknowledgedRiskIDs) == 0 {
		return &TrustGatePreconditionError{
			Code:    "BLOCKING_RISK_UNRESOLVED",
			Message: "blocking generator compatibility risks must be acknowledged before schema sync",
		}
	}
	return nil
}

// TrustGatePreconditionError 表示可信度闸门前置条件不满足（如同步前未处理阻断风险）。
type TrustGatePreconditionError struct {
	// Code 为稳定错误码，供 FFI 与 spec-03/spec-04 消费。
	Code string

	// Message 为脱敏可读说明。
	Message string
}

// Error 实现 error 接口。
func (e *TrustGatePreconditionError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
