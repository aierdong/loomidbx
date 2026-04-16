package schema

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

// json 为 schema 包内统一 JSON 编解码器，与后端其他包保持一致风格。
var json = jsoniter.ConfigCompatibleWithStandardLibrary

const (
	// ExtraKeySchemaTrustState 为 extra JSON 中可信度状态字段名。
	ExtraKeySchemaTrustState = "schema_trust_state"

	// ExtraKeySchemaLastBlockingReason 为 extra JSON 中最近一次阻断原因（稳定错误码/短文本，禁止敏感信息）。
	ExtraKeySchemaLastBlockingReason = "schema_last_blocking_reason"

	// ExtraKeyLastSchemaScanUnix 为 extra JSON 中最后一次完成扫描的 Unix 秒时间戳。
	ExtraKeyLastSchemaScanUnix = "last_schema_scan_unix"

	// ExtraKeyLastSchemaSyncUnix 为 extra JSON 中最后一次成功同步当前 schema 的 Unix 秒时间戳。
	ExtraKeyLastSchemaSyncUnix = "last_schema_sync_unix"

	// ExtraKeyCompatibilityReport 为 extra JSON 中最新兼容性重判定报告快照字段名。
	ExtraKeyCompatibilityReport = "compatibility_report"
)

// ConnectionSchemaMeta 表示从 ldb_connections.extra 解析出的 schema 子域元数据读模型。
type ConnectionSchemaMeta struct {
	// SchemaTrustState 为可信度状态；缺省按 Parse 规则回退为 trusted。
	SchemaTrustState SchemaTrustState `json:"schema_trust_state"`

	// SchemaLastBlockingReason 为最近一次阻断原因（可空）；仅用于 UI/FFI 分支，不得包含凭据。
	SchemaLastBlockingReason string `json:"schema_last_blocking_reason"`

	// LastSchemaScanUnix 为最后扫描完成时间（Unix 秒）；0 表示尚未记录。
	LastSchemaScanUnix int64 `json:"last_schema_scan_unix"`

	// LastSchemaSyncUnix 为最后成功同步当前 schema 时间（Unix 秒）；0 表示尚未记录。
	LastSchemaSyncUnix int64 `json:"last_schema_sync_unix"`

	// CompatibilityReport 为最新一次兼容性重判定报告快照；nil 表示尚未生成。
	CompatibilityReport *CompatibilityReportSnapshot `json:"compatibility_report,omitempty"`
}

// ConnectionSchemaMetaPatch 描述对 extra 中 schema 子域字段的部分更新；nil 指针表示不修改该字段。
type ConnectionSchemaMetaPatch struct {
	// TrustState 非空时写入 schema_trust_state。
	TrustState *SchemaTrustState

	// LastBlockingReason 非 nil 时写入或清空 schema_last_blocking_reason（空字符串表示清空）。
	LastBlockingReason *string

	// LastSchemaScanUnix 非 nil 时写入 last_schema_scan_unix。
	LastSchemaScanUnix *int64

	// LastSchemaSyncUnix 非 nil 时写入 last_schema_sync_unix。
	LastSchemaSyncUnix *int64

	// CompatibilityReport 非 nil 时写入 compatibility_report；允许写入空快照用于清空。
	CompatibilityReport *CompatibilityReportSnapshot
}

// IsEmpty 判断 patch 是否不包含任何有效字段更新。
func (p ConnectionSchemaMetaPatch) IsEmpty() bool {
	return p.TrustState == nil &&
		p.LastBlockingReason == nil &&
		p.LastSchemaScanUnix == nil &&
		p.LastSchemaSyncUnix == nil &&
		p.CompatibilityReport == nil
}

// CompatibilityRecheckStatus 表示兼容性重判定的执行状态。
type CompatibilityRecheckStatus string

const (
	// CompatibilityRecheckStatusSuccess 表示重判定成功并产出报告。
	CompatibilityRecheckStatusSuccess CompatibilityRecheckStatus = "success"

	// CompatibilityRecheckStatusFailed 表示重判定失败（报告可能为空或旧值）。
	CompatibilityRecheckStatusFailed CompatibilityRecheckStatus = "failed"

	// CompatibilityRecheckStatusSkippedNoGeneratorConfig 表示无生成器配置可判定，跳过并产出空报告。
	CompatibilityRecheckStatusSkippedNoGeneratorConfig CompatibilityRecheckStatus = "skipped_no_generator_config"
)

// CompatibilityReportSummary 为兼容性报告的摘要字段，便于 UI 快速展示。
type CompatibilityReportSummary struct {
	// Mode 与 risks API 的 mode 保持一致（configured/no_generator_config）。
	Mode GeneratorCompatibilityRiskMode `json:"mode"`

	// TotalRisks 为风险总数。
	TotalRisks int `json:"total_risks"`

	// BlockingRisks 为阻断级风险数量。
	BlockingRisks int `json:"blocking_risks"`
}

// CompatibilityReportSnapshot 表示连接维度最新一次兼容性重判定报告快照（落在 ldb_connections.extra）。
type CompatibilityReportSnapshot struct {
	// Status 为重判定状态。
	Status CompatibilityRecheckStatus `json:"status"`

	// GeneratedAtUnix 为报告生成时间（Unix 秒）。
	GeneratedAtUnix int64 `json:"generated_at_unix"`

	// ErrorCode 为失败时的稳定错误码；成功/跳过时为空。
	ErrorCode string `json:"error_code,omitempty"`

	// Summary 为报告摘要。
	Summary CompatibilityReportSummary `json:"summary"`

	// Risks 为风险列表；无配置或无风险时必须为 [] 而不是 nil（便于 JSON 契约稳定）。
	Risks []GeneratorCompatibilityRisk `json:"risks"`
}

// ParseConnectionSchemaMeta 从 extra JSON 解析 schema 子域元数据。
//
// 输入：
// - extraJSON: ldb_connections.extra 全文或空串。
//
// 输出：
// - ConnectionSchemaMeta: 解析后的读模型；缺省字段按兼容旧数据规则填充。
// - error: JSON 非法时返回错误。
func ParseConnectionSchemaMeta(extraJSON string) (ConnectionSchemaMeta, error) {
	trimmed := strings.TrimSpace(extraJSON)
	if trimmed == "" {
		return ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}, nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return ConnectionSchemaMeta{}, fmt.Errorf("parse connection extra: %w", err)
	}
	out := ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
	if v, ok := raw[ExtraKeySchemaTrustState]; ok {
		s, err := coerceString(v)
		if err != nil {
			return ConnectionSchemaMeta{}, fmt.Errorf("decode schema_trust_state: %w", err)
		}
		ts, err := ParseSchemaTrustState(s)
		if err != nil {
			return ConnectionSchemaMeta{}, err
		}
		out.SchemaTrustState = ts
	}
	if v, ok := raw[ExtraKeySchemaLastBlockingReason]; ok {
		s, err := coerceString(v)
		if err != nil {
			return ConnectionSchemaMeta{}, fmt.Errorf("decode schema_last_blocking_reason: %w", err)
		}
		out.SchemaLastBlockingReason = s
	}
	if v, ok := raw[ExtraKeyLastSchemaScanUnix]; ok {
		n, err := coerceInt64(v)
		if err != nil {
			return ConnectionSchemaMeta{}, fmt.Errorf("decode last_schema_scan_unix: %w", err)
		}
		out.LastSchemaScanUnix = n
	}
	if v, ok := raw[ExtraKeyLastSchemaSyncUnix]; ok {
		n, err := coerceInt64(v)
		if err != nil {
			return ConnectionSchemaMeta{}, fmt.Errorf("decode last_schema_sync_unix: %w", err)
		}
		out.LastSchemaSyncUnix = n
	}
	if v, ok := raw[ExtraKeyCompatibilityReport]; ok && v != nil {
		// 兼容 map[string]interface{} / json.RawMessage 等多种解码形态：先 marshal 再 unmarshal 到结构体。
		b, err := json.Marshal(v)
		if err != nil {
			return ConnectionSchemaMeta{}, fmt.Errorf("decode compatibility_report: %w", err)
		}
		var snap CompatibilityReportSnapshot
		if err := json.Unmarshal(b, &snap); err != nil {
			return ConnectionSchemaMeta{}, fmt.Errorf("decode compatibility_report: %w", err)
		}
		// 保证 risks 非 nil，避免上层 JSON 输出不稳定。
		if snap.Risks == nil {
			snap.Risks = []GeneratorCompatibilityRisk{}
		}
		out.CompatibilityReport = &snap
	}
	return out, nil
}

// MergeConnectionExtraSchemaMeta 将 schema 子域字段合并进 extra JSON，保留无关顶层键。
//
// 输入：
// - existingExtra: 原始 extra JSON，可为空。
// - patch: 局部更新；IsEmpty 时原样返回 existingExtra。
//
// 输出：
// - string: 合并后的 JSON 文本。
// - error: 输入 JSON 非法或写入失败时返回错误。
func MergeConnectionExtraSchemaMeta(existingExtra string, patch ConnectionSchemaMetaPatch) (string, error) {
	if patch.IsEmpty() {
		return existingExtra, nil
	}
	trimmed := strings.TrimSpace(existingExtra)
	var m map[string]interface{}
	if trimmed == "" {
		m = make(map[string]interface{})
	} else {
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			return "", fmt.Errorf("unmarshal connection extra: %w", err)
		}
		if m == nil {
			m = make(map[string]interface{})
		}
	}
	if patch.TrustState != nil {
		m[ExtraKeySchemaTrustState] = string(*patch.TrustState)
	}
	if patch.LastBlockingReason != nil {
		m[ExtraKeySchemaLastBlockingReason] = *patch.LastBlockingReason
	}
	if patch.LastSchemaScanUnix != nil {
		m[ExtraKeyLastSchemaScanUnix] = *patch.LastSchemaScanUnix
	}
	if patch.LastSchemaSyncUnix != nil {
		m[ExtraKeyLastSchemaSyncUnix] = *patch.LastSchemaSyncUnix
	}
	if patch.CompatibilityReport != nil {
		// 写入前保证 risks 非 nil，避免 JSON 输出不稳定。
		if patch.CompatibilityReport.Risks == nil {
			patch.CompatibilityReport.Risks = []GeneratorCompatibilityRisk{}
		}
		m[ExtraKeyCompatibilityReport] = patch.CompatibilityReport
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// coerceString 将 JSON 解码后的动态值转为字符串（兼容 JSON number 到整型字符串场景）。
func coerceString(v interface{}) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case float64:
		return fmt.Sprintf("%.0f", t), nil
	default:
		return "", fmt.Errorf("expected string, got %T", v)
	}
}

// coerceInt64 将 JSON 解码后的动态值转为 int64（JSON number 默认为 float64）。
func coerceInt64(v interface{}) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}
