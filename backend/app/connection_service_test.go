package app_test

import (
	"context"
	"database/sql"
	"errors"
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
	purger := &mockCredentialPurger{}
	svc, store := newServiceWithPurger(t, purger)
	ctx := context.Background()
	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{Name: "x", DBType: "sqlite"})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}
	if err := store.InsertDummyTableSchema(ctx, "schema-1", id, "orders"); err != nil {
		t.Fatalf("insert dummy schema failed: %v", err)
	}
	if err := store.InsertCredentialReference(ctx, storage.CredentialReference{
		ID:            "cred-1",
		ConnectionID:  id,
		Provider:      "keyring",
		CredentialRef: "kr://conn/x",
	}); err != nil {
		t.Fatalf("insert credential ref failed: %v", err)
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
	credCount, err := store.CountCredentialReferencesByConnection(ctx, id)
	if err != nil {
		t.Fatalf("count credential refs failed: %v", err)
	}
	if credCount != 1 {
		t.Fatalf("unexpected credential ref count before delete: %d", credCount)
	}
	if len(purger.purged) != 0 {
		t.Fatalf("purger should not be called before confirmation, got %d", len(purger.purged))
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
	credCount, err = store.CountCredentialReferencesByConnection(ctx, id)
	if err != nil {
		t.Fatalf("count credential refs after delete failed: %v", err)
	}
	if credCount != 0 {
		t.Fatalf("credential refs not cleaned, left %d rows", credCount)
	}
	if len(purger.purged) != 1 {
		t.Fatalf("purger called %d times, want 1", len(purger.purged))
	}
	if purger.purged[0].CredentialRef != "kr://conn/x" {
		t.Fatalf("unexpected purged reference: %+v", purger.purged[0])
	}
}

// TestDeleteRollbackWhenCredentialPurgeFails 验证凭据清理失败时删除事务回滚。
func TestDeleteRollbackWhenCredentialPurgeFails(t *testing.T) {
	purger := &mockCredentialPurger{err: errors.New("keyring unavailable")}
	svc, store := newServiceWithPurger(t, purger)
	ctx := context.Background()

	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{Name: "x", DBType: "sqlite"})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}
	if err := store.InsertDummyTableSchema(ctx, "schema-rollback", id, "orders"); err != nil {
		t.Fatalf("insert dummy schema failed: %v", err)
	}
	if err := store.InsertCredentialReference(ctx, storage.CredentialReference{
		ID:            "cred-rollback",
		ConnectionID:  id,
		Provider:      "keyring",
		CredentialRef: "kr://conn/rollback",
	}); err != nil {
		t.Fatalf("insert credential ref failed: %v", err)
	}

	errObj = svc.DeleteConnection(ctx, app.DeleteConnectionRequest{ID: id, ConfirmCascade: true})
	if errObj == nil || errObj.Code != app.CodeStorageError {
		t.Fatalf("expect storage error when purge failed, got %+v", errObj)
	}

	conn, err := store.GetConnectionByID(ctx, id)
	if err != nil || conn == nil {
		t.Fatalf("connection should remain after rollback, err=%v conn=%+v", err, conn)
	}
	schemaCount, err := store.CountTableSchemasByConnection(ctx, id)
	if err != nil {
		t.Fatalf("count schemas failed: %v", err)
	}
	if schemaCount != 1 {
		t.Fatalf("schemas should remain after rollback, got %d", schemaCount)
	}
	credCount, err := store.CountCredentialReferencesByConnection(ctx, id)
	if err != nil {
		t.Fatalf("count credential refs failed: %v", err)
	}
	if credCount != 1 {
		t.Fatalf("credential refs should remain after rollback, got %d", credCount)
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
	return newServiceWithPurger(t, nil)
}

func newServiceWithPurger(t *testing.T, purger app.CredentialPurger) (*app.ConnectionService, *storage.ConnectionStore) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "meta.db")
	store, err := storage.NewConnectionStore(tmp)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return app.NewConnectionServiceWithPurger(store, purger), store
}

type mockCredentialPurger struct {
	purged []storage.CredentialReference
	err    error
}

func (m *mockCredentialPurger) PurgeCredentialReference(_ context.Context, ref storage.CredentialReference) error {
	m.purged = append(m.purged, ref)
	return m.err
}
