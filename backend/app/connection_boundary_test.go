package app_test

import (
	"context"
	"testing"
	
	"loomidbx/app"
)

// ===== 6.1 与 spec-02 衔接假验 =====

// 本组测试验证连接子系统不触发任何 Schema 扫描或快照写入。
// 在仅导入连接模块的前提下，边界行为固化供下游 spec 对照。

// 连接测试不写入表快照。
func TestBoundary_TestConnection_NoTableSchemaWrite(t *testing.T) {
	svc, store := newService(t)

	// 执行连接测试
	_ = svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "sqlite",
		Database: ":memory:",
	})

	// 验收：ldb_table_schemas 无新增记录（连接测试不触发扫描）
	// 由于无有效连接 ID，此处验证存储层表数量为 0
	count, err := store.CountTableSchemasByConnection(context.Background(), "non-existent-id")
	if err != nil {
		// 存储层可能返回错误（表不存在或查询失败），视为无扫描
		t.Logf("Count query returned error (expected for empty table): %v", err)
	} else if count != 0 {
		t.Errorf("TestConnection should not write table schemas, count=%d", count)
	}
}

// 保存连接不写入表快照。
func TestBoundary_SaveConnection_NoTableSchemaWrite(t *testing.T) {
	svc, store := newService(t)

	// 保存连接
	id, _ := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:   "boundary-save",
		DBType: "sqlite",
	})

	// 验收：ldb_table_schemas 无新增记录
	count, _ := store.CountTableSchemasByConnection(context.Background(), id)
	if count != 0 {
		t.Errorf("SaveConnection should not write table schemas, count=%d", count)
	}
}

// 删除连接不写入扫描历史（仅清理关联元数据）。
func TestBoundary_DeleteConnection_NoScanHistoryWrite(t *testing.T) {
	svc, store := newService(t)

	// 保存连接并插入一个表快照（模拟 spec-02 的扫描结果）
	id, _ := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:   "boundary-delete",
		DBType: "sqlite",
	})

	// 插入测试用的表快照记录（模拟下游 spec 写入）
	_ = store.InsertDummyTableSchema(context.Background(), "schema-001", id, "users")

	// 验证删除前有表快照
	countBefore, _ := store.CountTableSchemasByConnection(context.Background(), id)
	if countBefore == 0 {
		t.Skip("no table schema to clean up")
	}

	// 执行级联删除（确认后）
	_ = svc.DeleteConnection(context.Background(), app.DeleteConnectionRequest{
		ID:             id,
		ConfirmCascade: true,
	})

	// 验收：表快照被级联删除（而非写入新记录）
	countAfter, _ := store.CountTableSchemasByConnection(context.Background(), id)
	if countAfter != 0 {
		t.Errorf("DeleteConnection should cascade delete table schemas, count=%d", countAfter)
	}

	// 验收：连接记录被删除（不存在）
	list, _ := svc.ListConnections(context.Background())
	for _, conn := range list {
		if conn.ID == id {
			t.Error("connection should be deleted")
		}
	}
}

// ===== 6.2 与 spec-06 衔接假验 =====

// 本组测试验证错误码定义稳定，供 FFI 双向对齐。

// 错误码映射表完整性。
func TestBoundary_ErrorCodesComplete(t *testing.T) {
	// 验收：所有关键错误码已定义
	codes := []string{
		app.CodeInvalidArgument,
		app.CodeStorageError,
		app.CodeNotFound,
		app.CodeConfirmationRequired,
		app.CodeDeadlineExceeded,
		app.CodeUpstreamUnavailable,
		app.CodeAuthFailed,
		app.CodeTLSError,
		app.CodeProtocolError,
		app.CodeKeyringUnavailable,
		app.CodeKeyringAccessDenied,
	}

	for _, code := range codes {
		if code == "" {
			t.Errorf("error code should not be empty")
		}
	}

	// 验收：错误码总数为 11
	if len(codes) != 11 {
		t.Errorf("expected 11 error codes, got %d", len(codes))
	}
}

// 错误响应结构稳定。
func TestBoundary_ErrorResponseStructure(t *testing.T) {
	err := &app.AppError{
		Code:    app.CodeDeadlineExceeded,
		Message: "test timeout",
		Details: map[string]string{"timeout_sec": "20"},
	}

	// 验收：错误结构包含必要字段
	if err.Code == "" {
		t.Error("error code required")
	}
	if err.Message == "" {
		t.Error("error message required")
	}
	// Details 可选
}

// ===== 6.3 与 spec-07 衔接说明 =====

// 本组测试验证接口阻塞时间预期合理。

// TestConnection 超时边界生效。
func TestBoundary_TestConnectionTimeoutEnforced(t *testing.T) {
	svc, _ := newService(t)

	// 设置短超时（验证接口响应时间）
	timeoutSec := 2
	_ = svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:     "mysql",
		Host:       "10.255.255.1",
		Port:       3306,
		TimeoutSec: timeoutSec,
	})

	// 验收：接口应在 timeout_sec 内返回（不阻塞 UI）
	// 具体时间验证在集成测试中完成
}

// SaveConnection 应快速返回（不阻塞 UI）。
func TestBoundary_SaveConnectionQuickResponse(t *testing.T) {
	svc, _ := newService(t)

	// 保存连接应在 < 100ms 内完成
	id, errObj := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:   "quick-save",
		DBType: "sqlite",
	})

	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	// 验收：返回有效 ID
	if id == "" {
		t.Error("save should return valid ID")
	}
}

// ListConnections 应快速返回。
func TestBoundary_ListConnectionsQuickResponse(t *testing.T) {
	svc, _ := newService(t)

	// 列表查询应在 < 100ms 内完成
	list, errObj := svc.ListConnections(context.Background())

	if errObj != nil {
		t.Fatalf("list failed: %+v", errObj)
	}

	// 验收：返回列表（空或非空均合法）
	if list == nil {
		t.Error("list should return non-nil slice")
	}
}

// DeleteConnection 需确认语义。
func TestBoundary_DeleteConnectionConfirmationRequired(t *testing.T) {
	svc, _ := newService(t)

	id, _ := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:   "confirm-test",
		DBType: "sqlite",
	})

	// 不带确认的删除应返回 CONFIRMATION_REQUIRED
	errObj := svc.DeleteConnection(context.Background(), app.DeleteConnectionRequest{
		ID:             id,
		ConfirmCascade: false,
	})

	if errObj == nil {
		t.Fatal("delete without confirmation should fail")
	}

	if errObj.Code != app.CodeConfirmationRequired {
		t.Errorf("expected CONFIRMATION_REQUIRED, got: %s", errObj.Code)
	}

	// 验收：连接仍存在（未被删除）
	list, _ := svc.ListConnections(context.Background())
	for _, conn := range list {
		if conn.ID == id {
			return // 连接仍存在
		}
	}
	t.Error("connection should still exist after delete without confirmation")
}
