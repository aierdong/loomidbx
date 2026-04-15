package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"loomidbx/app"
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

// 选择 keyring 存储时，密码写入密钥环，数据库仅存引用。
func TestSaveConnectionWithKeyringMode(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets:      make(map[string]string),
	}
	svc, store := newServiceWithDeps(t, nil, keyring)
	ctx := context.Background()

	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "keyring-conn",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "secret-for-keyring",
		Extra:    `{"credential_mode":"keyring"}`,
	})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	// 验证 keyring Set 被调用
	if keyring.setCalls == 0 {
		t.Fatal("keyring.Set should be called when credential_mode=keyring")
	}

	// 验证数据库中不存明文密码
	rec, err := store.GetConnectionByID(ctx, id)
	if err != nil {
		t.Fatalf("load saved record failed: %v", err)
	}
	if rec.Password != "" {
		t.Fatalf("password must not be stored in DB when using keyring, got %q", rec.Password)
	}

	// 验证 extra 中包含 credential_ref
	if rec.Extra == "" {
		t.Fatal("extra must contain credential_ref")
	}
	var extra map[string]string
	if err := json.Unmarshal([]byte(rec.Extra), &extra); err != nil {
		t.Fatalf("parse extra failed: %v", err)
	}
	if extra["credential_ref"] == "" {
		t.Fatal("credential_ref must be set in extra")
	}
	if extra["credential_ref"] != "connection:"+id {
		t.Fatalf("credential_ref should be connection:%s, got %s", id, extra["credential_ref"])
	}

	// 验证密码已存入 keyring
	ref := "connection:" + id
	if keyring.secrets[ref] != "secret-for-keyring" {
		t.Fatalf("keyring should store password at ref %s", ref)
	}
}

// keyring 存储时密钥环不可用返回 KEYRING_UNAVAILABLE。
func TestSaveConnectionKeyringUnavailable(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: app.ErrKeyringUnavailable,
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)
	ctx := context.Background()

	_, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "keyring-conn",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "secret-for-keyring",
		Extra:    `{"credential_mode":"keyring"}`,
	})
	if errObj == nil {
		t.Fatal("expect error when keyring unavailable")
	}
	if errObj.Code != app.CodeKeyringUnavailable {
		t.Fatalf("expect %s got %s", app.CodeKeyringUnavailable, errObj.Code)
	}
}

// keyring 存储时密钥环拒绝访问返回 KEYRING_ACCESS_DENIED。
func TestSaveConnectionKeyringAccessDenied(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: nil,
		setErr:       app.ErrKeyringAccessDenied,
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)
	ctx := context.Background()

	_, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "keyring-conn",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "secret-for-keyring",
		Extra:    `{"credential_mode":"keyring"}`,
	})
	if errObj == nil {
		t.Fatal("expect error when keyring access denied")
	}
	if errObj.Code != app.CodeKeyringAccessDenied {
		t.Fatalf("expect %s got %s", app.CodeKeyringAccessDenied, errObj.Code)
	}
}

// 保存后可从 keyring 读取凭据进行连接测试。
func TestConnectionTestAfterKeyringSave(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets:      make(map[string]string),
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)
	ctx := context.Background()

	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "keyring-conn",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "secret-for-keyring",
		Extra:    `{"credential_mode":"keyring"}`,
	})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	// 使用保存的连接参数进行测试（不含明文密码）
	testErr := svc.TestConnection(ctx, app.ConnectionRequest{
		ID:     id,
		DBType: "postgres",
		Host:   "127.0.0.1",
		Port:   5432,
		Extra:  `{"credential_ref":"connection:` + id + `"}`,
	})
	// 由于网络不可达，应返回网络错误而非凭据错误
	if testErr == nil {
		t.Fatal("expect network error")
	}
	if testErr.Code == app.CodeKeyringUnavailable || testErr.Code == app.CodeKeyringAccessDenied {
		t.Fatalf("keyring should work, got %s", testErr.Code)
	}
}

// 更新已有 keyring 连接时，新密码应写入 keyring 替换旧值。
func TestUpdateConnectionWithKeyringMode(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets:      make(map[string]string),
	}
	svc, store := newServiceWithDeps(t, nil, keyring)
	ctx := context.Background()

	// 先保存一个 keyring 连接
	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "keyring-conn",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "old-secret",
		Extra:    `{"credential_mode":"keyring"}`,
	})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	ref := "connection:" + id
	if keyring.secrets[ref] != "old-secret" {
		t.Fatalf("initial secret should be stored, got %s", keyring.secrets[ref])
	}
	initialSetCalls := keyring.setCalls

	// 更新连接，提供新密码
	_, errObj = svc.SaveConnection(ctx, app.ConnectionRequest{
		ID:       id,
		Name:     "keyring-conn-updated",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "new-secret",
	})
	if errObj != nil {
		t.Fatalf("update failed: %+v", errObj)
	}

	// 验证 keyring Set 再次被调用
	if keyring.setCalls != initialSetCalls+1 {
		t.Fatalf("keyring.Set should be called again for update, setCalls=%d, initial=%d", keyring.setCalls, initialSetCalls)
	}

	// 验证 keyring 中密码已更新
	if keyring.secrets[ref] != "new-secret" {
		t.Fatalf("secret should be updated in keyring, got %s", keyring.secrets[ref])
	}

	// 验证数据库中仍不存明文密码
	rec, err := store.GetConnectionByID(ctx, id)
	if err != nil {
		t.Fatalf("load record failed: %v", err)
	}
	if rec.Password != "" {
		t.Fatalf("password must not be stored in DB, got %q", rec.Password)
	}
}

// 更新 keyring 连接时不提供密码，保持 keyring 中原密码不变。
func TestUpdateConnectionKeyringNoPasswordChange(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets:      make(map[string]string),
	}
	svc, store := newServiceWithDeps(t, nil, keyring)
	ctx := context.Background()

	// 先保存一个 keyring 连接
	id, errObj := svc.SaveConnection(ctx, app.ConnectionRequest{
		Name:     "keyring-conn",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: "user",
		Password: "original-secret",
		Extra:    `{"credential_mode":"keyring"}`,
	})
	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	ref := "connection:" + id
	initialSetCalls := keyring.setCalls

	// 加载记录获取 extra
	rec, err := store.GetConnectionByID(ctx, id)
	if err != nil {
		t.Fatalf("load record failed: %v", err)
	}

	// 更新连接，不提供密码，但保持 extra
	_, errObj = svc.SaveConnection(ctx, app.ConnectionRequest{
		ID:       id,
		Name:     "keyring-conn-renamed",
		DBType:   "postgres",
		Host:     "192.168.1.1", // 仅更新 Host
		Port:     5432,
		Username: "user",
		Password: "", // 不提供密码
		Extra:    rec.Extra, // 保持原有 extra（含 credential_ref）
	})
	if errObj != nil {
		t.Fatalf("update failed: %+v", errObj)
	}

	// 验证 keyring Set 未再次被调用
	if keyring.setCalls != initialSetCalls {
		t.Fatalf("keyring.Set should not be called when password empty, setCalls=%d, initial=%d", keyring.setCalls, initialSetCalls)
	}

	// 验证 keyring 中密码保持不变
	if keyring.secrets[ref] != "original-secret" {
		t.Fatalf("secret should remain unchanged in keyring, got %s", keyring.secrets[ref])
	}
}
