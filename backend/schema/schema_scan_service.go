package schema

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const (
	// SchemaScanServiceErrCodeFailedPrecondition 表示服务编排前置条件不满足。
	SchemaScanServiceErrCodeFailedPrecondition = "FAILED_PRECONDITION"
)

// SchemaScanService 编排扫描结果落地前的预览、风险与同步输入准备流程。
type SchemaScanService struct {
	// starter 负责创建扫描/重扫任务运行时上下文。
	starter *SchemaScanStarter

	// runtime 保存任务状态与范围上下文。
	runtime *SchemaScanRuntimeStore

	// currentRepo 负责读取当前持久化 schema。
	currentRepo CurrentSchemaRepository

	// diffEngine 负责内存扫描图与当前 schema 的对比。
	diffEngine *SchemaDiffEngine

	// riskAnalyzer 负责按 Diff 结果计算兼容性风险。
	riskAnalyzer *GeneratorCompatibilityAnalyzer

	// generatorStore 负责读取连接级生成器配置快照。
	generatorStore GeneratorConfigSnapshotStore

	// trustGate 负责可信度状态迁移与阻断校验。
	trustGate SchemaTrustGate

	// syncService 负责真正执行当前 schema 覆盖同步。
	syncService *SchemaSyncService

	// mu 保护预览与待同步快照缓存并发读写。
	mu sync.RWMutex

	// diffByTask 保存 task_id 对应的 Diff 结果。
	diffByTask map[string]*SchemaDiffResult

	// riskByTask 保存 task_id 对应的风险模式与列表。
	riskByTask map[string]GeneratorCompatibilityRisksResult

	// pendingByTask 保存 task_id 对应待同步的 schema bundle。
	pendingByTask map[string]*CurrentSchemaBundle
}

// NewSchemaScanService 创建可直接作为 FFI 依赖注入的生产级扫描服务。
func NewSchemaScanService(
	starter *SchemaScanStarter,
	runtime *SchemaScanRuntimeStore,
	currentRepo CurrentSchemaRepository,
	diffEngine *SchemaDiffEngine,
	riskAnalyzer *GeneratorCompatibilityAnalyzer,
	generatorStore GeneratorConfigSnapshotStore,
	trustGate SchemaTrustGate,
) *SchemaScanService {
	if diffEngine == nil {
		diffEngine = NewSchemaDiffEngine()
	}
	if riskAnalyzer == nil {
		riskAnalyzer = NewGeneratorCompatibilityAnalyzer()
	}
	svc := &SchemaScanService{
		starter:        starter,
		runtime:        runtime,
		currentRepo:    currentRepo,
		diffEngine:     diffEngine,
		riskAnalyzer:   riskAnalyzer,
		generatorStore: generatorStore,
		trustGate:      trustGate,
		diffByTask:     make(map[string]*SchemaDiffResult),
		riskByTask:     make(map[string]GeneratorCompatibilityRisksResult),
		pendingByTask:  make(map[string]*CurrentSchemaBundle),
	}
	svc.syncService = NewSchemaSyncService(runtime, svc, currentRepo, trustGate)
	return svc
}

// StartSchemaScan 启动扫描任务并返回 task_id 与初始状态。
func (s *SchemaScanService) StartSchemaScan(connectionID string, scope SchemaScanScope, tableNames []string, trigger string) (*SchemaScanStartResult, *SchemaScanStartError) {
	return s.starter.StartSchemaScan(context.Background(), StartSchemaScanRequest{
		ConnectionID: connectionID,
		Scope:        scope,
		TableNames:   tableNames,
		Trigger:      trigger,
	})
}

// StartSchemaRescan 启动重扫任务并返回 task_id 与初始状态。
func (s *SchemaScanService) StartSchemaRescan(connectionID string, strategy SchemaRescanStrategy, reason string, impactedTableNames []string) (*SchemaScanStartResult, *SchemaScanStartError) {
	return s.starter.StartSchemaRescan(context.Background(), StartSchemaRescanRequest{
		ConnectionID:       connectionID,
		Strategy:           strategy,
		Reason:             reason,
		ImpactedTableNames: impactedTableNames,
	})
}

// GetSchemaScanStatus 返回运行时任务状态快照。
func (s *SchemaScanService) GetSchemaScanStatus(taskID string) (SchemaScanStatusSnapshot, *SchemaScanStatusError) {
	return s.runtime.GetSchemaScanStatus(taskID)
}

// CompleteSchemaScan 在扫描完成后构建 Diff/风险/待同步快照并标记预览就绪。
func (s *SchemaScanService) CompleteSchemaScan(ctx context.Context, taskID string, scanned *SchemaGraph) *SchemaDiffError {
	trimmedTaskID := strings.TrimSpace(taskID)
	if trimmedTaskID == "" {
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "task_id is required"}
	}
	runtimeCtx, ok := s.runtime.GetRuntimeContext(trimmedTaskID)
	if !ok {
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "schema scan task not found"}
	}
	if runtimeCtx.Status != SchemaScanTaskRunning {
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "schema scan task must be running before complete"}
	}
	if scanned == nil {
		s.runtime.MarkFailed(ctx, trimmedTaskID, fmt.Errorf("scanned schema snapshot is required"))
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "scanned schema snapshot is required"}
	}

	current, err := s.currentRepo.LoadCurrentSchema(ctx, runtimeCtx.ConnectionID)
	if err != nil {
		s.runtime.MarkFailed(ctx, trimmedTaskID, err)
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "load current schema failed"}
	}
	diff, diffErr := s.diffEngine.Compare(current, scanned, SchemaDiffCompareOptions{
		Scope:      runtimeCtx.Scope,
		TableNames: runtimeCtx.TableNames,
	})
	if diffErr != nil {
		s.runtime.MarkFailed(ctx, trimmedTaskID, diffErr)
		return diffErr
	}
	riskResult, riskErr := s.computeRisks(ctx, runtimeCtx.ConnectionID, diff)
	if riskErr != nil {
		s.runtime.MarkFailed(ctx, trimmedTaskID, riskErr)
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: riskErr.Error()}
	}
	if _, err := s.trustGate.UpdateTrustState(ctx, runtimeCtx.ConnectionID, TrustStateUpdateInput{
		HasBlockingRisk: hasBlockingRisk(riskResult.Risks),
		RescanCompleted: true,
		SyncSucceeded:   false,
	}); err != nil {
		s.runtime.MarkFailed(ctx, trimmedTaskID, err)
		return &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "update trust state failed"}
	}

	s.storePreviewArtifacts(trimmedTaskID, diff, riskResult, schemaGraphToBundle(runtimeCtx.ConnectionID, scanned))
	s.runtime.MarkCompleted(trimmedTaskID, true)
	return nil
}

// PreviewSchemaDiff 返回任务的 Diff 预览结果。
func (s *SchemaScanService) PreviewSchemaDiff(taskID string) (*SchemaDiffResult, *SchemaDiffError) {
	trimmedTaskID := strings.TrimSpace(taskID)
	if trimmedTaskID == "" {
		return nil, &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "task_id is required"}
	}
	runtimeCtx, ok := s.runtime.GetRuntimeContext(trimmedTaskID)
	if !ok {
		return nil, &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "schema scan task not found"}
	}
	if runtimeCtx.Status != SchemaScanTaskCompleted || !runtimeCtx.PreviewReady {
		return nil, &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "diff preview not ready"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	diff, ok := s.diffByTask[trimmedTaskID]
	if !ok || diff == nil {
		return nil, &SchemaDiffError{Code: SchemaScanServiceErrCodeFailedPrecondition, Message: "diff preview not ready"}
	}
	return cloneSchemaDiffResult(diff), nil
}

// GetGeneratorCompatibilityRisks 返回任务对应风险模式与风险列表。
func (s *SchemaScanService) GetGeneratorCompatibilityRisks(taskID string) (GeneratorCompatibilityRisksResult, error) {
	riskResult, err := GetGeneratorCompatibilityRisks(
		context.Background(),
		taskID,
		s.runtime,
		s,
		s.generatorStore,
		s.riskAnalyzer,
	)
	if err != nil {
		return GeneratorCompatibilityRisksResult{}, err
	}
	trimmedTaskID := strings.TrimSpace(taskID)
	s.mu.Lock()
	s.riskByTask[trimmedTaskID] = cloneRiskResult(riskResult)
	s.mu.Unlock()
	return cloneRiskResult(riskResult), nil
}

// ApplySchemaSync 执行同步流程；内部复用 SchemaSyncService。
func (s *SchemaScanService) ApplySchemaSync(taskID string, ackRiskIDs []string) (*ApplySchemaSyncResult, *SchemaSyncError) {
	return s.syncService.ApplySchemaSync(context.Background(), ApplySchemaSyncRequest{
		TaskID:     taskID,
		AckRiskIDs: ackRiskIDs,
	})
}

// GetCurrentSchema 返回连接当前持久化 schema。
func (s *SchemaScanService) GetCurrentSchema(connectionID string, _ string) (*CurrentSchemaBundle, error) {
	return s.currentRepo.LoadCurrentSchema(context.Background(), strings.TrimSpace(connectionID))
}

// GetSchemaTrustState 返回连接可信度状态视图。
func (s *SchemaScanService) GetSchemaTrustState(connectionID string) (TrustStateView, error) {
	return s.trustGate.GetSchemaTrustState(context.Background(), strings.TrimSpace(connectionID))
}

// LoadPendingSchemaBundle 供同步服务读取待同步快照。
func (s *SchemaScanService) LoadPendingSchemaBundle(_ context.Context, taskID string) (*CurrentSchemaBundle, error) {
	trimmedTaskID := strings.TrimSpace(taskID)
	if trimmedTaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.pendingByTask[trimmedTaskID]
	if !ok || bundle == nil {
		return nil, fmt.Errorf("pending schema bundle not found")
	}
	return cloneCurrentSchemaBundleInternal(bundle), nil
}

// LoadSchemaDiffByTaskID 返回任务对应 Diff 快照，供风险分析生产链路按需读取。
func (s *SchemaScanService) LoadSchemaDiffByTaskID(taskID string) (*SchemaDiffResult, error) {
	trimmedTaskID := strings.TrimSpace(taskID)
	if trimmedTaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	diff, ok := s.diffByTask[trimmedTaskID]
	if !ok || diff == nil {
		return nil, fmt.Errorf("diff preview not ready")
	}
	return cloneSchemaDiffResult(diff), nil
}

// computeRisks 读取生成器配置并计算风险列表与模式。
func (s *SchemaScanService) computeRisks(ctx context.Context, connectionID string, diff *SchemaDiffResult) (GeneratorCompatibilityRisksResult, error) {
	if s.generatorStore == nil {
		return GeneratorCompatibilityRisksResult{
			Mode:  GeneratorCompatibilityModeNoGeneratorConfig,
			Risks: []GeneratorCompatibilityRisk{},
		}, nil
	}
	snapshot, err := s.generatorStore.LoadByConnectionID(ctx, strings.TrimSpace(connectionID))
	if err != nil {
		return GeneratorCompatibilityRisksResult{}, err
	}
	if snapshot == nil || len(snapshot.Columns) == 0 {
		return GeneratorCompatibilityRisksResult{
			Mode:  GeneratorCompatibilityModeNoGeneratorConfig,
			Risks: []GeneratorCompatibilityRisk{},
		}, nil
	}
	risks := s.riskAnalyzer.Analyze(diff, snapshot)
	if risks == nil {
		risks = []GeneratorCompatibilityRisk{}
	}
	return GeneratorCompatibilityRisksResult{
		Mode:  GeneratorCompatibilityModeConfigured,
		Risks: risks,
	}, nil
}

// storePreviewArtifacts 将本次完成扫描的预览产物写入内存缓存。
func (s *SchemaScanService) storePreviewArtifacts(taskID string, diff *SchemaDiffResult, risk GeneratorCompatibilityRisksResult, pending *CurrentSchemaBundle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diffByTask[taskID] = cloneSchemaDiffResult(diff)
	s.riskByTask[taskID] = cloneRiskResult(risk)
	s.pendingByTask[taskID] = cloneCurrentSchemaBundleInternal(pending)
}

// hasBlockingRisk 判断风险列表中是否存在阻断级风险。
func hasBlockingRisk(risks []GeneratorCompatibilityRisk) bool {
	for _, r := range risks {
		if r.Severity == GeneratorCompatibilityRiskSeverityBlocking {
			return true
		}
	}
	return false
}

// schemaGraphToBundle 将扫描内存图转换为可同步覆盖的当前 schema bundle。
func schemaGraphToBundle(connectionID string, g *SchemaGraph) *CurrentSchemaBundle {
	out := &CurrentSchemaBundle{
		Tables:  make([]TableSchemaPersisted, 0),
		Columns: make([]ColumnSchemaPersisted, 0),
	}
	if g == nil {
		return out
	}
	for i := range g.Tables {
		table := g.Tables[i]
		tableID := fmt.Sprintf("scan_tbl_%d", i+1)
		out.Tables = append(out.Tables, TableSchemaPersisted{
			ID:           tableID,
			ConnectionID: strings.TrimSpace(connectionID),
			DatabaseName: table.DatabaseName,
			SchemaName:   table.SchemaName,
			TableName:    table.TableName,
		})
		for _, col := range table.Columns {
			out.Columns = append(out.Columns, ColumnSchemaPersisted{
				ID:              fmt.Sprintf("%s_col_%s", tableID, strings.TrimSpace(col.Name)),
				TableSchemaID:   tableID,
				ColumnName:      col.Name,
				OrdinalPos:      col.OrdinalPos,
				DataType:        col.DataType,
				AbstractType:    col.AbstractType,
				IsPrimaryKey:    containsStringCaseInsensitive(table.PrimaryKey, col.Name),
				IsNullable:      col.IsNullable,
				IsUnique:        containsUniqueConstraintColumn(table.UniqueConstraints, col.Name),
				IsAutoIncrement: col.IsAutoIncrement,
				DefaultValue:    col.DefaultValue,
			})
		}
	}
	return out
}

// containsStringCaseInsensitive 判断切片中是否包含目标值（大小写不敏感）。
func containsStringCaseInsensitive(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

// containsUniqueConstraintColumn 判断列是否出现在任一唯一约束中。
func containsUniqueConstraintColumn(defs []UniqueConstraintDef, column string) bool {
	for _, def := range defs {
		for _, c := range def.Columns {
			if strings.EqualFold(strings.TrimSpace(c), strings.TrimSpace(column)) {
				return true
			}
		}
	}
	return false
}

// cloneSchemaDiffResult 深拷贝 Diff 结果，避免调用方修改内部缓存。
func cloneSchemaDiffResult(in *SchemaDiffResult) *SchemaDiffResult {
	if in == nil {
		return &SchemaDiffResult{}
	}
	out := &SchemaDiffResult{
		TableDiffs: make([]SchemaTableDiff, len(in.TableDiffs)),
		Summary:    in.Summary,
	}
	for i := range in.TableDiffs {
		out.TableDiffs[i] = in.TableDiffs[i]
		out.TableDiffs[i].ColumnDiffs = make([]SchemaColumnDiff, len(in.TableDiffs[i].ColumnDiffs))
		copy(out.TableDiffs[i].ColumnDiffs, in.TableDiffs[i].ColumnDiffs)
		out.TableDiffs[i].TableLevelChanges = append([]string(nil), in.TableDiffs[i].TableLevelChanges...)
	}
	return out
}

// cloneRiskResult 深拷贝风险结果，避免缓存被外部修改。
func cloneRiskResult(in GeneratorCompatibilityRisksResult) GeneratorCompatibilityRisksResult {
	out := GeneratorCompatibilityRisksResult{
		Mode:  in.Mode,
		Risks: make([]GeneratorCompatibilityRisk, len(in.Risks)),
	}
	copy(out.Risks, in.Risks)
	return out
}

// cloneCurrentSchemaBundleInternal 深拷贝当前 schema bundle。
func cloneCurrentSchemaBundleInternal(in *CurrentSchemaBundle) *CurrentSchemaBundle {
	if in == nil {
		return &CurrentSchemaBundle{}
	}
	out := &CurrentSchemaBundle{
		Tables:  make([]TableSchemaPersisted, len(in.Tables)),
		Columns: make([]ColumnSchemaPersisted, len(in.Columns)),
	}
	copy(out.Tables, in.Tables)
	copy(out.Columns, in.Columns)
	return out
}
