# spec-03-generator-framework 实施批次与模板

## 批次划分（Batch A/B/C）

说明：实现以最新 `design.md` 为准；如与 `tasks.md` 历史表述冲突，以 `design.md` 语义覆盖执行。

### Batch A：领域基础与配置主链路

- 任务范围：`1.1,1.2,1.3,2.1,2.2,2.3,2.4,2.5,3.1,3.2,3.3,3.4`
- 重点目标：
  - 打通 `Generator/Registry/Resolver/Validator/ConfigRepository` 主链路
  - 完成字段配置校验与持久化编排，输出稳定字段级错误路径
  - 落地 `modified_source` 枚举约束

### Batch B：预览能力与 FFI 边界契约

- 任务范围：`4.1,4.2,4.3,4.4,4.5,4.6,5.1,5.2,5.3,5.4`
- 重点目标：
  - 完成 `field/table` 预览、seed 可复现、partial-success 策略
  - `scope=table` 返回 `field_results[]` 并与 `samples` 保持一致性
  - 完成 FFI 稳定 JSON 契约、越界错误、脱敏与扩展点

### Batch C：测试矩阵与跨 Spec 联调收口

- 任务范围：`6.1,6.2,6.3,6.4,6.5,6.6,6.7,6.8,6.9,6.10,6.11,6.12,6.13`
- 重点目标：
  - 完成单元/集成/契约测试矩阵
  - 覆盖 schema trust 三态（`trusted/pending_rescan/pending_adjustment`）
  - 验证 `PREVIEW_TABLE_PARTIAL_SUCCESS_V1` 与跨 `spec-02/04/08/09` 联调断言

## /spec-impl 模板

### 通用模板

```markdown
/spec-impl 你现在执行 spec-03 的实现批次 {批次编号}，任务范围仅限：{任务号列表}。

注意：以最新 design.md 为准，覆盖 tasks.md 中旧表述。

必须实现/重点验收：
- ...
- ...

完成后必须按以下格式输出：
- 将已完成任务在 tasks.md 中标记完成
- 已完成任务：<逐条列出任务号>
- 变更文件：<路径列表>
- 验证结果：<测试命令与结果摘要>
- 未解决问题/风险：<如无写“无”>
- 下一批前置条件：<满足/不满足 + 原因>
```

### Batch A 模板

```markdown
/spec-impl 你现在执行 spec-03 的实现批次 A，任务范围仅限：1.1,1.2,1.3,2.1,2.2,2.3,2.4,2.5,3.1,3.2,3.3,3.4。

注意：以最新 design.md 为准，覆盖 tasks.md 中旧表述。

必须实现/重点验收：
- 打通 Generator/Registry/Resolver/Validator/ConfigRepository 主链路。
- SaveFieldGeneratorConfig 返回稳定字段级错误路径，并落地 modified_source 枚举约束。

完成后必须按以下格式输出：
- 将已完成任务在 tasks.md 中标记完成
- 已完成任务：<逐条列出任务号>
- 变更文件：<路径列表>
- 验证结果：<测试命令与结果摘要>
- 未解决问题/风险：<如无写“无”>
- 下一批前置条件：<满足/不满足 + 原因>
```

### Batch B 模板

```markdown
/spec-impl 你现在执行 spec-03 的实现批次 B，任务范围仅限：4.1,4.2,4.3,4.4,4.5,4.6,5.1,5.2,5.3,5.4。

注意：以最新 design.md 为准，覆盖 tasks.md 中旧表述。

必须实现/重点验收：
- 完成 field/table 预览、seed 可复现、partial-success 与 field_results[] 一致性。
- 完成 FFI 稳定契约、越界错误与脱敏要求。

完成后必须按以下格式输出：
- 将已完成任务在 tasks.md 中标记完成
- 已完成任务：<逐条列出任务号>
- 变更文件：<路径列表>
- 验证结果：<测试命令与结果摘要>
- 未解决问题/风险：<如无写“无”>
- 下一批前置条件：<满足/不满足 + 原因>
```

### Batch C 模板

```markdown
/spec-impl 你现在执行 spec-03 的实现批次 C，任务范围仅限：6.1,6.2,6.3,6.4,6.5,6.6,6.7,6.8,6.9,6.10,6.11,6.12,6.13。

注意：以最新 design.md 为准，覆盖 tasks.md 中旧表述。

必须实现/重点验收：
- 完成单元/集成/契约测试矩阵，覆盖 schema trust 三态。
- 完成 PREVIEW_TABLE_PARTIAL_SUCCESS_V1 契约与跨 spec 联调断言。

完成后必须按以下格式输出：
- 将已完成任务在 tasks.md 中标记完成
- 已完成任务：<逐条列出任务号>
- 变更文件：<路径列表>
- 验证结果：<测试命令与结果摘要>
- 未解决问题/风险：<如无写“无”>
- 下一批前置条件：<满足/不满足 + 原因>
```
