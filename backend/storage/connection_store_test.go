package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"loomidbx/backend/schema"
)

// TestBuildConnectionUpsertSQLByBackend 验证不同存储后端生成不同 upsert 方言。
func TestBuildConnectionUpsertSQLByBackend(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		containsAll []string
	}{
		{
			name:    "sqlite upsert",
			backend: backendSQLite,
			containsAll: []string{
				"ON CONFLICT(id) DO UPDATE SET",
				"excluded.name",
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			},
		},
		{
			name:    "mysql upsert",
			backend: backendMySQL,
			containsAll: []string{
				"ON DUPLICATE KEY UPDATE",
				"VALUES(name)",
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			},
		},
		{
			name:    "postgres upsert",
			backend: backendPostgres,
			containsAll: []string{
				"ON CONFLICT(id) DO UPDATE SET",
				"EXCLUDED.name",
				"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &ConnectionStore{backend: tt.backend}
			sql := store.buildConnectionUpsertSQL()
			for _, fragment := range tt.containsAll {
				if !strings.Contains(sql, fragment) {
					t.Fatalf("sql missing fragment %q, got: %s", fragment, sql)
				}
			}
		})
	}
}

// TestDetectStorageBackend 验证环境变量到后端标识的映射规则。
func TestDetectStorageBackend(t *testing.T) {
	t.Setenv(storageBackendEnv, "mysql")
	if got := detectStorageBackend(); got != backendMySQL {
		t.Fatalf("want %s got %s", backendMySQL, got)
	}

	t.Setenv(storageBackendEnv, "postgres")
	if got := detectStorageBackend(); got != backendPostgres {
		t.Fatalf("want %s got %s", backendPostgres, got)
	}

	t.Setenv(storageBackendEnv, "unknown")
	if got := detectStorageBackend(); got != backendSQLite {
		t.Fatalf("want default %s got %s", backendSQLite, got)
	}
}

// TestPlaceholderByBackend 验证不同后端的占位符规则。
func TestPlaceholderByBackend(t *testing.T) {
	pg := &ConnectionStore{backend: backendPostgres}
	if got := pg.placeholder(3); got != "$3" {
		t.Fatalf("postgres placeholder mismatch: %s", got)
	}

	sqlite := &ConnectionStore{backend: backendSQLite}
	if got := sqlite.placeholder(3); got != "?" {
		t.Fatalf("sqlite placeholder mismatch: %s", got)
	}
}

// TestQueryBuildersUseBackendPlaceholders 验证查询构建器使用正确方言占位符。
func TestQueryBuildersUseBackendPlaceholders(t *testing.T) {
	tests := []struct {
		name      string
		backend   string
		fragments []string
	}{
		{
			name:    "postgres placeholders",
			backend: backendPostgres,
			fragments: []string{
				"WHERE id = $1",
				"WHERE connection_id = $1",
				"VALUES ($1, $2, 'testdb', $3, 1, $4)",
			},
		},
		{
			name:    "sqlite placeholders",
			backend: backendSQLite,
			fragments: []string{
				"WHERE id = ?",
				"WHERE connection_id = ?",
				"VALUES (?, ?, 'testdb', ?, 1, ?)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &ConnectionStore{backend: tt.backend}
			sqls := []string{
				store.buildGetConnectionByIDSQL(),
				store.buildDeleteColumnSchemasByConnectionIDSQL(),
				store.buildDeleteTableSchemasByConnectionIDSQL(),
				store.buildDeleteConnectionByIDSQL(),
				store.buildInsertDummyTableSchemaSQL(),
				store.buildCountTableSchemasByConnectionSQL(),
			}
			joined := strings.Join(sqls, "\n")
			for _, fragment := range tt.fragments {
				if !strings.Contains(joined, fragment) {
					t.Fatalf("missing fragment %q in generated SQL:\n%s", fragment, joined)
				}
			}
		})
	}
}

// TestMigrationStatementsUseBackendTypes 验证 migration 中字段类型按后端映射。
func TestMigrationStatementsUseBackendTypes(t *testing.T) {
	tests := []struct {
		name      string
		backend   string
		fragments []string
	}{
		{
			name:    "sqlite migration types",
			backend: backendSQLite,
			fragments: []string{
				"id TEXT PRIMARY KEY",
				"extra TEXT",
				"created_at INTEGER",
				"scan_version INTEGER NOT NULL DEFAULT 1",
				"ldb_column_schemas",
				"abstract_type",
			},
		},
		{
			name:    "mysql migration types",
			backend: backendMySQL,
			fragments: []string{
				"id VARCHAR(64) PRIMARY KEY",
				"db_type VARCHAR(32) NOT NULL",
				"extra JSON",
				"created_at BIGINT",
				"scan_version INT NOT NULL DEFAULT 1",
				"ldb_column_schemas",
				"abstract_type",
			},
		},
		{
			name:    "postgres migration types",
			backend: backendPostgres,
			fragments: []string{
				"id VARCHAR(64) PRIMARY KEY",
				"db_type VARCHAR(32) NOT NULL",
				"extra JSONB",
				"created_at BIGINT",
				"scan_version INTEGER NOT NULL DEFAULT 1",
				"ldb_column_schemas",
				"abstract_type",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &ConnectionStore{backend: tt.backend}
			stmts := store.buildMigrationStatements()
			if len(stmts) == 0 {
				t.Fatal("migration statements should not be empty")
			}
			joined := strings.Join(stmts, "\n")
			for _, fragment := range tt.fragments {
				if !strings.Contains(joined, fragment) {
					t.Fatalf("expected migration sql contains %q, got: %s", fragment, joined)
				}
			}
		})
	}
}

// TestPatchConnectionSchemaExtra_preservesExtraKeys 验证合并 schema 元数据不破坏 extra 中其它键。
func TestPatchConnectionSchemaExtra_preservesExtraKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "meta-patch.db")
	store, err := NewConnectionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	id := "c-patch-1"
	if err := store.UpsertConnection(ctx, ConnectionRecord{
		ID: id, Name: "n", DBType: "sqlite", Extra: `{"charset":"utf8"}`,
	}); err != nil {
		t.Fatal(err)
	}
	ts := schema.SchemaTrustPendingAdjustment
	reason := "BLOCKING_RISK_UNRESOLVED"
	if err := store.PatchConnectionSchemaExtra(ctx, id, schema.ConnectionSchemaMetaPatch{
		TrustState: &ts, LastBlockingReason: &reason,
	}); err != nil {
		t.Fatal(err)
	}
	rec, err := store.GetConnectionByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := schema.ParseConnectionSchemaMeta(rec.Extra)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaTrustState != ts {
		t.Fatalf("trust: got %q", meta.SchemaTrustState)
	}
	if meta.SchemaLastBlockingReason != reason {
		t.Fatalf("reason: got %q", meta.SchemaLastBlockingReason)
	}
	if !strings.Contains(rec.Extra, "charset") {
		t.Fatalf("lost unrelated keys: %s", rec.Extra)
	}
}

// TestDeleteConnectionCascade_dropsColumnSchemas 验证删除连接时先清理列级当前 schema。
func TestDeleteConnectionCascade_dropsColumnSchemas(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "meta-cascade.db")
	store, err := NewConnectionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cid := "conn-cascade"
	if err := store.UpsertConnection(ctx, ConnectionRecord{ID: cid, Name: "n", DBType: "sqlite"}); err != nil {
		t.Fatal(err)
	}
	tid := "tbl1"
	if err := store.InsertDummyTableSchema(ctx, tid, cid, "t1"); err != nil {
		t.Fatal(err)
	}
	_, err = store.db.ExecContext(ctx, `
		INSERT INTO ldb_column_schemas (id, table_schema_id, column_name, ordinal_pos, data_type, abstract_type,
			is_primary_key, is_nullable, is_unique, is_auto_increment, extra)
		VALUES ('col1', ?, 'c1', 1, 'INT', 'int', 1, 0, 0, 0, '{}')
	`, tid)
	if err != nil {
		t.Fatal(err)
	}
	var colCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM ldb_column_schemas`).Scan(&colCount); err != nil {
		t.Fatal(err)
	}
	if colCount != 1 {
		t.Fatalf("colCount %d", colCount)
	}
	if err := store.DeleteConnectionCascade(ctx, cid, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM ldb_column_schemas`).Scan(&colCount); err != nil {
		t.Fatal(err)
	}
	if colCount != 0 {
		t.Fatalf("want 0 column rows, got %d", colCount)
	}
}
