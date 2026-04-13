# Technology Stack

## Architecture

**桌面端跨平台架构**：Flutter (前端 UI) + Go (后端核心引擎)，通过 FFI 通信。Go 编译为动态库（`.so`/`.dll`），Flutter 通过 `dart:ffi` 调用。

**设计理念**：
- 前端负责 UI 交互和状态管理
- 后端负责数据库连接、Schema 解析、数据生成和批量写入
- 通过 JSON 序列化传递数据（牺牲少量性能换取开发便利）

## Core Technologies

- **前端**: Flutter (Dart) - 跨平台桌面 UI，支持 Windows/macOS/Linux
- **后端**: Go - 数据库连接、Schema 解析、数据生成引擎
- **通信**: CGo (`-buildmode=c-shared`) + Dart FFI
- **数据序列化**: JSON (`json-iterator` for Go)
- **数据库驱动**: Go `database/sql` 标准包
- **Python 表达式（后续阶段）**: `github.com/go-python/gpython` (嵌入式，无需外部运行时，MVP 不启用)
- **配置存储**: SQLite (默认) 或用户指定数据库

## Key Libraries

**Go 后端**:
- `database/sql` - 数据库连接标准接口
- `json-iterator` - 高性能 JSON 序列化
- `github.com/go-python/gpython` - 嵌入式 Python 解释器（后续阶段用于 Python 计算字段）

**Flutter 前端**:
- `dart:ffi` - 调用 Go 动态库
- 状态管理方案: Riverpod

## Development Standards

### 后端 (Go)

**代码规范**:
- 遵循 `gofmt` 标准格式
- 包命名全小写
- FFI 导出函数统一前缀 `LDB_`（如 `LDB_Connect`、`LDB_ScanSchema`）

**架构模式**:
- 每种数据库适配器放在独立子包，实现统一 `Connector` 接口
- 每种生成器放在独立文件，实现统一 `Generator` 接口
- 使用 `goroutine` 提高并发性能

**接口设计**:
```go
// Connector 接口
type Connector interface {
    Connect(params) error
    ScanSchema(db, schema) (Schema, error)
    InsertBatch(table, rows) error
}

// Generator 接口
// 完整定义见 ./generator.md
type Generator interface {
    Generate(ctx context.Context) (interface{}, error)
    GenerateBatch(ctx context.Context, count int) ([]interface{}, error)
    Reset() error  // 用于有状态生成器（序列、自增），无状态生成器上为 no-op、不报错
    Type() GeneratorType
}
```

### 前端 (Flutter)

**代码组织**:
- FFI 绑定代码与业务逻辑分离，放在 `lib/ffi/` 目录
- 所有耗时操作必须异步执行（使用 `Isolate`），禁止阻塞主线程
- FFI 调用在独立 `Isolate` 中执行

**内存管理**:
- Go 端分配的字符串需导出对应的 `Free` 函数
- Dart 端调用后负责释放内存

### 类型系统

**数据类型抽象**：所有数据库字段类型统一映射为 5 种抽象类型
- `int` - INT, BIGINT, SMALLINT
- `string` - VARCHAR, TEXT, CHAR
- `decimal` - FLOAT, DOUBLE, NUMERIC
- `datetime` - DATE, TIMESTAMP, DATETIME
- `boolean` - BOOL, TINYINT(1)

## Development Environment

### Required Tools
- Go 1.26+
- Flutter 3.x+
- Dart SDK
- CGo 工具链（用于编译动态库）

### Common Commands
```bash
# Go 后端编译为动态库
go build -buildmode=c-shared -o libldb.so ./backend/cmd

# Flutter 前端运行
flutter run -d windows  # 或 macos, linux

# Go 测试
go test ./...
```

## Key Technical Decisions

**为什么选择 Flutter + Go FFI？**
- Flutter 提供跨平台桌面 UI，一套代码支持三大平台
- Go 提供高性能数据库操作和并发能力
- FFI 通信避免网络开销，适合桌面应用

**为什么使用 JSON 序列化而非原生 Struct？**
- 简化 Go Struct 与 Dart 类型映射
- 开发效率优先，性能损失可接受（非热路径）

**为什么规划嵌入 gpython 而非调用外部 Python？**
- 避免依赖用户本机 Python 环境
- 仅需支持标准库函数，嵌入式方案足够
- 当前 MVP 只支持 SQL 计算表达式，Python 能力因复杂度控制而延后

**配置持久化策略**
- 默认 SQLite：零配置，开箱即用
- 可选外部数据库：企业用户可集中管理配置
- **表命名规范**：所有配置存储表必须使用 `ldb_` 前缀（如 `ldb_connections`、`ldb_table_schemas`、`ldb_column_gen_configs`），与用户业务数据隔离

**数据库支持优先级**
- Phase 1: MySQL, PostgreSQL, SQLite
- Phase 2: Oracle, MSSQL, ClickHouse
- Phase 3: Hive, 云数据库 (AWS/GCP/Alibaba)

---
_Document standards and patterns, not every dependency_
