package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// passwordForStorage 根据凭据来源策略生成可持久化值，禁止静默明文落库。
func (s *ConnectionService) passwordForStorage(ctx context.Context, req ConnectionRequest) (string, *AppError) {
	if req.Password == "" {
		return "", nil
	}
	if strings.HasPrefix(req.Password, envCredentialPrefix) {
		return req.Password, nil
	}

	ref := keyringRefFromExtra(req.Extra)
	if ref != "" {
		if err := s.keyringAccessor.IsAvailable(ctx); err != nil {
			return "", keyringErrorToAppError(err)
		}
		return "", nil
	}

	if strings.HasPrefix(req.Password, aesCredentialPrefix) {
		return req.Password, nil
	}

	enc, err := encryptSecret(req.Password)
	if err != nil {
		return "", &AppError{Code: CodeStorageError, Message: "encrypt credential failed"}
	}
	return enc, nil
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
