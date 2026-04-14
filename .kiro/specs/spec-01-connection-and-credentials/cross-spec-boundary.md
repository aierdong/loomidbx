# Spec 间衔接假验与对接说明

本文档固定 spec-01（连接与凭据管理）与下游 spec（spec-02、spec-06、spec-07）的边界契约，供后续实现对照。

---

## 1. 与 spec-02（Schema 扫描）衔接假验

### 1.1 边界声明

**spec-01 不实现的职责**：
- `ScanSchema`、Diff、表结构快照持久化（属 spec-02）
- 任何 Schema 元数据扫描或写入调用

**spec-02 需依赖的接口**：
- 从 spec-01 获得「已验证有效的数据库连接句柄」
- 连接参数解析结果（含凭据注入）

### 1.2 测试桩验证

**假验目标**：在仅导入连接子系统模块的前提下，验证本阶段不触发任何 Schema 扫描或快照写入调用。

```go
// 测试用例示意（在 spec-02 实现时对照）
func TestConnectionModule_NoSchemaScan(t *testing.T) {
    // 导入 spec-01 的连接模块
    svc := app.NewConnectionService(store)
    
    // 执行连接测试
    err := svc.TestConnection(ctx, req)
    
    // 验收：不调用任何 ldb_table_schemas 写入
    count := store.CountTableSchemasByConnection(ctx, connID)
    assert.Equal(t, 0, count, "TestConnection should not write any table schemas")
    
    // 验收：不调用任何 ldb_column_schemas 写入
    // 验收：不调用任何 ldb_scan_history 写入
}
```

### 1.3 联调检查清单

| 检查项 | spec-01 状态 | spec-02 需验证 |
|--------|-------------|---------------|
| 连接测试不触发扫描 | ✓ 已验证 | 继承验证 |
| TestConnection 仅执行 Ping | ✓ 已实现 | 扫描入口需另行调用 |
| 连接 ID 可作为外键引用 | ✓ 已实现 | 扫描结果需关联连接 ID |
| 凭据解析后可注入 DSN | ✓ 已实现 | 扫描时复用凭据解析 |

---

## 2. 与 spec-06（FFI 契约细化）衔接假验

### 2.1 错误码映射固定

spec-01 已定义的稳定错误码（供 spec-06 双向对齐）：

| 错误码 | 触发场景 | HTTP 映射建议 |
|--------|---------|--------------|
| INVALID_ARGUMENT | 参数缺失、格式非法、环境变量缺失 | 400 Bad Request |
| STORAGE_ERROR | 元数据读写失败 | 500 Internal Error |
| NOT_FOUND | 连接不存在 | 404 Not Found |
| CONFIRMATION_REQUIRED | 删除未携带确认标志 | 409 Conflict |
| DEADLINE_EXCEEDED | 连接测试超时 | 504 Gateway Timeout |
| UPSTREAM_UNAVAILABLE | 目标数据库不可达 | 503 Service Unavailable |
| AUTH_FAILED | 认证失败 | 401 Unauthorized |
| TLS_ERROR | TLS/SSL 错误 | 502 Bad Gateway |
| PROTOCOL_ERROR | 数据库协议错误 | 400 Bad Request |
| KEYRING_UNAVAILABLE | 密钥环不可用 | 503 Service Unavailable |
| KEYRING_ACCESS_DENIED | 密钥环拒绝访问 | 403 Forbidden |

### 2.2 样例载荷夹具

**成功响应模板**：
```json
{
  "ok": true,
  "data": { "id": "uuid-string" },
  "error": null
}
```

**错误响应模板**：
```json
{
  "ok": false,
  "data": null,
  "error": {
    "code": "DEADLINE_EXCEEDED",
    "message": "connection test timeout",
    "details": { "timeout_sec": "20" }
  }
}
```

### 2.3 契约演进回归钩子

- FFI golden 测试已冻结响应形状（见 `ffi/json_adapter_golden_test.go`）
- 错误码快照已固定（见 `TestGolden_ErrorCodesSnapshot`）
- spec-06 需继承上述测试，并在新增 FFI 接口时补充 golden

---

## 3. 与 spec-07（UI 主流程）衔接说明

### 3.1 UI 可调用的异步入口

**当前 FFI 入口（均为同步阻塞）**：

| 入口 | 阻塞时间预期 | UI 建议 |
|------|-------------|---------|
| TestConnection | 0.5s ~ timeout_sec（默认 20s） | 在 Isolate 中执行，超时可取消 |
| SaveConnection | < 0.1s | 可在主线程执行 |
| ListConnections | < 0.1s | 可在主线程执行 |
| DeleteConnection | < 0.1s | 需先 UI 确认弹窗 |

### 3.2 接口级对接说明

**Flutter 调用模式**：
```dart
// 建议封装为异步调用
Future<TestResult> testConnection(ConnectionRequest req) async {
  return await compute(_ffiTestConnection, req.toJson());
}

// 或使用 Isolate.run
Future<TestResult> testConnection(ConnectionRequest req) async {
  return Isolate.run(() => _ffiTestConnection(req.toJson()));
}
```

**超时处理建议**：
- UI 层设置独立超时（如 30s），与后端 `timeout_sec` 配合
- 后端超时返回 `DEADLINE_EXCEEDED`，UI 可据此显示友好提示
- 用户可配置连接的 `timeout_sec` 以适应不同网络环境

**删除确认流程**：
```
UI 流程：
1. 用户点击删除按钮
2. UI 弹出确认对话框（警示用户）
3. 用户确认后，调用 DeleteConnection(confirm_cascade=true)
4. 若返回 CONFIRMATION_REQUIRED，提示用户再次确认
5. 成功后刷新连接列表
```

### 3.3 前端排期建议

| 功能 | spec-01 状态 | 前端所需接口 |
|------|-------------|-------------|
| 连接表单 | 已就绪 | SaveConnection |
| 连接测试按钮 | 已就绪 | TestConnection（需 Isolate） |
| 连接列表 | 已就绪 | ListConnections |
| 删除连接 | 已就绪 | DeleteConnection（需确认弹窗） |
| 环境变量注入 | 已就绪 | 前端无需特殊处理，后端自动解析 |

---

## 4. 边界假验测试代码

以下测试代码固定边界行为，供下游 spec 实现时对照验证。

```go
// backend/app/connection_boundary_test.go

// 验证：连接模块不触发扫描
func TestBoundary_NoSchemaScanOnConnect(t *testing.T) {
    svc, store := newService(t)
    
    // 执行连接测试
    _ = svc.TestConnection(ctx, app.ConnectionRequest{
        DBType: "sqlite",
        Database: ":memory:",
    })
    
    // 验收：不写入任何表快照
    count, _ := store.CountTableSchemasByConnection(ctx, "any-id")
    assert.Equal(t, 0, count)
}

// 验证：连接模块不触发扫描历史
func TestBoundary_NoScanHistoryOnConnect(t *testing.T) {
    // 验收：ldb_scan_history 无新增记录
}
```

---

## 5. 附录：跨 spec 依赖图

```
spec-01 (连接与凭据)
    │
    ├──► spec-02 (Schema 扫描)
    │       依赖：连接句柄、凭据解析结果
    │
    ├──► spec-06 (FFI 契约细化)
    │       依赖：错误码定义、响应模板
    │
    └──► spec-07 (UI 主流程)
            依赖：所有 FFI 入口、超时/确认语义
```

---

*文档版本：spec-01 批次 F 衔接固化*
*生成时间：2026-04-14*