package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"loomidbx/backend/storage"
)

const (
	// CodeInvalidArgument 表示输入参数不满足最小约束。
	CodeInvalidArgument      = "INVALID_ARGUMENT"

	// CodeStorageError 表示元数据存储读写失败。
	CodeStorageError         = "STORAGE_ERROR"

	// CodeNotFound 表示请求的连接记录不存在。
	CodeNotFound             = "NOT_FOUND"

	// CodeConfirmationRequired 表示删除等危险操作缺少确认标记。
	CodeConfirmationRequired = "CONFIRMATION_REQUIRED"

	// CodeDeadlineExceeded 表示连接测试超时。
	CodeDeadlineExceeded     = "DEADLINE_EXCEEDED"

	// CodeUpstreamUnavailable 表示目标数据库不可达或拒绝连接。
	CodeUpstreamUnavailable  = "UPSTREAM_UNAVAILABLE"

	// CodeKeyringUnavailable 表示当前平台或环境无法使用密钥环。
	CodeKeyringUnavailable   = "KEYRING_UNAVAILABLE"

	// CodeKeyringAccessDenied 表示密钥环可用但访问被拒绝。
	CodeKeyringAccessDenied  = "KEYRING_ACCESS_DENIED"
)

// AppError 是应用层统一错误模型，供 FFI 直接序列化返回。
type AppError struct {
	// Code 是稳定错误码，用于前端分支与契约映射。
	Code    string            `json:"code"`

	// Message 是面向调用方的可读错误描述。
	Message string            `json:"message"`

	// Details 是可选附加信息，禁止包含敏感明文数据。
	Details map[string]string `json:"details,omitempty"`
}

// ConnectionRequest 描述连接保存/测试请求。
type ConnectionRequest struct {
	// ID 为连接唯一标识；为空时表示创建，非空时表示更新。
	ID         string `json:"id,omitempty"`

	// Name 为连接展示名称。
	Name       string `json:"name"`

	// DBType 为数据库类型，如 mysql/postgres/sqlite。
	DBType     string `json:"db_type"`

	// Host 为数据库主机地址。
	Host       string `json:"host,omitempty"`

	// Port 为数据库端口。
	Port       int    `json:"port,omitempty"`

	// Username 为连接用户名。
	Username   string `json:"username,omitempty"`

	// Password 为敏感凭据，当前实现仅在内存中透传。
	Password   string `json:"password,omitempty"`

	// Database 为目标数据库名。
	Database   string `json:"database,omitempty"`

	// Extra 为扩展 JSON 字符串（如 sslmode 等）。
	Extra      string `json:"extra,omitempty"`

	// TimeoutSec 为连接测试超时时间（秒），<=0 使用默认值 20。
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

// DeleteConnectionRequest 描述删除连接请求。
type DeleteConnectionRequest struct {
	// ID 为待删除连接 ID。
	ID             string `json:"id"`

	// ConfirmCascade 为级联删除确认标记，必须显式为 true。
	ConfirmCascade bool   `json:"confirm_cascade"`
}

// ConnectionSummary 是列表接口返回的非敏感连接摘要。
type ConnectionSummary struct {
	// ID 为连接唯一标识。
	ID       string `json:"id"`

	// Name 为连接展示名称。
	Name     string `json:"name"`

	// DBType 为数据库类型。
	DBType   string `json:"db_type"`

	// Host 为数据库主机地址。
	Host     string `json:"host,omitempty"`

	// Port 为数据库端口。
	Port     int    `json:"port,omitempty"`

	// Username 为连接用户名。
	Username string `json:"username,omitempty"`

	// Database 为目标数据库名。
	Database string `json:"database,omitempty"`
	
	// Extra 为扩展配置，不包含明文凭据。
	Extra    string `json:"extra,omitempty"`
}

// ConnectionService 编排连接 CRUD 与连接测试能力。
type ConnectionService struct {
	// store 负责 ldb_connections 等元数据表访问。
	store *storage.ConnectionStore
}

// NewConnectionService 创建连接应用服务实例。
//
// 输入：
// - store: 元数据存储访问对象。
//
// 输出：
// - *ConnectionService: 初始化后的服务实例。
func NewConnectionService(store *storage.ConnectionStore) *ConnectionService {
	return &ConnectionService{store: store}
}

// SaveConnection 保存或更新连接记录。
//
// 输入：
// - ctx: 请求上下文，用于取消和超时传播。
// - req: 连接请求；req.ID 为空时创建，非空时按 ID 更新。
//
// 输出：
// - string: 保存成功后的连接 ID（更新时保持不变）。
// - *AppError: 失败时返回结构化错误；成功为 nil。
func (s *ConnectionService) SaveConnection(ctx context.Context, req ConnectionRequest) (string, *AppError) {
	if req.Name == "" || req.DBType == "" {
		return "", &AppError{Code: CodeInvalidArgument, Message: "name and db_type are required"}
	}
	id := req.ID
	if id == "" {
		id = uuid.NewString()
	}
	rec := storage.ConnectionRecord{
		ID:       id,
		Name:     req.Name,
		DBType:   req.DBType,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Database: req.Database,
		Extra:    req.Extra,
	}
	if req.ID != "" {
		existing, err := s.store.GetConnectionByID(ctx, req.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", &AppError{Code: CodeStorageError, Message: "load existing connection failed"}
		}
		if existing != nil {
			rec.CreatedAt = existing.CreatedAt
		}
	}
	if err := s.store.UpsertConnection(ctx, rec); err != nil {
		return "", &AppError{
			Code:    CodeStorageError,
			Message: "persist connection failed",
			Details: map[string]string{"cause": sanitizeError(err)},
		}
	}
	return id, nil
}

// ListConnections 返回所有连接摘要列表（不包含密码等敏感字段）。
//
// 输入：
// - ctx: 请求上下文。
//
// 输出：
// - []ConnectionSummary: 连接摘要数组。
// - *AppError: 失败时返回结构化错误；成功为 nil。
func (s *ConnectionService) ListConnections(ctx context.Context) ([]ConnectionSummary, *AppError) {
	recs, err := s.store.ListConnections(ctx)
	if err != nil {
		return nil, &AppError{Code: CodeStorageError, Message: "list connections failed"}
	}
	out := make([]ConnectionSummary, 0, len(recs))
	for _, rec := range recs {
		out = append(out, ConnectionSummary{
			ID:       rec.ID,
			Name:     rec.Name,
			DBType:   rec.DBType,
			Host:     rec.Host,
			Port:     rec.Port,
			Username: rec.Username,
			Database: rec.Database,
			Extra:    rec.Extra,
		})
	}
	return out, nil
}

// DeleteConnection 删除连接记录，并在确认后执行级联删除。
//
// 输入：
// - ctx: 请求上下文。
// - req: 删除请求，必须携带 ConfirmCascade=true。
//
// 输出：
// - *AppError: 失败时返回结构化错误；成功为 nil。
func (s *ConnectionService) DeleteConnection(ctx context.Context, req DeleteConnectionRequest) *AppError {
	if req.ID == "" {
		return &AppError{Code: CodeInvalidArgument, Message: "id is required"}
	}
	if !req.ConfirmCascade {
		return &AppError{Code: CodeConfirmationRequired, Message: "delete requires confirm_cascade=true"}
	}
	if err := s.store.DeleteConnectionCascade(ctx, req.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &AppError{Code: CodeNotFound, Message: "connection not found"}
		}
		return &AppError{Code: CodeStorageError, Message: "delete connection failed"}
	}
	return nil
}

// TestConnection 执行同步连接测试。
//
// 输入：
// - ctx: 请求上下文。
// - req: 连接测试请求，TimeoutSec 默认 20 秒，可配置。
//
// 输出：
// - *AppError: 测试失败时返回结构化错误；成功为 nil。
func (s *ConnectionService) TestConnection(ctx context.Context, req ConnectionRequest) *AppError {
	timeoutSec := req.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	if timeoutSec > 300 {
		timeoutSec = 300
	}

	if req.DBType == "sqlite" {
		// sqlite 无网络连接场景，按成功处理。
		return nil
	}
	if req.Host == "" || req.Port == 0 {
		return &AppError{Code: CodeInvalidArgument, Message: "host and port are required for network db"}
	}

	dialCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		if errors.Is(dialCtx.Err(), context.DeadlineExceeded) {
			return &AppError{
				Code:    CodeDeadlineExceeded,
				Message: "connection test timeout",
				Details: map[string]string{"timeout_sec": fmt.Sprintf("%d", timeoutSec)},
			}
		}
		return &AppError{
			Code:    CodeUpstreamUnavailable,
			Message: "connection test failed",
			Details: map[string]string{"cause": sanitizeError(err)},
		}
	}
	_ = conn.Close()
	return nil
}

// sanitizeError 对错误消息做长度截断，避免泄漏过多底层细节。
//
// 主要参数：
// - err: 原始错误对象。
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}
