package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"loomidbx/backend/connector"
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
		connectorManager: connector.NewDriverManager(),
	}
}

// SaveConnection 保存或更新连接记录。
//
// 输入：
// - ctx: 请求上下文。
// - req: 连接请求；req.ID 为空表示创建，非空时更新。
//
// 输出：
// - string: 久化后的连接 ID。
// - *AppError: 失败时返回结构化错误。
func (s *ConnectionService) SaveConnection(ctx context.Context, req ConnectionRequest) (string, *AppError) {
	if req.Name == "" || req.DBType == "" {
		return "", &AppError{Code: CodeInvalidArgument, Message: "name and db_type are required"}
	}

	id := req.ID
	if id == "" {
		id = uuid.NewString()
	}

	// 确保请求中包含 ID，用于构建 keyring 引用
	reqWithID := req
	reqWithID.ID = id

	// 对于更新操作，获取旧记录以继承 credential_ref 等信息
	var existing *storage.ConnectionRecord
	if req.ID != "" {
		existingRec, err := s.store.GetConnectionByID(ctx, req.ID)
		if err != nil && err != sql.ErrNoRows {
			return "", &AppError{Code: CodeStorageError, Message: "load existing connection failed"}
		}
		existing = existingRec
	}

	// 如果请求中没有 extra，使用旧记录的 extra（继承 credential_ref）
	if reqWithID.Extra == "" && existing != nil && existing.Extra != "" {
		reqWithID.Extra = existing.Extra
	}

	passwordToStore, updatedExtra, appErr := s.passwordForStorage(ctx, reqWithID)
	if appErr != nil {
		return "", appErr
	}

	rec := storage.ConnectionRecord{
		ID:       id,
		Name:     reqWithID.Name,
		DBType:   reqWithID.DBType,
		Host:     reqWithID.Host,
		Port:     reqWithID.Port,
		Username: reqWithID.Username,
		Password: passwordToStore,
		Database: reqWithID.Database,
		Extra:    updatedExtra,
	}
	if existing != nil {
		rec.CreatedAt = existing.CreatedAt
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
//
// 错误分类：
// - AUTH_FAILED: 认证失败（用户名/密码错误）
// - TLS_ERROR: TLS/SSL 协商失败或证书问题
// - PROTOCOL_ERROR: 数据库协议层错误
// - UPSTREAM_UNAVAILABLE: 网络层不可达
// - DEADLINE_EXCEEDED: 连接超时
func (s *ConnectionService) TestConnection(ctx context.Context, req ConnectionRequest) *AppError {
	// 参数校验
	if req.DBType == "" {
		return &AppError{Code: CodeInvalidArgument, Message: "db_type is required"}
	}

	// 解析凭据
	resolvedSecret, appErr := s.resolveCredential(ctx, req)
	if appErr != nil {
		return appErr
	}

	// 构建连接参数
	params := connector.ConnectParams{
		DbType:     req.DBType,
		Host:       req.Host,
		Port:       req.Port,
		Username:   req.Username,
		Password:   resolvedSecret,
		Database:   req.Database,
		Extra:      req.Extra,
		TimeoutSec: req.TimeoutSec,
	}

	// 通过 connector 执行连接测试
	result := s.connectorManager.PingWithTimeout(ctx, params)

	// 成功时返回 nil
	if result.Category == connector.CategoryNone {
		return nil
	}

	// 根据错误分类映射错误码
	return mapConnectResultToAppError(result, params.TimeoutSec, req.Password, resolvedSecret)
}

// mapConnectResultToAppError 将连接器结果映射为应用层错误。
func mapConnectResultToAppError(result connector.ConnectResult, timeoutSec int, rawPassword, resolvedSecret string) *AppError {
	timeout := timeoutSec
	if timeout <= 0 {
		timeout = 20
	}

	details := result.Details
	if details == nil {
		details = make(map[string]string)
	}

	// 脱敏处理原始错误
	if result.RawError != nil {
		details["cause"] = sanitizeError(result.RawError, rawPassword, resolvedSecret)
	}

	switch result.Category {
	case connector.CategoryAuth:
		return &AppError{
			Code:    CodeAuthFailed,
			Message: "authentication failed",
			Details: details,
		}
	case connector.CategoryTLS:
		return &AppError{
			Code:    CodeTLSError,
			Message: "TLS/SSL error",
			Details: details,
		}
	case connector.CategoryProtocol:
		return &AppError{
			Code:    CodeProtocolError,
			Message: "protocol error",
			Details: details,
		}
	case connector.CategoryTimeout:
		details["timeout_sec"] = fmt.Sprintf("%d", timeout)
		return &AppError{
			Code:    CodeDeadlineExceeded,
			Message: "connection test timeout",
			Details: details,
		}
	case connector.CategoryNetwork:
		return &AppError{
			Code:    CodeUpstreamUnavailable,
			Message: "network unreachable",
			Details: details,
		}
	default:
		return &AppError{
			Code:    CodeUpstreamUnavailable,
			Message: "connection test failed",
			Details: details,
		}
	}
}
