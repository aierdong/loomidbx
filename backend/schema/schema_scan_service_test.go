package schema

import (
	"context"
	"testing"
)

// inMemoryCurrentSchemaRepo 为扫描服务测试提供内存版当前 schema 仓储。
type inMemoryCurrentSchemaRepo struct {
	// byConn 保存 connection_id 对应当前 schema。
	byConn map[string]*CurrentSchemaBundle
}

// LoadCurrentSchema 返回连接当前 schema 副本。
func (r *inMemoryCurrentSchemaRepo) LoadCurrentSchema(_ context.Context, connectionID string) (*CurrentSchemaBundle, error) {
	bundle, ok := r.byConn[connectionID]
	if !ok {
		return &CurrentSchemaBundle{}, nil
	}
	return cloneCurrentSchemaBundleInternal(bundle), nil
}

// TransactionalReplaceCurrentSchema 以覆盖语义替换连接当前 schema。
func (r *inMemoryCurrentSchemaRepo) TransactionalReplaceCurrentSchema(_ context.Context, connectionID string, next *CurrentSchemaBundle) error {
	r.byConn[connectionID] = cloneCurrentSchemaBundleInternal(next)
	return nil
}

// faultInjectingCurrentSchemaRepo 为扫描服务测试提供可注入写入失败的当前 schema 仓储。
type faultInjectingCurrentSchemaRepo struct {
	// byConn 保存 connection_id 对应当前 schema。
	byConn map[string]*CurrentSchemaBundle
	// replaceErrQueue 为按调用顺序注入的写入错误队列；为空时表示写入成功。
	replaceErrQueue []error
	// replaceCalls 记录写入调用次数，便于断言失败/恢复路径。
	replaceCalls int
}

// LoadCurrentSchema 返回连接当前 schema 副本。
func (r *faultInjectingCurrentSchemaRepo) LoadCurrentSchema(_ context.Context, connectionID string) (*CurrentSchemaBundle, error) {
	bundle, ok := r.byConn[connectionID]
	if !ok {
		return &CurrentSchemaBundle{}, nil
	}
	return cloneCurrentSchemaBundleInternal(bundle), nil
}

// TransactionalReplaceCurrentSchema 按队列注入错误并在成功时覆盖写入。
func (r *faultInjectingCurrentSchemaRepo) TransactionalReplaceCurrentSchema(_ context.Context, connectionID string, next *CurrentSchemaBundle) error {
	r.replaceCalls++
	if len(r.replaceErrQueue) > 0 {
		injectedErr := r.replaceErrQueue[0]
		r.replaceErrQueue = r.replaceErrQueue[1:]
		if injectedErr != nil {
			return injectedErr
		}
	}
	r.byConn[connectionID] = cloneCurrentSchemaBundleInternal(next)
	return nil
}

func TestSchemaScanService_CompletePreviewAndSync_NoGeneratorConfig(t *testing.T) {
	ctx := context.Background()
	runtime := NewSchemaScanRuntimeStore()
	starter := NewSchemaScanStarterWithTaskID(runtime, func() string { return "task-1" })
	repo := &inMemoryCurrentSchemaRepo{
		byConn: map[string]*CurrentSchemaBundle{
			"conn-1": buildServiceBaseline("conn-1"),
		},
	}
	trustRepo := newFakeTrustRepo()
	trustRepo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
	service := NewSchemaScanService(
		starter,
		runtime,
		repo,
		NewSchemaDiffEngine(),
		NewGeneratorCompatibilityAnalyzer(),
		nil,
		NewSchemaTrustGate(trustRepo),
		NoopCompatibilityRecheckService{},
	)

	start, startErr := service.StartSchemaScan("conn-1", SchemaScanScopeAll, nil, "manual")
	if startErr != nil {
		t.Fatalf("start failed: %v", startErr)
	}
	if start.TaskID != "task-1" {
		t.Fatalf("unexpected task id: %s", start.TaskID)
	}

	completeErr := service.CompleteSchemaScan(ctx, start.TaskID, buildServiceScannedGraphWithEmail())
	if completeErr != nil {
		t.Fatalf("complete scan failed: %v", completeErr)
	}

	preview, previewErr := service.PreviewSchemaDiff(start.TaskID)
	if previewErr != nil {
		t.Fatalf("preview failed: %v", previewErr)
	}
	if preview.Summary.AddedColumns == 0 {
		t.Fatalf("expected added columns in preview summary: %+v", preview.Summary)
	}

	risks, riskErr := service.GetGeneratorCompatibilityRisks(start.TaskID)
	if riskErr != nil {
		t.Fatalf("risks failed: %v", riskErr)
	}
	if risks.Mode != GeneratorCompatibilityModeNoGeneratorConfig {
		t.Fatalf("unexpected risk mode: %s", risks.Mode)
	}
	if len(risks.Risks) != 0 {
		t.Fatalf("expected empty risks when no generator config: %+v", risks.Risks)
	}

	syncResult, syncErr := service.ApplySchemaSync(start.TaskID, nil)
	if syncErr != nil {
		t.Fatalf("sync failed: %v", syncErr)
	}
	if !syncResult.SyncApplied {
		t.Fatalf("expected sync applied: %+v", syncResult)
	}

	current, err := repo.LoadCurrentSchema(ctx, "conn-1")
	if err != nil {
		t.Fatalf("reload current failed: %v", err)
	}
	if !bundleHasColumn(current, "users", "email") {
		t.Fatalf("expected synced schema to contain users.email")
	}
}

func TestSchemaScanService_PreviewSchemaDiff_NotReady(t *testing.T) {
	runtime := NewSchemaScanRuntimeStore()
	starter := NewSchemaScanStarterWithTaskID(runtime, func() string { return "task-not-ready" })
	repo := &inMemoryCurrentSchemaRepo{
		byConn: map[string]*CurrentSchemaBundle{
			"conn-1": buildServiceBaseline("conn-1"),
		},
	}
	trustRepo := newFakeTrustRepo()
	trustRepo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
	service := NewSchemaScanService(
		starter,
		runtime,
		repo,
		NewSchemaDiffEngine(),
		NewGeneratorCompatibilityAnalyzer(),
		nil,
		NewSchemaTrustGate(trustRepo),
		NoopCompatibilityRecheckService{},
	)

	start, startErr := service.StartSchemaScan("conn-1", SchemaScanScopeAll, nil, "manual")
	if startErr != nil {
		t.Fatalf("start failed: %v", startErr)
	}

	_, previewErr := service.PreviewSchemaDiff(start.TaskID)
	if previewErr == nil {
		t.Fatal("expected preview not ready error")
	}
	if previewErr.Code != SchemaScanServiceErrCodeFailedPrecondition {
		t.Fatalf("unexpected error code: %s", previewErr.Code)
	}
}

func TestSchemaScanService_BlockingRiskBlocksSyncWithoutAck(t *testing.T) {
	ctx := context.Background()
	runtime := NewSchemaScanRuntimeStore()
	starter := NewSchemaScanStarterWithTaskID(runtime, func() string { return "task-blocking" })
	repo := &inMemoryCurrentSchemaRepo{
		byConn: map[string]*CurrentSchemaBundle{
			"conn-1": buildServiceBaseline("conn-1"),
		},
	}
	trustRepo := newFakeTrustRepo()
	trustRepo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
	genStore := GeneratorConfigSnapshotStoreStub{
		SnapshotsByConnectionID: map[string]*GeneratorConfigSnapshot{
			"conn-1": {
				Columns: []GeneratorColumnConfig{
					{
						ConnectionID: "conn-1",
						DatabaseName: "appdb",
						SchemaName:   "public",
						TableName:    "users",
						ColumnName:   "name",
						GeneratorType: "EnumValueGenerator",
						ConfigID:     "cfg-users-name",
					},
				},
			},
		},
	}
	service := NewSchemaScanService(
		starter,
		runtime,
		repo,
		NewSchemaDiffEngine(),
		NewGeneratorCompatibilityAnalyzer(),
		genStore,
		NewSchemaTrustGate(trustRepo),
		NoopCompatibilityRecheckService{},
	)

	start, startErr := service.StartSchemaScan("conn-1", SchemaScanScopeAll, nil, "manual")
	if startErr != nil {
		t.Fatalf("start failed: %v", startErr)
	}

	completeErr := service.CompleteSchemaScan(ctx, start.TaskID, buildServiceScannedGraphNameTypeChanged())
	if completeErr != nil {
		t.Fatalf("complete scan failed: %v", completeErr)
	}

	risks, riskErr := service.GetGeneratorCompatibilityRisks(start.TaskID)
	if riskErr != nil {
		t.Fatalf("risks failed: %v", riskErr)
	}
	if risks.Mode != GeneratorCompatibilityModeConfigured {
		t.Fatalf("unexpected risk mode: %s", risks.Mode)
	}
	if len(risks.Risks) == 0 {
		t.Fatalf("expected blocking risks, got none")
	}
	if !hasBlockingRisk(risks.Risks) {
		t.Fatalf("expected at least one blocking risk: %+v", risks.Risks)
	}

	_, syncErr := service.ApplySchemaSync(start.TaskID, nil)
	if syncErr == nil {
		t.Fatal("expected blocking risk sync error")
	}
	if syncErr.Code != "BLOCKING_RISK_UNRESOLVED" {
		t.Fatalf("unexpected sync error code: %s", syncErr.Code)
	}
}

func TestSchemaScanService_EndToEndLoop_ScanDiffRiskSync(t *testing.T) {
	ctx := context.Background()
	runtime := NewSchemaScanRuntimeStore()
	starter := NewSchemaScanStarterWithTaskID(runtime, func() string {
		if _, err := runtime.GetSchemaScanStatus("task-e2e-all"); err != nil {
			return "task-e2e-all"
		}
		return "task-e2e-impacted"
	})
	repo := &inMemoryCurrentSchemaRepo{
		byConn: map[string]*CurrentSchemaBundle{
			"conn-1": buildServiceBaseline("conn-1"),
		},
	}
	trustRepo := newFakeTrustRepo()
	trustRepo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
	genStore := GeneratorConfigSnapshotStoreStub{
		SnapshotsByConnectionID: map[string]*GeneratorConfigSnapshot{
			"conn-1": {
				Columns: []GeneratorColumnConfig{
					{
						ConnectionID: "conn-1",
						DatabaseName: "appdb",
						SchemaName:   "public",
						TableName:    "users",
						ColumnName:   "name",
						GeneratorType: "EnumValueGenerator",
						ConfigID:     "cfg-users-name",
					},
				},
			},
		},
	}
	service := NewSchemaScanService(
		starter,
		runtime,
		repo,
		NewSchemaDiffEngine(),
		NewGeneratorCompatibilityAnalyzer(),
		genStore,
		NewSchemaTrustGate(trustRepo),
		NoopCompatibilityRecheckService{},
	)

	startAll, startAllErr := service.StartSchemaScan("conn-1", SchemaScanScopeAll, nil, "manual")
	if startAllErr != nil {
		t.Fatalf("start all scan failed: %v", startAllErr)
	}
	if startAll.TaskID != "task-e2e-all" {
		t.Fatalf("unexpected all-scan task id: %s", startAll.TaskID)
	}
	if completeErr := service.CompleteSchemaScan(ctx, startAll.TaskID, buildServiceScannedGraphWithEmail()); completeErr != nil {
		t.Fatalf("complete all scan failed: %v", completeErr)
	}
	previewAll, previewAllErr := service.PreviewSchemaDiff(startAll.TaskID)
	if previewAllErr != nil {
		t.Fatalf("preview all diff failed: %v", previewAllErr)
	}
	if previewAll.Summary.AddedColumns == 0 {
		t.Fatalf("all scan should include added columns: %+v", previewAll.Summary)
	}
	risksAll, risksAllErr := service.GetGeneratorCompatibilityRisks(startAll.TaskID)
	if risksAllErr != nil {
		t.Fatalf("get all risks failed: %v", risksAllErr)
	}
	if risksAll.Mode != GeneratorCompatibilityModeConfigured {
		t.Fatalf("unexpected all risk mode: %s", risksAll.Mode)
	}
	if len(risksAll.Risks) != 0 {
		t.Fatalf("all scan should not produce blocking risks: %+v", risksAll.Risks)
	}
	syncAll, syncAllErr := service.ApplySchemaSync(startAll.TaskID, nil)
	if syncAllErr != nil {
		t.Fatalf("sync all failed: %v", syncAllErr)
	}
	if !syncAll.SyncApplied || syncAll.TrustState != SchemaTrustTrusted {
		t.Fatalf("unexpected all sync result: %+v", syncAll)
	}
	currentAfterAll, err := repo.LoadCurrentSchema(ctx, "conn-1")
	if err != nil {
		t.Fatalf("reload current after all sync failed: %v", err)
	}
	if !bundleHasColumn(currentAfterAll, "users", "email") {
		t.Fatalf("expected users.email after all sync")
	}

	startImpacted, startImpactedErr := service.StartSchemaRescan(
		"conn-1",
		SchemaRescanStrategyImpacted,
		"users table changed",
		[]string{"users"},
	)
	if startImpactedErr != nil {
		t.Fatalf("start impacted rescan failed: %v", startImpactedErr)
	}
	if startImpacted.TaskID != "task-e2e-impacted" {
		t.Fatalf("unexpected impacted task id: %s", startImpacted.TaskID)
	}
	if completeErr := service.CompleteSchemaScan(ctx, startImpacted.TaskID, buildServiceScannedGraphNameTypeChangedWithEmail()); completeErr != nil {
		t.Fatalf("complete impacted rescan failed: %v", completeErr)
	}
	previewImpacted, previewImpactedErr := service.PreviewSchemaDiff(startImpacted.TaskID)
	if previewImpactedErr != nil {
		t.Fatalf("preview impacted diff failed: %v", previewImpactedErr)
	}
	if previewImpacted.Summary.ModifiedColumns == 0 {
		t.Fatalf("impacted rescan should include modified columns: %+v", previewImpacted.Summary)
	}
	risksImpacted, risksImpactedErr := service.GetGeneratorCompatibilityRisks(startImpacted.TaskID)
	if risksImpactedErr != nil {
		t.Fatalf("get impacted risks failed: %v", risksImpactedErr)
	}
	if risksImpacted.Mode != GeneratorCompatibilityModeConfigured || !hasBlockingRisk(risksImpacted.Risks) {
		t.Fatalf("impacted rescan should produce blocking risks: %+v", risksImpacted)
	}

	blockedSync, blockedSyncErr := service.ApplySchemaSync(startImpacted.TaskID, nil)
	if blockedSyncErr == nil {
		t.Fatal("expected blocked sync without ack")
	}
	if blockedSyncErr.Code != "BLOCKING_RISK_UNRESOLVED" {
		t.Fatalf("unexpected blocked sync code: %s", blockedSyncErr.Code)
	}
	if blockedSync == nil || blockedSync.SyncApplied || blockedSync.TrustState != SchemaTrustPendingAdjustment {
		t.Fatalf("unexpected blocked sync result: %+v", blockedSync)
	}
	trustAfterBlocked, trustAfterBlockedErr := service.GetSchemaTrustState("conn-1")
	if trustAfterBlockedErr != nil {
		t.Fatalf("get trust after blocked sync failed: %v", trustAfterBlockedErr)
	}
	if trustAfterBlocked.State != SchemaTrustPendingAdjustment {
		t.Fatalf("trust state should be pending_adjustment after blocked sync: %+v", trustAfterBlocked)
	}

	syncManual, syncManualErr := service.ApplySchemaSync(startImpacted.TaskID, []string{"ack-risk-1"})
	if syncManualErr != nil {
		t.Fatalf("manual sync after ack failed: %v", syncManualErr)
	}
	if !syncManual.SyncApplied || syncManual.TrustState != SchemaTrustTrusted {
		t.Fatalf("unexpected manual sync result: %+v", syncManual)
	}
	currentAfterManual, err := repo.LoadCurrentSchema(ctx, "conn-1")
	if err != nil {
		t.Fatalf("reload current after manual sync failed: %v", err)
	}
	if got := bundleColumnDataType(currentAfterManual, "users", "name"); got != "bigint" {
		t.Fatalf("users.name data_type should be synced to bigint, got %q", got)
	}
}

func TestSchemaScanService_EndToEndLoop_SyncFailureInjection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		// name 为子用例名称。
		name string
		// injectedErr 为首次同步时注入的写入错误。
		injectedErr error
		// wantCode 为首次同步失败期望错误码。
		wantCode string
	}{
		{
			name:        "concurrent_conflict",
			injectedErr: &SchemaSyncConcurrentConflictError{Message: "version changed"},
			wantCode:    SchemaSyncErrCodeFailedPrecondition,
		},
		{
			name:        "storage_write_failure",
			injectedErr: context.DeadlineExceeded,
			wantCode:    SchemaSyncErrCodeStorageError,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			runtime := NewSchemaScanRuntimeStore()
			starter := NewSchemaScanStarterWithTaskID(runtime, func() string { return "task-failure-loop" })
			repo := &faultInjectingCurrentSchemaRepo{
				byConn: map[string]*CurrentSchemaBundle{
					"conn-1": buildServiceBaseline("conn-1"),
				},
				replaceErrQueue: []error{tc.injectedErr, nil},
			}
			trustRepo := newFakeTrustRepo()
			trustRepo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
			service := NewSchemaScanService(
				starter,
				runtime,
				repo,
				NewSchemaDiffEngine(),
				NewGeneratorCompatibilityAnalyzer(),
				nil,
				NewSchemaTrustGate(trustRepo),
				NoopCompatibilityRecheckService{},
			)

			start, startErr := service.StartSchemaScan("conn-1", SchemaScanScopeAll, nil, "manual")
			if startErr != nil {
				t.Fatalf("start scan failed: %v", startErr)
			}
			if completeErr := service.CompleteSchemaScan(ctx, start.TaskID, buildServiceScannedGraphWithEmail()); completeErr != nil {
				t.Fatalf("complete scan failed: %v", completeErr)
			}
			preview, previewErr := service.PreviewSchemaDiff(start.TaskID)
			if previewErr != nil {
				t.Fatalf("preview diff failed: %v", previewErr)
			}
			if preview.Summary.AddedColumns == 0 {
				t.Fatalf("expected added columns before sync failure injection: %+v", preview.Summary)
			}
			risks, risksErr := service.GetGeneratorCompatibilityRisks(start.TaskID)
			if risksErr != nil {
				t.Fatalf("get risks failed: %v", risksErr)
			}
			if risks.Mode != GeneratorCompatibilityModeNoGeneratorConfig || len(risks.Risks) != 0 {
				t.Fatalf("expected no-generator-config with empty risks: %+v", risks)
			}

			firstSyncResult, firstSyncErr := service.ApplySchemaSync(start.TaskID, nil)
			if firstSyncErr == nil {
				t.Fatal("expected first sync to fail by injected error")
			}
			if firstSyncErr.Code != tc.wantCode {
				t.Fatalf("unexpected first sync error code: got %s want %s", firstSyncErr.Code, tc.wantCode)
			}
			if firstSyncResult == nil || firstSyncResult.SyncApplied {
				t.Fatalf("first sync should be rejected: %+v", firstSyncResult)
			}
			currentAfterFailure, err := repo.LoadCurrentSchema(ctx, "conn-1")
			if err != nil {
				t.Fatalf("load current after first failure failed: %v", err)
			}
			if bundleHasColumn(currentAfterFailure, "users", "email") {
				t.Fatalf("current schema should remain unchanged after first failure")
			}
			trustAfterFailure, trustErr := service.GetSchemaTrustState("conn-1")
			if trustErr != nil {
				t.Fatalf("get trust after first failure failed: %v", trustErr)
			}
			if trustAfterFailure.State != SchemaTrustTrusted {
				t.Fatalf("trust state should remain trusted when no blocking risk: %+v", trustAfterFailure)
			}

			secondSyncResult, secondSyncErr := service.ApplySchemaSync(start.TaskID, nil)
			if secondSyncErr != nil {
				t.Fatalf("second sync should recover after injected failure is cleared: %v", secondSyncErr)
			}
			if secondSyncResult == nil || !secondSyncResult.SyncApplied {
				t.Fatalf("second sync should be applied: %+v", secondSyncResult)
			}
			currentAfterRecovery, err := repo.LoadCurrentSchema(ctx, "conn-1")
			if err != nil {
				t.Fatalf("load current after recovery failed: %v", err)
			}
			if !bundleHasColumn(currentAfterRecovery, "users", "email") {
				t.Fatalf("current schema should be updated after recovery sync")
			}
			if repo.replaceCalls < 2 {
				t.Fatalf("replace should be called at least twice, got %d", repo.replaceCalls)
			}
		})
	}
}

func TestSchemaScanService_EndToEndLoop_BlockingRiskAckStillFailsOnStorageError(t *testing.T) {
	ctx := context.Background()
	runtime := NewSchemaScanRuntimeStore()
	starter := NewSchemaScanStarterWithTaskID(runtime, func() string { return "task-blocking-ack-storage" })
	repo := &faultInjectingCurrentSchemaRepo{
		byConn: map[string]*CurrentSchemaBundle{
			"conn-1": buildServiceBaseline("conn-1"),
		},
		replaceErrQueue: []error{context.DeadlineExceeded, nil},
	}
	trustRepo := newFakeTrustRepo()
	trustRepo.meta["conn-1"] = ConnectionSchemaMeta{SchemaTrustState: SchemaTrustTrusted}
	genStore := GeneratorConfigSnapshotStoreStub{
		SnapshotsByConnectionID: map[string]*GeneratorConfigSnapshot{
			"conn-1": {
				Columns: []GeneratorColumnConfig{
					{
						ConnectionID: "conn-1",
						DatabaseName: "appdb",
						SchemaName:   "public",
						TableName:    "users",
						ColumnName:   "name",
						GeneratorType: "EnumValueGenerator",
						ConfigID:     "cfg-users-name",
					},
				},
			},
		},
	}
	service := NewSchemaScanService(
		starter,
		runtime,
		repo,
		NewSchemaDiffEngine(),
		NewGeneratorCompatibilityAnalyzer(),
		genStore,
		NewSchemaTrustGate(trustRepo),
		NoopCompatibilityRecheckService{},
	)

	start, startErr := service.StartSchemaScan("conn-1", SchemaScanScopeAll, nil, "manual")
	if startErr != nil {
		t.Fatalf("start scan failed: %v", startErr)
	}
	if completeErr := service.CompleteSchemaScan(ctx, start.TaskID, buildServiceScannedGraphNameTypeChangedWithEmail()); completeErr != nil {
		t.Fatalf("complete scan failed: %v", completeErr)
	}
	risks, risksErr := service.GetGeneratorCompatibilityRisks(start.TaskID)
	if risksErr != nil {
		t.Fatalf("get risks failed: %v", risksErr)
	}
	if risks.Mode != GeneratorCompatibilityModeConfigured || !hasBlockingRisk(risks.Risks) {
		t.Fatalf("expected configured mode with blocking risks: %+v", risks)
	}

	blockedSyncResult, blockedSyncErr := service.ApplySchemaSync(start.TaskID, nil)
	if blockedSyncErr == nil || blockedSyncErr.Code != "BLOCKING_RISK_UNRESOLVED" {
		t.Fatalf("expected blocking gate error before ack, got result=%+v err=%v", blockedSyncResult, blockedSyncErr)
	}
	if blockedSyncResult == nil || blockedSyncResult.SyncApplied || blockedSyncResult.TrustState != SchemaTrustPendingAdjustment {
		t.Fatalf("unexpected blocked sync result before ack: %+v", blockedSyncResult)
	}

	ackSyncResult, ackSyncErr := service.ApplySchemaSync(start.TaskID, []string{"ack-risk-1"})
	if ackSyncErr == nil {
		t.Fatal("expected storage error even after ack")
	}
	if ackSyncErr.Code != SchemaSyncErrCodeStorageError {
		t.Fatalf("unexpected ack sync error code: %s", ackSyncErr.Code)
	}
	if ackSyncResult == nil || ackSyncResult.SyncApplied {
		t.Fatalf("ack sync should fail when storage write fails: %+v", ackSyncResult)
	}
	if ackSyncResult.TrustState != SchemaTrustPendingAdjustment {
		t.Fatalf("trust state should remain pending_adjustment on storage failure: %+v", ackSyncResult)
	}
	currentAfterAckFailure, err := repo.LoadCurrentSchema(ctx, "conn-1")
	if err != nil {
		t.Fatalf("load current after ack storage failure failed: %v", err)
	}
	if bundleHasColumn(currentAfterAckFailure, "users", "email") {
		t.Fatalf("current schema should remain unchanged when ack sync fails by storage error")
	}
	trustAfterAckFailure, trustErr := service.GetSchemaTrustState("conn-1")
	if trustErr != nil {
		t.Fatalf("get trust after ack storage failure failed: %v", trustErr)
	}
	if trustAfterAckFailure.State != SchemaTrustPendingAdjustment {
		t.Fatalf("trust state should remain pending_adjustment after ack storage failure: %+v", trustAfterAckFailure)
	}

	recoveryResult, recoveryErr := service.ApplySchemaSync(start.TaskID, []string{"ack-risk-1"})
	if recoveryErr != nil {
		t.Fatalf("recovery sync should succeed after storage fault is cleared: %v", recoveryErr)
	}
	if recoveryResult == nil || !recoveryResult.SyncApplied || recoveryResult.TrustState != SchemaTrustTrusted {
		t.Fatalf("unexpected recovery sync result: %+v", recoveryResult)
	}
}

// buildServiceBaseline 构造服务单测用基线当前 schema。
func buildServiceBaseline(connectionID string) *CurrentSchemaBundle {
	return &CurrentSchemaBundle{
		Tables: []TableSchemaPersisted{
			{
				ID:           "tbl-users",
				ConnectionID: connectionID,
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		Columns: []ColumnSchemaPersisted{
			{
				ID:            "col-id",
				TableSchemaID: "tbl-users",
				ColumnName:    "id",
				OrdinalPos:    1,
				DataType:      "bigint",
				AbstractType:  "int",
				IsPrimaryKey:  true,
				IsNullable:    false,
				IsUnique:      true,
			},
			{
				ID:            "col-name",
				TableSchemaID: "tbl-users",
				ColumnName:    "name",
				OrdinalPos:    2,
				DataType:      "varchar(255)",
				AbstractType:  "string",
				IsNullable:    false,
			},
		},
	}
}

// buildServiceScannedGraphWithEmail 构造新增 email 的扫描结果。
func buildServiceScannedGraphWithEmail() *SchemaGraph {
	return &SchemaGraph{
		Tables: []TableDef{
			{
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
				Columns: []ColumnDef{
					{Name: "id", OrdinalPos: 1, DataType: "bigint", AbstractType: "int", IsNullable: false},
					{Name: "name", OrdinalPos: 2, DataType: "varchar(255)", AbstractType: "string", IsNullable: false},
					{Name: "email", OrdinalPos: 3, DataType: "varchar(255)", AbstractType: "string", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
				UniqueConstraints: []UniqueConstraintDef{
					{Name: "uq_users_email", Columns: []string{"email"}},
				},
			},
		},
	}
}

// buildServiceScannedGraphNameTypeChanged 构造 name 字段类型不兼容的扫描结果。
func buildServiceScannedGraphNameTypeChanged() *SchemaGraph {
	return &SchemaGraph{
		Tables: []TableDef{
			{
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
				Columns: []ColumnDef{
					{Name: "id", OrdinalPos: 1, DataType: "bigint", AbstractType: "int", IsNullable: false},
					{Name: "name", OrdinalPos: 2, DataType: "bigint", AbstractType: "int", IsNullable: false},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}
}

// buildServiceScannedGraphNameTypeChangedWithEmail 构造包含 email 且 name 类型不兼容的扫描结果。
func buildServiceScannedGraphNameTypeChangedWithEmail() *SchemaGraph {
	return &SchemaGraph{
		Tables: []TableDef{
			{
				DatabaseName: "appdb",
				SchemaName:   "public",
				TableName:    "users",
				Columns: []ColumnDef{
					{Name: "id", OrdinalPos: 1, DataType: "bigint", AbstractType: "int", IsNullable: false},
					{Name: "name", OrdinalPos: 2, DataType: "bigint", AbstractType: "int", IsNullable: false},
					{Name: "email", OrdinalPos: 3, DataType: "varchar(255)", AbstractType: "string", IsNullable: true},
				},
				PrimaryKey: []string{"id"},
				UniqueConstraints: []UniqueConstraintDef{
					{Name: "uq_users_email", Columns: []string{"email"}},
				},
			},
		},
	}
}

// bundleHasColumn 判断某表是否包含目标列。
func bundleHasColumn(bundle *CurrentSchemaBundle, tableName string, columnName string) bool {
	if bundle == nil {
		return false
	}
	tableIDByName := make(map[string]string)
	for _, t := range bundle.Tables {
		tableIDByName[t.TableName] = t.ID
	}
	tableID := tableIDByName[tableName]
	if tableID == "" {
		return false
	}
	for _, c := range bundle.Columns {
		if c.TableSchemaID == tableID && c.ColumnName == columnName {
			return true
		}
	}
	return false
}

// bundleColumnDataType 返回目标表列的 DataType；不存在时返回空字符串。
func bundleColumnDataType(bundle *CurrentSchemaBundle, tableName string, columnName string) string {
	if bundle == nil {
		return ""
	}
	tableIDByName := make(map[string]string)
	for _, t := range bundle.Tables {
		tableIDByName[t.TableName] = t.ID
	}
	tableID := tableIDByName[tableName]
	if tableID == "" {
		return ""
	}
	for _, c := range bundle.Columns {
		if c.TableSchemaID == tableID && c.ColumnName == columnName {
			return c.DataType
		}
	}
	return ""
}
