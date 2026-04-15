package schema

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// GeneratorCompatibilityRiskMode 表示 GetGeneratorCompatibilityRisks 的 mode 字段（与 design.md API Contract 对齐）。
type GeneratorCompatibilityRiskMode string

const (
	// GeneratorCompatibilityModeConfigured 表示已存在生成器配置，风险列表由分析器填充。
	GeneratorCompatibilityModeConfigured GeneratorCompatibilityRiskMode = "configured"

	// GeneratorCompatibilityModeNoGeneratorConfig 表示无生成器配置；risks 必须为空且整体成功返回（非错误）。
	GeneratorCompatibilityModeNoGeneratorConfig GeneratorCompatibilityRiskMode = "no_generator_config"
)

// GeneratorCompatibilityRisk 描述一条生成器兼容性风险（字段在任务 4.1 扩展）。
type GeneratorCompatibilityRisk struct {
	// ID 为稳定风险标识，供确认与同步 ack 引用。
	ID string

	// Type 为风险类型（删除/重命名缺失/类型不兼容）。
	Type GeneratorCompatibilityRiskType

	// Severity 为风险级别；4.1 的阻断级风险使用 blocking。
	Severity GeneratorCompatibilityRiskSeverity

	// Object 为受影响对象（如 db.table.column）。
	Object string

	// Reason 为触发风险的原因说明。
	Reason string

	// SuggestedAction 为建议动作，供 UI 指导用户修复配置。
	SuggestedAction string

	// DatabaseName 为受影响数据库。
	DatabaseName string

	// SchemaName 为受影响逻辑 schema（可为空）。
	SchemaName string

	// TableName 为受影响表名。
	TableName string

	// ColumnName 为受影响列名。
	ColumnName string

	// GeneratorConfigID 为关联生成器配置 ID（若可定位）。
	GeneratorConfigID string
}

// GeneratorCompatibilityRisksResult 为 GetGeneratorCompatibilityRisks 的成功返回体。
type GeneratorCompatibilityRisksResult struct {
	// Mode 为 configured 或 no_generator_config。
	Mode GeneratorCompatibilityRiskMode

	// Risks 为风险列表；no_generator_config 时必须为空。
	Risks []GeneratorCompatibilityRisk
}

// GeneratorCompatibilityRiskType 表示兼容性风险分类。
type GeneratorCompatibilityRiskType string

const (
	// GeneratorCompatibilityRiskTypeColumnDeleted 表示目标字段被删除。
	GeneratorCompatibilityRiskTypeColumnDeleted GeneratorCompatibilityRiskType = "column_deleted"

	// GeneratorCompatibilityRiskTypeColumnMissingOrRenamed 表示配置字段缺失或疑似重命名。
	GeneratorCompatibilityRiskTypeColumnMissingOrRenamed GeneratorCompatibilityRiskType = "column_missing_or_renamed"

	// GeneratorCompatibilityRiskTypeColumnTypeIncompatible 表示字段类型变化导致配置不兼容。
	GeneratorCompatibilityRiskTypeColumnTypeIncompatible GeneratorCompatibilityRiskType = "column_type_incompatible"
)

// GeneratorCompatibilityRiskSeverity 表示风险级别。
type GeneratorCompatibilityRiskSeverity string

const (
	// GeneratorCompatibilityRiskSeverityBlocking 表示阻断级风险。
	GeneratorCompatibilityRiskSeverityBlocking GeneratorCompatibilityRiskSeverity = "blocking"
)

// GeneratorColumnConfig 为单列级生成器配置快照，用于 4.1 风险分析。
type GeneratorColumnConfig struct {
	// ConnectionID 为配置所属连接 ID。
	ConnectionID string

	// DatabaseName 为配置所属数据库。
	DatabaseName string

	// SchemaName 为配置所属逻辑 schema（可为空）。
	SchemaName string

	// TableName 为配置所属表名。
	TableName string

	// ColumnName 为配置绑定字段名。
	ColumnName string

	// ConfigID 为配置唯一标识（来自 spec-03 存储层）。
	ConfigID string
}

// GeneratorConfigSnapshot 为某连接当前可用的生成器配置快照。
type GeneratorConfigSnapshot struct {
	// Columns 为列级生成器配置集合。
	Columns []GeneratorColumnConfig
}

// GeneratorConfigSnapshotStore 抽象 spec-03 的配置存储读取接口（此处仅定义契约，不绑定实现）。
type GeneratorConfigSnapshotStore interface {
	// LoadByConnectionID 读取连接下的生成器配置快照；若无配置返回空快照，不作为错误。
	LoadByConnectionID(ctx context.Context, connectionID string) (*GeneratorConfigSnapshot, error)
}

// GeneratorConfigSnapshotStoreStub 为 spec-03 未就绪时的可测 stub。
type GeneratorConfigSnapshotStoreStub struct {
	// SnapshotsByConnectionID 为按连接预置的快照映射。
	SnapshotsByConnectionID map[string]*GeneratorConfigSnapshot

	// Err 为强制返回错误（用于测试错误分支）。
	Err error
}

// LoadByConnectionID 返回预置快照；缺失连接时返回空快照（不与 no_generator_config 语义冲突）。
func (s GeneratorConfigSnapshotStoreStub) LoadByConnectionID(_ context.Context, connectionID string) (*GeneratorConfigSnapshot, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	if s.SnapshotsByConnectionID == nil {
		return &GeneratorConfigSnapshot{}, nil
	}
	if snap, ok := s.SnapshotsByConnectionID[connectionID]; ok && snap != nil {
		return cloneGeneratorConfigSnapshot(snap), nil
	}
	return &GeneratorConfigSnapshot{}, nil
}

// GeneratorCompatibilityAnalyzer 基于 Diff 与生成器配置计算阻断级风险清单。
type GeneratorCompatibilityAnalyzer struct{}

// NewGeneratorCompatibilityAnalyzer 构造兼容性分析器。
func NewGeneratorCompatibilityAnalyzer() *GeneratorCompatibilityAnalyzer {
	return &GeneratorCompatibilityAnalyzer{}
}

// Analyze 对 Diff 与配置快照进行分析，产出对象/原因/建议动作。
func (a *GeneratorCompatibilityAnalyzer) Analyze(diff *SchemaDiffResult, snapshot *GeneratorConfigSnapshot) []GeneratorCompatibilityRisk {
	_ = a
	if diff == nil || snapshot == nil || len(snapshot.Columns) == 0 {
		return nil
	}
	idx := buildDiffRiskIndex(diff)
	risks := make([]GeneratorCompatibilityRisk, 0)
	for _, cfg := range snapshot.Columns {
		tableKey := tableColumnRiskKey(cfg.DatabaseName, cfg.SchemaName, cfg.TableName, "")
		colKey := tableColumnRiskKey(cfg.DatabaseName, cfg.SchemaName, cfg.TableName, cfg.ColumnName)

		if idx.removedTables[tableKey] {
			risks = append(risks, newBlockingRisk(
				GeneratorCompatibilityRiskTypeColumnDeleted,
				cfg,
				fmt.Sprintf("字段 %s.%s.%s 所在表已被删除", cfg.DatabaseName, cfg.TableName, cfg.ColumnName),
				"请先删除该字段的生成器配置，或将配置重新映射到新表字段后再同步。",
			))
			continue
		}

		if mod, ok := idx.typeIncompatible[colKey]; ok {
			risks = append(risks, newBlockingRisk(
				GeneratorCompatibilityRiskTypeColumnTypeIncompatible,
				cfg,
				fmt.Sprintf("字段类型不兼容：%s -> %s（抽象类型 %s -> %s）", mod.oldType, mod.newType, mod.oldAbstractType, mod.newAbstractType),
				"请调整该字段生成器类型或参数，使其与新字段类型兼容后再同步。",
			))
			continue
		}

		if removed, ok := idx.removedColumns[colKey]; ok {
			if candidate := findRenameCandidate(removed.abstractType, idx.addedColumnsByTable[tableKey]); candidate != "" {
				risks = append(risks, newBlockingRisk(
					GeneratorCompatibilityRiskTypeColumnMissingOrRenamed,
					cfg,
					fmt.Sprintf("字段 %s 已缺失，疑似重命名为 %s", cfg.ColumnName, candidate),
					fmt.Sprintf("请将生成器配置从 %s 映射到 %s，并确认参数语义后再同步。", cfg.ColumnName, candidate),
				))
			} else {
				risks = append(risks, newBlockingRisk(
					GeneratorCompatibilityRiskTypeColumnDeleted,
					cfg,
					fmt.Sprintf("字段 %s 已从 schema 中删除", cfg.ColumnName),
					"请删除该字段生成器配置或改配到仍存在的字段后再同步。",
				))
			}
		}
	}
	sort.Slice(risks, func(i, j int) bool { return risks[i].ID < risks[j].ID })
	return risks
}

// diffRiskIndex 为风险分析准备的中间索引结构。
type diffRiskIndex struct {
	// removedTables 记录被删除表的键集合。
	removedTables map[string]bool

	// removedColumns 记录被删除字段及其元数据。
	removedColumns map[string]removedColumnMeta

	// addedColumnsByTable 记录每个表新增字段，用于重命名/缺失推断。
	addedColumnsByTable map[string][]addedColumnMeta

	// typeIncompatible 记录类型不兼容字段。
	typeIncompatible map[string]typeIncompatibleMeta
}

// removedColumnMeta 为删除字段的分析元数据。
type removedColumnMeta struct {
	// abstractType 为删除字段的抽象类型。
	abstractType string
}

// addedColumnMeta 为新增字段的分析元数据。
type addedColumnMeta struct {
	// columnName 为新增字段名。
	columnName string

	// abstractType 为新增字段抽象类型。
	abstractType string
}

// typeIncompatibleMeta 为类型不兼容字段的前后类型元数据。
type typeIncompatibleMeta struct {
	// oldType 为变更前原始类型。
	oldType string

	// newType 为变更后原始类型。
	newType string

	// oldAbstractType 为变更前抽象类型。
	oldAbstractType string

	// newAbstractType 为变更后抽象类型。
	newAbstractType string
}

// buildDiffRiskIndex 将 Diff 结果转换为风险分析索引，降低逐配置扫描复杂度。
func buildDiffRiskIndex(diff *SchemaDiffResult) diffRiskIndex {
	out := diffRiskIndex{
		removedTables:      make(map[string]bool),
		removedColumns:     make(map[string]removedColumnMeta),
		addedColumnsByTable: make(map[string][]addedColumnMeta),
		typeIncompatible:   make(map[string]typeIncompatibleMeta),
	}
	for _, td := range diff.TableDiffs {
		tableKey := tableColumnRiskKey(td.DatabaseName, td.SchemaName, td.TableName, "")
		if td.Kind == SchemaDiffKindRemoved {
			out.removedTables[tableKey] = true
		}
		for _, cd := range td.ColumnDiffs {
			colKey := tableColumnRiskKey(td.DatabaseName, td.SchemaName, td.TableName, cd.ColumnName)
			switch cd.Kind {
			case SchemaColumnDiffKindRemoved:
				meta := removedColumnMeta{}
				if cd.Old != nil {
					meta.abstractType = normalizeRiskToken(cd.Old.AbstractType)
				}
				out.removedColumns[colKey] = meta
			case SchemaColumnDiffKindAdded:
				meta := addedColumnMeta{
					columnName: cd.ColumnName,
				}
				if cd.New != nil {
					meta.abstractType = normalizeRiskToken(cd.New.AbstractType)
				}
				out.addedColumnsByTable[tableKey] = append(out.addedColumnsByTable[tableKey], meta)
			case SchemaColumnDiffKindModified:
				if !containsAnyAttr(cd.AttributeChanges, "abstract_type", "data_type") {
					continue
				}
				meta := typeIncompatibleMeta{}
				if cd.Old != nil {
					meta.oldType = cd.Old.DataType
					meta.oldAbstractType = cd.Old.AbstractType
				}
				if cd.New != nil {
					meta.newType = cd.New.DataType
					meta.newAbstractType = cd.New.AbstractType
				}
				out.typeIncompatible[colKey] = meta
			}
		}
	}
	return out
}

// containsAnyAttr 判断属性变化列表是否包含任一目标属性。
func containsAnyAttr(attrs []string, candidates ...string) bool {
	for _, a := range attrs {
		for _, c := range candidates {
			if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(c)) {
				return true
			}
		}
	}
	return false
}

// findRenameCandidate 在同表新增字段中寻找可能的重命名候选。
func findRenameCandidate(oldAbstractType string, added []addedColumnMeta) string {
	normalizedOld := normalizeRiskToken(oldAbstractType)
	for _, c := range added {
		if normalizedOld == "" || normalizeRiskToken(c.abstractType) == normalizedOld {
			return c.columnName
		}
	}
	return ""
}

// newBlockingRisk 构造阻断级风险对象，统一字段填充规则。
func newBlockingRisk(typ GeneratorCompatibilityRiskType, cfg GeneratorColumnConfig, reason string, action string) GeneratorCompatibilityRisk {
	object := strings.Trim(strings.Join([]string{cfg.DatabaseName, cfg.TableName, cfg.ColumnName}, "."), ".")
	id := string(typ) + ":" + tableColumnRiskKey(cfg.DatabaseName, cfg.SchemaName, cfg.TableName, cfg.ColumnName)
	return GeneratorCompatibilityRisk{
		ID:                id,
		Type:              typ,
		Severity:          GeneratorCompatibilityRiskSeverityBlocking,
		Object:            object,
		Reason:            reason,
		SuggestedAction:   action,
		DatabaseName:      cfg.DatabaseName,
		SchemaName:        cfg.SchemaName,
		TableName:         cfg.TableName,
		ColumnName:        cfg.ColumnName,
		GeneratorConfigID: cfg.ConfigID,
	}
}

// tableColumnRiskKey 生成表/列复合键，确保比较大小写无关且稳定。
func tableColumnRiskKey(db, schema, table, col string) string {
	return strings.Join([]string{
		normalizeRiskToken(db),
		normalizeRiskToken(schema),
		normalizeRiskToken(table),
		normalizeRiskToken(col),
	}, "\x00")
}

// normalizeRiskToken 归一化风险索引 token，避免空白与大小写差异导致误判。
func normalizeRiskToken(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// cloneGeneratorConfigSnapshot 深拷贝快照，防止调用方修改内部切片。
func cloneGeneratorConfigSnapshot(in *GeneratorConfigSnapshot) *GeneratorConfigSnapshot {
	if in == nil {
		return &GeneratorConfigSnapshot{}
	}
	out := &GeneratorConfigSnapshot{
		Columns: make([]GeneratorColumnConfig, len(in.Columns)),
	}
	copy(out.Columns, in.Columns)
	return out
}

// SchemaDiffByTaskReader 抽象按任务读取 Diff 结果的能力，用于兼容性风险实时计算。
type SchemaDiffByTaskReader interface {
	// LoadSchemaDiffByTaskID 按 task_id 返回 Diff 结果；不存在或未就绪时返回错误。
	LoadSchemaDiffByTaskID(taskID string) (*SchemaDiffResult, error)
}

// GetGeneratorCompatibilityRisks 按 task_id 返回兼容性风险模式与清单，并在 configured 模式执行真实风险分析。
//
// 输入参数：
//   - ctx: 请求上下文。
//   - taskID: 扫描任务 ID。
//   - runtime: 扫描运行时存储，用于解析 connection_id 与任务状态。
//   - diffReader: 任务 Diff 读取器，用于获取 Analyze 输入。
//   - generatorStore: 连接级生成器配置快照读取器。
//   - analyzer: 兼容性分析器实现；为空时使用默认分析器。
//
// 返回值：
//   - GeneratorCompatibilityRisksResult: 成功时包含 mode 与 risks；无生成器配置时为 no_generator_config 且 risks 为空，非错误。
//   - error: 参数非法、任务不存在、预览未就绪或上游读取失败时返回错误。
func GetGeneratorCompatibilityRisks(
	ctx context.Context,
	taskID string,
	runtime *SchemaScanRuntimeStore,
	diffReader SchemaDiffByTaskReader,
	generatorStore GeneratorConfigSnapshotStore,
	analyzer *GeneratorCompatibilityAnalyzer,
) (GeneratorCompatibilityRisksResult, error) {
	if runtime == nil {
		return GeneratorCompatibilityRisksResult{}, fmt.Errorf("schema runtime store is required")
	}
	trimmedTaskID := strings.TrimSpace(taskID)
	if trimmedTaskID == "" {
		return GeneratorCompatibilityRisksResult{}, fmt.Errorf("task_id is required")
	}
	taskCtx, ok := runtime.GetRuntimeContext(trimmedTaskID)
	if !ok {
		return GeneratorCompatibilityRisksResult{}, &SchemaScanStatusError{
			Code:    SchemaScanStatusErrCodeTaskNotFound,
			Message: "schema scan task not found",
		}
	}
	if taskCtx.Status != SchemaScanTaskCompleted || !taskCtx.PreviewReady {
		return GeneratorCompatibilityRisksResult{}, fmt.Errorf("risk result not ready")
	}
	if generatorStore == nil {
		return GeneratorCompatibilityRisksResult{
			Mode:  GeneratorCompatibilityModeNoGeneratorConfig,
			Risks: []GeneratorCompatibilityRisk{},
		}, nil
	}
	snapshot, err := generatorStore.LoadByConnectionID(ctx, strings.TrimSpace(taskCtx.ConnectionID))
	if err != nil {
		return GeneratorCompatibilityRisksResult{}, err
	}
	if snapshot == nil || len(snapshot.Columns) == 0 {
		return GeneratorCompatibilityRisksResult{
			Mode:  GeneratorCompatibilityModeNoGeneratorConfig,
			Risks: []GeneratorCompatibilityRisk{},
		}, nil
	}
	if diffReader == nil {
		return GeneratorCompatibilityRisksResult{}, fmt.Errorf("schema diff reader is required when generator config exists")
	}
	diff, err := diffReader.LoadSchemaDiffByTaskID(trimmedTaskID)
	if err != nil {
		return GeneratorCompatibilityRisksResult{}, err
	}
	if analyzer == nil {
		analyzer = NewGeneratorCompatibilityAnalyzer()
	}
	risks := analyzer.Analyze(diff, snapshot)
	if risks == nil {
		risks = []GeneratorCompatibilityRisk{}
	}
	return GeneratorCompatibilityRisksResult{
		Mode:  GeneratorCompatibilityModeConfigured,
		Risks: risks,
	}, nil
}
