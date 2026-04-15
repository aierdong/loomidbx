package schema

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteSchemaInspector_Inspect_All_DeterministicOrderAndConstraints(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		PRAGMA foreign_keys = ON;

		CREATE TABLE orgs (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);

		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			org_id INTEGER NOT NULL,
			email TEXT NOT NULL UNIQUE,
			name TEXT,
			FOREIGN KEY (org_id) REFERENCES orgs(id)
		);

		CREATE TABLE posts (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			slug TEXT NOT NULL,
			title TEXT,
			UNIQUE(user_id, slug),
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
	`)
	if err != nil {
		t.Fatalf("setup schema: %v", err)
	}

	ins := NewSQLiteSchemaInspector()
	graph, upErr := ins.Inspect(context.Background(), db, InspectScope{Scope: SchemaScanScopeAll})
	if upErr != nil {
		t.Fatalf("inspect error: code=%s msg=%s", upErr.Code, upErr.Message)
	}
	if graph == nil {
		t.Fatalf("graph is nil")
	}

	if len(graph.Tables) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(graph.Tables))
	}

	// 表顺序必须确定性：按表名升序
	if graph.Tables[0].TableName != "orgs" || graph.Tables[1].TableName != "posts" || graph.Tables[2].TableName != "users" {
		t.Fatalf("unexpected table order: %v, %v, %v", graph.Tables[0].TableName, graph.Tables[1].TableName, graph.Tables[2].TableName)
	}

	// 校验 users：列顺序、PK、UNIQUE(email)、FK(org_id -> orgs.id)
	var users TableDef
	for _, tbl := range graph.Tables {
		if tbl.TableName == "users" {
			users = tbl
			break
		}
	}
	if users.TableName == "" {
		t.Fatalf("users table not found")
	}
	if len(users.Columns) < 4 {
		t.Fatalf("expected >=4 user columns, got %d", len(users.Columns))
	}
	if users.Columns[0].Name != "id" || users.Columns[1].Name != "org_id" {
		t.Fatalf("unexpected users column order start: %s, %s", users.Columns[0].Name, users.Columns[1].Name)
	}
	if len(users.PrimaryKey) != 1 || users.PrimaryKey[0] != "id" {
		t.Fatalf("unexpected users pk: %#v", users.PrimaryKey)
	}

	// unique 约束里应包含 email
	foundEmailUnique := false
	for _, u := range users.UniqueConstraints {
		if len(u.Columns) == 1 && u.Columns[0] == "email" {
			foundEmailUnique = true
			break
		}
	}
	if !foundEmailUnique {
		t.Fatalf("expected unique(email) in users.UniqueConstraints, got: %#v", users.UniqueConstraints)
	}

	// fk：org_id -> orgs.id
	foundOrgFK := false
	for _, fk := range users.ForeignKeys {
		if fk.RefTable == "orgs" && len(fk.Columns) == 1 && fk.Columns[0] == "org_id" && len(fk.RefColumns) == 1 && fk.RefColumns[0] == "id" {
			foundOrgFK = true
			break
		}
	}
	if !foundOrgFK {
		t.Fatalf("expected fk org_id->orgs.id, got: %#v", users.ForeignKeys)
	}

	// 校验 posts：UNIQUE(user_id, slug) 必须按列序稳定
	var posts TableDef
	for _, tbl := range graph.Tables {
		if tbl.TableName == "posts" {
			posts = tbl
			break
		}
	}
	foundCompositeUnique := false
	for _, u := range posts.UniqueConstraints {
		if len(u.Columns) == 2 && u.Columns[0] == "user_id" && u.Columns[1] == "slug" {
			foundCompositeUnique = true
			break
		}
	}
	if !foundCompositeUnique {
		t.Fatalf("expected unique(user_id,slug) in posts.UniqueConstraints, got: %#v", posts.UniqueConstraints)
	}
}

func TestClassifyUpstreamError_SanitizesPasswordPattern(t *testing.T) {
	err := errorsNew("dial tcp password=supersecret&user=alice: connection refused")
	ce := ClassifyUpstreamError(context.Background(), err)
	if ce == nil {
		t.Fatalf("expected classified error")
	}
	if ce.Code == "" {
		t.Fatalf("expected code")
	}
	if containsRawSecret(ce.Message, "supersecret") {
		t.Fatalf("expected secret sanitized, got: %s", ce.Message)
	}
}

func errorsNew(s string) error { return &stringError{s: s} }

type stringError struct{ s string }

func (e *stringError) Error() string { return e.s }

func containsRawSecret(msg, secret string) bool {
	return secret != "" && msg != "" && (msg == secret || (len(msg) >= len(secret) && contains(msg, secret)))
}

func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

