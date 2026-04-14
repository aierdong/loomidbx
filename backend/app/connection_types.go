package app

import (
	"context"
	"errors"
	"loomidbx/backend/storage"
)

const (
	// CodeInvalidArgument 表示调用参数不满足最小约束。
	CodeInvalidArgument = "INVALID_ARGUMENT"

	// CodeStorageError 表示元数据读写失败或事务失败。
	CodeStorageError = "STORAGE_ERROR"

	// CodeNotFound 表示指定连接记录不存在。
	CodeNotFound = "NOT_FOUND"

	// CodeConfirmationRequired 表示危险操作缺少显式确认。
	CodeConfirmationRequired = "CONFIRMATION_REQUIRED"

	// CodeDeadlineExceeded 表示连接测试在超时边界内未完成。
	CodeDeadlineExceeded = "DEADLINE_EXCEEDED"

	// CodeUpstreamUnavailable 表示目标数据库不可达或拒绝连接。
	CodeUpstreamUnavailable = "UPSTREAM_UNAVAILABLE"

	// CodeKeyringUnavailable 表示当前环境不支持或未启用密钥环。
	CodeKeyringUnavailable = "KEYRING_UNAVAILABLE"

	// CodeKeyringAccessDenied 表示密钥环可用但访问被拒绝。
	CodeKeyringAccessDenied = "KEYRING_ACCESS_DENIED"
)

const (
	// envCredentialPrefix 表示凭据值来自环境变量占位符。
	envCredentialPrefix = "env:"

	// aesCredentialPrefix 表示凭据值采用 AES-GCM 密文封装。
	aesCredentialPrefix = "aesgcm:"

	// aesMasterKeyEnv 指定 AES 主密钥来源环境变量名。
	aesMasterKeyEnv = "LOOMIDBX_AES_MASTER_KEY"
)

// AppError 是应用层统一错误模型，供 FFI 直接返回给调用方。
type AppError struct {
	// Code 为稳定错误码，供上层做分支处理。
	Code string `json:"code"`

	// Message 为可读错误描述，不应包含敏感明文。
	Message string `json:"message"`

	// Details 为可选上下文信息，禁止放入密码/token 等敏感值。
	Details map[string]string `json:"details,omitempty"`
}

// ConnectionRequest 描述连接保存与连接测试的输入载荷。
type ConnectionRequest struct {
	// ID 为空时创建连接，非空时按既有连接更新。
	ID string `json:"id,omitempty"`

	// Name 为连接展示名，保存时必填。
	Name string `json:"name"`

	// DBType 为数据库类型，如 mysql/postgres/sqlite。
	DBType string `json:"db_type"`

	// Host 为目标数据库地址；网络型数据库测试时必填。
	Host string `json:"host,omitempty"`

	// Port 为目标数据库端口；网络型数据库测试时必填。
	Port int `json:"port,omitempty"`

	// Username 为连接用户名。
	Username string `json:"username,omitempty"`

	// Password 为敏感凭据输入，可能是 env 占位或密文串。
	Password string `json:"password,omitempty"`

	// Database 为目标数据库名。
	Database string `json:"database,omitempty"`

	// Extra 为扩展配置 JSON，可携带凭据引用等元信息。
	Extra string `json:"extra,omitempty"`

	// TimeoutSec 为连接测试超时秒数，<=0 使用默认值。
	TimeoutSec int `json:"timeout_sec,omitempty"`
}

// DeleteConnectionRequest 描述连接删除请求。
type DeleteConnectionRequest struct {
	// ID 为待删除连接 ID。
	ID string `json:"id"`

	// ConfirmCascade 必须显式为 true 才允许执行级联删除。
	ConfirmCascade bool `json:"confirm_cascade"`
}

// ConnectionSummary 是列表接口返回的非敏感连接摘要。
type ConnectionSummary struct {
	// ID 为连接唯一标识。
	ID string `json:"id"`

	// Name 为连接展示名。
	Name string `json:"name"`

	// DBType 为数据库类型。
	DBType string `json:"db_type"`

	// Host 为数据库地址。
	Host string `json:"host,omitempty"`

	// Port 为数据库端口。
	Port int `json:"port,omitempty"`

	// Username 为连接用户名。
	Username string `json:"username,omitempty"`

	// Database 为数据库名。
	Database string `json:"database,omitempty"`

	// Extra 为扩展配置，不包含敏感明文。
	Extra string `json:"extra,omitempty"`
}

// ConnectionService 负责连接 CRUD、凭据解析与连接测试编排。
type ConnectionService struct {
	// store 负责连接元数据持久化。
	store *storage.ConnectionStore

	// credentialPurger 负责删除连接时的外部凭据清理。
	credentialPurger CredentialPurger

	// keyringAccessor 负责读取系统密钥环凭据。
	keyringAccessor KeyringAccessor
}

// CredentialPurger 定义删除连接时的外部凭据清理契约。
type CredentialPurger interface {
	// PurgeCredentialReference 清理指定凭据引用（如 keyring 项）。
	PurgeCredentialReference(ctx context.Context, ref storage.CredentialReference) error
}

// noopCredentialPurger 用于未注入清理器时的空实现。
type noopCredentialPurger struct{}

// PurgeCredentialReference 在空实现中直接返回成功。
func (noopCredentialPurger) PurgeCredentialReference(context.Context, storage.CredentialReference) error {
	return nil
}

// KeyringAccessor 定义密钥环可用性探测与凭据读取能力。
type KeyringAccessor interface {
	// IsAvailable 检查当前运行环境是否可访问密钥环。
	IsAvailable(ctx context.Context) error

	// Get 按引用从密钥环读取凭据明文（仅在内存中使用）。
	Get(ctx context.Context, ref string) (string, error)
}

// noopKeyringAccessor 用于未注入密钥环实现时的默认拒绝。
type noopKeyringAccessor struct{}

// IsAvailable 在默认实现中返回密钥环不可用。
func (noopKeyringAccessor) IsAvailable(context.Context) error { return ErrKeyringUnavailable }

// Get 在默认实现中返回密钥环不可用。
func (noopKeyringAccessor) Get(context.Context, string) (string, error) {
	return "", ErrKeyringUnavailable
}

var (
	// ErrKeyringUnavailable 表示平台不支持或运行环境未启用密钥环。
	ErrKeyringUnavailable = errors.New("keyring unavailable")
	
	// ErrKeyringAccessDenied 表示密钥环已存在但权限被拒绝。
	ErrKeyringAccessDenied = errors.New("keyring access denied")
)
