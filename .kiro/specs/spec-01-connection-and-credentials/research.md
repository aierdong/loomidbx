# Research & Design Decisions

## Summary

- **Feature**: `spec-01-connection-and-credentials`
- **Discovery Scope**: Extension / 与既有 `docs/schema.md`、`steering/database-schema.md` 对齐
- **Key Findings**:
  - 项目已约定元数据存储为 `StorageDriver` + `ldb_connections`，密码列为 AES-256 加密落盘，FFI 已有 `TestConnection` / `SaveConnection` / `ListConnections` / `DeleteConnection` 命名空间。
  - 本 spec 追加「系统密钥环引用」与「环境变量注入」时，需在不动摇 `ldb_connections` 主键与外键语义的前提下，扩展 `extra` JSON 或增加可选列记录凭据来源与 key 引用。
  - 跨平台密钥环在实现层通常采用按 OS 分支：`windows` 凭据管理器、`darwin` Keychain、`linux` 秘链(libsecret) 或文档化降级路径。

## Research Log

### 与权威详设的一致性

- **Context**: `docs/schema.md` 定义 `ldb_connections` DDL 与 FFI 清单。
- **Findings**: 连接测试与持久化属于 Connector + Storage 交叉面；spec-01 交付应可追溯至同一 JSON 契约，避免与 spec-06 重复发明错误模型。
- **Implications**: 设计以「扩展字段与解析规则」为主，不重写扫描与 Mapper。

### 凭据存储策略组合

- **Context**: 需求要求密钥环、环境变量与禁止明文配置表。
- **Findings**: 既有 steering 采用列加密；密钥环可作为「不落库明文」的升级路径，环境变量适用于 CI 与无头环境。
- **Implications**: 在设计中固定「解析优先级」与「不可用时降级策略」，并在任务阶段为各 OS 建立矩阵测试。

## Design Decisions

| 决策 | 选择 | 说明 |
|------|------|------|
| 元数据表 | 延续 `ldb_connections` | 与 Migration 版本线一致 |
| 列加密 | 保留 AES-256 列语义 | 与 steering 一致；密钥环模式可替代或并存于 `extra` |
| FFI | JSON `ok/data/error` | 与 database-schema steering §8 一致 |
