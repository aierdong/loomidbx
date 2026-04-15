// Package app 提供密钥环抽象层，支持 Windows/macOS/Linux 平台。
//
// 实现设计文档中定义的 Minimum Platform Support Matrix：
// - Windows 10+: Credential Manager / DPAPI 适配层
// - macOS 12+: Keychain Services 适配层
// - Linux (XDG): Secret Service/libsecret 适配层
package app

import (
	"context"
	"fmt"
	"runtime"

	"loomidbx/storage"
)

const (
	// keyringServiceName 是密钥环中的服务标识，用于区分不同应用的凭据。
	keyringServiceName = "LoomiDBX"

	// keyringUserPrefix 是密钥环用户名前缀，用于构建唯一引用。
	keyringUserPrefix = "connection:"
)

// PlatformKeyringAccessor 实现跨平台密钥环访问。
//
// 使用 github.com/zalando/go-keyring 库提供：
// - Windows: Credential Manager / DPAPI
// - macOS: Keychain Services
// - Linux: Secret Service / libsecret
type PlatformKeyringAccessor struct {
	// service 是密钥环服务名。
	service string
}

// NewPlatformKeyringAccessor 创建平台密钥环访问器。
//
// 输出：
// - *PlatformKeyringAccessor: 平台密钥环实现。
// - error: 当前平台不支持时返回 ErrKeyringUnavailable。
func NewPlatformKeyringAccessor() (*PlatformKeyringAccessor, error) {
	// 检查平台是否在支持矩阵内
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return &PlatformKeyringAccessor{service: keyringServiceName}, nil
	default:
		return nil, ErrKeyringUnavailable
	}
}

// IsAvailable 检查当前运行环境是否可访问密钥环。
//
// 探测策略：
// 1. 尝试执行一次无副作用的 Get 操作
// 2. 若返回 ErrKeyringNotFound 表示密钥环可用但无此条目
// 3. 若返回其他错误表示密钥环不可用或访问被拒绝
//
// 输入：
// - ctx: 请求上下文（当前实现不使用，但保留以供未来扩展）。
//
// 输出：
// - error: 密钥环可用时返回 nil，否则返回 ErrKeyringUnavailable 或 ErrKeyringAccessDenied。
func (k *PlatformKeyringAccessor) IsAvailable(ctx context.Context) error {
	return isKeyringAvailable(k.service)
}

// Get 按引用从密钥环读取凭据明文。
//
// 输入：
// - ctx: 请求上下文。
// - ref: 密钥环引用标识（通常为连接 ID）。
//
// 输出：
// - string: 凭据明文（仅在内存中使用，不持久化）。
// - error: 读取失败时返回 ErrKeyringUnavailable 或 ErrKeyringAccessDenied。
func (k *PlatformKeyringAccessor) Get(ctx context.Context, ref string) (string, error) {
	return getKeyringSecret(k.service, ref)
}

// Set 将凭据存入密钥环并返回引用标识。
//
// 输入：
// - ctx: 请求上下文。
// - ref: 密钥环引用标识（通常为连接 ID）。
// - secret: 凭据明文。
//
// 输出：
// - error: 存储失败时返回 ErrKeyringUnavailable 或 ErrKeyringAccessDenied。
func (k *PlatformKeyringAccessor) Set(ctx context.Context, ref string, secret string) error {
	return setKeyringSecret(k.service, ref, secret)
}

// Delete 从密钥环删除指定引用的凭据。
//
// 输入：
// - ctx: 请求上下文。
// - ref: 密钥环引用标识。
//
// 输出：
// - error: 删除失败时返回错误。
func (k *PlatformKeyringAccessor) Delete(ctx context.Context, ref string) error {
	return deleteKeyringSecret(k.service, ref)
}

// BuildKeyringRef 根据连接 ID 构建密钥环引用标识。
func BuildKeyringRef(connectionID string) string {
	return keyringUserPrefix + connectionID
}

// KeyringPurger 实现删除连接时的密钥环凭据清理。
type KeyringPurger struct {
	accessor *PlatformKeyringAccessor
}

// NewKeyringPurger 创建密钥环清理器。
func NewKeyringPurger(accessor *PlatformKeyringAccessor) *KeyringPurger {
	return &KeyringPurger{accessor: accessor}
}

// PurgeCredentialReference 清理密钥环中的凭据引用。
func (p *KeyringPurger) PurgeCredentialReference(ctx context.Context, ref storage.CredentialReference) error {
	if p.accessor == nil {
		return nil
	}
	if ref.Provider != "keyring" {
		return nil
	}
	return p.accessor.Delete(ctx, ref.CredentialRef)
}

// wrapKeyringError 将底层密钥环错误映射为应用层统一错误。
func wrapKeyringError(err error) error {
	if err == nil {
		return nil
	}

	// 密钥环条目不存在不算错误
	if isKeyringNotFound(err) {
		return nil
	}

	// 检测错误消息中是否包含拒绝相关关键词
	errStr := err.Error()
	if containsAccessDeniedKeywords(errStr) {
		return ErrKeyringAccessDenied
	}

	// 其他错误统一视为密钥环不可用
	return ErrKeyringUnavailable
}

// containsAccessDeniedKeywords 检测错误消息是否包含访问拒绝关键词。
func containsAccessDeniedKeywords(msg string) bool {
	deniedKeywords := []string{
		"access denied",
		"permission denied",
		"not authorized",
		"unauthorized",
		"authentication failed",
	}
	for _, kw := range deniedKeywords {
		if containsIgnoreCase(msg, kw) {
			return true
		}
	}
	return false
}

// containsIgnoreCase 检测字符串是否包含子串（忽略大小写）。
func containsIgnoreCase(s, substr string) bool {
	if substr == "" {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	// 使用简单的逐字符比较
	sLower := makeLower(s)
	substrLower := makeLower(substr)

	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// makeLower 将整个字符串转换为小写。
func makeLower(s string) string {
	result := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			result[i] = byte(c - 'A' + 'a')
		} else {
			result[i] = byte(c)
		}
	}
	return string(result)
}

// fmtKeyringError 格式化密钥环错误信息。
func fmtKeyringError(operation, ref string, err error) error {
	return fmt.Errorf("keyring %s failed for ref=%s: %w", operation, ref, err)
}