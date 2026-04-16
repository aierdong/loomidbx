package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"loomidbx/generator"
	"loomidbx/schema"
)

// DefaultCompatibilityRecheckService 为 schema 同步后的全量重判定提供默认实现。
//
// 该实现使用「当前 schema」+「连接维度生成器配置快照」重新判定每条字段配置的兼容性，
// 并将最新报告快照写入 ldb_connections.extra（compatibility_report）。
type DefaultCompatibilityRecheckService struct {
	// currentRepo 用于读取连接当前持久化 schema（事实数据源）。
	currentRepo schema.CurrentSchemaRepository

	// generatorStore 用于读取连接维度字段配置快照（最小必要信息）。
	generatorStore schema.GeneratorConfigSnapshotStore

	// registry 用于解析字段候选生成器集合。
	registry *generator.GeneratorRegistry

	// trustGate 用于在发现阻断风险后回写 pending_adjustment 状态机。
	trustGate schema.SchemaTrustGate

	// metaRepo 用于将报告快照落入 connection extra。
	metaRepo schema.TrustConnectionMetaRepository
}

// NewDefaultCompatibilityRecheckService 创建默认重判定服务实现。
//
// 输入：
// - currentRepo: 当前 schema 仓储。
// - generatorStore: 生成器配置快照读取器。
// - registry: 生成器注册表。
// - trustGate: schema 可信度闸门。
// - metaRepo: connection extra 写入仓储。
//
// 输出：
// - *DefaultCompatibilityRecheckService: 服务实例。
func NewDefaultCompatibilityRecheckService(
	currentRepo schema.CurrentSchemaRepository,
	generatorStore schema.GeneratorConfigSnapshotStore,
	registry *generator.GeneratorRegistry,
	trustGate schema.SchemaTrustGate,
	metaRepo schema.TrustConnectionMetaRepository,
) *DefaultCompatibilityRecheckService {
	return &DefaultCompatibilityRecheckService{
		currentRepo:    currentRepo,
		generatorStore: generatorStore,
		registry:       registry,
		trustGate:      trustGate,
		metaRepo:       metaRepo,
	}
}

// RevalidateAllConfigs 对指定连接执行全量重判定并落库最新报告快照。
//
// 输入：
// - ctx: 调用上下文。
// - connectionID: 连接标识。
//
// 输出：
// - schema.CompatibilityReportSnapshot: 报告快照（落库与返回契约一致）。
// - error: 仅在依赖缺失或落库失败等不可恢复场景返回错误。
func (s *DefaultCompatibilityRecheckService) RevalidateAllConfigs(ctx context.Context, connectionID string) (schema.CompatibilityReportSnapshot, error) {
	connID := strings.TrimSpace(connectionID)
	if connID == "" {
		return schema.CompatibilityReportSnapshot{}, fmt.Errorf("connection_id is required")
	}
	if s.currentRepo == nil {
		return schema.CompatibilityReportSnapshot{}, fmt.Errorf("current schema repository is required")
	}
	if s.registry == nil {
		return schema.CompatibilityReportSnapshot{}, fmt.Errorf("generator registry is required")
	}
	if s.trustGate == nil {
		return schema.CompatibilityReportSnapshot{}, fmt.Errorf("schema trust gate is required")
	}
	if s.metaRepo == nil {
		return schema.CompatibilityReportSnapshot{}, fmt.Errorf("trust meta repository is required")
	}
	if s.generatorStore == nil {
		// generatorStore 缺失等价于“无配置”，保持语义稳定。
		snap := emptyNoConfigSnapshot()
		if err := s.persistSnapshot(ctx, connID, snap); err != nil {
			return schema.CompatibilityReportSnapshot{}, err
		}
		return snap, nil
	}

	cfgSnapshot, err := s.generatorStore.LoadByConnectionID(ctx, connID)
	if err != nil {
		return schema.CompatibilityReportSnapshot{}, err
	}
	if cfgSnapshot == nil || len(cfgSnapshot.Columns) == 0 {
		snap := emptyNoConfigSnapshot()
		if err := s.persistSnapshot(ctx, connID, snap); err != nil {
			return schema.CompatibilityReportSnapshot{}, err
		}
		return snap, nil
	}

	current, err := s.currentRepo.LoadCurrentSchema(ctx, connID)
	if err != nil {
		return schema.CompatibilityReportSnapshot{}, err
	}
	fieldIdx := buildCurrentFieldIndex(current)
	resolver := generator.NewGeneratorTypeResolver(s.registry)

	risks := make([]schema.GeneratorCompatibilityRisk, 0)
	for _, cfg := range cfgSnapshot.Columns {
		locator := strings.ToLower(strings.TrimSpace(cfg.TableName)) + "|" + strings.ToLower(strings.TrimSpace(cfg.ColumnName))
		col, ok := fieldIdx[locator]
		if !ok {
			risks = append(risks, schema.GeneratorCompatibilityRisk{
				ID:               "column_deleted:" + schemaRiskKey(cfg.DatabaseName, cfg.SchemaName, cfg.TableName, cfg.ColumnName),
				Type:             schema.GeneratorCompatibilityRiskTypeColumnDeleted,
				Severity:         schema.GeneratorCompatibilityRiskSeverityBlocking,
				Object:           strings.Trim(strings.Join([]string{cfg.DatabaseName, cfg.TableName, cfg.ColumnName}, "."), "."),
				Reason:           fmt.Sprintf("字段 %s.%s 已不存在，无法继续使用既有生成器配置", cfg.TableName, cfg.ColumnName),
				SuggestedAction:  "请删除该字段配置，或将配置重新映射到仍存在的字段后再继续。",
				DatabaseName:     cfg.DatabaseName,
				SchemaName:       cfg.SchemaName,
				TableName:        cfg.TableName,
				ColumnName:       cfg.ColumnName,
				GeneratorConfigID: cfg.ConfigID,
			})
			continue
		}
		field := generator.FieldSchema{
			ConnectionID: connID,
			Table:        cfg.TableName,
			Column:       cfg.ColumnName,
			ColumnID:     col.ID,
			AbstractType: strings.TrimSpace(col.AbstractType),
		}
		candidates, resolveErr := resolver.ResolveCandidates(field)
		if resolveErr != nil || candidates == nil {
			risks = append(risks, schema.GeneratorCompatibilityRisk{
				ID:               "column_type_incompatible:" + schemaRiskKey(cfg.DatabaseName, cfg.SchemaName, cfg.TableName, cfg.ColumnName),
				Type:             schema.GeneratorCompatibilityRiskTypeColumnTypeIncompatible,
				Severity:         schema.GeneratorCompatibilityRiskSeverityBlocking,
				Object:           strings.Trim(strings.Join([]string{cfg.DatabaseName, cfg.TableName, cfg.ColumnName}, "."), "."),
				Reason:           "无法解析字段候选生成器集合（字段 abstract_type 可能不受支持）",
				SuggestedAction:  "请重新选择与该字段类型兼容的生成器后再继续。",
				DatabaseName:     cfg.DatabaseName,
				SchemaName:       cfg.SchemaName,
				TableName:        cfg.TableName,
				ColumnName:       cfg.ColumnName,
				GeneratorConfigID: cfg.ConfigID,
			})
			continue
		}
		if strings.TrimSpace(cfg.GeneratorType) == "" {
			// 兼容旧快照缺失 generator_type：不判定为风险，避免误报；由后续存储/快照完善后再启用。
			continue
		}
		if !containsGeneratorType(candidates.Candidates, generator.GeneratorType(strings.TrimSpace(cfg.GeneratorType))) {
			risks = append(risks, schema.GeneratorCompatibilityRisk{
				ID:               "column_type_incompatible:" + schemaRiskKey(cfg.DatabaseName, cfg.SchemaName, cfg.TableName, cfg.ColumnName),
				Type:             schema.GeneratorCompatibilityRiskTypeColumnTypeIncompatible,
				Severity:         schema.GeneratorCompatibilityRiskSeverityBlocking,
				Object:           strings.Trim(strings.Join([]string{cfg.DatabaseName, cfg.TableName, cfg.ColumnName}, "."), "."),
				Reason:           fmt.Sprintf("字段类型已变化或候选集收敛：当前字段类型不再支持生成器 %s", strings.TrimSpace(cfg.GeneratorType)),
				SuggestedAction:  "请重新选择与当前字段类型兼容的生成器后再继续。",
				DatabaseName:     cfg.DatabaseName,
				SchemaName:       cfg.SchemaName,
				TableName:        cfg.TableName,
				ColumnName:       cfg.ColumnName,
				GeneratorConfigID: cfg.ConfigID,
			})
		}
	}
	sort.Slice(risks, func(i, j int) bool { return risks[i].ID < risks[j].ID })

	blocking := 0
	for _, r := range risks {
		if r.Severity == schema.GeneratorCompatibilityRiskSeverityBlocking {
			blocking++
		}
	}
	out := schema.CompatibilityReportSnapshot{
		Status:          schema.CompatibilityRecheckStatusSuccess,
		GeneratedAtUnix: time.Now().Unix(),
		Summary: schema.CompatibilityReportSummary{
			Mode:          schema.GeneratorCompatibilityModeConfigured,
			TotalRisks:    len(risks),
			BlockingRisks: blocking,
		},
		Risks: risks,
	}

	if blocking > 0 {
		// 回写 trust state：发现阻断风险 -> pending_adjustment，阻断后续执行链路。
		if _, err := s.trustGate.UpdateTrustState(ctx, connID, schema.TrustStateUpdateInput{
			HasBlockingRisk: true,
			RescanCompleted: true,
			SyncSucceeded:   true,
		}); err != nil {
			return schema.CompatibilityReportSnapshot{}, err
		}
	}
	if err := s.persistSnapshot(ctx, connID, out); err != nil {
		return schema.CompatibilityReportSnapshot{}, err
	}
	return out, nil
}

// buildCurrentFieldIndex 构建当前 schema 的字段定位索引。
func buildCurrentFieldIndex(bundle *schema.CurrentSchemaBundle) map[string]schema.ColumnSchemaPersisted {
	out := make(map[string]schema.ColumnSchemaPersisted)
	if bundle == nil {
		return out
	}
	tableIDToName := make(map[string]string)
	for _, t := range bundle.Tables {
		tableIDToName[t.ID] = strings.TrimSpace(t.TableName)
	}
	for _, c := range bundle.Columns {
		tableName := tableIDToName[c.TableSchemaID]
		if strings.TrimSpace(tableName) == "" || strings.TrimSpace(c.ColumnName) == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(tableName)) + "|" + strings.ToLower(strings.TrimSpace(c.ColumnName))
		out[key] = c
	}
	return out
}

// containsGeneratorType 判断候选集合是否包含目标生成器类型。
func containsGeneratorType(candidates []generator.GeneratorType, target generator.GeneratorType) bool {
	for _, c := range candidates {
		if strings.EqualFold(string(c), string(target)) {
			return true
		}
	}
	return false
}

// emptyNoConfigSnapshot 构造“无配置跳过”的空报告快照。
func emptyNoConfigSnapshot() schema.CompatibilityReportSnapshot {
	return schema.CompatibilityReportSnapshot{
		Status:          schema.CompatibilityRecheckStatusSkippedNoGeneratorConfig,
		GeneratedAtUnix: time.Now().Unix(),
		Summary: schema.CompatibilityReportSummary{
			Mode:          schema.GeneratorCompatibilityModeNoGeneratorConfig,
			TotalRisks:    0,
			BlockingRisks: 0,
		},
		Risks: []schema.GeneratorCompatibilityRisk{},
	}
}

// persistSnapshot 将报告快照写入 connection extra。
func (s *DefaultCompatibilityRecheckService) persistSnapshot(ctx context.Context, connectionID string, snap schema.CompatibilityReportSnapshot) error {
	// 写入前保证 risks 非 nil，保持 JSON 契约稳定。
	if snap.Risks == nil {
		snap.Risks = []schema.GeneratorCompatibilityRisk{}
	}
	return s.metaRepo.PatchConnectionSchemaMeta(ctx, connectionID, schema.ConnectionSchemaMetaPatch{
		CompatibilityReport: &snap,
	})
}

// schemaRiskKey 生成表/列复合键，确保比较大小写无关且稳定。
func schemaRiskKey(db, sch, table, col string) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(db)),
		strings.ToLower(strings.TrimSpace(sch)),
		strings.ToLower(strings.TrimSpace(table)),
		strings.ToLower(strings.TrimSpace(col)),
	}, "\x00")
}

