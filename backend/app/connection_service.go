package app

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"loomidbx/backend/storage"
)

// NewConnectionService 创建连接应用服务实例。
//
// 输入：
// - store: 连接元数据存储实现。
//
// 输出：
// - *ConnectionService: 初始化后的服务对象。
func NewConnectionService(store *storage.ConnectionStore) *ConnectionService {
	return NewConnectionServiceWithDeps(store, nil, nil)
}

// NewConnectionServiceWithPurger 创建带凭据清理器的连接应用服务实例。
//
// 输入：
// - store: 连接元数据存储实现。
// - purger: 外部凭据清理器（可为 nil）。
//
// 输出：
// - *ConnectionService: 初始化后的服务对象。
func NewConnectionServiceWithPurger(store *storage.ConnectionStore, purger CredentialPurger) *ConnectionService {
	return NewConnectionServiceWithDeps(store, purger, nil)
}

// NewConnectionServiceWithDeps 创建带凭据清理器与密钥环访问器的连接应用服务实例。
//
// 输入：
// - store: 连接元数据存储实现。
// - purger: 外部凭据清理器（可为 nil）。
// - keyring: 密钥环读取器（可为 nil）。
//
// 输出：
// - *ConnectionService: 初始化后的服务对象。
func NewConnectionServiceWithDeps(store *storage.ConnectionStore, purger CredentialPurger, keyring KeyringAccessor) *ConnectionService {
	if purger == nil {
		purger = noopCredentialPurger{}
	}
	if keyring == nil {
		keyring = noopKeyringAccessor{}
	}
	return &ConnectionService{
		store:            store,
		credentialPurger: purger,
		keyringAccessor:  keyring,
	}
}

// SaveConnection 保存或更新连接记录。
//
// 输入：
// - ctx: 请求上下文。
// - req: 连接请求；req.ID 为空表示创建，非空表示更新。
//
// 输出：
// - string: 持久化后的连接 ID。
// - *AppError: 失败时返回结构化错误。
func (s *ConnectionService) SaveConnection(ctx context.Context, req ConnectionRequest) (string, *AppError) {
	if req.Name == "" || req.DBType == "" {
		return "", &AppError{Code: CodeInvalidArgument, Message: "name and db_type are required"}
	}

	id := req.ID
	if id == "" {
		id = uuid.NewString()
	}

	passwordToStore, appErr := s.passwordForStorage(ctx, req)
	if appErr != nil {
		return "", appErr
	}

	rec := storage.ConnectionRecord{
		ID:       id,
		Name:     req.Name,
		DBType:   req.DBType,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: passwordToStore,
		Database: req.Database,
		Extra:    req.Extra,
	}
	if req.ID != "" {
		existing, err := s.store.GetConnectionByID(ctx, req.ID)
		if err != nil && err != sql.ErrNoRows {
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
// - []ConnectionSummary: 连接摘要集合。
// - *AppError: 失败时返回结构化错误。
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
// - req: 删除请求，需显式携带 ConfirmCascade=true。
//
// 输出：
// - *AppError: 失败时返回结构化错误。
func (s *ConnectionService) DeleteConnection(ctx context.Context, req DeleteConnectionRequest) *AppError {
	if req.ID == "" {
		return &AppError{Code: CodeInvalidArgument, Message: "id is required"}
	}
	if !req.ConfirmCascade {
		return &AppError{Code: CodeConfirmationRequired, Message: "delete requires confirm_cascade=true"}
	}
	if err := s.store.DeleteConnectionCascade(ctx, req.ID, s.credentialPurgeFunc()); err != nil {
		if err == sql.ErrNoRows {
			return &AppError{Code: CodeNotFound, Message: "connection not found"}
		}
		return &AppError{Code: CodeStorageError, Message: "delete connection failed"}
	}
	return nil
}

// credentialPurgeFunc 将应用层清理接口适配为存储层回调签名。
func (s *ConnectionService) credentialPurgeFunc() storage.DeleteCredentialReferenceFunc {
	return func(ctx context.Context, ref storage.CredentialReference) error {
		return s.credentialPurger.PurgeCredentialReference(ctx, ref)
	}
}

// TestConnection 执行同步连接测试。
//
// 输入：
// - ctx: 请求上下文。
// - req: 连接测试请求，包含连接参数与超时设置。
//
// 输出：
// - *AppError: 成功返回 nil，失败返回结构化错误。
func (s *ConnectionService) TestConnection(ctx context.Context, req ConnectionRequest) *AppError {
	resolvedSecret, appErr := s.resolveCredential(ctx, req)
	if appErr != nil {
		return appErr
	}

	timeoutSec := req.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	if timeoutSec > 300 {
		timeoutSec = 300
	}

	if req.DBType == "sqlite" {
		// sqlite 不需要网络探测，视为连接可达。
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
		if dialCtx.Err() == context.DeadlineExceeded {
			return &AppError{
				Code:    CodeDeadlineExceeded,
				Message: "connection test timeout",
				Details: map[string]string{"timeout_sec": fmt.Sprintf("%d", timeoutSec)},
			}
		}
		return &AppError{
			Code:    CodeUpstreamUnavailable,
			Message: "connection test failed",
			Details: map[string]string{"cause": sanitizeError(err, req.Password, resolvedSecret)},
		}
	}
	_ = conn.Close()
	return nil
}
