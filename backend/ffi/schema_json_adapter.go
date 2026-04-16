package ffi

import (
	"encoding/json"
	"fmt"
	"strings"

	"loomidbx/schema"
)

const (
	// schemaFFIErrCodeFailedPrecondition 为 FFI 层统一前置条件错误码。
	schemaFFIErrCodeFailedPrecondition = "FAILED_PRECONDITION"
)

// SchemaScanStarter 定义 StartSchemaScan/StartSchemaRescan 的最小依赖接口。
type SchemaScanStarter interface {
	// StartSchemaScan 启动扫描任务并返回任务创建结果。
	StartSchemaScan(connectionID string, scope schema.SchemaScanScope, tableNames []string, trigger string) (*schema.SchemaScanStartResult, *schema.SchemaScanStartError)
	// StartSchemaRescan 启动重扫任务并返回任务创建结果。
	StartSchemaRescan(connectionID string, strategy schema.SchemaRescanStrategy, reason string, impactedTableNames []string) (*schema.SchemaScanStartResult, *schema.SchemaScanStartError)
}

// SchemaScanStatusReader 定义 GetSchemaScanStatus 的读取依赖。
type SchemaScanStatusReader interface {
	// GetSchemaScanStatus 按 task_id 返回扫描运行时状态。
	GetSchemaScanStatus(taskID string) (schema.SchemaScanStatusSnapshot, *schema.SchemaScanStatusError)
}

// SchemaDiffPreviewer 定义 PreviewSchemaDiff 的读取依赖。
type SchemaDiffPreviewer interface {
	// PreviewSchemaDiff 返回给定任务的 Diff 预览。
	PreviewSchemaDiff(taskID string) (*schema.SchemaDiffResult, *schema.SchemaDiffError)
}

// GeneratorRiskReader 定义 GetGeneratorCompatibilityRisks 的读取依赖。
type GeneratorRiskReader interface {
	// GetGeneratorCompatibilityRisks 返回风险模式与风险列表。
	GetGeneratorCompatibilityRisks(taskID string) (schema.GeneratorCompatibilityRisksResult, error)
}

// SchemaSyncer 定义 ApplySchemaSync 的执行依赖。
type SchemaSyncer interface {
	// ApplySchemaSync 按 task_id 执行同步动作。
	ApplySchemaSync(taskID string, ackRiskIDs []string) (*schema.ApplySchemaSyncResult, *schema.SchemaSyncError)
}

// CurrentSchemaReader 定义 GetCurrentSchema 的读取依赖。
type CurrentSchemaReader interface {
	// GetCurrentSchema 返回当前持久化 schema。
	GetCurrentSchema(connectionID string, scope string) (*schema.CurrentSchemaBundle, error)
}

// TrustStateReader 定义 GetSchemaTrustState 的读取依赖。
type TrustStateReader interface {
	// GetSchemaTrustState 返回连接可信度状态视图。
	GetSchemaTrustState(connectionID string) (schema.TrustStateView, error)
}

// SchemaFFIDependencies 汇总 schema FFI 适配器依赖。
type SchemaFFIDependencies struct {
	// Starter 提供扫描与重扫启动能力。
	Starter SchemaScanStarter
	// StatusReader 提供扫描状态读取能力。
	StatusReader SchemaScanStatusReader
	// Previewer 提供 Diff 预览能力。
	Previewer SchemaDiffPreviewer
	// RiskReader 提供兼容性风险读取能力。
	RiskReader GeneratorRiskReader
	// Syncer 提供 schema 同步能力。
	Syncer SchemaSyncer
	// CurrentReader 提供当前 schema 读取能力。
	CurrentReader CurrentSchemaReader
	// TrustReader 提供信任状态读取能力。
	TrustReader TrustStateReader
}

// SchemaFFIAdapter 为 schema 相关 FFI JSON 契约适配器。
type SchemaFFIAdapter struct {
	// deps 为各接口方法依赖。
	deps SchemaFFIDependencies
}

// NewSchemaFFIAdapter 创建 schema FFI 适配器。
func NewSchemaFFIAdapter(deps SchemaFFIDependencies) *SchemaFFIAdapter {
	return &SchemaFFIAdapter{deps: deps}
}

// StartSchemaScan 执行 StartSchemaScan 的 JSON 适配。
func (a *SchemaFFIAdapter) StartSchemaScan(reqJSON string) string {
	var req struct {
		ConnectionID string   `json:"connection_id"`
		Scope        string   `json:"scope"`
		TableNames   []string `json:"table_names"`
		Trigger      string   `json:"trigger"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.Starter == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "schema starter is not configured"))
	}
	result, startErr := a.deps.Starter.StartSchemaScan(
		strings.TrimSpace(req.ConnectionID),
		schema.SchemaScanScope(strings.TrimSpace(req.Scope)),
		req.TableNames,
		strings.TrimSpace(req.Trigger),
	)
	if startErr != nil {
		return marshalResponse(ffiSchemaError(startErr.Code, startErr.Message))
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: result})
}

// GetSchemaScanStatus 执行 GetSchemaScanStatus 的 JSON 适配。
func (a *SchemaFFIAdapter) GetSchemaScanStatus(reqJSON string) string {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.StatusReader == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "schema status reader is not configured"))
	}
	snapshot, statusErr := a.deps.StatusReader.GetSchemaScanStatus(strings.TrimSpace(req.TaskID))
	if statusErr != nil {
		return marshalResponse(ffiSchemaError(mapSchemaFFIErrorCode(statusErr.Code), statusErr.Message))
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: snapshot})
}

// PreviewSchemaDiff 执行 PreviewSchemaDiff 的 JSON 适配。
func (a *SchemaFFIAdapter) PreviewSchemaDiff(reqJSON string) string {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.Previewer == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "schema diff previewer is not configured"))
	}
	result, previewErr := a.deps.Previewer.PreviewSchemaDiff(strings.TrimSpace(req.TaskID))
	if previewErr != nil {
		return marshalResponse(ffiSchemaError(previewErr.Code, previewErr.Message))
	}
	riskData := map[string]interface{}{
		"mode":  string(schema.GeneratorCompatibilityModeNoGeneratorConfig),
		"risks": []schema.GeneratorCompatibilityRisk{},
	}
	if a.deps.RiskReader != nil {
		risks, riskErr := a.deps.RiskReader.GetGeneratorCompatibilityRisks(strings.TrimSpace(req.TaskID))
		if riskErr != nil {
			return marshalResponse(ffiSchemaError(schemaFFIErrCodeFailedPrecondition, riskErr.Error()))
		}
		if risks.Risks == nil {
			risks.Risks = []schema.GeneratorCompatibilityRisk{}
		}
		riskData = map[string]interface{}{
			"mode":  risks.Mode,
			"risks": risks.Risks,
		}
	}
	blockingCount := 0
	if rawRisks, ok := riskData["risks"].([]schema.GeneratorCompatibilityRisk); ok {
		for _, r := range rawRisks {
			if r.Severity == schema.GeneratorCompatibilityRiskSeverityBlocking {
				blockingCount++
			}
		}
	}
	payload := map[string]interface{}{
		"diff":  result,
		"risk":  riskData,
		"action": map[string]interface{}{
			"can_view_diff":                    true,
			"can_view_risks":                   true,
			"can_apply_sync":                   blockingCount == 0,
			"requires_adjustment_before_sync":  blockingCount > 0,
		},
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: payload})
}

// GetGeneratorCompatibilityRisks 执行 GetGeneratorCompatibilityRisks 的 JSON 适配。
func (a *SchemaFFIAdapter) GetGeneratorCompatibilityRisks(reqJSON string) string {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.RiskReader == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "generator risk reader is not configured"))
	}
	result, riskErr := a.deps.RiskReader.GetGeneratorCompatibilityRisks(strings.TrimSpace(req.TaskID))
	if riskErr != nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", riskErr.Error()))
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: result})
}

// ApplySchemaSync 执行 ApplySchemaSync 的 JSON 适配。
func (a *SchemaFFIAdapter) ApplySchemaSync(reqJSON string) string {
	var req struct {
		TaskID     string   `json:"task_id"`
		AckRiskIDs []string `json:"ack_risk_ids"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.Syncer == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "schema syncer is not configured"))
	}
	result, syncErr := a.deps.Syncer.ApplySchemaSync(strings.TrimSpace(req.TaskID), req.AckRiskIDs)
	if syncErr != nil {
		return marshalResponse(ffiSchemaError(syncErr.Code, syncErr.Message))
	}
	if result == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "schema syncer returned empty result"))
	}
	payload := map[string]interface{}{
		"sync_applied":          result.SyncApplied,
		"trust_state":           result.TrustState,
		"compatibility_recheck": result.CompatibilityRecheck,
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: payload})
}

// StartSchemaRescan 执行 StartSchemaRescan 的 JSON 适配。
func (a *SchemaFFIAdapter) StartSchemaRescan(reqJSON string) string {
	var req struct {
		ConnectionID       string   `json:"connection_id"`
		Strategy           string   `json:"strategy"`
		Reason             string   `json:"reason"`
		ImpactedTableNames []string `json:"impacted_table_names"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.Starter == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "schema starter is not configured"))
	}
	result, rescanErr := a.deps.Starter.StartSchemaRescan(
		strings.TrimSpace(req.ConnectionID),
		schema.SchemaRescanStrategy(strings.TrimSpace(req.Strategy)),
		strings.TrimSpace(req.Reason),
		req.ImpactedTableNames,
	)
	if rescanErr != nil {
		return marshalResponse(ffiSchemaError(rescanErr.Code, rescanErr.Message))
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: result})
}

// GetCurrentSchema 执行 GetCurrentSchema 的 JSON 适配，并对响应脱敏（移除连接标识）。
func (a *SchemaFFIAdapter) GetCurrentSchema(reqJSON string) string {
	var req struct {
		ConnectionID string `json:"connection_id"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.CurrentReader == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "current schema reader is not configured"))
	}
	bundle, schemaErr := a.deps.CurrentReader.GetCurrentSchema(strings.TrimSpace(req.ConnectionID), strings.TrimSpace(req.Scope))
	if schemaErr != nil {
		return marshalResponse(ffiSchemaError(mapSchemaFFIErrorCode(extractErrorCode(schemaErr)), schemaErr.Error()))
	}
	payload := map[string]interface{}{
		"schema":       sanitizeCurrentSchema(bundle),
		"trust_state":  "",
		"schema_scope": strings.TrimSpace(req.Scope),
	}
	if a.deps.TrustReader != nil {
		view, err := a.deps.TrustReader.GetSchemaTrustState(strings.TrimSpace(req.ConnectionID))
		if err == nil {
			payload["trust_state"] = view.State
		}
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: payload})
}

// GetSchemaTrustState 执行 GetSchemaTrustState 的 JSON 适配。
func (a *SchemaFFIAdapter) GetSchemaTrustState(reqJSON string) string {
	var req struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	if a.deps.TrustReader == nil {
		return marshalResponse(ffiSchemaError("FAILED_PRECONDITION", "trust state reader is not configured"))
	}
	view, trustErr := a.deps.TrustReader.GetSchemaTrustState(strings.TrimSpace(req.ConnectionID))
	if trustErr != nil {
		return marshalResponse(ffiSchemaError(mapSchemaFFIErrorCode(extractErrorCode(trustErr)), trustErr.Error()))
	}
	return marshalResponse(&FFIResponse{Ok: true, Data: map[string]interface{}{
		"trust_state":          view.State,
		"reason":               view.LastBlockingReason,
		"last_schema_scan_unix": view.LastSchemaScanUnix,
		"last_schema_sync_unix": view.LastSchemaSyncUnix,
		"compatibility_report":  view.CompatibilityReport,
	}})
}

// RejectExecutionRequest 拒绝任何“数据生成/写入执行”类请求，明确标注超出 schema 子系统职责边界。
func (a *SchemaFFIAdapter) RejectExecutionRequest(reqJSON string) string {
	var req struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshalResponse(ffiResponseFromParseError(err))
	}
	op := strings.TrimSpace(req.Operation)
	if op == "" {
		return marshalResponse(ffiSchemaError("INVALID_ARGUMENT", "operation is required"))
	}
	return marshalResponse(ffiSchemaError(
		schema.SchemaBoundaryErrCodeOutOfScope,
		fmt.Sprintf("operation %s is out of schema subsystem scope; delegate to spec-03/spec-04 execution APIs", op),
	))
}

// mapSchemaFFIErrorCode 将领域错误码映射到 FFI 对外稳定错误码集合。
func mapSchemaFFIErrorCode(code string) string {
	switch strings.TrimSpace(code) {
	case "":
		return schemaFFIErrCodeFailedPrecondition
	case schema.SchemaScanStatusErrCodeTaskNotFound:
		return schemaFFIErrCodeFailedPrecondition
	default:
		return strings.TrimSpace(code)
	}
}

// ffiSchemaError 构建 schema FFI 错误响应。
func ffiSchemaError(code string, message string) *FFIResponse {
	return &FFIResponse{
		Ok: false,
		Error: &FFIError{
			Code:    mapSchemaFFIErrorCode(code),
			Message: strings.TrimSpace(message),
		},
	}
}

// sanitizeCurrentSchema 对当前 schema 返回体进行脱敏，避免暴露连接标识等敏感上下文。
func sanitizeCurrentSchema(bundle *schema.CurrentSchemaBundle) map[string]interface{} {
	if bundle == nil {
		return map[string]interface{}{
			"tables":  []map[string]interface{}{},
			"columns": []map[string]interface{}{},
		}
	}
	tables := make([]map[string]interface{}, 0, len(bundle.Tables))
	for _, t := range bundle.Tables {
		tables = append(tables, map[string]interface{}{
			"id":            t.ID,
			"database_name": t.DatabaseName,
			"schema_name":   t.SchemaName,
			"table_name":    t.TableName,
			"table_comment": t.TableComment,
			"scan_version":  t.ScanVersion,
			"scanned_at":    t.ScannedAt,
		})
	}
	columns := make([]map[string]interface{}, 0, len(bundle.Columns))
	for _, c := range bundle.Columns {
		columns = append(columns, map[string]interface{}{
			"id":                c.ID,
			"table_schema_id":   c.TableSchemaID,
			"column_name":       c.ColumnName,
			"ordinal_pos":       c.OrdinalPos,
			"data_type":         c.DataType,
			"abstract_type":     c.AbstractType,
			"is_primary_key":    c.IsPrimaryKey,
			"is_nullable":       c.IsNullable,
			"is_unique":         c.IsUnique,
			"is_auto_increment": c.IsAutoIncrement,
			"default_value":     c.DefaultValue,
			"column_comment":    c.ColumnComment,
			"fk_ref_table":      c.FKRefTable,
			"fk_ref_column":     c.FKRefColumn,
			"extra":             c.Extra,
		})
	}
	return map[string]interface{}{
		"tables":  tables,
		"columns": columns,
	}
}

// extractErrorCode 从通用 error 文本中提取前缀错误码。
func extractErrorCode(err error) string {
	if err == nil {
		return ""
	}
	raw := strings.TrimSpace(err.Error())
	idx := strings.Index(raw, ":")
	if idx <= 0 {
		return ""
	}
	return strings.TrimSpace(raw[:idx])
}
