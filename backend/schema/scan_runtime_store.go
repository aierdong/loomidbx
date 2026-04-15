package schema

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// SchemaScanStatusErrCodeTaskNotFound 表示按 task_id 查询不到运行时任务。
	SchemaScanStatusErrCodeTaskNotFound = "TASK_NOT_FOUND"
)

// SchemaScanStatusError 表示 GetSchemaScanStatus 的结构化错误。
type SchemaScanStatusError struct {
	// Code 为稳定错误码（当前仅 TASK_NOT_FOUND）。
	Code string

	// Message 为可读错误信息，不包含敏感字段。
	Message string
}

// Error 将状态查询错误转换为 error 接口字符串。
func (e *SchemaScanStatusError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// SchemaScanRuntimeStartRequest 描述创建扫描运行时任务所需的输入参数。
type SchemaScanRuntimeStartRequest struct {
	// TaskID 为任务唯一标识；为空时将拒绝创建。
	TaskID string

	// ConnectionID 为目标连接 ID，用于状态隔离与日志关联。
	ConnectionID string

	// Scope 为扫描范围（all/table）。
	Scope SchemaScanScope

	// TableNames 为 Scope=table 时的目标表列表。
	TableNames []string

	// Trigger 为任务触发来源（manual/auto_rescan 等）。
	Trigger string

	// RescanReason 为 StartSchemaRescan 的 reason；非重扫任务为空。
	RescanReason string

	// RescanStrategy 为 full/impacted；非重扫任务为空。
	RescanStrategy string
}

// SchemaScanStatusSnapshot 表示 GetSchemaScanStatus 返回的任务状态快照。
type SchemaScanStatusSnapshot struct {
	// TaskID 为任务标识，和请求入参一一对应。
	TaskID string

	// Status 为运行时状态机值（running/completed/failed/cancelled）。
	Status SchemaScanTaskStatus

	// Progress 为 0-1 区间进度值，超界值会在写入时被裁剪。
	Progress float64

	// PreviewReady 表示预览数据是否就绪。
	PreviewReady bool

	// ErrorCode 为失败状态下的分类错误码。
	ErrorCode string

	// ErrorMessage 为失败状态下的脱敏错误描述。
	ErrorMessage string

	// Scope 为扫描范围（与 StartSchemaScan 的 scope 对齐）。
	Scope SchemaScanScope

	// TableNames 为 Scope=table 时的目标表列表。
	TableNames []string

	// Trigger 为任务触发来源（manual、rescan_full 等）。
	Trigger string

	// RescanReason 为 StartSchemaRescan 的 reason；非重扫任务为空。
	RescanReason string

	// RescanStrategy 为 full/impacted；非重扫任务为空。
	RescanStrategy string
}

// SchemaScanRuntimeStore 管理扫描任务运行时上下文（仅内存，不落独立历史表）。
type SchemaScanRuntimeStore struct {
	// mu 保护 tasks 的并发读写。
	mu sync.RWMutex

	// tasks 保存 task_id 到运行时上下文的映射。
	tasks map[string]SchemaScanRuntimeContext

	// errorMessages 保存失败任务的脱敏错误描述，避免扩展持久模型字段。
	errorMessages map[string]string
}

// NewSchemaScanRuntimeStore 创建仅内存态的扫描任务运行时存储。
func NewSchemaScanRuntimeStore() *SchemaScanRuntimeStore {
	return &SchemaScanRuntimeStore{
		tasks:         make(map[string]SchemaScanRuntimeContext),
		errorMessages: make(map[string]string),
	}
}

// StartTask 以 running 状态创建或覆盖一个运行时任务上下文。
func (s *SchemaScanRuntimeStore) StartTask(req SchemaScanRuntimeStartRequest) {
	if req.TaskID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	s.tasks[req.TaskID] = SchemaScanRuntimeContext{
		TaskID:       req.TaskID,
		ConnectionID: req.ConnectionID,
		Status:       SchemaScanTaskRunning,
		Progress:     0,
		PreviewReady: false,
		Scope:        req.Scope,
		TableNames:   cloneStrings(req.TableNames),
		Trigger:      req.Trigger,
		RescanReason: req.RescanReason,
		RescanStrategy: req.RescanStrategy,
		StartedAtUnix: now,
		ErrorCode:    "",
	}
	delete(s.errorMessages, req.TaskID)
}

// UpdateProgress 更新任务进度，仅在 running 状态生效。
func (s *SchemaScanRuntimeStore) UpdateProgress(taskID string, progress float64) {
	if taskID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, ok := s.tasks[taskID]
	if !ok || ctx.Status != SchemaScanTaskRunning {
		return
	}
	ctx.Progress = clampProgress(progress)
	s.tasks[taskID] = ctx
}

// MarkCompleted 将任务标记为 completed，并将进度置为 1。
func (s *SchemaScanRuntimeStore) MarkCompleted(taskID string, previewReady bool) {
	if taskID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, ok := s.tasks[taskID]
	if !ok {
		return
	}
	ctx.Status = SchemaScanTaskCompleted
	ctx.Progress = 1
	ctx.PreviewReady = previewReady
	ctx.ErrorCode = ""
	s.tasks[taskID] = ctx
	delete(s.errorMessages, taskID)
}

// MarkFailed 将任务标记为 failed，并写入脱敏后的分类错误信息。
func (s *SchemaScanRuntimeStore) MarkFailed(ctx context.Context, taskID string, err error, secrets ...string) {
	if taskID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	taskCtx, ok := s.tasks[taskID]
	if !ok {
		return
	}

	taskCtx.Status = SchemaScanTaskFailed
	taskCtx.PreviewReady = false
	classified := ClassifyUpstreamError(ctx, err, secrets...)
	if classified != nil {
		taskCtx.ErrorCode = classified.Code
		s.errorMessages[taskID] = classified.Message
	}
	s.tasks[taskID] = taskCtx
}

// CancelTask 将任务标记为 cancelled，并清理失败错误信息。
func (s *SchemaScanRuntimeStore) CancelTask(taskID string) {
	if taskID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, ok := s.tasks[taskID]
	if !ok {
		return
	}
	ctx.Status = SchemaScanTaskCancelled
	ctx.PreviewReady = false
	ctx.ErrorCode = ""
	s.tasks[taskID] = ctx
	delete(s.errorMessages, taskID)
}

// GetRuntimeContext 返回任务运行时上下文副本；不存在时 ok=false。
func (s *SchemaScanRuntimeStore) GetRuntimeContext(taskID string) (SchemaScanRuntimeContext, bool) {
	if taskID == "" {
		return SchemaScanRuntimeContext{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, ok := s.tasks[taskID]
	if !ok {
		return SchemaScanRuntimeContext{}, false
	}
	out := ctx
	out.TableNames = cloneStrings(ctx.TableNames)
	return out, true
}

// GetSchemaScanStatus 按 task_id 返回运行时任务状态快照。
func (s *SchemaScanRuntimeStore) GetSchemaScanStatus(taskID string) (SchemaScanStatusSnapshot, *SchemaScanStatusError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ctx, ok := s.tasks[taskID]
	if !ok {
		return SchemaScanStatusSnapshot{}, &SchemaScanStatusError{
			Code:    SchemaScanStatusErrCodeTaskNotFound,
			Message: "schema scan task not found",
		}
	}

	return SchemaScanStatusSnapshot{
		TaskID:       ctx.TaskID,
		Status:       ctx.Status,
		Progress:     ctx.Progress,
		PreviewReady: ctx.PreviewReady,
		ErrorCode:    ctx.ErrorCode,
		ErrorMessage: s.errorMessages[taskID],
		Scope:        ctx.Scope,
		TableNames:   cloneStrings(ctx.TableNames),
		Trigger:      ctx.Trigger,
		RescanReason: ctx.RescanReason,
		RescanStrategy: ctx.RescanStrategy,
	}, nil
}

// clampProgress 将进度裁剪到 0-1 区间，避免 UI 显示异常百分比。
func clampProgress(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// cloneStrings 返回输入切片的浅拷贝，避免调用方后续修改污染运行时上下文。
func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
