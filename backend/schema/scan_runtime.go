package schema

// SchemaScanTaskStatus 表示扫描任务运行时状态（仅内存上下文，不落库为独立扫描历史表）。
type SchemaScanTaskStatus string

const (
	// SchemaScanTaskRunning 表示扫描任务正在执行。
	SchemaScanTaskRunning SchemaScanTaskStatus = "running"

	// SchemaScanTaskCompleted 表示扫描任务已成功完成并可进入 Diff/预览阶段。
	SchemaScanTaskCompleted SchemaScanTaskStatus = "completed"

	// SchemaScanTaskFailed 表示扫描任务失败（错误码经 FFI 分类输出，不含敏感信息）。
	SchemaScanTaskFailed SchemaScanTaskStatus = "failed"

	// SchemaScanTaskCancelled 表示扫描任务被用户或系统取消。
	SchemaScanTaskCancelled SchemaScanTaskStatus = "cancelled"
)

// SchemaScanScope 表示一次扫描的目标范围（与 StartSchemaScan 的 scope 对齐）。
type SchemaScanScope string

const (
	// SchemaScanScopeAll 表示全库扫描。
	SchemaScanScopeAll SchemaScanScope = "all"

	// SchemaScanScopeTables 表示按表名列表扫描（可退化为单表）。
	SchemaScanScopeTables SchemaScanScope = "table"
)

// SchemaScanRuntimeContext 描述扫描任务运行时上下文：task_id、状态、进度与预览就绪标志仅在进程内持有。
type SchemaScanRuntimeContext struct {
	// TaskID 为扫描任务标识（与 GetSchemaScanStatus 入参一致）。
	TaskID string

	// ConnectionID 为目标连接 ID。
	ConnectionID string

	// Status 为任务生命周期状态。
	Status SchemaScanTaskStatus

	// Progress 为 0–1 的粗粒度进度，供 UI 展示。
	Progress float64

	// PreviewReady 表示是否已可调用 PreviewSchemaDiff。
	PreviewReady bool

	// Scope 为扫描范围枚举。
	Scope SchemaScanScope

	// TableNames 在 Scope=table 时为目标表名列表；全库时通常为空。
	TableNames []string

	// Trigger 为发起来源（用户操作/自动重扫等），仅用于诊断与日志聚合。
	Trigger string

	// StartedAtUnix 为任务开始时间（Unix 秒）。
	StartedAtUnix int64

	// ErrorCode 为失败时的稳定错误码；成功时应为空。
	ErrorCode string
}
