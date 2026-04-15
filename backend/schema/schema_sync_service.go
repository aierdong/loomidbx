package schema

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	// SchemaSyncErrCodeFailedPrecondition 表示 ApplySchemaSync 前置条件不满足（任务状态/并发冲突等）。
	SchemaSyncErrCodeFailedPrecondition = "FAILED_PRECONDITION"

	// SchemaSyncErrCodeStorageError 表示当前 schema 落库写入失败。
	SchemaSyncErrCodeStorageError = "STORAGE_ERROR"
)

// SchemaSyncRuntimeReader 读取扫描任务运行时上下文（用于 task_id -> connection_id 解析）。
type SchemaSyncRuntimeReader interface {
	// GetRuntimeContext 按 task_id 读取运行时上下文；不存在时返回 ok=false。
	GetRuntimeContext(taskID string) (SchemaScanRuntimeContext, bool)
}

// SchemaSyncPreviewStore 读取待同步的扫描快照（由扫描完成后的预览阶段写入）。
type SchemaSyncPreviewStore interface {
	// LoadPendingSchemaBundle 按 task_id 返回待同步 schema；不存在时返回错误。
	LoadPendingSchemaBundle(ctx context.Context, taskID string) (*CurrentSchemaBundle, error)
}

// ApplySchemaSyncRequest 对齐 design.md 的 ApplySchemaSync(task_id, ack_risk_ids[]) 请求。
type ApplySchemaSyncRequest struct {
	// TaskID 为扫描任务 ID。
	TaskID string

	// AckRiskIDs 为用户已确认处理的阻断风险 ID。
	AckRiskIDs []string
}

// ApplySchemaSyncResult 表示同步动作结果。
type ApplySchemaSyncResult struct {
	// SyncApplied 表示是否已成功落库替换当前 schema。
	SyncApplied bool

	// TrustState 为同步动作结束后的可信度状态。
	TrustState SchemaTrustState
}

// SchemaSyncError 表示 ApplySchemaSync 的结构化错误。
type SchemaSyncError struct {
	// Code 为稳定错误码。
	Code string

	// Message 为脱敏可读描述。
	Message string
}

// Error 实现 error 接口。
func (e *SchemaSyncError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// SchemaSyncConcurrentConflictError 表示并发覆盖冲突（例如乐观锁版本不一致）。
type SchemaSyncConcurrentConflictError struct {
	// Message 为可读冲突说明。
	Message string
}

// Error 实现 error 接口。
func (e *SchemaSyncConcurrentConflictError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// SchemaSyncService 实现 ApplySchemaSync 的事务覆盖与风险闸门语义。
type SchemaSyncService struct {
	// runtimeReader 提供 task_id 的运行时上下文查询。
	runtimeReader SchemaSyncRuntimeReader

	// previewStore 提供 task_id 对应待同步 schema 快照。
	previewStore SchemaSyncPreviewStore

	// currentRepo 负责事务替换当前 schema。
	currentRepo CurrentSchemaRepository

	// trustGate 负责阻断风险校验与可信度状态迁移。
	trustGate SchemaTrustGate
}

// NewSchemaSyncService 构造 SchemaSyncService。
func NewSchemaSyncService(
	runtimeReader SchemaSyncRuntimeReader,
	previewStore SchemaSyncPreviewStore,
	currentRepo CurrentSchemaRepository,
	trustGate SchemaTrustGate,
) *SchemaSyncService {
	return &SchemaSyncService{
		runtimeReader: runtimeReader,
		previewStore:  previewStore,
		currentRepo:   currentRepo,
		trustGate:     trustGate,
	}
}

// ApplySchemaSync 根据 task_id 将预览快照事务性覆盖到当前 schema。
//
// 输入参数：
//   - ctx: 请求上下文。
//   - req: 同步请求（task_id, ack_risk_ids）。
//
// 返回值：
//   - *ApplySchemaSyncResult: 同步结果与最新 trust_state。
//   - *SchemaSyncError: 结构化错误（阻断未处理/前置条件/存储失败）。
func (s *SchemaSyncService) ApplySchemaSync(ctx context.Context, req ApplySchemaSyncRequest) (*ApplySchemaSyncResult, *SchemaSyncError) {
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		return nil, &SchemaSyncError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: "task_id is required",
		}
	}
	runtimeCtx, ok := s.runtimeReader.GetRuntimeContext(taskID)
	if !ok || runtimeCtx.Status != SchemaScanTaskCompleted {
		return nil, &SchemaSyncError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: "schema scan task must be completed before sync",
		}
	}
	connectionID := strings.TrimSpace(runtimeCtx.ConnectionID)
	if connectionID == "" {
		return nil, &SchemaSyncError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: "connection_id is missing in scan task context",
		}
	}
	if err := s.trustGate.CheckBlockingRisksHandled(ctx, connectionID, req.AckRiskIDs); err != nil {
		var gateErr *TrustGatePreconditionError
		if errors.As(err, &gateErr) && gateErr.Code == "BLOCKING_RISK_UNRESOLVED" {
			return &ApplySchemaSyncResult{
				SyncApplied: false,
				TrustState:  SchemaTrustPendingAdjustment,
			}, &SchemaSyncError{
				Code:    gateErr.Code,
				Message: gateErr.Message,
			}
		}
		return nil, &SchemaSyncError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: err.Error(),
		}
	}
	next, err := s.previewStore.LoadPendingSchemaBundle(ctx, taskID)
	if err != nil {
		return nil, &SchemaSyncError{
			Code:    SchemaSyncErrCodeFailedPrecondition,
			Message: "schema preview not ready for sync",
		}
	}
	if err := s.currentRepo.TransactionalReplaceCurrentSchema(ctx, connectionID, next); err != nil {
		var conflictErr *SchemaSyncConcurrentConflictError
		if errors.As(err, &conflictErr) {
			state, viewErr := s.trustGate.GetSchemaTrustState(ctx, connectionID)
			if viewErr != nil {
				return nil, &SchemaSyncError{
					Code:    SchemaSyncErrCodeFailedPrecondition,
					Message: conflictErr.Error(),
				}
			}
			return &ApplySchemaSyncResult{
				SyncApplied: false,
				TrustState:  state.State,
			}, &SchemaSyncError{
				Code:    SchemaSyncErrCodeFailedPrecondition,
				Message: conflictErr.Error(),
			}
		}
		state, viewErr := s.trustGate.GetSchemaTrustState(ctx, connectionID)
		if viewErr != nil {
			return nil, &SchemaSyncError{
				Code:    SchemaSyncErrCodeStorageError,
				Message: "replace current schema failed",
			}
		}
		return &ApplySchemaSyncResult{
			SyncApplied: false,
			TrustState:  state.State,
		}, &SchemaSyncError{
			Code:    SchemaSyncErrCodeStorageError,
			Message: "replace current schema failed",
		}
	}
	nextTrustState, err := s.trustGate.UpdateTrustState(ctx, connectionID, TrustStateUpdateInput{
		HasBlockingRisk: false,
		SyncSucceeded:   true,
		RescanCompleted: true,
	})
	if err != nil {
		return nil, &SchemaSyncError{
			Code:    SchemaSyncErrCodeStorageError,
			Message: "current schema synced but trust state update failed",
		}
	}
	return &ApplySchemaSyncResult{
		SyncApplied: true,
		TrustState:  nextTrustState,
	}, nil
}
