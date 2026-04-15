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

// TrustStateUpdateInput 描述一次状态迁移的输入；编排层在扫描/Diff/风险分析/同步各阶段填入对应信号。
type TrustStateUpdateInput struct {
	// HasBlockingRisk 表示是否存在阻断级生成器兼容性风险。
	HasBlockingRisk bool

	// ConnectionConfigChanged 表示连接配置是否发生需要重扫的变更（驱动、DSN、凭据、目标库等）。
	// 与任意当前状态组合时优先迁移到 pending_rescan（见 design.md 状态迁移表）。
	ConnectionConfigChanged bool

	// RescanCompleted 表示本次扫描/重扫流程已完成，Diff 与风险分析结果已就绪。
	RescanCompleted bool

	// SyncSucceeded 表示当前 schema 已成功落库同步（ApplySchemaSync 成功）。
	SyncSucceeded bool
}

// TrustConnectionMetaRepository 为可信度闸门提供 ldb_connections.extra 中 schema 子域的加载与合并写入。
type TrustConnectionMetaRepository interface {
	// LoadConnectionSchemaMeta 读取并解析连接的 schema 元数据；连接不存在时返回错误。
	LoadConnectionSchemaMeta(ctx context.Context, connectionID string) (ConnectionSchemaMeta, error)

	// PatchConnectionSchemaMeta 将 patch 合并写入 extra，保留无关顶层键。
	PatchConnectionSchemaMeta(ctx context.Context, connectionID string, patch ConnectionSchemaMetaPatch) error
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
