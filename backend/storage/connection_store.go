package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"loomidbx/schema"

	_ "modernc.org/sqlite"
)

// defaultMetaPath 是默认元数据库文件路径。
const defaultMetaPath = "loomidbx.db"

// storageBackendEnv 是元数据存储后端环境变量名。
const storageBackendEnv = "LOOMIDBX_STORAGE"

const (
	// backendSQLite 表示 SQLite 元数据库后端。
	backendSQLite = "sqlite"

	// backendMySQL 表示 MySQL 元数据库后端。
	backendMySQL = "mysql"

	// backendPostgres 表示 Postgres 元数据库后端。
	backendPostgres = "postgres"
)

// ConnectionRecord 映射 ldb_connections 表的一条连接记录。
type ConnectionRecord struct {
	// ID 为连接唯一标识。
	ID string

	// Name 为连接展示名称。
	Name string

	// DBType 为数据库类型标识。
	DBType string

	// Host 为数据库主机地址。
	Host string

	// Port 为数据库端口。
	Port int

	// Username 为连接用户名。
	Username string

	// Password 为敏感凭据字段（调用方负责加密/安全策略）。
	Password string

	// Database 为目标数据库名。
	Database string

	// Extra 为扩展 JSON 文本。
	Extra string

	// CreatedAt 为记录创建时间（Unix 秒）。
	CreatedAt int64

	// UpdatedAt 为记录最后更新时间（Unix 秒）。
	UpdatedAt int64
}

// ConnectionStore 负责连接元数据的持久化读写。
type ConnectionStore struct {
	// db 是元数据库连接句柄。
	db *sql.DB

	// backend 是当前元数据库后端类型（sqlite/mysql/postgres）。
	backend string
}

// CredentialReference 表示连接关联的凭据引用记录（例如密钥环 token）。
type CredentialReference struct {
	// ID 为凭据引用记录唯一标识。
	ID string

	// ConnectionID 为所属连接 ID。
	ConnectionID string

	// Provider 为凭据提供方标识（如 keyring/env）。
	Provider string

	// CredentialRef 为提供方内部引用标识。
	CredentialRef string
}

// NewConnectionStore 创建基于 sqlite 的连接存储。
//
// 输入：
// - dbPath: 元数据库文件路径；为空时使用默认路径。
//
// 输出：
// - *ConnectionStore: 初始化后的存储对象。
// - error: 初始化失败错误。
func NewConnectionStore(dbPath string) (*ConnectionStore, error) {
	if dbPath == "" {
		dbPath = defaultMetaPath
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open metadata db: %w", err)
	}
	store := &ConnectionStore{
		db:      db,
		backend: detectStorageBackend(),
	}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// NewConnectionStoreFromDB 使用现有数据库句柄创建存储对象（测试场景常用）。
//
// 输入：
// - db: 已打开的数据库句柄，不能为空。
//
// 输出：
// - *ConnectionStore: 初始化后的存储对象。
// - error: 初始化失败错误。
func NewConnectionStoreFromDB(db *sql.DB) (*ConnectionStore, error) {
	if db == nil {
		return nil, errors.New("nil metadata db")
	}
	store := &ConnectionStore{
		db:      db,
		backend: detectStorageBackend(),
	}
	if err := store.migrate(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

// Close 关闭存储底层数据库连接。
//
// 输出：
// - error: 关闭失败错误；重复关闭返回 nil。
func (s *ConnectionStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// UpsertConnection 按连接 ID 创建或更新记录。
//
// 输入：
// - ctx: 请求上下文。
// - rec: 待保存连接记录。
//
// 输出：
// - error: 保存失败错误。
func (s *ConnectionStore) UpsertConnection(ctx context.Context, rec ConnectionRecord) error {
	now := time.Now().Unix()
	if rec.CreatedAt == 0 {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	upsertSQL := s.buildConnectionUpsertSQL()
	_, err := s.db.ExecContext(ctx, upsertSQL,
		rec.ID, rec.Name, rec.DBType, rec.Host, rec.Port,
		rec.Username, rec.Password, rec.Database, rec.Extra,
		rec.CreatedAt, rec.UpdatedAt)
	if err != nil {
		return fmt.Errorf("persist connection: %w", err)
	}
	return nil
}

// detectStorageBackend 基于环境变量识别元数据库后端。
//
// 输出：
// - string: 标准化后的后端标识（sqlite/mysql/postgres）。
func detectStorageBackend() string {
	backend := strings.TrimSpace(strings.ToLower(os.Getenv(storageBackendEnv)))
	switch backend {
	case backendMySQL:
		return backendMySQL
	case backendPostgres:
		return backendPostgres
	default:
		return backendSQLite
	}
}

// buildConnectionUpsertSQL 根据后端方言生成 ldb_connections 的 upsert SQL。
//
// 输出：
// - string: 可用于 ExecContext 的 upsert 语句。
func (s *ConnectionStore) buildConnectionUpsertSQL() string {
	switch s.backend {
	case backendMySQL:
		return `
		INSERT INTO ldb_connections (
			id, name, db_type, host, port, username, password, database, extra, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			name=VALUES(name),
			db_type=VALUES(db_type),
			host=VALUES(host),
			port=VALUES(port),
			username=VALUES(username),
			password=VALUES(password),
			database=VALUES(database),
			extra=VALUES(extra),
			updated_at=VALUES(updated_at)
		`
	case backendPostgres:
		return `
		INSERT INTO ldb_connections (
			id, name, db_type, host, port, username, password, database, extra, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT(id) DO UPDATE SET
			name=EXCLUDED.name,
			db_type=EXCLUDED.db_type,
			host=EXCLUDED.host,
			port=EXCLUDED.port,
			username=EXCLUDED.username,
			password=EXCLUDED.password,
			database=EXCLUDED.database,
			extra=EXCLUDED.extra,
			updated_at=EXCLUDED.updated_at
		`
	default:
		// SQLite 语法与 Postgres 类似，但占位符使用 ?。
		return `
		INSERT INTO ldb_connections (
			id, name, db_type, host, port, username, password, database, extra, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			db_type=excluded.db_type,
			host=excluded.host,
			port=excluded.port,
			username=excluded.username,
			password=excluded.password,
			database=excluded.database,
			extra=excluded.extra,
			updated_at=excluded.updated_at
		`
	}
}

// GetConnectionByID 按 ID 查询连接记录。
//
// 输入：
// - ctx: 请求上下文。
// - id: 连接唯一标识。
//
// 输出：
// - *ConnectionRecord: 命中的连接记录。
// - error: 查询失败或未找到错误。
func (s *ConnectionStore) GetConnectionByID(ctx context.Context, id string) (*ConnectionRecord, error) {
	row := s.db.QueryRowContext(ctx, s.buildGetConnectionByIDSQL(), id)
	var rec ConnectionRecord
	if err := row.Scan(&rec.ID, &rec.Name, &rec.DBType, &rec.Host, &rec.Port,
		&rec.Username, &rec.Password, &rec.Database, &rec.Extra,
		&rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListConnections 列出全部连接记录。
//
// 输入：
// - ctx: 请求上下文。
//
// 输出：
// - []ConnectionRecord: 连接记录列表。
// - error: 查询失败错误。
func (s *ConnectionStore) ListConnections(ctx context.Context) ([]ConnectionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, db_type, host, port, username, password, database, extra, created_at, updated_at
		FROM ldb_connections ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer rows.Close()

	out := make([]ConnectionRecord, 0)
	for rows.Next() {
		var rec ConnectionRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.DBType, &rec.Host, &rec.Port,
			&rec.Username, &rec.Password, &rec.Database, &rec.Extra,
			&rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connections: %w", err)
	}
	return out, nil
}

// PatchConnectionSchemaExtra 合并 schema 子域字段到 ldb_connections.extra（保留无关键）。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
// - patch: 局部更新；空 patch 不写库。
//
// 输出：
// - error: 连接不存在或合并/写入失败时返回错误。
func (s *ConnectionStore) PatchConnectionSchemaExtra(ctx context.Context, connectionID string, patch schema.ConnectionSchemaMetaPatch) error {
	if patch.IsEmpty() {
		return nil
	}
	rec, err := s.GetConnectionByID(ctx, connectionID)
	if err != nil {
		return err
	}
	merged, err := schema.MergeConnectionExtraSchemaMeta(rec.Extra, patch)
	if err != nil {
		return fmt.Errorf("merge connection extra: %w", err)
	}
	rec.Extra = merged
	return s.UpsertConnection(ctx, *rec)
}

// LoadConnectionSchemaMeta 读取并解析连接 extra 中的 schema 子域元数据。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
//
// 输出：
// - schema.ConnectionSchemaMeta: 解析后的读模型。
// - error: 连接不存在或解析失败时返回错误。
func (s *ConnectionStore) LoadConnectionSchemaMeta(ctx context.Context, connectionID string) (schema.ConnectionSchemaMeta, error) {
	rec, err := s.GetConnectionByID(ctx, connectionID)
	if err != nil {
		return schema.ConnectionSchemaMeta{}, err
	}
	return schema.ParseConnectionSchemaMeta(rec.Extra)
}

// LoadCurrentSchema 按连接读取当前 schema（ldb_table_schemas / ldb_column_schemas）。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
//
// 输出：
// - *schema.CurrentSchemaBundle: 当前 schema 聚合结果；无记录时返回空 bundle。
// - error: 查询失败错误。
func (s *ConnectionStore) LoadCurrentSchema(ctx context.Context, connectionID string) (*schema.CurrentSchemaBundle, error) {
	trimmedConnectionID := strings.TrimSpace(connectionID)
	if trimmedConnectionID == "" {
		return nil, errors.New("connection_id is required")
	}
	bundle := &schema.CurrentSchemaBundle{
		Tables:  make([]schema.TableSchemaPersisted, 0),
		Columns: make([]schema.ColumnSchemaPersisted, 0),
	}

	tableRows, err := s.db.QueryContext(ctx, s.buildLoadCurrentSchemaTablesSQL(), trimmedConnectionID)
	if err != nil {
		return nil, fmt.Errorf("load current schema tables: %w", err)
	}
	defer tableRows.Close()
	for tableRows.Next() {
		var row schema.TableSchemaPersisted
		var schemaName sql.NullString
		var tableComment sql.NullString
		if err := tableRows.Scan(
			&row.ID,
			&row.ConnectionID,
			&row.DatabaseName,
			&schemaName,
			&row.TableName,
			&tableComment,
			&row.ScanVersion,
			&row.ScannedAt,
		); err != nil {
			return nil, fmt.Errorf("scan current schema table row: %w", err)
		}
		row.SchemaName = strings.TrimSpace(schemaName.String)
		row.TableComment = strings.TrimSpace(tableComment.String)
		bundle.Tables = append(bundle.Tables, row)
	}
	if err := tableRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current schema table rows: %w", err)
	}
	if len(bundle.Tables) == 0 {
		return bundle, nil
	}

	columnRows, err := s.db.QueryContext(ctx, s.buildLoadCurrentSchemaColumnsSQL(), trimmedConnectionID)
	if err != nil {
		return nil, fmt.Errorf("load current schema columns: %w", err)
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var row schema.ColumnSchemaPersisted
		var ordinalPos sql.NullInt64
		var isPrimaryKey int
		var isNullable int
		var isUnique int
		var isAutoIncrement int
		var defaultValue sql.NullString
		var columnComment sql.NullString
		var fkRefTable sql.NullString
		var fkRefColumn sql.NullString
		var extra sql.NullString
		if err := columnRows.Scan(
			&row.ID,
			&row.TableSchemaID,
			&row.ColumnName,
			&ordinalPos,
			&row.DataType,
			&row.AbstractType,
			&isPrimaryKey,
			&isNullable,
			&isUnique,
			&isAutoIncrement,
			&defaultValue,
			&columnComment,
			&fkRefTable,
			&fkRefColumn,
			&extra,
		); err != nil {
			return nil, fmt.Errorf("scan current schema column row: %w", err)
		}
		if ordinalPos.Valid {
			row.OrdinalPos = int(ordinalPos.Int64)
		}
		row.IsPrimaryKey = isPrimaryKey != 0
		row.IsNullable = isNullable != 0
		row.IsUnique = isUnique != 0
		row.IsAutoIncrement = isAutoIncrement != 0
		row.DefaultValue = strings.TrimSpace(defaultValue.String)
		row.ColumnComment = strings.TrimSpace(columnComment.String)
		row.FKRefTable = strings.TrimSpace(fkRefTable.String)
		row.FKRefColumn = strings.TrimSpace(fkRefColumn.String)
		row.Extra = strings.TrimSpace(extra.String)
		bundle.Columns = append(bundle.Columns, row)
	}
	if err := columnRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current schema column rows: %w", err)
	}
	return bundle, nil
}

// TransactionalReplaceCurrentSchema 在同一事务内覆盖连接当前 schema（先删后写）。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
// - next: 下一版当前 schema 聚合。
//
// 输出：
// - error: 覆盖失败错误。
func (s *ConnectionStore) TransactionalReplaceCurrentSchema(ctx context.Context, connectionID string, next *schema.CurrentSchemaBundle) error {
	trimmedConnectionID := strings.TrimSpace(connectionID)
	if trimmedConnectionID == "" {
		return errors.New("connection_id is required")
	}
	if next == nil {
		next = &schema.CurrentSchemaBundle{}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start replace current schema tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, s.buildDeleteColumnSchemasByConnectionIDSQL(), trimmedConnectionID); err != nil {
		return fmt.Errorf("delete current schema columns: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.buildDeleteTableSchemasByConnectionIDSQL(), trimmedConnectionID); err != nil {
		return fmt.Errorf("delete current schema tables: %w", err)
	}

	tableIDSet := make(map[string]struct{}, len(next.Tables))
	for _, table := range next.Tables {
		tableID := strings.TrimSpace(table.ID)
		if tableID == "" {
			return errors.New("table_schema.id is required")
		}
		tableConnectionID := strings.TrimSpace(table.ConnectionID)
		if tableConnectionID != "" && tableConnectionID != trimmedConnectionID {
			return fmt.Errorf("table_schema.connection_id mismatch: %s", tableID)
		}
		databaseName := strings.TrimSpace(table.DatabaseName)
		tableName := strings.TrimSpace(table.TableName)
		if databaseName == "" || tableName == "" {
			return fmt.Errorf("table_schema database_name/table_name is required: %s", tableID)
		}
		scanVersion := table.ScanVersion
		if scanVersion <= 0 {
			scanVersion = 1
		}
		scannedAt := table.ScannedAt
		if scannedAt <= 0 {
			scannedAt = time.Now().Unix()
		}
		if _, err := tx.ExecContext(
			ctx,
			s.buildInsertCurrentSchemaTableSQL(),
			tableID,
			trimmedConnectionID,
			databaseName,
			nilIfEmpty(table.SchemaName),
			tableName,
			nilIfEmpty(table.TableComment),
			scanVersion,
			scannedAt,
		); err != nil {
			return fmt.Errorf("insert current schema table %s: %w", tableID, err)
		}
		tableIDSet[tableID] = struct{}{}
	}

	for _, column := range next.Columns {
		columnID := strings.TrimSpace(column.ID)
		if columnID == "" {
			return errors.New("column_schema.id is required")
		}
		tableSchemaID := strings.TrimSpace(column.TableSchemaID)
		if tableSchemaID == "" {
			return fmt.Errorf("column_schema.table_schema_id is required: %s", columnID)
		}
		if _, ok := tableIDSet[tableSchemaID]; !ok {
			return fmt.Errorf("column_schema.table_schema_id not found in tables: %s", columnID)
		}
		columnName := strings.TrimSpace(column.ColumnName)
		dataType := strings.TrimSpace(column.DataType)
		abstractType := strings.TrimSpace(column.AbstractType)
		if columnName == "" || dataType == "" || abstractType == "" {
			return fmt.Errorf("column_schema column_name/data_type/abstract_type is required: %s", columnID)
		}
		if _, err := tx.ExecContext(
			ctx,
			s.buildInsertCurrentSchemaColumnSQL(),
			columnID,
			tableSchemaID,
			columnName,
			column.OrdinalPos,
			dataType,
			abstractType,
			boolToInt(column.IsPrimaryKey),
			boolToInt(column.IsNullable),
			boolToInt(column.IsUnique),
			boolToInt(column.IsAutoIncrement),
			nilIfEmpty(column.DefaultValue),
			nilIfEmpty(column.ColumnComment),
			nilIfEmpty(column.FKRefTable),
			nilIfEmpty(column.FKRefColumn),
			nilIfEmpty(column.Extra),
		); err != nil {
			return fmt.Errorf("insert current schema column %s: %w", columnID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace current schema tx: %w", err)
	}
	return nil
}

// PatchConnectionSchemaMeta 将 schema 子域 patch 合并写入 extra（语义同 PatchConnectionSchemaExtra，供 schema.TrustConnectionMetaRepository 实现）。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
// - patch: 局部更新。
//
// 输出：
// - error: 连接不存在或合并/写入失败时返回错误。
func (s *ConnectionStore) PatchConnectionSchemaMeta(ctx context.Context, connectionID string, patch schema.ConnectionSchemaMetaPatch) error {
	return s.PatchConnectionSchemaExtra(ctx, connectionID, patch)
}

// DeleteCredentialReferenceFunc 定义删除流程中的外部凭据清理回调。
type DeleteCredentialReferenceFunc func(ctx context.Context, ref CredentialReference) error

// DeleteConnectionCascade 在同一事务中删除连接及其下游元数据，并先清理凭据引用。
//
// 输入：
// - ctx: 请求上下文。
// - id: 待删除连接 ID。
// - purgeFn: 外部凭据清理回调（可为 nil）。
//
// 输出：
// - error: 删除失败错误；不存在时返回 sql.ErrNoRows。
func (s *ConnectionStore) DeleteConnectionCascade(ctx context.Context, id string, purgeFn DeleteCredentialReferenceFunc) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start delete tx: %w", err)
	}
	defer tx.Rollback()

	refs, err := s.listCredentialReferencesTx(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("load credential refs: %w", err)
	}
	for _, ref := range refs {
		if purgeFn != nil {
			if err := purgeFn(ctx, ref); err != nil {
				return fmt.Errorf("purge credential reference %s: %w", ref.ID, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, s.buildDeleteCredentialRefsByConnectionIDSQL(), id); err != nil {
		return fmt.Errorf("delete credential refs: %w", err)
	}

	// 先删除列级当前 schema，再删除表级当前 schema，避免孤儿记录。
	if _, err := tx.ExecContext(ctx, s.buildDeleteColumnSchemasByConnectionIDSQL(), id); err != nil {
		return fmt.Errorf("cascade delete column schemas: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.buildDeleteTableSchemasByConnectionIDSQL(), id); err != nil {
		return fmt.Errorf("cascade delete table schemas: %w", err)
	}
	res, err := tx.ExecContext(ctx, s.buildDeleteConnectionByIDSQL(), id)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete tx: %w", err)
	}
	return nil
}

// InsertDummyTableSchema 为测试插入关联的表快照记录。
//
// 输入：
// - ctx: 请求上下文。
// - id: 表快照记录 ID。
// - connectionID: 所属连接 ID。
// - tableName: 表名。
//
// 输出：
// - error: 插入失败错误。
func (s *ConnectionStore) InsertDummyTableSchema(ctx context.Context, id, connectionID, tableName string) error {
	_, err := s.db.ExecContext(ctx, s.buildInsertDummyTableSchemaSQL(), id, connectionID, tableName, time.Now().Unix())
	return err
}

// CountTableSchemasByConnection 统计指定连接的表快照数量（测试辅助方法）。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
//
// 输出：
// - int: 表快照数量。
// - error: 统计失败错误。
func (s *ConnectionStore) CountTableSchemasByConnection(ctx context.Context, connectionID string) (int, error) {
	row := s.db.QueryRowContext(ctx, s.buildCountTableSchemasByConnectionSQL(), connectionID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// InsertCredentialReference 为测试插入连接凭据引用记录。
//
// 输入：
// - ctx: 请求上下文。
// - ref: 待插入凭据引用记录。
//
// 输出：
// - error: 插入失败错误。
func (s *ConnectionStore) InsertCredentialReference(ctx context.Context, ref CredentialReference) error {
	_, err := s.db.ExecContext(
		ctx,
		s.buildInsertCredentialReferenceSQL(),
		ref.ID,
		ref.ConnectionID,
		ref.Provider,
		ref.CredentialRef,
		time.Now().Unix(),
		time.Now().Unix(),
	)
	return err
}

// CountCredentialReferencesByConnection 统计指定连接凭据引用数量（测试辅助方法）。
//
// 输入：
// - ctx: 请求上下文。
// - connectionID: 连接 ID。
//
// 输出：
// - int: 凭据引用数量。
// - error: 统计失败错误。
func (s *ConnectionStore) CountCredentialReferencesByConnection(ctx context.Context, connectionID string) (int, error) {
	row := s.db.QueryRowContext(ctx, s.buildCountCredentialRefsByConnectionSQL(), connectionID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// placeholder 返回指定参数位置的 SQL 占位符。
//
// 输入：
// - pos: 参数序号（从 1 开始）。
//
// 输出：
// - string: 针对当前 backend 的占位符文本。
func (s *ConnectionStore) placeholder(pos int) string {
	if s.backend == backendPostgres {
		return fmt.Sprintf("$%d", pos)
	}
	return "?"
}

// buildGetConnectionByIDSQL 构建按 ID 查询连接记录的 SQL。
func (s *ConnectionStore) buildGetConnectionByIDSQL() string {
	return fmt.Sprintf(`
		SELECT id, name, db_type, host, port, username, password, database, extra, created_at, updated_at
		FROM ldb_connections WHERE id = %s
	`, s.placeholder(1))
}

// buildLoadCurrentSchemaTablesSQL 构建按连接加载表级当前 schema 的 SQL。
func (s *ConnectionStore) buildLoadCurrentSchemaTablesSQL() string {
	return fmt.Sprintf(`
		SELECT id, connection_id, database_name, schema_name, table_name, table_comment, scan_version, scanned_at
		FROM ldb_table_schemas
		WHERE connection_id = %s
		ORDER BY LOWER(database_name), LOWER(COALESCE(schema_name, '')), LOWER(table_name), id
	`, s.placeholder(1))
}

// buildLoadCurrentSchemaColumnsSQL 构建按连接加载列级当前 schema 的 SQL。
func (s *ConnectionStore) buildLoadCurrentSchemaColumnsSQL() string {
	return fmt.Sprintf(`
		SELECT
			c.id,
			c.table_schema_id,
			c.column_name,
			c.ordinal_pos,
			c.data_type,
			c.abstract_type,
			c.is_primary_key,
			c.is_nullable,
			c.is_unique,
			c.is_auto_increment,
			c.default_value,
			c.column_comment,
			c.fk_ref_table,
			c.fk_ref_column,
			c.extra
		FROM ldb_column_schemas c
		INNER JOIN ldb_table_schemas t ON t.id = c.table_schema_id
		WHERE t.connection_id = %s
		ORDER BY LOWER(t.database_name), LOWER(COALESCE(t.schema_name, '')), LOWER(t.table_name), c.ordinal_pos, LOWER(c.column_name), c.id
	`, s.placeholder(1))
}

// buildDeleteTableSchemasByConnectionIDSQL 构建按 connection_id 删除表快照记录的 SQL。
func (s *ConnectionStore) buildDeleteTableSchemasByConnectionIDSQL() string {
	return fmt.Sprintf(`DELETE FROM ldb_table_schemas WHERE connection_id = %s`, s.placeholder(1))
}

// buildDeleteColumnSchemasByConnectionIDSQL 构建按连接删除列 schema 行的 SQL（先于表行删除，满足外键顺序）。
func (s *ConnectionStore) buildDeleteColumnSchemasByConnectionIDSQL() string {
	return fmt.Sprintf(`
		DELETE FROM ldb_column_schemas WHERE table_schema_id IN (
			SELECT id FROM ldb_table_schemas WHERE connection_id = %s
		)`, s.placeholder(1))
}

// buildDeleteConnectionByIDSQL 构建按 ID 删除连接记录的 SQL。
func (s *ConnectionStore) buildDeleteConnectionByIDSQL() string {
	return fmt.Sprintf(`DELETE FROM ldb_connections WHERE id = %s`, s.placeholder(1))
}

// buildDeleteCredentialRefsByConnectionIDSQL 构建按 connection_id 删除凭据引用 SQL。
func (s *ConnectionStore) buildDeleteCredentialRefsByConnectionIDSQL() string {
	return fmt.Sprintf(`DELETE FROM ldb_connection_credentials WHERE connection_id = %s`, s.placeholder(1))
}

// buildInsertCurrentSchemaTableSQL 构建表级当前 schema 插入 SQL。
func (s *ConnectionStore) buildInsertCurrentSchemaTableSQL() string {
	return fmt.Sprintf(`
		INSERT INTO ldb_table_schemas (
			id, connection_id, database_name, schema_name, table_name, table_comment, scan_version, scanned_at
		) VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
	`, s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6), s.placeholder(7), s.placeholder(8))
}

// buildInsertCurrentSchemaColumnSQL 构建列级当前 schema 插入 SQL。
func (s *ConnectionStore) buildInsertCurrentSchemaColumnSQL() string {
	return fmt.Sprintf(`
		INSERT INTO ldb_column_schemas (
			id, table_schema_id, column_name, ordinal_pos, data_type, abstract_type,
			is_primary_key, is_nullable, is_unique, is_auto_increment,
			default_value, column_comment, fk_ref_table, fk_ref_column, extra
		) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
	`, s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6), s.placeholder(7), s.placeholder(8), s.placeholder(9), s.placeholder(10), s.placeholder(11), s.placeholder(12), s.placeholder(13), s.placeholder(14), s.placeholder(15))
}

// buildInsertDummyTableSchemaSQL 构建测试辅助表快照插入 SQL。
func (s *ConnectionStore) buildInsertDummyTableSchemaSQL() string {
	return fmt.Sprintf(`
		INSERT INTO ldb_table_schemas (id, connection_id, database_name, table_name, scan_version, scanned_at)
		VALUES (%s, %s, 'testdb', %s, 1, %s)
	`, s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4))
}

// buildCountTableSchemasByConnectionSQL 构建按 connection_id 统计表快照数量的 SQL。
func (s *ConnectionStore) buildCountTableSchemasByConnectionSQL() string {
	return fmt.Sprintf(`SELECT COUNT(1) FROM ldb_table_schemas WHERE connection_id = %s`, s.placeholder(1))
}

// buildInsertCredentialReferenceSQL 构建测试辅助凭据引用插入 SQL。
func (s *ConnectionStore) buildInsertCredentialReferenceSQL() string {
	return fmt.Sprintf(`
		INSERT INTO ldb_connection_credentials (id, connection_id, provider, credential_ref, created_at, updated_at)
		VALUES (%s, %s, %s, %s, %s, %s)
	`, s.placeholder(1), s.placeholder(2), s.placeholder(3), s.placeholder(4), s.placeholder(5), s.placeholder(6))
}

// buildCountCredentialRefsByConnectionSQL 构建按 connection_id 统计凭据引用数量 SQL。
func (s *ConnectionStore) buildCountCredentialRefsByConnectionSQL() string {
	return fmt.Sprintf(`SELECT COUNT(1) FROM ldb_connection_credentials WHERE connection_id = %s`, s.placeholder(1))
}

// migrate 初始化本批所需的 ldb_ 元数据表结构。
//
// 主要参数：
// - ctx: 请求上下文。
func (s *ConnectionStore) migrate(ctx context.Context) error {
	stmts := s.buildMigrationStatements()
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

// buildMigrationStatements 构建当前后端需要执行的 migration SQL 列表。
//
// 输出：
// - []string: 依序执行的 DDL 语句。
func (s *ConnectionStore) buildMigrationStatements() []string {
	types := s.migrationTypeSet()
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS ldb_connections (
			id %s PRIMARY KEY,
			name %s NOT NULL,
			db_type %s NOT NULL,
			host %s,
			port %s,
			username %s,
			password %s,
			database %s,
			extra %s,
			created_at %s,
			updated_at %s
		)`, types.IDType, types.NameType, types.DBType, types.HostType, types.PortType,
			types.UsernameType, types.PasswordType, types.DatabaseType, types.JSONType,
			types.TimestampType, types.TimestampType),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS ldb_table_schemas (
			id %s PRIMARY KEY,
			connection_id %s NOT NULL,
			database_name %s NOT NULL,
			schema_name %s,
			table_name %s NOT NULL,
			table_comment %s,
			scan_version %s NOT NULL DEFAULT 1,
			scanned_at %s NOT NULL,
			FOREIGN KEY (connection_id) REFERENCES ldb_connections(id)
		)`, types.IDType, types.IDType, types.NameType, types.NameType, types.NameType,
			types.CommentType, types.CounterType, types.TimestampType),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS ldb_column_schemas (
			id %s PRIMARY KEY,
			table_schema_id %s NOT NULL,
			column_name %s NOT NULL,
			ordinal_pos %s,
			data_type %s NOT NULL,
			abstract_type %s NOT NULL,
			is_primary_key %s,
			is_nullable %s,
			is_unique %s,
			is_auto_increment %s,
			default_value %s,
			column_comment %s,
			fk_ref_table %s,
			fk_ref_column %s,
			extra %s,
			FOREIGN KEY (table_schema_id) REFERENCES ldb_table_schemas(id)
		)`, types.IDType, types.IDType, types.NameType, types.CounterType, types.NameType, types.NameType,
			types.CounterType, types.CounterType, types.CounterType, types.CounterType,
			types.CommentType, types.CommentType, types.NameType, types.NameType, types.JSONType),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS ldb_connection_credentials (
			id %s PRIMARY KEY,
			connection_id %s NOT NULL,
			provider %s NOT NULL,
			credential_ref %s NOT NULL,
			created_at %s NOT NULL,
			updated_at %s NOT NULL,
			FOREIGN KEY (connection_id) REFERENCES ldb_connections(id)
		)`, types.IDType, types.IDType, types.NameType, types.NameType, types.TimestampType, types.TimestampType),
	}
}

// listCredentialReferencesTx 在事务内读取指定连接的全部凭据引用。
func (s *ConnectionStore) listCredentialReferencesTx(ctx context.Context, tx *sql.Tx, connectionID string) ([]CredentialReference, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, connection_id, provider, credential_ref
		FROM ldb_connection_credentials
		WHERE connection_id = %s
		ORDER BY id
	`, s.placeholder(1)), connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refs := make([]CredentialReference, 0)
	for rows.Next() {
		var ref CredentialReference
		if err := rows.Scan(&ref.ID, &ref.ConnectionID, &ref.Provider, &ref.CredentialRef); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

// migrationTypes 描述 migration 阶段按后端映射的列类型集合。
type migrationTypes struct {
	// IDType 用于主键及外键 ID 列类型。
	IDType string

	// NameType 用于名称类字符串字段类型。
	NameType string

	// DBType 用于数据库类型枚举字段。
	DBType string

	// HostType 用于主机地址字段。
	HostType string

	// PortType 用于端口字段。
	PortType string

	// UsernameType 用于用户名字段。
	UsernameType string

	// PasswordType 用于密文密码字段。
	PasswordType string

	// DatabaseType 用于数据库名称字段。
	DatabaseType string

	// JSONType 用于 JSON 扩展字段。
	JSONType string

	// TimestampType 用于 Unix 秒时间戳字段。
	TimestampType string

	// CounterType 用于扫描版本等计数字段。
	CounterType string

	// CommentType 用于注释文本字段。
	CommentType string
}

// migrationTypeSet 返回当前后端推荐的 migration 列类型组合。
//
// 输出：
// - migrationTypes: 按后端映射后的字段类型集合。
func (s *ConnectionStore) migrationTypeSet() migrationTypes {
	switch s.backend {
	case backendMySQL:
		return migrationTypes{
			IDType:        "VARCHAR(64)",
			NameType:      "VARCHAR(255)",
			DBType:        "VARCHAR(32)",
			HostType:      "VARCHAR(255)",
			PortType:      "INT",
			UsernameType:  "VARCHAR(255)",
			PasswordType:  "TEXT",
			DatabaseType:  "VARCHAR(255)",
			JSONType:      "JSON",
			TimestampType: "BIGINT",
			CounterType:   "INT",
			CommentType:   "TEXT",
		}
	case backendPostgres:
		return migrationTypes{
			IDType:        "VARCHAR(64)",
			NameType:      "VARCHAR(255)",
			DBType:        "VARCHAR(32)",
			HostType:      "VARCHAR(255)",
			PortType:      "INTEGER",
			UsernameType:  "VARCHAR(255)",
			PasswordType:  "TEXT",
			DatabaseType:  "VARCHAR(255)",
			JSONType:      "JSONB",
			TimestampType: "BIGINT",
			CounterType:   "INTEGER",
			CommentType:   "TEXT",
		}
	default:
		return migrationTypes{
			IDType:        "TEXT",
			NameType:      "TEXT",
			DBType:        "TEXT",
			HostType:      "TEXT",
			PortType:      "INTEGER",
			UsernameType:  "TEXT",
			PasswordType:  "TEXT",
			DatabaseType:  "TEXT",
			JSONType:      "TEXT",
			TimestampType: "INTEGER",
			CounterType:   "INTEGER",
			CommentType:   "TEXT",
		}
	}
}

// nilIfEmpty 将空字符串转换为 nil，便于写入可空字段。
func nilIfEmpty(v string) interface{} {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

// boolToInt 将布尔值映射为 0/1，兼容多后端整数字段持久化。
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
