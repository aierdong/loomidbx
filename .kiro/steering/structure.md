# Project Structure

## Organization Philosophy

**前后端分离 + 接口驱动**：前端 (Flutter) 和后端 (Go) 各自独立开发，通过 FFI 接口通信。后端按功能模块划分（连接器、生成器、写入器），前端按界面层级组织（screens、widgets、providers）。

**关注点分离**：
- 后端专注数据处理逻辑，不涉及 UI
- 前端专注交互体验，不涉及数据库细节
- FFI 层作为清晰的边界

## Directory Patterns

### 后端 Go 代码 (`/backend/`)

**Location**: `/backend/`
**Purpose**: 数据库连接、Schema 解析、数据生成引擎、FFI 导出接口
**Example**:
```
backend/
├── cmd/              # 入口，编译为动态库
├── connector/        # 数据库适配器（MySQL, Postgres, Oracle...）
├── schema/           # Schema 扫描与抽象
├── generator/        # 数据生成器（按类型分类）
│   ├── int/          # 整数生成器（序列、区间、分布）
│   ├── string/       # 字符串生成器（正则、枚举、外部源、AI）
│   ├── decimal/      # 小数生成器
│   ├── datetime/     # 日期时间生成器
│   └── boolean/      # 布尔生成器
├── writer/           # 数据回写（批量插入）
├── ffi/              # CGo 导出接口定义
└── storage/          # 配置持久化（连接、规则）
```

**组织原则**：
- 每种数据库适配器放在 `connector/` 下独立子包，实现统一 `Connector` 接口
- 每种生成器放在对应类型目录下独立文件，实现统一 `Generator` 接口
- FFI 导出函数集中在 `ffi/` 包，统一前缀 `LDB_`

### 前端 Flutter 代码 (`/frontend/`)

**Location**: `/frontend/`
**Purpose**: 跨平台桌面 UI，连接管理、Schema 展示、生成器配置、数据生成向导
**Example**:
```
frontend/
├── lib/
│   ├── ffi/          # Dart FFI 绑定（调用 Go 动态库）
│   ├── models/       # 对应后端 JSON 结构的 Dart 模型
│   ├── screens/      # 主界面、连接管理、生成向导等
│   ├── widgets/      # 可复用 UI 组件（树形面板、字段配置等）
│   └── providers/    # 状态管理
└── pubspec.yaml
```

**组织原则**：
- `ffi/` 层与业务逻辑分离，仅负责 FFI 调用和内存管理
- `models/` 与后端 JSON 结构一一对应
- `screens/` 按功能模块划分（连接管理、表详情、生成向导）
- `widgets/` 存放可复用组件（树形面板、字段配置面板）

### 配置与文档

**Location**: 根目录
**Purpose**: 项目文档、AI 上下文、开发规范
**Example**:
- `ai-agent-context.md` - AI 编程助手上下文
- `product-outline.md` - 产品定位和功能规划
- `CLAUDE.md` - 开发规范和工作流

## Naming Conventions

### Go 后端
- **包名**: 全小写，单数形式（`connector`, `generator`, `schema`）
- **文件名**: 小写 + 下划线（`mysql_connector.go`, `sequence_generator.go`）
- **导出函数**: PascalCase（`Connect`, `ScanSchema`）
- **FFI 导出**: `LDB_` 前缀 + PascalCase（`LDB_Connect`, `LDB_ScanSchema`）
- **接口**: 以 `-er` 结尾（`Connector`, `Generator`）

### Flutter 前端
- **文件名**: 小写 + 下划线（`connection_tree.dart`, `field_config_panel.dart`）
- **类名**: PascalCase（`ConnectionTree`, `FieldConfigPanel`）
- **函数/变量**: camelCase（`scanSchema`, `generatorConfig`）
- **私有成员**: `_` 前缀（`_buildTree`, `_config`）

## Import Organization

### Go
```go
// 标准库
import (
    "context"
    "database/sql"
)

// 第三方库
import (
    jsoniter "github.com/json-iterator/go"
    "github.com/go-python/gpython"
)

// 项目内部
import (
    "loomidbx/connector"
    "loomidbx/schema"
)
```

### Dart
```dart
// Flutter SDK
import 'package:flutter/material.dart';

// 第三方包
import 'package:provider/provider.dart';

// 项目内部（相对路径）
import '../ffi/bindings.dart';
import '../models/schema.dart';
```

## Code Organization Principles

**后端依赖规则**：
- `connector` 依赖 `schema`（返回 Schema 结构）
- `generator` 依赖 `schema`（根据字段类型生成）
- `writer` 依赖 `connector` 和 `generator`
- `ffi` 依赖所有模块（作为统一入口）
- `storage` 独立（仅负责配置持久化）

**前端依赖规则**：
- `ffi` 层不依赖业务逻辑
- `models` 仅定义数据结构，不包含逻辑
- `screens` 依赖 `widgets` 和 `providers`
- `providers` 依赖 `ffi` 和 `models`

**接口优先**：
- 后端核心模块（Connector, Generator）均定义接口
- 新增数据库/生成器只需实现接口，无需修改核心逻辑

**并发安全**：
- Go 后端使用 `goroutine` 并发生成数据
- 写入顺序须满足外键依赖拓扑排序
- 每批数据作为一个事务提交

**错误处理**：
- 后端错误通过 JSON 返回给前端（`{"error": "..."}`）
- 写入失败立即停止，回滚当前批次，不重试

---
_Document patterns, not file trees. New files following patterns shouldn't require updates_
