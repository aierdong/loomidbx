package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

const (
	// CredentialModeAES 表示凭据使用 AES 列加密存储（默认）。
	CredentialModeAES = "aes"

	// CredentialModeKeyring 表示凭据使用密钥环存储。
	CredentialModeKeyring = "keyring"

	// CredentialModeEnvOnly 表示凭据仅从环境变量获取，不持久化。
	CredentialModeEnvOnly = "env_only"
)

// passwordForStorage 根据凭据来源策略生成可持久化值，禁止静默明文落库。
// 当选择 keyring 存储时，写入密钥环并返回更新的 extra JSON。
func (s *ConnectionService) passwordForStorage(ctx context.Context, req ConnectionRequest) (passwordToStore string, updatedExtra string, appErr *AppError) {
	originalPassword := req.Password
	originalExtra := req.Extra

	if originalPassword == "" {
		return "", originalExtra, nil
	}

	// 环境变量占位符直接保存，无需额外处理
	if strings.HasPrefix(originalPassword, envCredentialPrefix) {
		return originalPassword, originalExtra, nil
	}

	// 解析 extra 以获取凭据策略配置
	extraConfig := parseCredentialExtra(originalExtra)

	// 已存在 credential_ref 表示凭据已在 keyring 中
	if extraConfig.CredentialRef != "" {
		if err := s.keyringAccessor.IsAvailable(ctx); err != nil {
			return "", "", keyringErrorToAppError(err)
		}

		// 用户提供了新密码（非 AES 加密格式），需要更新 keyring
		if originalPassword != "" && !strings.HasPrefix(originalPassword, aesCredentialPrefix) && !strings.HasPrefix(originalPassword, envCredentialPrefix) {
			if err := s.keyringAccessor.Set(ctx, extraConfig.CredentialRef, originalPassword); err != nil {
				return "", "", keyringErrorToAppError(err)
			}
		}
		// 不存储密码到数据库，extra 保持不变
		return "", originalExtra, nil
	}

	// 已是 AES 加密格式，直接保存
	if strings.HasPrefix(originalPassword, aesCredentialPrefix) {
		return originalPassword, originalExtra, nil
	}

	// 用户选择 keyring 存储新凭据：写入密钥环并生成引用
	if extraConfig.CredentialMode == CredentialModeKeyring {
		if err := s.keyringAccessor.IsAvailable(ctx); err != nil {
			return "", "", keyringErrorToAppError(err)
		}

		// 构建密钥环引用（使用连接 ID 或生成临时引用）
		ref := BuildKeyringRef(req.ID)
		if ref == "" {
			// 对于新建连接（无 ID），使用临时引用标识
			ref = keyringUserPrefix + "pending"
		}

		// 将凭据写入密钥环
		if err := s.keyringAccessor.Set(ctx, ref, originalPassword); err != nil {
			return "", "", keyringErrorToAppError(err)
		}

		// 更新 extra，添加 credential_ref
		extraConfig.CredentialRef = ref
		updatedExtraBytes, err := json.Marshal(extraConfig)
		if err != nil {
			return "", "", &AppError{Code: CodeStorageError, Message: "serialize credential config failed"}
		}

		// 密码不落库，仅保存引用
		return "", string(updatedExtraBytes), nil
	}

	// 默认 AES 列加密路径
	enc, err := encryptSecret(originalPassword)
	if err != nil {
		return "", "", &AppError{Code: CodeStorageError, Message: "encrypt credential failed"}
	}
	return enc, originalExtra, nil
}

// parseCredentialExtra 从 extra JSON 中解析凭据配置，解析失败时返回空结构。
func parseCredentialExtra(extra string) credentialExtra {
	if strings.TrimSpace(extra) == "" {
		return credentialExtra{}
	}
	var payload credentialExtra
	if err := json.Unmarshal([]byte(extra), &payload); err != nil {
		return credentialExtra{}
	}
	return payload
}

// resolveCredential 按 env -> keyring -> AES -> 直传 的优先级解析运行时凭据。
func (s *ConnectionService) resolveCredential(ctx context.Context, req ConnectionRequest) (string, *AppError) {
	if strings.HasPrefix(req.Password, envCredentialPrefix) {
		name := strings.TrimSpace(strings.TrimPrefix(req.Password, envCredentialPrefix))
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", &AppError{
				Code:    CodeInvalidArgument,
				Message: "environment variable not found",
				Details: map[string]string{"env": name},
			}
		}
		return val, nil
	}

	if ref := keyringRefFromExtra(req.Extra); ref != "" {
		if err := s.keyringAccessor.IsAvailable(ctx); err != nil {
			return "", keyringErrorToAppError(err)
		}
		secret, err := s.keyringAccessor.Get(ctx, ref)
		if err != nil {
			return "", keyringErrorToAppError(err)
		}
		return secret, nil
	}

	if strings.HasPrefix(req.Password, aesCredentialPrefix) {
		secret, err := decryptSecret(req.Password)
		if err != nil {
			return "", &AppError{Code: CodeInvalidArgument, Message: "decrypt credential failed"}
		}
		return secret, nil
	}

	return req.Password, nil
}

// keyringErrorToAppError 将底层密钥环错误归一映射为稳定业务错误码。
func keyringErrorToAppError(err error) *AppError {
	switch {
	case errors.Is(err, ErrKeyringAccessDenied):
		return &AppError{Code: CodeKeyringAccessDenied, Message: "keyring access denied"}
	default:
		return &AppError{Code: CodeKeyringUnavailable, Message: "keyring unavailable"}
	}
}

// credentialExtra 表示 extra 中与凭据解析相关的最小字段集合。
type credentialExtra struct {
	// CredentialRef 为密钥环引用标识（而非明文凭据）。
	CredentialRef string `json:"credential_ref"`

	// CredentialMode 表示凭据存储策略：aes（默认）、keyring、env_only。
	CredentialMode string `json:"credential_mode"`
}

// keyringRefFromExtra 从扩展 JSON 中提取密钥环引用，解析失败时按无引用处理。
func keyringRefFromExtra(extra string) string {
	if strings.TrimSpace(extra) == "" {
		return ""
	}
	var payload credentialExtra
	if err := json.Unmarshal([]byte(extra), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.CredentialRef)
}
