# Implementation Plan

## Tasks

- [x] 1. 建立生成器抽象、注册中心与能力声明模型
- [x] 1.1 定义统一 `Generator` 接口与标准上下文/返回值对象（含错误分类）。
  - 覆盖生成器元信息、能力标签与类型语义。
  - _Requirements: 1.1, 1.4_
- [x] 1.2 实现 `GeneratorRegistry`，支持注册、查询、重复冲突检测。
  - 对同一 `generator_type` 的重复注册返回稳定错误码。
  - _Requirements: 1.2, 1.3_
- [x] 1.3 构建能力查询接口，支持按字段类型/能力标签筛选生成器。
  - 供 UI/FFI 与下游执行层复用。
  - _Requirements: 1.4_

- [x] 2. 实现字段类型映射与候选生成器解析
- [x] 2.1 建立 MVP 基础类型到默认生成器的映射规则。
  - 覆盖字符串、整数、浮点、布尔、日期时间、枚举/集合类。
  - _Requirements: 2.1_
- [x] 2.2 实现 `GeneratorTypeResolver`，按最新 schema 解析候选集与默认项。
  - 支持 schema 变化后重计算候选列表。
  - _Requirements: 2.2, 2.3_
- [x] 2.3 对“无可用生成器”场景返回可定位错误与建议动作。
  - 输出字段标识、字段类型、推荐下一步。
  - _Requirements: 2.4_
- [x] 2.4 实现通用 `ENUM` 候选值生成器（`EnumValueGenerator`）及参数模型。
  - 支持不同字段类型使用对应类型候选集合（如整型候选、字符串候选）。
  - _Requirements: 2.5_
- [x] 2.5 固化 MVP 默认生成器映射表（抽象类型 -> 默认生成器）并暴露可查询结果。
  - 至少覆盖 `int/decimal/string/boolean/datetime` 的默认项，供候选解析与 UI 初始建议复用。
  - _Requirements: 2.6_

- [x] 3. 实现字段级配置模型、校验与存储接口
- [x] 3.1 设计字段配置数据结构（`generator_type`、参数、空值策略、种子策略、启用状态）。
  - 明确请求定位键（`connection_id + table + column`）与持久化唯一键（`column_schema_id`）映射关系，以及配置版本字段。
  - _Requirements: 3.1_
- [x] 3.2 实现配置校验器（必填、类型、范围、schema 兼容性）。
  - 对校验失败返回字段级错误路径与修复提示。
  - _Requirements: 3.2, 3.3_
- [x] 3.3 实现配置仓储接口与服务编排，支持按连接/表维度查询与更新。
  - 服务层先将请求定位键解析为 `column_schema_id`，仓储层严格基于 `column_schema_id` upsert；保留最小必要审计信息（更新时间、修改来源）。
  - _Requirements: 3.4_
- [x] 3.4 为字段配置增加 `modified_source` 固定枚举校验与存储约束。
  - 非法枚举值拒绝保存，并返回字段级错误路径。
  - _Requirements: 3.5_
- [x] 3.5 实现 Schema Trust 门禁服务编排（含只读例外）。
  - 对 `SaveFieldGeneratorConfig`、`ValidateFieldGeneratorConfig`、`GetFieldGeneratorCandidates`、`PreviewGeneration` 在 `pending_rescan/pending_adjustment` 状态下短路返回 `FAILED_PRECONDITION`；对 `GetFieldGeneratorConfig` 保持成功并附带 `warnings[]`。
  - _Requirements: 3.4, 4.1, 5.3_

- [x] 4. 实现预览服务与可复现生成机制
- [x] 4.1 构建 `GeneratorPreviewService`，支持单字段/单表样本预览。
  - 预览路径严格禁止真实写入动作。
  - _Requirements: 4.1, 5.1_
- [x] 4.2 实现固定种子复现机制，确保同配置同输入可重复输出。
  - 处理生成器不支持确定性的降级提示。
  - _Requirements: 4.2, 4.3_
- [x] 4.3 处理外部依赖未就绪场景（外部 feed/计算字段上下文）。
  - 统一返回 `FAILED_PRECONDITION` 并指明上游依赖。
  - _Requirements: 4.4_
- [x] 4.4 实现 `scope=table` 预览部分失败策略（Partial Success）。
  - 字段级失败不影响其他字段样本返回，禁止静默丢失失败/跳过字段。
  - _Requirements: 4.5_
- [x] 4.5 在 `scope=table` 响应中输出字段级结果清单 `field_results[]`。
  - 至少包含 `field`、`status(ok|skipped|failed)`、`error_code?`、`warning?`，并与 `samples` 一致性校验。
  - _Requirements: 4.6_
- [x] 4.6 固化 `scope=table` 预览标准契约样例 `PREVIEW_TABLE_PARTIAL_SUCCESS_V1` 作为联调基线。
  - 样例定义以 `design.md` 为准，必须包含 `samples`、`metadata`、`warnings[]`、`field_results[]`，并明确 `status=ok` 字段与 `samples` 字段集合一致性规则。
  - _Requirements: 4.7, 5.1_
- [x] 4.7 显式固化预览 `metadata` 最小字段契约并完成输出校验。
  - `scope=field/table` 均需稳定返回 `generator_type`、参数摘要、`deterministic` 与 `warnings[]` 相关元信息（按范围输出），避免调用方依赖隐式字段。
  - _Requirements: 4.3, 5.1_

- [x] 5. 暴露 FFI 契约并强化边界与安全
- [x] 5.1 提供配置保存、配置查询、候选查询、预览、校验等接口并统一 JSON 外壳。
  - 统一错误码映射并保持契约稳定。
  - _Requirements: 3.4, 4.1, 4.3, 5.3_
- [x] 5.2 对跨表编排/写入类请求返回范围外错误，禁止职责泄漏到执行层。
  - 错误文案明确提示由 `spec-04` 处理。
  - _Requirements: 5.1, 5.2_
- [x] 5.3 全链路脱敏，确保日志与响应不泄露凭据、密钥或 token。
  - 增加敏感字段屏蔽测试样例。
  - _Requirements: 5.3_
- [x] 5.4 为 `spec-08/spec-09` 预留扩展契约点并保持 MVP 接口兼容。
  - 定义 capability 字段与依赖就绪检查钩子。
  - _Requirements: 5.4_

- [x] 6. 测试与跨 Spec 联调
- [x] 6.1 单元测试：注册冲突、类型候选解析、配置校验、种子复现。
  - _Requirements: 1.2, 2.3, 3.2, 4.2_
- [x] 6.2 集成测试：配置保存 -> 候选查询 -> 预览返回 -> 错误映射全链路。
  - _Requirements: 3.4, 4.1, 5.3_
- [x] 6.3 契约测试：FFI JSON 结构、错误码、边界错误一致性。
  - _Requirements: 5.1, 5.2, 5.3_
- [x] 6.4 跨 spec 联调任务：
  - 与 `spec-02` 验证 schema 变化后候选生成器重算与配置再校验；
  - 与 `spec-04` 验证边界外请求被阻断且错误可传播；
  - 与 `spec-08/spec-09` 验证扩展 capability 与依赖校验契约。
  - _Requirements: 2.2, 4.4, 5.2, 5.4_
- [x] 6.5 单元测试：`EnumValueGenerator` 候选值类型一致性校验与混合类型拦截。
  - 覆盖 `int/string/decimal/datetime/boolean` 的合法与非法样例。
  - _Requirements: 2.5, 3.2_
- [x] 6.6 契约/集成测试：`modified_source` 固定枚举校验。
  - 覆盖合法枚举保存成功、非法枚举保存失败（`INVALID_ARGUMENT`）及错误路径返回。
  - _Requirements: 3.5, 5.3_
- [x] 6.7 Schema Trust 门禁契约测试：`trusted` 状态下各核心接口成功路径 JSON 结构。
  - 覆盖 `SaveFieldGeneratorConfig`、`ValidateFieldGeneratorConfig`、`GetFieldGeneratorCandidates`、`PreviewGeneration`、`GetFieldGeneratorConfig`。
  - 断言统一外壳：`ok=true`、`error=null`，并校验各接口 `data` 关键字段（如 `saved/config_version`、`valid/errors`、`candidates/default_generator`、`samples/metadata/warnings`、`config/warnings`）。
  - _Requirements: 3.4, 4.1, 5.3_
- [x] 6.8 Schema Trust 门禁契约测试：`pending_rescan` 状态下失败/只读例外 JSON 结构。
  - 对 `SaveFieldGeneratorConfig`、`ValidateFieldGeneratorConfig`、`GetFieldGeneratorCandidates`、`PreviewGeneration` 断言：`ok=false`、`data=null`、`error.code=FAILED_PRECONDITION`、`error.reason=SCHEMA_TRUST_PENDING_RESCAN`。
  - 对 `GetFieldGeneratorConfig` 断言只读例外：`ok=true`、`data.config` 存在、`data.warnings[]` 包含 `reason=SCHEMA_TRUST_PENDING_RESCAN`、`error=null`。
  - _Requirements: 3.4, 4.1, 5.3_
- [x] 6.9 Schema Trust 门禁契约测试：`pending_adjustment` 状态下失败/只读例外 JSON 结构。
  - 对 `SaveFieldGeneratorConfig`、`ValidateFieldGeneratorConfig`、`GetFieldGeneratorCandidates`、`PreviewGeneration` 断言：`ok=false`、`data=null`、`error.code=FAILED_PRECONDITION`、`error.reason=SCHEMA_TRUST_PENDING_ADJUSTMENT`。
  - 对 `GetFieldGeneratorConfig` 断言只读例外：`ok=true`、`data.config` 存在、`data.warnings[]` 包含 `reason=SCHEMA_TRUST_PENDING_ADJUSTMENT`、`error=null`。
  - _Requirements: 3.4, 4.1, 5.3_
- [x] 6.10 Schema Trust 门禁补充契约测试：`ListGeneratorCapabilities` 在三种状态下结构稳定。
  - 断言 `trusted/pending_rescan/pending_adjustment` 均返回 `ok=true`、`data.generators[]` 结构稳定，不混入 trust gate 顶层失败。
  - _Requirements: 5.3_
- [x] 6.11 契约/集成测试：`scope=table` 预览部分失败与字段级结果清单。
  - 覆盖 `ok/skipped/failed` 混合场景，断言 `field_results[]` 完整返回且与 `samples` 中成功字段集合一致。
  - _Requirements: 4.5, 4.6, 5.1_
- [x] 6.12 契约测试：`scope=table` 预览响应符合 `PREVIEW_TABLE_PARTIAL_SUCCESS_V1` 结构。
  - 基于 `PREVIEW_TABLE_PARTIAL_SUCCESS_V1` 断言 `samples/metadata/warnings/field_results` 四段结构稳定，并校验字段命名与可空位（如 `error_code`、`warning`）一致。
  - _Requirements: 4.7, 5.1_
- [x] 6.13 单元测试：默认生成器映射表的稳定性与回归保护。
  - 覆盖五种抽象类型默认项、schema 变化后二次解析仍可返回稳定默认建议。
  - _Requirements: 2.6, 2.2_

## Requirements Coverage Matrix（计划自检）

| ID | 覆盖任务 |
| -- | -------- |
| 1.1–1.4 | 1.1, 1.2, 1.3 |
| 2.1–2.6 | 2.1, 2.2, 2.3, 2.4, 2.5, 6.5, 6.13 |
| 3.1–3.5 | 3.1, 3.2, 3.3, 3.4, 3.5, 6.6, 6.7, 6.8, 6.9 |
| 4.1–4.7 | 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 6.4, 6.7, 6.8, 6.9, 6.11, 6.12 |
| 5.1–5.4 | 4.1, 5.1, 5.2, 5.3, 5.4, 6.3, 6.4, 6.7, 6.8, 6.9, 6.10 |
