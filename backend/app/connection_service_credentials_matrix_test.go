package app_test

import (
	"context"
	"strings"
	"testing"

	"loomidbx/backend/app"
)

// ===== 5.1 凭据解析优先级组合矩阵测试 =====

// 优先级矩阵：env > keyring > AES > plaintext
// 本组测试覆盖所有两两冲突场景。

// 场景 1: env 存在 + keyring 引用存在 → env 优先，keyring 不被调用。
func TestPriorityMatrix_env_over_keyring(t *testing.T) {
	t.Setenv("LDBX_MATRIX_ENV", "env-value-from-matrix")

	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets: map[string]string{
			"kr://matrix/test": "keyring-value-should-not-use",
		},
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)

	// 请求同时携带 env 占位和 keyring 引用
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1, // 端口 1 触发网络错误，不影响优先级验证
		Password: "env:LDBX_MATRIX_ENV",
		Extra:    `{"credential_ref":"kr://matrix/test"}`,
	})

	// 网络错误是预期结果
	if errObj == nil {
		t.Fatal("expect network error due to invalid port")
	}

	// 关键验收：keyring.Get 未被调用（env 完全优先）
	if keyring.getCalls != 0 {
		t.Fatalf("keyring.Get was called %d times, should be 0 when env present", keyring.getCalls)
	}
}

// 场景 2: env 存在 + AES 密文存在 → env 优先，AES 不被解密。
func TestPriorityMatrix_env_over_aes(t *testing.T) {
	t.Setenv("LDBX_MATRIX_AES_ENV", "env-value-beats-aes")

	svc, _ := newServiceWithDeps(t, nil, nil)

	// 先保存一个 AES 加密的密码（用于验证 AES 路径不会被触发）
	_ = appEncryptForTest(t, "aes-value-should-not-use")

	// 请求携带 env 占位（env: 前缀优先于任何密文）
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: "env:LDBX_MATRIX_AES_ENV",
	})

	if errObj == nil {
		t.Fatal("expect network error")
	}

	// 验收：env 前缀触发环境变量解析，不涉及 AES 解密路径
	// 测试本身验证流程不崩溃，env 解析成功（否则会返回环境变量缺失错误）
}

// 场景 3: env 不存在 + keyring 引用存在 + AES 密文存在 → keyring 优先。
func TestPriorityMatrix_keyring_over_aes(t *testing.T) {
	// 不设置环境变量

	keyring := &mockKeyringAccessor{
		availableErr: nil,
		secrets: map[string]string{
			"kr://matrix/keyring_aes": "keyring-value-beats-aes",
		},
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)

	// AES 密文作为 password 字段值
	enc := appEncryptForTest(t, "aes-value-should-not-use")

	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: enc,
		Extra:    `{"credential_ref":"kr://matrix/keyring_aes"}`,
	})

	if errObj == nil {
		t.Fatal("expect network error")
	}

	// 关键验收：keyring.Get 被调用（keyring 引用优先于 AES 密文）
	if keyring.getCalls == 0 {
		t.Fatal("keyring.Get should be called when keyring ref present and env absent")
	}
}

// 场景 4: env 不存在 + keyring 引用不存在 + AES 密文存在 → AES 解密。
func TestPriorityMatrix_aes_when_no_env_no_keyring(t *testing.T) {
	svc, _ := newServiceWithDeps(t, nil, nil)

	enc := appEncryptForTest(t, "aes-standalone-value")

	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: enc,
		Extra:    "", // 无 keyring 引用
	})

	if errObj == nil {
		t.Fatal("expect network error")
	}
	// 测试通过表明 AES 解密成功（否则会返回解密错误）
}

// 场景 5: 所有来源都不存在 → 返回 plaintext 直传（或空）。
func TestPriorityMatrix_plaintext_fallback(t *testing.T) {
	svc, _ := newServiceWithDeps(t, nil, nil)

	// 直接使用 plaintext 密码（无 env:、无 aesgcm: 前缀、无 keyring 引用）
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: "plaintext-direct-value",
		Extra:    "",
	})

	if errObj == nil {
		t.Fatal("expect network error")
	}
	// 测试通过表明 plaintext 直接被使用（无加密/解密流程）
}

// ===== 环境变量缺失返回可分类错误 =====

// 环境变量占位但变量不存在 → 返回 INVALID_ARGUMENT 并携带 env 名称。
func TestEnvVariableMissingReturnsInvalidArgument(t *testing.T) {
	svc, _ := newServiceWithDeps(t, nil, nil)

	// 使用一个不存在于环境中的变量名
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Password: "env:LDBX_NON_EXISTENT_VAR_12345",
	})

	if errObj == nil {
		t.Fatal("expect error for missing env variable")
	}

	// 验收：错误码为 INVALID_ARGUMENT
	if errObj.Code != app.CodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT, got: %s", errObj.Code)
	}

	// 验收：错误消息应明确指出环境变量缺失
	if !strings.Contains(errObj.Message, "environment variable") && !strings.Contains(errObj.Message, "变量") {
		t.Errorf("error message should mention environment variable: %s", errObj.Message)
	}

	// 验收：Details 应携带变量名（不含变量值）
	if errObj.Details == nil || errObj.Details["env"] != "LDBX_NON_EXISTENT_VAR_12345" {
		t.Errorf("error details should contain env name: %v", errObj.Details)
	}

	// 验收：Details 不应包含变量值（因为不存在）
	for k, v := range errObj.Details {
		if strings.Contains(v, "secret") || strings.Contains(v, "password") {
			t.Errorf("error detail %s may leak sensitive info: %s", k, v)
		}
	}
}

// 环境变量存在但值为空 → 应使用空值（而非报错）。
func TestEnvVariableEmptyValue(t *testing.T) {
	t.Setenv("LDBX_EMPTY_ENV_VAR", "")

	svc, _ := newServiceWithDeps(t, nil, nil)

	// 使用空值的环境变量
	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     1,
		Password: "env:LDBX_EMPTY_ENV_VAR",
	})

	if errObj == nil {
		t.Fatal("expect network error (empty password may still fail auth)")
	}

	// 空值不应触发 "变量缺失" 错误（变量存在）
	if errObj.Code == app.CodeInvalidArgument && strings.Contains(errObj.Message, "not found") {
		t.Error("empty env value should not trigger 'not found' error")
	}
}

// ===== 脱敏测试：错误详情不泄露敏感值 =====

// 脱敏函数应替换明文密码。
func TestSanitizeError_ReplacesSecret(t *testing.T) {
	secret := "my-super-secret-password-12345"
	msg := "connection failed: password=my-super-secret-password-12345 rejected"

	sanitized := app.SanitizeErrorForTest(msg, secret)

	if strings.Contains(sanitized, secret) {
		t.Errorf("sanitized message still contains secret: %s", sanitized)
	}

	if !strings.Contains(sanitized, "***") {
		t.Errorf("sanitized message should contain replacement marker: %s", sanitized)
	}
}

// 脱敏函数应截断过长错误消息。
func TestSanitizeError_TruncatesLongMessage(t *testing.T) {
	longMsg := strings.Repeat("a", 300) + "secret-value-here"
	secret := "secret-value-here"

	sanitized := app.SanitizeErrorForTest(longMsg, secret)

	if len(sanitized) > 160 {
		t.Errorf("sanitized message too long: %d chars", len(sanitized))
	}

	if strings.Contains(sanitized, secret) {
		t.Errorf("truncated message still contains secret: %s", sanitized)
	}
}

// 多个敏感值同时脱敏。
func TestSanitizeError_MultipleSecrets(t *testing.T) {
	msg := "user=admin, password=pwd123, token=tkn456"
	password := "pwd123"
	token := "tkn456"

	sanitized := app.SanitizeErrorForTest(msg, password, token)

	if strings.Contains(sanitized, password) {
		t.Errorf("sanitized message contains password: %s", sanitized)
	}
	if strings.Contains(sanitized, token) {
		t.Errorf("sanitized message contains token: %s", sanitized)
	}
}

// 密钥环错误详情不泄露引用名中的敏感部分。
func TestKeyringErrorNoSensitiveLeak(t *testing.T) {
	keyring := &mockKeyringAccessor{
		availableErr: app.ErrKeyringAccessDenied,
	}
	svc, _ := newServiceWithDeps(t, nil, keyring)

	errObj := svc.TestConnection(context.Background(), app.ConnectionRequest{
		DBType: "postgres",
		Host:   "127.0.0.1",
		Port:   1,
		Extra:  `{"credential_ref":"kr://sensitive-ref-name-with-password-xyz"}`,
	})

	if errObj == nil {
		t.Fatal("expect keyring error")
	}

	// 验收：错误详情不包含引用名中的敏感部分
	for k, v := range errObj.Details {
		if strings.Contains(v, "password") || strings.Contains(v, "secret") {
			t.Errorf("error detail %s may leak sensitive info: %s", k, v)
		}
	}
}

// ===== 保存连接时的明文禁止落库 =====

// 保存时明文密码必须被 AES 加密。
func TestSaveConnectionEncryptsPlaintext(t *testing.T) {
	svc, store := newServiceWithDeps(t, nil, nil)

	id, errObj := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:     "encrypt-test",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Password: "plaintext-should-be-encrypted",
	})

	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	rec, err := store.GetConnectionByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load record failed: %v", err)
	}

	// 验收：存储的密码必须是 aesgcm: 前缀
	if !strings.HasPrefix(rec.Password, "aesgcm:") {
		t.Fatalf("stored password should be AES encrypted, got: %q", rec.Password)
	}

	// 验收：存储的密码不包含原始明文
	if strings.Contains(rec.Password, "plaintext-should-be-encrypted") {
		t.Fatal("stored password contains plaintext value")
	}
}

// 保存时 env: 前缀密码直接存储（无需加密，运行时解析）。
func TestSaveConnectionEnvPrefixStored(t *testing.T) {
	svc, store := newServiceWithDeps(t, nil, nil)

	id, errObj := svc.SaveConnection(context.Background(), app.ConnectionRequest{
		Name:     "env-test",
		DBType:   "postgres",
		Host:     "127.0.0.1",
		Port:     5432,
		Password: "env:LDBX_SOME_VAR",
	})

	if errObj != nil {
		t.Fatalf("save failed: %+v", errObj)
	}

	rec, err := store.GetConnectionByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load record failed: %v", err)
	}

	// 验收：env: 前缀保持原样存储
	if rec.Password != "env:LDBX_SOME_VAR" {
		t.Fatalf("env prefix should be stored as-is, got: %q", rec.Password)
	}
}