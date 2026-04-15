package schema

import "context"

// TrustStateView 为 GetSchemaTrustState / FFI 查询使用的可信度读模型（来源：ldb_connections.extra）。
type TrustStateView struct {
	// State 为当前可信度状态枚举。
	State SchemaTrustState

	// LastBlockingReason 为最近一次阻断原因短码或稳定错误标识；无阻断时为空。
	LastBlockingReason string

	// LastSchemaScanUnix 为最后扫描完成时间（Unix 秒）；0 表示未记录。
	LastSchemaScanUnix int64

	// LastSchemaSyncUnix 为最后成功同步当前 schema 时间（Unix 秒）；0 表示未记录。
	LastSchemaSyncUnix int64
}

// TrustStateUpdateInput 描述一次状态迁移的输入占位；Diff 明细与风险载荷在后续任务中扩展。
type TrustStateUpdateInput struct {
	// HasBlockingRisk 表示是否存在阻断级生成器兼容性风险。
	HasBlockingRisk bool

	// ConnectionConfigChanged 表示连接配置是否发生需要重扫的变更。
	ConnectionConfigChanged bool

	// RescanCompletedNoBlockingRisk 表示重扫完成且分析结果为无阻断风险（与同步成功组合用于恢复 trusted）。
	RescanCompletedNoBlockingRisk bool

	// SyncSucceeded 表示当前 schema 已成功落库同步。
	SyncSucceeded bool
}

// SchemaTrustGate 维护 trusted/pending_rescan/pending_adjustment 状态机，并对阻断风险做准入控制。
type SchemaTrustGate interface {
	// GetSchemaTrustState 返回连接当前可信度与 extra 中的扫描/同步元数据摘要。
	GetSchemaTrustState(ctx context.Context, connectionID string) (TrustStateView, error)

	// UpdateTrustState 基于扫描/Diff/风险结果迁移状态并持久化到连接 extra。
	UpdateTrustState(ctx context.Context, connectionID string, in TrustStateUpdateInput) (SchemaTrustState, error)

	// CheckBlockingRisksHandled 在 ApplySchemaSync 前校验用户是否已确认处理阻断级风险。
	CheckBlockingRisksHandled(ctx context.Context, connectionID string, acknowledgedRiskIDs []string) error
}
