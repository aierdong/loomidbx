package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"loomidbx/backend/app"
	"loomidbx/backend/storage"
)

func newService(t *testing.T) (*app.ConnectionService, *storage.ConnectionStore) {
	return newServiceWithDeps(t, nil, nil)
}

func newServiceWithPurger(t *testing.T, purger app.CredentialPurger) (*app.ConnectionService, *storage.ConnectionStore) {
	return newServiceWithDeps(t, purger, nil)
}

func newServiceWithDeps(t *testing.T, purger app.CredentialPurger, keyring app.KeyringAccessor) (*app.ConnectionService, *storage.ConnectionStore) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "meta.db")
	store, err := storage.NewConnectionStore(tmp)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return app.NewConnectionServiceWithDeps(store, purger, keyring), store
}

type mockCredentialPurger struct {
	purged []storage.CredentialReference
	err    error
}

func (m *mockCredentialPurger) PurgeCredentialReference(_ context.Context, ref storage.CredentialReference) error {
	m.purged = append(m.purged, ref)
	return m.err
}

type mockKeyringAccessor struct {
	availableErr error
	getErr       error
	secrets      map[string]string
	getCalls     int
}

func (m *mockKeyringAccessor) IsAvailable(context.Context) error {
	return m.availableErr
}

func (m *mockKeyringAccessor) Get(_ context.Context, ref string) (string, error) {
	m.getCalls++
	if m.getErr != nil {
		return "", m.getErr
	}
	return m.secrets[ref], nil
}

func appEncryptForTest(t *testing.T, plain string) string {
	t.Helper()
	svc, store := newService(t)
	id, appErr := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:     "tmp",
		DBType:   "sqlite",
		Password: plain,
	})
	if appErr != nil {
		t.Fatalf("save for encrypt fixture failed: %+v", appErr)
	}
	rec, err := store.GetConnectionByID(context.Background(), id)
	if err != nil {
		t.Fatalf("read encrypt fixture failed: %v", err)
	}
	return rec.Password
}
