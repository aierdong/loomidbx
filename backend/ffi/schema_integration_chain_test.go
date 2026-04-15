package ffi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"loomidbx/ffi"
	"loomidbx/schema"
)

// TestSchemaIntegration_FullChain_WithTwoDialects 验证 6.2 端到端链路：
// 全库扫描、单表重扫、内存 Diff、UI Diff 契约、自动/手动同步。
func TestSchemaIntegration_FullChain_WithTwoDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		// name 为子用例名称。
		name string
		// baselineType 为初始 schema 的 name 列类型。
		baselineType string
		// fullScanNameType 为全库扫描阶段 name 列类型（无阻断风险）。
		fullScanNameType string
		// rescanNameType 为单表重扫阶段 name 列类型（制造阻断风险）。
		rescanNameType string
	}{
		{
			name:             "mysql",
			baselineType:     "varchar(255)",
			fullScanNameType: "varchar(255)",
			rescanNameType:   "bigint",
		},
		{
			name:             "postgres",
			baselineType:     "text",
			fullScanNameType: "text",
			rescanNameType:   "int8",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const connID = "conn-integration"
			h := newSchemaIntegrationHarness(t, connID)
			h.seedCurrentSchema(buildCurrentSchemaBaseline(connID, tc.baselineType))

			// 设置生成器配置：绑定 users.name，便于在重扫时触发类型不兼容阻断风险。
			h.setGeneratorSnapshot(connID, &schema.GeneratorConfigSnapshot{
				Columns: []schema.GeneratorColumnConfig{
					{
						ConnectionID: connID,
						DatabaseName: "appdb",
						SchemaName:   "public",
						TableName:    "users",
						ColumnName:   "name",
						ConfigID:     "cfg-users-name",
					},
				},
			})

			adapter := ffi.NewSchemaFFIAdapter(h.dependencies())

			// 1) 全库扫描：无阻断风险，允许自动同步。
			taskAll := extractTaskID(t, adapter.StartSchemaScan(`{"connection_id":"conn-integration","scope":"all","trigger":"manual"}`))
			h.completeScanTask(
				t,
				taskAll,
				buildScannedGraphForAll(tc.fullScanNameType),
				schema.SchemaScanScopeAll,
				nil,
			)

			fullPreview := adapter.PreviewSchemaDiff(fmt.Sprintf(`{"task_id":"%s"}`, taskAll))
			assertPreviewAction(t, fullPreview, true, false)

			fullSyncResp := adapter.ApplySchemaSync(fmt.Sprintf(`{"task_id":"%s","ack_risk_ids":[]}`, taskAll))
			assertFFIOKWithData(t, fullSyncResp)

			// 2) 单表重扫：制造阻断风险，先阻止同步，再手动确认后同步。
			taskRescan := extractTaskID(t, adapter.StartSchemaRescan(`{"connection_id":"conn-integration","strategy":"impacted","reason":"schema changed","impacted_table_names":["users"]}`))
			h.completeScanTask(
				t,
				taskRescan,
				buildScannedGraphForImpactedRescan(tc.rescanNameType),
				schema.SchemaScanScopeTables,
				[]string{"users"},
			)

			rescanPreview := adapter.PreviewSchemaDiff(fmt.Sprintf(`{"task_id":"%s"}`, taskRescan))
			assertPreviewAction(t, rescanPreview, false, true)

			blockedSyncResp := adapter.ApplySchemaSync(fmt.Sprintf(`{"task_id":"%s","ack_risk_ids":[]}`, taskRescan))
			assertFFIErrorCode(t, blockedSyncResp, "BLOCKING_RISK_UNRESOLVED")

			manualSyncResp := adapter.ApplySchemaSync(fmt.Sprintf(`{"task_id":"%s","ack_risk_ids":["ack-blocking-risk"]}`, taskRescan))
			assertFFIOKWithData(t, manualSyncResp)
		})
	}
}

// schemaIntegrationHarness 为集成测试提供端到端依赖拼装。
type schemaIntegrationHarness struct {
	// t 为当前测试上下文。
	t *testing.T
	// runtime 保存扫描任务运行时状态。
	runtime *schema.SchemaScanRuntimeStore
	// starter 实现 StartSchemaScan / StartSchemaRescan。
	starter *schema.SchemaScanStarter
	// currentRepo 模拟当前 schema 持久化仓储。
	currentRepo *integrationCurrentSchemaRepository
	// previewStore 保存 task_id 对应待同步 bundle。
	previewStore *integrationPreviewStore
	// trustRepo 为 TrustGate 提供连接元数据存取。
	trustRepo *integrationTrustRepo
	// trustGate 负责风险闸门与状态迁移。
	trustGate schema.SchemaTrustGate
	// diffEngine 负责内存 Diff。
	diffEngine *schema.SchemaDiffEngine
	// analyzer 负责兼容性风险分析。
	analyzer *schema.GeneratorCompatibilityAnalyzer
	// generatorSnapshots 保存连接级生成器配置快照。
	generatorSnapshots map[string]*schema.GeneratorConfigSnapshot
	// diffByTask 保存 task_id 对应 Diff 结果。
	diffByTask map[string]*schema.SchemaDiffResult
	// risksByTask 保存 task_id 对应风险结果。
	risksByTask map[string]schema.GeneratorCompatibilityRisksResult
	// syncService 为 ApplySchemaSync 提供真实服务实现。
	syncService *schema.SchemaSyncService
	// taskSeq 为可预测 task_id 序号。
	taskSeq int
}

// integrationCurrentSchemaRepository 为测试提供内存 CurrentSchemaRepository。
type integrationCurrentSchemaRepository struct {
	// byConn 维护 connection_id 到当前 bundle 的映射。
	byConn map[string]*schema.CurrentSchemaBundle
}

// LoadCurrentSchema 返回连接当前 schema 副本。
func (r *integrationCurrentSchemaRepository) LoadCurrentSchema(_ context.Context, connectionID string) (*schema.CurrentSchemaBundle, error) {
	b, ok := r.byConn[connectionID]
	if !ok {
		return &schema.CurrentSchemaBundle{}, nil
	}
	return cloneCurrentSchemaBundle(b), nil
}

// TransactionalReplaceCurrentSchema 以覆盖语义替换当前 schema。
func (r *integrationCurrentSchemaRepository) TransactionalReplaceCurrentSchema(_ context.Context, connectionID string, next *schema.CurrentSchemaBundle) error {
	if strings.TrimSpace(connectionID) == "" {
		return fmt.Errorf("connection id is required")
	}
	r.byConn[connectionID] = cloneCurrentSchemaBundle(next)
	return nil
}

// integrationPreviewStore 为测试提供待同步快照读取。
type integrationPreviewStore struct {
	// byTask 保存 task_id 到 bundle 的映射。
	byTask map[string]*schema.CurrentSchemaBundle
}

// LoadPendingSchemaBundle 返回 task_id 对应待同步 bundle。
func (s *integrationPreviewStore) LoadPendingSchemaBundle(_ context.Context, taskID string) (*schema.CurrentSchemaBundle, error) {
	b, ok := s.byTask[taskID]
	if !ok {
		return nil, fmt.Errorf("bundle not found")
	}
	return cloneCurrentSchemaBundle(b), nil
}

// integrationTrustRepo 为测试提供内存 TrustConnectionMetaRepository。
type integrationTrustRepo struct {
	// byConn 保存连接元数据。
	byConn map[string]schema.ConnectionSchemaMeta
}

// LoadConnectionSchemaMeta 读取连接元数据。
func (r *integrationTrustRepo) LoadConnectionSchemaMeta(_ context.Context, connectionID string) (schema.ConnectionSchemaMeta, error) {
	meta, ok := r.byConn[connectionID]
	if !ok {
		return schema.ConnectionSchemaMeta{}, fmt.Errorf("connection not found")
	}
	return meta, nil
}

// PatchConnectionSchemaMeta 合并写入连接元数据补丁。
func (r *integrationTrustRepo) PatchConnectionSchemaMeta(_ context.Context, connectionID string, patch schema.ConnectionSchemaMetaPatch) error {
	cur, ok := r.byConn[connectionID]
	if !ok {
		return fmt.Errorf("connection not found")
	}
	if patch.TrustState != nil {
		cur.SchemaTrustState = *patch.TrustState
	}
	if patch.LastBlockingReason != nil {
		cur.SchemaLastBlockingReason = *patch.LastBlockingReason
	}
	if patch.LastSchemaScanUnix != nil {
		cur.LastSchemaScanUnix = *patch.LastSchemaScanUnix
	}
	if patch.LastSchemaSyncUnix != nil {
		cur.LastSchemaSyncUnix = *patch.LastSchemaSyncUnix
	}
	r.byConn[connectionID] = cur
	return nil
}

// integrationSchemaStarterAdapter 适配 schema starter 到 FFI 接口。
type integrationSchemaStarterAdapter struct {
	// starter 为真实 starter 实例。
	starter *schema.SchemaScanStarter
}

// StartSchemaScan 启动扫描任务。
func (a integrationSchemaStarterAdapter) StartSchemaScan(connectionID string, scope schema.SchemaScanScope, tableNames []string, trigger string) (*schema.SchemaScanStartResult, *schema.SchemaScanStartError) {
	return a.starter.StartSchemaScan(context.Background(), schema.StartSchemaScanRequest{
		ConnectionID: connectionID,
		Scope:        scope,
		TableNames:   tableNames,
		Trigger:      trigger,
	})
}

// StartSchemaRescan 启动重扫任务。
func (a integrationSchemaStarterAdapter) StartSchemaRescan(connectionID string, strategy schema.SchemaRescanStrategy, reason string, impactedTableNames []string) (*schema.SchemaScanStartResult, *schema.SchemaScanStartError) {
	return a.starter.StartSchemaRescan(context.Background(), schema.StartSchemaRescanRequest{
		ConnectionID:       connectionID,
		Strategy:           strategy,
		Reason:             reason,
		ImpactedTableNames: impactedTableNames,
	})
}

// integrationStatusReader 适配运行时状态读取。
type integrationStatusReader struct {
	// runtime 为扫描运行时存储。
	runtime *schema.SchemaScanRuntimeStore
}

// GetSchemaScanStatus 返回任务状态。
func (r integrationStatusReader) GetSchemaScanStatus(taskID string) (schema.SchemaScanStatusSnapshot, *schema.SchemaScanStatusError) {
	return r.runtime.GetSchemaScanStatus(taskID)
}

// integrationPreviewer 适配 Diff 预览读取。
type integrationPreviewer struct {
	// diffByTask 为 task_id 对应 Diff。
	diffByTask map[string]*schema.SchemaDiffResult
}

// PreviewSchemaDiff 返回任务 Diff。
func (p integrationPreviewer) PreviewSchemaDiff(taskID string) (*schema.SchemaDiffResult, *schema.SchemaDiffError) {
	d, ok := p.diffByTask[taskID]
	if !ok {
		return nil, &schema.SchemaDiffError{
			Code:    schema.SchemaDiffErrCodeFailedPrecondition,
			Message: "diff preview not ready",
		}
	}
	return d, nil
}

// integrationRiskReader 适配风险读取。
type integrationRiskReader struct {
	// risksByTask 为 task_id 对应风险结果。
	risksByTask map[string]schema.GeneratorCompatibilityRisksResult
}

// GetGeneratorCompatibilityRisks 返回任务风险模式与列表。
func (r integrationRiskReader) GetGeneratorCompatibilityRisks(taskID string) (schema.GeneratorCompatibilityRisksResult, error) {
	result, ok := r.risksByTask[taskID]
	if !ok {
		return schema.GeneratorCompatibilityRisksResult{}, fmt.Errorf("risk result not ready")
	}
	return result, nil
}

// integrationSyncer 适配 SyncService 到 FFI 接口。
type integrationSyncer struct {
	// svc 为真实同步服务。
	svc *schema.SchemaSyncService
}

// ApplySchemaSync 执行同步。
func (s integrationSyncer) ApplySchemaSync(taskID string, ackRiskIDs []string) (*schema.ApplySchemaSyncResult, *schema.SchemaSyncError) {
	return s.svc.ApplySchemaSync(context.Background(), schema.ApplySchemaSyncRequest{
		TaskID:     taskID,
		AckRiskIDs: ackRiskIDs,
	})
}

// integrationCurrentReader 适配当前 schema 读取。
type integrationCurrentReader struct {
	// repo 为当前 schema 仓储。
	repo *integrationCurrentSchemaRepository
}

// GetCurrentSchema 返回当前 schema。
func (r integrationCurrentReader) GetCurrentSchema(connectionID string, _ string) (*schema.CurrentSchemaBundle, error) {
	return r.repo.LoadCurrentSchema(context.Background(), connectionID)
}

// integrationTrustReader 适配信任状态读取。
type integrationTrustReader struct {
	// gate 为信任状态闸门。
	gate schema.SchemaTrustGate
}

// GetSchemaTrustState 返回连接可信度状态。
func (r integrationTrustReader) GetSchemaTrustState(connectionID string) (schema.TrustStateView, error) {
	return r.gate.GetSchemaTrustState(context.Background(), connectionID)
}

// newSchemaIntegrationHarness 创建集成测试依赖图。
func newSchemaIntegrationHarness(t *testing.T, connectionID string) *schemaIntegrationHarness {
	t.Helper()

	h := &schemaIntegrationHarness{
		t:                 t,
		runtime:           schema.NewSchemaScanRuntimeStore(),
		currentRepo:       &integrationCurrentSchemaRepository{byConn: make(map[string]*schema.CurrentSchemaBundle)},
		previewStore:      &integrationPreviewStore{byTask: make(map[string]*schema.CurrentSchemaBundle)},
		trustRepo:         &integrationTrustRepo{byConn: make(map[string]schema.ConnectionSchemaMeta)},
		diffEngine:        schema.NewSchemaDiffEngine(),
		analyzer:          schema.NewGeneratorCompatibilityAnalyzer(),
		generatorSnapshots: make(map[string]*schema.GeneratorConfigSnapshot),
		diffByTask:        make(map[string]*schema.SchemaDiffResult),
		risksByTask:       make(map[string]schema.GeneratorCompatibilityRisksResult),
	}
	h.trustRepo.byConn[connectionID] = schema.ConnectionSchemaMeta{
		SchemaTrustState: schema.SchemaTrustTrusted,
	}
	h.trustGate = schema.NewSchemaTrustGate(h.trustRepo)
	h.starter = schema.NewSchemaScanStarterWithTaskID(h.runtime, func() string {
		h.taskSeq++
		return fmt.Sprintf("task-%d", h.taskSeq)
	})
	h.syncService = schema.NewSchemaSyncService(h.runtime, h.previewStore, h.currentRepo, h.trustGate)
	return h
}

// dependencies 构建 FFI 适配器依赖。
func (h *schemaIntegrationHarness) dependencies() ffi.SchemaFFIDependencies {
	return ffi.SchemaFFIDependencies{
		Starter:      integrationSchemaStarterAdapter{starter: h.starter},
		StatusReader: integrationStatusReader{runtime: h.runtime},
		Previewer:    integrationPreviewer{diffByTask: h.diffByTask},
		RiskReader:   integrationRiskReader{risksByTask: h.risksByTask},
		Syncer:       integrationSyncer{svc: h.syncService},
		CurrentReader: integrationCurrentReader{
			repo: h.currentRepo,
		},
		TrustReader: integrationTrustReader{gate: h.trustGate},
	}
}

// seedCurrentSchema 设置连接当前 schema 基线。
func (h *schemaIntegrationHarness) seedCurrentSchema(bundle *schema.CurrentSchemaBundle) {
	h.currentRepo.byConn["conn-integration"] = cloneCurrentSchemaBundle(bundle)
}

// setGeneratorSnapshot 设置连接生成器快照。
func (h *schemaIntegrationHarness) setGeneratorSnapshot(connectionID string, snapshot *schema.GeneratorConfigSnapshot) {
	h.generatorSnapshots[connectionID] = snapshot
}

// completeScanTask 模拟扫描完成并写入 Diff/风险/待同步数据。
func (h *schemaIntegrationHarness) completeScanTask(
	t *testing.T,
	taskID string,
	scanned *schema.SchemaGraph,
	scope schema.SchemaScanScope,
	tableNames []string,
) {
	t.Helper()

	rt, ok := h.runtime.GetRuntimeContext(taskID)
	if !ok {
		t.Fatalf("runtime task not found: %s", taskID)
	}
	current, err := h.currentRepo.LoadCurrentSchema(context.Background(), rt.ConnectionID)
	if err != nil {
		t.Fatalf("load current schema failed: %v", err)
	}
	diff, diffErr := h.diffEngine.Compare(current, scanned, schema.SchemaDiffCompareOptions{
		Scope:      scope,
		TableNames: tableNames,
	})
	if diffErr != nil {
		t.Fatalf("compute diff failed: %v", diffErr)
	}
	h.diffByTask[taskID] = diff

	snapshot := h.generatorSnapshots[rt.ConnectionID]
	risks := h.analyzer.Analyze(diff, snapshot)
	mode := schema.GeneratorCompatibilityModeNoGeneratorConfig
	if snapshot != nil && len(snapshot.Columns) > 0 {
		mode = schema.GeneratorCompatibilityModeConfigured
	}
	h.risksByTask[taskID] = schema.GeneratorCompatibilityRisksResult{
		Mode:  mode,
		Risks: risks,
	}
	h.previewStore.byTask[taskID] = schemaGraphToCurrentBundle(rt.ConnectionID, scanned)

	_, updateErr := h.trustGate.UpdateTrustState(context.Background(), rt.ConnectionID, schema.TrustStateUpdateInput{
		HasBlockingRisk: containsBlockingRisk(risks),
		RescanCompleted: true,
		SyncSucceeded:   false,
	})
	if updateErr != nil {
		t.Fatalf("update trust state failed: %v", updateErr)
	}
	h.runtime.MarkCompleted(taskID, true)
}

// extractTaskID 从 StartSchemaScan/StartSchemaRescan 返回中解析 task_id。
func extractTaskID(t *testing.T, resp string) string {
	t.Helper()
	assertFFIOKWithData(t, resp)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data payload: %s", resp)
	}
	rawTaskID, _ := data["TaskID"].(string)
	if strings.TrimSpace(rawTaskID) == "" {
		rawTaskID, _ = data["task_id"].(string)
	}
	if strings.TrimSpace(rawTaskID) == "" {
		t.Fatalf("task_id should not be empty: %s", resp)
	}
	return rawTaskID
}

// assertPreviewAction 校验 UI Diff 呈现契约中的 action 行为。
func assertPreviewAction(t *testing.T, resp string, canApply bool, requiresAdjust bool) {
	t.Helper()
	assertFFIOKWithData(t, resp)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	data, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data payload: %s", resp)
	}
	action, ok := data["action"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing action payload: %s", resp)
	}
	if action["can_apply_sync"] != canApply {
		t.Fatalf("can_apply_sync mismatch: got %v want %v", action["can_apply_sync"], canApply)
	}
	if action["requires_adjustment_before_sync"] != requiresAdjust {
		t.Fatalf("requires_adjustment_before_sync mismatch: got %v want %v", action["requires_adjustment_before_sync"], requiresAdjust)
	}
}

// containsBlockingRisk 判断风险列表是否包含阻断级风险。
func containsBlockingRisk(risks []schema.GeneratorCompatibilityRisk) bool {
	for _, r := range risks {
		if r.Severity == schema.GeneratorCompatibilityRiskSeverityBlocking {
			return true
		}
	}
	return false
}

// cloneCurrentSchemaBundle 深拷贝 bundle，避免测试状态串扰。
func cloneCurrentSchemaBundle(in *schema.CurrentSchemaBundle) *schema.CurrentSchemaBundle {
	if in == nil {
		return &schema.CurrentSchemaBundle{}
	}
	out := &schema.CurrentSchemaBundle{
		Tables:  make([]schema.TableSchemaPersisted, len(in.Tables)),
		Columns: make([]schema.ColumnSchemaPersisted, len(in.Columns)),
	}
	copy(out.Tables, in.Tables)
	copy(out.Columns, in.Columns)
	return out
}

// schemaGraphToCurrentBundle 将扫描内存图转换为可同步的当前 schema bundle。
func schemaGraphToCurrentBundle(connectionID string, g *schema.SchemaGraph) *schema.CurrentSchemaBundle {
	if g == nil {
		return &schema.CurrentSchemaBundle{}
	}
	out := &schema.CurrentSchemaBundle{}
	for ti, table := range g.Tables {
		tableID := fmt.Sprintf("tbl-%d", ti+1)
		out.Tables = append(out.Tables, schema.TableSchemaPersisted{
			ID:           tableID,
			ConnectionID: connectionID,
			DatabaseName: table.DatabaseName,
			SchemaName:   table.SchemaName,
			TableName:    table.TableName,
		})
		for _, col := range table.Columns {
			out.Columns = append(out.Columns, schema.ColumnSchemaPersisted{
				ID:              fmt.Sprintf("%s-col-%s", tableID, col.Name),
				TableSchemaID:   tableID,
				ColumnName:      col.Name,
				OrdinalPos:      col.OrdinalPos,
				DataType:        col.DataType,
				AbstractType:    col.AbstractType,
				IsPrimaryKey:    containsString(table.PrimaryKey, col.Name),
				IsNullable:      col.IsNullable,
				IsUnique:        containsUniqueColumn(table.UniqueConstraints, col.Name),
				IsAutoIncrement: col.IsAutoIncrement,
				DefaultValue:    col.DefaultValue,
			})
		}
	}
	return out
}

// containsString 判断字符串切片中是否存在目标值（大小写不敏感）。
func containsString(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

// containsUniqueColumn 判断列是否在任一唯一约束中。
func containsUniqueColumn(defs []schema.UniqueConstraintDef, column string) bool {
	for _, def := range defs {
		for _, c := range def.Columns {
			if strings.EqualFold(strings.TrimSpace(c), strings.TrimSpace(column)) {
				return true
			}
		}
	}
	return false
}

// buildCurrentSchemaBaseline 构造基线当前 schema（users: id,name）。
func buildCurrentSchemaBaseline(connectionID string, nameType string) *schema.CurrentSchemaBundle {
	return &schema.CurrentSchemaBundle{
		Tables: []schema.TableSchemaPersisted{
			{
				ID:           "tbl-users",
				ConnectionID: connectionID,
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		Columns: []schema.ColumnSchemaPersisted{
			{
				ID:              "col-id",
				TableSchemaID:   "tbl-users",
				ColumnName:      "id",
				OrdinalPos:      1,
				DataType:        "bigint",
				AbstractType:    "int",
				IsPrimaryKey:    true,
				IsNullable:      false,
				IsUnique:        true,
				IsAutoIncrement: true,
			},
			{
				ID:            "col-name",
				TableSchemaID: "tbl-users",
				ColumnName:    "name",
				OrdinalPos:    2,
				DataType:      nameType,
				AbstractType:  "string",
				IsNullable:    false,
			},
		},
	}
}

// buildScannedGraphForAll 构造全库扫描结果（新增 email，name 维持字符串类型）。
func buildScannedGraphForAll(nameType string) *schema.SchemaGraph {
	return &schema.SchemaGraph{
		Tables: []schema.TableDef{
			{
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
				Columns: []schema.ColumnDef{
					{Name: "id", OrdinalPos: 1, DataType: "bigint", AbstractType: "int", IsNullable: false, IsAutoIncrement: true},
					{Name: "name", OrdinalPos: 2, DataType: nameType, AbstractType: "string", IsNullable: false},
					{Name: "email", OrdinalPos: 3, DataType: "varchar(255)", AbstractType: "string", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
				UniqueConstraints: []schema.UniqueConstraintDef{
					{Name: "uq_users_email", Columns: []string{"email"}},
				},
			},
		},
	}
}

// buildScannedGraphForImpactedRescan 构造按表重扫结果（name 变更为整型，触发阻断风险）。
func buildScannedGraphForImpactedRescan(nameType string) *schema.SchemaGraph {
	return &schema.SchemaGraph{
		Tables: []schema.TableDef{
			{
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
				Columns: []schema.ColumnDef{
					{Name: "id", OrdinalPos: 1, DataType: "bigint", AbstractType: "int", IsNullable: false, IsAutoIncrement: true},
					{Name: "name", OrdinalPos: 2, DataType: nameType, AbstractType: "int", IsNullable: false},
					{Name: "email", OrdinalPos: 3, DataType: "varchar(255)", AbstractType: "string", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
				UniqueConstraints: []schema.UniqueConstraintDef{
					{Name: "uq_users_email", Columns: []string{"email"}},
				},
			},
		},
	}
}
