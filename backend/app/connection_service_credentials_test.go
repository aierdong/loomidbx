package app_test

import (
	"context"
	"strings"
	"testing"

	"loomidbx/backend/app"
)

// 凭据解析优先级固定为 env > keyring > AES。
func TestCredentialResolutionPriority(t *testing.T) {
	t.Setenv("LDBX_TEST_SECRET", "env-secret")
	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets: map[string]string{
			"kr://conn/main": "keyring-secret",
		},
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)

	enc := appEncryptForTest(t, "aes-secret")
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: "env:LDBX_TEST_SECRET",
		Extra:    `{"credential_ref":"kr://conn/main"}`,
	})
	if errObj == nil {
		t.Fatal("expect network error")
	}
	if keyring.getCalls != 0 {
		t.Fatalf("keyring should not be called when env present, getCalls=%d", keyring.getCalls)
	}

	errObj = svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: enc,
		Extra:    `{"credential_ref":"kr://conn/main"}`,
	})
	if errObj == nil {
		t.Fatal("expect network error")
	}
	if keyring.getCalls == 0 {
		t.Fatal("keyring should be called when env absent and keyring ref present")
	}
}

// 密钥环不可用时返回 KEYRING_UNAVAILABLE。
func TestKeyringUnavailableCode(t *testing.T) {
	svc, _ := newServiceWithDeps(t, nil, &mockKeyringAccessor{
		availableErr: app.ErrKeyringUnavailable,
	})
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType: "postgres",
		Host:   "127.0.0.1",
		Port:   1,
		Extra:  `{"credential_ref":"kr://conn/missing"}`,
	})
	if errObj == nil || errObj.Code != app.CodeKeyringUnavailable {
		t.Fatalf("expect %s got %+v", app.CodeKeyringUnavailable, errObj)
	}
}

// 密钥环拒绝访问时返回 KEYRING_ACCESS_DENIED。
func TestKeyringAccessDeniedCode(t *testing.T) {
	svc, _ := newServiceWithDeps(t, nil, &mockKeyringAccessor{
		availableErr: app.ErrKeyringAccessDenied,
	})
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType: "postgres",
		Host:   "127.0.0.1",
		Port:   1,
		Extra:  `{"credential_ref":"kr://conn/denied"}`,
	})
	if errObj == nil || errObj.Code != app.CodeKeyringAccessDenied {
		t.Fatalf("expect %s got %+v", app.CodeKeyringAccessDenied, errObj)
	}
}

// 持久化层不能保存明文密码。
func TestSaveConnectionNeverStoresPlaintext(t *testing.T) {
	svc, store := newServiceWithDeps(t, nil, nil)
	ctx := context.Background()
	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "secure",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "super-secret-password",
	})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}
	rec, err := store.GetConnectionByID(ctx, id)
	if err != nil {
		t.Fatalf("load saved record failed: %v", err)
	}
	if rec.Password == "super-secret-password" {
		t.Fatal("plaintext password must not be stored")
	}
	if !strings.HasPrefix(rec.Password, "aesgcm:") {
		t.Fatalf("stored password should be encrypted payload, got %q", rec.Password)
	}
}
