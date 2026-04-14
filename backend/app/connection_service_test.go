package app_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"loomidbx/backend/app"
	"loomidbx/backend/storage"
	_ "modernc.org/sqlite"
)

// TestSaveAndEditKeepsID 验证更新连接时 ID 不发生变化。
func TestSaveAndEditKeepsID(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:   "local",
		DBType: "sqlite",
	})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}
	updatedID, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		ID:     id,
		Name:   "local-updated",
		DBType: "sqlite",
	})
	if errObj != nil {
		t.Fatalf("update failed: %+v", errObj)
	}
	if updatedID != id {
		t.Fatalf("id changed after edit: want %s got %s", id, updatedID)
	}
}

// TestDeleteRequiresConfirmationAndCascades 验证删除前确认与级联删除语义。
func TestDeleteRequiresConfirmationAndCascades(t *testing.T) {
	svc, store := newService(t)
	ctx := context.Background()
	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{Name: "x", DBType: "sqlite"})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}
	if err := store.InsertDummyTableSchema(ctx, "schema-1", id, "orders"); err != nil {
		t.Fatalf("insert dummy schema failed: %v", err)
	}

	errObj = svc.DeleteConnection(ctx, app.DeleteConnectionRequest{ID: id, ConfirmCascade: false})
	if errObj == nil || errObj.Code != app.CodeConfirmationRequired {
		t.Fatalf("expect confirmation required, got %+v", errObj)
	}
	count, err := store.CountTableSchemasByConnection(ctx, id)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected schema count before delete: %d", count)
	}

	errObj = svc.DeleteConnection(ctx, app.DeleteConnectionRequest{ID: id, ConfirmCascade: true})
	if errObj != nil {
		t.Fatalf("delete failed: %+v", errObj)
	}
	count, err = store.CountTableSchemasByConnection(ctx, id)
	if err != nil {
		t.Fatalf("count after delete failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("cascade delete not applied, left %d rows", count)
	}
}

// TestStorageFailureDoesNotReportSuccess 验证持久化失败时不会误报成功。
func TestStorageFailureDoesNotReportSuccess(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "meta.db")
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	store, err := storage.NewConnectionStoreFromDB(db)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	_ = db.Close()

	svc := app.NewConnectionService(store)
	_, errObj := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:   "x",
		DBType: "sqlite",
	})
	if errObj == nil || errObj.Code != app.CodeStorageError {
		t.Fatalf("expected storage error, got %+v", errObj)
	}
}

// TestConnectionDefaultTimeoutAndNoFalseSuccess 验证连接测试失败路径返回错误而非成功。
func TestConnectionDefaultTimeoutAndNoFalseSuccess(t *testing.T) {
	svc, _ := newService(t)
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType: "postgres",
		Host:   "127.0.0.1",
		Port:   1,
	})
	if errObj == nil {
		t.Fatal("expect failure for unreachable endpoint")
	}
	if errObj.Code != app.CodeUpstreamUnavailable {
		t.Fatalf("unexpected code: %+v", errObj)
	}
}

// newService 创建测试专用服务实例与临时元数据库。
//
// 输入：
// - t: 测试上下文。
//
// 输出：
// - *app.ConnectionService: 业务服务实例。
// - *storage.ConnectionStore: 底层存储实例（用于测试辅助断言）。
func newService(t *testing.T) (*app.ConnectionService, *storage.ConnectionStore) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "meta.db")
	store, err := storage.NewConnectionStore(tmp)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return app.NewConnectionService(store), store
}
