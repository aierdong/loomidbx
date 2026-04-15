package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// SchemaScanErrCodeInvalidArgument 表示 StartSchemaScan / StartSchemaRescan 参数不合法。
	SchemaScanErrCodeInvalidArgument = "INVALID_ARGUMENT"

	// SchemaScanTriggerRescanFull 表示全量重扫任务的触发标记（写入运行时 Trigger）。
	SchemaScanTriggerRescanFull = "rescan_full"

	// SchemaScanTriggerRescanImpacted 表示按受影响表重扫任务的触发标记（写入运行时 Trigger）。
	SchemaScanTriggerRescanImpacted = "rescan_impacted"

	defaultSchemaScanTrigger = "unspecified"
)

// SchemaRescanStrategy 表示 StartSchemaRescan 的策略（full：等价全库扫描；impacted：仅扫描受影响表集合）。
type SchemaRescanStrategy string

const (
	// SchemaRescanStrategyFull 表示全库重扫（映射为 scope=all）。
	SchemaRescanStrategyFull SchemaRescanStrategy = "full"

	// SchemaRescanStrategyImpacted 表示按受影响表集合重扫（映射为 scope=table）。
	SchemaRescanStrategyImpacted SchemaRescanStrategy = "impacted"
)

// StartSchemaScanRequest 对齐 design.md 中 StartSchemaScan(connection_id, scope, table_names, trigger) 的编排入参。
type StartSchemaScanRequest struct {
	// ConnectionID 为目标连接 ID。
	ConnectionID string

	// Scope 为 all 或 table。
	Scope SchemaScanScope

	// TableNames 在 scope=table 时为非空表名列表；scope=all 时必须为空（经去空白后无有效表名）。
	TableNames []string

	// Trigger 为触发来源；空时写入默认占位值以便日志聚合。
	Trigger string
}

// StartSchemaRescanRequest 对齐 design.md 中 StartSchemaRescan；strategy=impacted 时必须提供 ImpactedTableNames。
type StartSchemaRescanRequest struct {
	// ConnectionID 为目标连接 ID。
	ConnectionID string

	// Strategy 为 full 或 impacted。
	Strategy SchemaRescanStrategy

	// Reason 为人类可读的触发原因（必填，脱敏责任在调用方）。
	Reason string

	// ImpactedTableNames 仅在 strategy=impacted 时使用；经去重排序后须非空。
	ImpactedTableNames []string
}

// SchemaScanStartResult 表示成功创建扫描任务后的即时返回（task_id + 初始 status）。
type SchemaScanStartResult struct {
	// TaskID 为新建任务 ID。
	TaskID string

	// Status 为初始运行时状态（当前恒为 running）。
	Status SchemaScanTaskStatus
}

// SchemaScanStartError 表示扫描启动阶段的参数类错误（INVALID_ARGUMENT）。
type SchemaScanStartError struct {
	// Code 为稳定错误码。
	Code string

	// Message 为可读说明，不包含敏感字段。
	Message string
}

// Error 实现 error 接口。
func (e *SchemaScanStartError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// SchemaScanStarter 将 design.md 的 StartSchemaScan / StartSchemaRescan 契约与扫描运行时存储（B02/B03）衔接：
// 负责范围校验、记录触发原因与范围，并返回 task_id 与初始进度状态（running，progress=0）。
type SchemaScanStarter struct {
	// store 为进程内扫描任务运行时存储。
	store *SchemaScanRuntimeStore

	// newTaskID 生成任务 ID；测试可注入固定值。
	newTaskID func() string
}

// NewSchemaScanStarter 创建默认使用 UUID 的任务启动器。
func NewSchemaScanStarter(store *SchemaScanRuntimeStore) *SchemaScanStarter {
	return &SchemaScanStarter{
		store:     store,
		newTaskID: uuid.NewString,
	}
}

// NewSchemaScanStarterWithTaskID 创建可注入任务 ID 生成器的启动器（用于单元测试）。
func NewSchemaScanStarterWithTaskID(store *SchemaScanRuntimeStore, newTaskID func() string) *SchemaScanStarter {
	return &SchemaScanStarter{
		store:     store,
		newTaskID: newTaskID,
	}
}

// StartSchemaScan 校验 scope 与 table_names 组合，创建 running 任务并返回 task_id 与 status。
//
// 输入参数：ctx 预留取消传播；req 为扫描请求。
// 返回值：成功时返回任务标识与初始状态；失败时返回 SchemaScanStartError（参数不合法）。
func (s *SchemaScanStarter) StartSchemaScan(ctx context.Context, req StartSchemaScanRequest) (*SchemaScanStartResult, *SchemaScanStartError) {
	_ = ctx
	if err := validateStartSchemaScan(req); err != nil {
		return nil, err
	}
	taskID := s.newTaskID()
	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = defaultSchemaScanTrigger
	}
	var tables []string
	if req.Scope == SchemaScanScopeTables {
		tables = dedupeAndSort(req.TableNames)
	}
	s.store.StartTask(SchemaScanRuntimeStartRequest{
		TaskID:         taskID,
		ConnectionID:   strings.TrimSpace(req.ConnectionID),
		Scope:          req.Scope,
		TableNames:     tables,
		Trigger:        trigger,
		RescanReason:   "",
		RescanStrategy: "",
	})
	return &SchemaScanStartResult{TaskID: taskID, Status: SchemaScanTaskRunning}, nil
}

// StartSchemaRescan 校验 strategy/reason/受影响表集合，映射为 StartSchemaScan 的 scope 语义并记录 reason。
//
// 输入参数：ctx 预留取消传播；req 为重扫请求。
// 返回值：成功时返回 task_id 与初始 status；失败时返回 SchemaScanStartError。
func (s *SchemaScanStarter) StartSchemaRescan(ctx context.Context, req StartSchemaRescanRequest) (*SchemaScanStartResult, *SchemaScanStartError) {
	_ = ctx
	if err := validateStartSchemaRescan(req); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(req.Reason)
	taskID := s.newTaskID()
	switch req.Strategy {
	case SchemaRescanStrategyFull:
		s.store.StartTask(SchemaScanRuntimeStartRequest{
			TaskID:         taskID,
			ConnectionID:   strings.TrimSpace(req.ConnectionID),
			Scope:          SchemaScanScopeAll,
			TableNames:     nil,
			Trigger:        SchemaScanTriggerRescanFull,
			RescanReason:   reason,
			RescanStrategy: string(SchemaRescanStrategyFull),
		})
	case SchemaRescanStrategyImpacted:
		tables := dedupeAndSort(req.ImpactedTableNames)
		s.store.StartTask(SchemaScanRuntimeStartRequest{
			TaskID:         taskID,
			ConnectionID:   strings.TrimSpace(req.ConnectionID),
			Scope:          SchemaScanScopeTables,
			TableNames:     tables,
			Trigger:        SchemaScanTriggerRescanImpacted,
			RescanReason:   reason,
			RescanStrategy: string(SchemaRescanStrategyImpacted),
		})
	}
	return &SchemaScanStartResult{TaskID: taskID, Status: SchemaScanTaskRunning}, nil
}

func validateStartSchemaScan(req StartSchemaScanRequest) *SchemaScanStartError {
	if strings.TrimSpace(req.ConnectionID) == "" {
		return &SchemaScanStartError{
			Code:    SchemaScanErrCodeInvalidArgument,
			Message: "connection_id is required",
		}
	}
	switch req.Scope {
	case SchemaScanScopeAll:
		if len(dedupeAndSort(req.TableNames)) > 0 {
			return &SchemaScanStartError{
				Code:    SchemaScanErrCodeInvalidArgument,
				Message: "table_names must be empty when scope=all",
			}
		}
	case SchemaScanScopeTables:
		if len(dedupeAndSort(req.TableNames)) == 0 {
			return &SchemaScanStartError{
				Code:    SchemaScanErrCodeInvalidArgument,
				Message: "table_names required when scope=table",
			}
		}
	default:
		return &SchemaScanStartError{
			Code:    SchemaScanErrCodeInvalidArgument,
			Message: "invalid scope",
		}
	}
	return nil
}

func validateStartSchemaRescan(req StartSchemaRescanRequest) *SchemaScanStartError {
	if strings.TrimSpace(req.ConnectionID) == "" {
		return &SchemaScanStartError{
			Code:    SchemaScanErrCodeInvalidArgument,
			Message: "connection_id is required",
		}
	}
	if strings.TrimSpace(req.Reason) == "" {
		return &SchemaScanStartError{
			Code:    SchemaScanErrCodeInvalidArgument,
			Message: "reason is required",
		}
	}
	switch req.Strategy {
	case SchemaRescanStrategyFull:
		return nil
	case SchemaRescanStrategyImpacted:
		if len(dedupeAndSort(req.ImpactedTableNames)) == 0 {
			return &SchemaScanStartError{
				Code:    SchemaScanErrCodeInvalidArgument,
				Message: "impacted_table_names required when strategy=impacted",
			}
		}
		return nil
	default:
		return &SchemaScanStartError{
			Code:    SchemaScanErrCodeInvalidArgument,
			Message: "invalid rescan strategy",
		}
	}
}
