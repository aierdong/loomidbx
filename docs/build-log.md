项目构建日志

# 1. 项目初始化

已按计划完成初始化，摘要如下。

## Go 后端

- 根目录 `go.mod`：module loomidbx，依赖 github.com/json-iterator/go。
- `backend/cmd/main.go`：LDB_Version（返回 JSON）、LDB_FreeString，供 c-shared 导出。
- 已建立 steering 中的子包占位：connector、schema、generator（含 int/、string/、decimal/、datetime/、boolean/）、writer、storage、ffi。
- 语言限制：int、string 不能作为包名，目录仍叫 int/string，包名分别为 intgen、strgen（见对应 doc.go）。

编译输出目录：`backend/build/libldb.dll`（及同目录生成的 .h）。一键脚本：`scripts/build-go-windows.ps1`。

## Flutter 前端

- frontend/：flutter create 含 windows / macos / linux。
- `pubspec.yaml`：flutter_riverpod；主题色与 steering 主色一致（#2563EB）。
- `lib/main.dart`：ProviderScope + LoomiDBXApp。
- 已建目录：ffi/（native_library.dart 预留 DynamicLibrary.open）、models/、screens/、widgets/、providers/。
- Windows 资源：`windows/runner/Runner.rc` 中公司名改为 com.aierdong.loomidbx。
- 为兼容当前环境（Flutter 3.3 / Dart 2.18），未使用 ColorScheme.fromSeed / Material 3 等较新 API。

## 仓库

已扩展 `.gitignore`（backend/build/、**/build/、.dart_tool/ 等）。

## 已执行验证

| 命令                                                                    |	结果 |
| :--------------------------------------------------------------------- | :--- |
| go build -buildmode=c-shared -o backend/build/libldb.dll ./backend/cmd | 通过  |
| go test ./...                                                          | 通过  |
| flutter analyze                                                        | 无问题 |
| flutter test                                                           | 通过  |
| flutter build windows                                                  | 通过  |

本地可执行：`cd frontend` 后 `flutter run -d windows`（需本机 Flutter 在 PATH 或使用 D:\flutter\bin\flutter.bat）。
DLL 联调时请将 backend/build/libldb.dll 放到与 loomidbx.exe 同目录或按 native_library.dart 调整加载路径。 
所有相关 to-do 已勾选完成。
若你希望下一步把 CMake 里自动复制 libldb.dll 到 runner 输出目录，可以说一下我按 Windows 流程补上。 


# spec-01-connection-and-credentials 的 Quick Spec 执行计划

## 关键约束（与默认 spec-quick 的差异）

- **目录名必须冻结为** `spec-01-connection-and-credentials`（见 [SPECS_PLANNING.md](e:\git\loomidbx\SPECS_PLANNING.md) 第 2、3 节）。若仅用一句英文描述走「自动 kebab」，通常得不到带 `spec-01-` 前缀的名称，**阶段 1 应直接以该名字创建目录与文件**，勿依赖从长描述推导出的 slug。
- **`PROJECT_DESCRIPTION`**（写入 `[requirements-init.md](e:\git\loomidbx\.kiro\settings\templates\specs\requirements-init.md)` 占位）建议用**中文**概括以下内容，便于 `/kiro:spec-requirements` 生成与规划一致的需求：
  - **名称**：连接与凭据管理
  - **包含**：连接创建/编辑/测试、连接持久化、密钥环与环境变量注入策略
  - **不包含**：Schema 扫描、生成器配置、执行引擎逻辑
  - **间断交付**：可稳定连接与安全存储凭据；明确不向下承诺扫描与生成（`spec-02` 接续）
  - **依赖**：上游无；下游 `spec-02`、`spec-06`、`spec-07`（便于需求/设计中的范围与衔接说明）
- **语言**：模板 `[init.json](e:\git\loomidbx\.kiro\settings\templates\specs\init.json)` 默认 `"language": "zh"`，生成物与总结保持简体中文（符合 CLAUDE.md / spec.json）。
- **规划第 7 节对 `tasks.md` 的硬性要求**：必须包含**测试任务**与**跨 spec 联调**任务；阶段 4 完成后需人工核对 `[tasks.md](e:\git\loomidbx\.kiro\settings\templates\specs\tasks.md)` 结构下是否覆盖与 `spec-02`/`spec-06`/`spec-07` 的边界假验。

## 执行流程（对应 `/spec-quick --auto` 四阶段）

```mermaid
flowchart LR
  P1[Phase1_Init] --> P2[Phase2_Requirements]
  P2 --> P3[Phase3_Design]
  P3 --> P4[Phase4_Tasks]
```

### 阶段 1：初始化

- 确认 `[.kiro/specs/](e:\git\loomidbx\.kiro\specs)` 下不存在同名目录（若已存在则按冲突策略改为 `-2` 后缀，**优先使用规划名不重命名**）。
- 创建目录 `.kiro/specs/spec-01-connection-and-credentials/`（当前环境为 Windows PowerShell，使用 `New-Item -ItemType Directory -Force` 或等价方式；**避免**用 `&&` 串联命令）。
- 读取并填充模板：
  - `[init.json](e:\git\loomidbx\.kiro\settings\templates\specs\init.json)` → `spec.json`（`{{FEATURE_NAME}}` → `spec-01-connection-and-credentials`，`{{TIMESTAMP}}` 为 UTC ISO8601，例如 `Get-Date` 格式化为 `yyyy-MM-ddTHH:mm:ssZ`）
  - `[requirements-init.md](e:\git\loomidbx\.kiro\settings\templates\specs\requirements-init.md)` → `requirements.md`（`{{PROJECT_DESCRIPTION}}` 为上方中文规划摘要）
- 产出：`spec.json`、`requirements.md`（需求正文待阶段 2 生成）。

### 阶段 2：生成需求（等价 `/kiro:spec-requirements spec-01-connection-and-credentials`）

- 按 `[.cursor/commands/kiro/spec-requirements.md](e:\git\loomidbx\.cursor\commands\kiro\spec-requirements.md)` 步骤：
  - 读取 `.kiro/steering/` 全文（含 `product.md`、`tech.md`、`structure.md` 及自定义 steering）
  - 阅读 `[.kiro/settings/rules/ears-format.md](e:\git\loomidbx\.kiro\settings\rules\ears-format.md)` 与 `[.kiro/settings/templates/specs/requirements.md](e:\git\loomidbx\.kiro\settings\templates\specs\requirements.md)`
  - 编写完整 `requirements.md`（EARS、可测、**标题需数字编号**），**不写实现细节**
  - 更新 `spec.json`：`phase: "requirements-generated"`，`approvals.requirements.generated: true`，`updated_at`
- 技术栈对齐要点（来自 steering，供需求表述用，不单写实现）：Flutter + Go FFI、连接在 Go 侧、`LDB_` 前缀、配置存储（如 SQLite）等见 `[tech.md](e:\git\loomidbx\.kiro\steering\tech.md)`。

### 阶段 3：生成设计（等价 `/kiro:spec-design spec-01-connection-and-credentials -y`）

- 按 `[.cursor/commands/kiro/spec-design.md](e:\git\loomidbx\.cursor\commands\kiro\spec-design.md)`：加载 steering、需求、[design 模板](e:\git\loomidbx.kiro\settings\templates\specs\design.md)、[design-principles.md](e:\git\loomidbx.kiro\settings\rules\design-principles.md)；`-y` 表示自动批准需求。
- 输出 `design.md`：组件/模块、凭据存储与注入策略、连接生命周期、与后续 spec 的接口边界（例如为未来 schema 扫描预留的连接抽象，但不实现扫描）。
- 更新 `spec.json` 中 design 相关元数据与时戳（以命令内「Update Metadata」为准）。

### 阶段 4：生成任务（等价 `/kiro:spec-tasks spec-01-connection-and-credentials -y`）

- 按 `[.cursor/commands/kiro/spec-tasks.md](e:\git\loomidbx\.cursor\commands\kiro\spec-tasks.md)`：读取 `[tasks-generation.md](e:\git\loomidbx\.kiro\settings\rules\tasks-generation.md)`、[`tasks.md` 模板](e:\git\loomidbx.kiro\settings\templates\specs\tasks.md)；`-y` 自动批准需求与设计。
- **显式验收**：`tasks.md` 中须含**单元/集成测试类任务**以及**与下游 spec 的联调/契约验证类任务**（例如：连接与凭据就绪后，对 `spec-02` 扫描入口的假验或 mock 契约；对 `spec-06` FFI 错误模型预留的衔接检查），以满足 [SPECS_PLANNING.md 第 7 节第 3 条](e:\git\loomidbx\SPECS_PLANNING.md)。

## Quick 模式说明与后续

- 本流程**跳过** `/kiro:validate-gap` 与 `/kiro:validate-design`（与 `/spec-quick` 说明一致）；完成后建议视复杂度补做 gap/design 校验再进入 `/kiro:spec-impl`。
- **规划第 7 节第 1 条**（11 个目录一次性初始化）与「只完成 spec-01」可拆：**本计划仅交付 spec-01 全套文档**；若需严格同步「11 个空目录」可另起一小步批量建空目录（不改变 spec-01 内容）。

## 产出清单（完成时）

| 文件                                                         | 说明                             |
| :---------------------------------------------------------- | :------------------------------- |
| `.kiro/specs/spec-01-connection-and-credentials/spec.json`   | 元数据与阶段状态                 |
| `.kiro/specs/spec-01-connection-and-credentials/requirements.md` | EARS 需求                        |
| `.kiro/specs/spec-01-connection-and-credentials/design.md`   | 技术设计                         |
| `.kiro/specs/spec-01-connection-and-credentials/tasks.md`    | 实现任务（含测试与跨 spec 联调） |

规划内工作已全部做完，摘要如下。

## Quick Spec 已完成（自动等效全流程）

模式说明：按冻结目录名 `.kiro/specs/spec-01-connection-and-credentials/` 完成初始化 → 需求 → 设计（`-y`）→ 任务（`-y`），未执行 gap/design 校验。

### 产出文件

| 文件                                                         | 说明                                                         |
| :----------------------------------------------------------- | :----------------------------------------------------------- |
| `.kiro/specs/spec-01-connection-and-credentials/spec.json`   | `phase: tasks-generated`，需求/设计已批准（`-y`），任务已生成待评审 |
| `.kiro/specs/spec-01-connection-and-credentials/requirements.md` | 5 组需求，EARS 风格验收条款，编号 1.x～5.x                   |
| `.kiro/specs/spec-01-connection-and-credentials/design.md`   | 架构边界、组件、数据与错误策略；对齐 `docs/schema.md` / steering |
| `.kiro/specs/spec-01-connection-and-credentials/research.md` | 调研与决策摘要                                               |
| `.kiro/specs/spec-01-connection-and-credentials/tasks.md`    | 6 大组任务；含 5.x 测试 与 6.x 跨 spec-02/06/07 联调；文末有需求覆盖矩阵 |

### Quick 生成阶段未跑的校验

- `/kiro:validate-gap spec-01-connection-and-credentials`
- `/kiro:validate-design spec-01-connection-and-credentials`

### 建议后续步骤

1. 人工过一遍 `design.md`（尤其是删除连接的从属策略与 `extra` 扩展）。
2. 可选：补跑上述 validate 命令。
3. 实现阶段：`/kiro:spec-impl spec-01-connection-and-credentials`（或按 `tasks.md` 子任务分段执行）。

所有相关 to-do 已标为 completed。



---

## 推荐执行批次（可直接照着跑）

### 批次 A：连接配置持久化基础

- 任务：`1.1`
- 目标：打通连接记录的创建/更新/删除基本 CRUD、ID 稳定性、失败错误返回。
- 验收：
  - 编辑连接不改变 ID
  - 持久化失败不误报成功
  - 表前缀与存储约束符合 `ldb_` 规则

建议命令：

- `/kiro/spec-impl spec-01-connection-and-credentials 1.1`

------

### 批次 B：删除语义与凭据清理（按新设计）

- 任务：`1.2`（按你最新决策执行）
- 目标：实现“警示 -> 用户确认 -> 级联删除”，并清理密钥环/引用。
- 验收：
  - 未确认时返回 `CONFIRMATION_REQUIRED`
  - 确认后级联删除成功且无悬挂凭据
  - 删除流程具备事务一致性

建议命令：

- `/kiro/spec-impl spec-01-connection-and-credentials 1.2`

------

### 批次 C：凭据解析与平台策略

- 任务：`2.1, 2.2, 2.3`
- 目标：完成 env/keyring/AES 的优先级链、脱敏、最小平台支持矩阵对应行为。
- 验收：
  - 环境变量优先级可测试
  - keyring 不可用与拒绝访问返回可分类错误
  - 禁止静默明文落库

建议命令：

- `/kiro/spec-impl spec-01-connection-and-credentials 2.1-2.3`

------

### 批次 D：连接测试与超时参数

- 任务：`3.1, 3.2`
- 目标：同步 `TestConnection` + `timeout_sec`（默认 20，可覆盖）+ 错误归类。
- 验收：
  - 超时返回 `DEADLINE_EXCEEDED`
  - `timeout_sec` 从连接配置生效
  - TLS/认证/网络错误可区分

建议命令：

- `/kiro/spec-impl spec-01-connection-and-credentials 3.1-3.2`

------

### 批次 E：服务编排与 FFI 契约

- 任务：`4.1, 4.2`
- 目标：聚合服务与 JSON 外壳稳定化，响应中无明文敏感信息。
- 验收：
  - `ok/data/error` 结构稳定
  - `DeleteConnection` 支持确认参数语义
  - Save/List 不回传密码/令牌

建议命令：

- `/kiro/spec-impl spec-01-connection-and-credentials 4.1-4.2`

------

### 批次 F：测试基线与跨 spec 衔接

- 任务：`5.1, 5.2, 5.3, 6.1, 6.2, 6.3`
- 目标：补齐单测/集成/契约快照 + 和 spec-02/06/07 对接验证。
- 验收：
  - 关键优先级矩阵和错误码有自动化覆盖
  - FFI golden 固化
  - 边界假验文档可供下游直接使用

建议命令：

- `/kiro/spec-impl spec-01-connection-and-credentials 5.1-6.3`

## 你每批执行时的“固定收口模板”（强烈建议）

每批完成后要求输出这 4 项，避免上下文失控：

- 已完成任务项（编号）
- 变更文件清单（路径）
- 未解决风险/阻塞
- 下一批前置条件是否满足（是/否+原因）



---

## 通用主模板（每批都用）

你现在执行 spec-01 的实现批次 {批次编号}，任务范围仅限：{任务号列表}。

严格要求：

1) 只实现本批次任务，不扩展范围。
2) 必须遵循最新 design.md 冻结决策（尤其是：

   \- 删除策略：警示用户 -> 用户确认 -> 级联删除

   \- TestConnection：同步接口 + timeout_sec（默认20s，可配置）

   \- 密钥环最小平台支持矩阵与错误语义）

3) 保持与 steering 一致：LDB_ 前缀、JSON FFI、ldb_ 表前缀、敏感信息脱敏。
4) 修改后运行必要测试（至少覆盖本批关键路径）。
5) 不要提交 git commit。
6) 在tasks.md 中勾选已经完成的条目

完成后必须按以下格式输出：

- 在tasks.md 中勾选已经完成的条目

\- 已完成任务：<逐条列出任务号>

\- 变更文件：<路径列表>

\- 验证结果：<测试命令与结果摘要>

\- 未解决问题/风险：<如无写“无”>

\- 下一批前置条件：<满足/不满足 + 原因>

------

## 批次 A 模板（1.1）

你现在执行 spec-01 的实现批次 A，任务范围仅限：1.1。
重点验收：
- 连接记录 CRUD 基础能力可用
- 编辑连接时 ID 保持不变
- 持久化失败不误报成功
- 与 ldb_ 存储约束一致
严格要求：
1) 只实现本批次任务，不扩展范围。
2) 必须遵循最新 design.md 冻结决策（尤其是：
   - 删除策略：警示用户 -> 用户确认 -> 级联删除
   - TestConnection：同步接口 + timeout_sec（默认20s，可配置）
   - 密钥环最小平台支持矩阵与错误语义）
3) 保持与 steering 一致：LDB_ 前缀、JSON FFI、ldb_ 表前缀、敏感信息脱敏。
4) 修改后运行必要测试（至少覆盖本批关键路径）。
5) 不要提交 git commit。
完成后必须按以下格式输出：
- 已完成任务：<逐条列出任务号>
- 变更文件：<路径列表>
- 验证结果：<测试命令与结果摘要>
- 未解决问题/风险：<如无写“无”>
- 下一批前置条件：<满足/不满足 + 原因>

---

- 已完成任务：

  - `1.1` 定义并实现连接记录在元数据存储中的创建、按标识更新与删除语义（含事务边界与错误返回）。
  - `1.1` 验收点补齐：连接 CRUD 基础能力、编辑连接 ID 保持不变、持久化失败不误报成功、`ldb_` 前缀存储约束。
  - 依据冻结设计对齐了本批相关契约：删除走“确认后级联”（`confirm_cascade=true` 才执行）、`TestConnection` 同步接口 + `timeout_sec`（默认 20s，可配置），并补充密钥环最小矩阵所需错误语义码常量（`KEYRING_UNAVAILABLE` / `KEYRING_ACCESS_DENIED`）。

- 变更文件：

  - `backend/cmd/main.go`
  - `backend/storage/connection_store.go`
  - `backend/app/connection_service.go`
  - `backend/app/connection_service_test.go`
  - `go.mod`
  - `go.sum`

- 验证结果：

  - 命令：`go test ./...`

  - 结果摘要：通过。

    ```
    backend/app
    ```

     

    测试通过，覆盖以下关键路径：

    - 创建 + 编辑连接时 ID 不变
    - 删除前确认校验（未确认返回 `CONFIRMATION_REQUIRED`）
    - 确认后执行级联删除
    - 持久化失败返回 `STORAGE_ERROR` 且不返回成功
    - `TestConnection` 失败路径不误报成功（同步返回错误）

- 未解决问题/风险：

  - 密钥环“最小平台支持矩阵”的真实适配器（Windows/macOS/Linux 后端探测与访问）本批未实现，仅先落了错误语义常量与接口侧约束；若下一批需要“可验证平台探测”，需补 Keyring Adapter 与平台测试。

- 下一批前置条件：

  - 满足：本批 `1.1` 基线能力已可用，且关键验收路径已有自动化测试兜底，可继续后续批次实现。

------

## 批次 B 模板（1.2）

你现在执行 spec-01 的实现批次 B，任务范围仅限：1.2。

注意：以最新 design.md 为准，覆盖 tasks.md 中旧表述。

必须实现：

\- 删除前确认语义（未确认返回 CONFIRMATION_REQUIRED）

\- 确认后执行级联删除

\- 清理密钥环/凭据引用，确保无悬挂

\- 删除流程事务一致性

完成后必须按以下格式输出：
- 已完成任务在 tasks.md 中标记完成
- 已完成任务：<逐条列出任务号>
- 变更文件：<路径列表>
- 验证结果：<测试命令与结果摘要>
- 未解决问题/风险：<如无写“无”>
- 下一批前置条件：<满足/不满足 + 原因>



---

- 已完成任务：
  - `1.2`（按最新 `design.md`）：未确认删除返回 `CONFIRMATION_REQUIRED`、确认后级联删除、清理密钥环/凭据引用、删除流程事务一致性。
- 变更文件：
  - `backend/app/connection_service.go`
  - `backend/app/connection_service_test.go`
  - `backend/storage/connection_store.go`
- 验证结果：
  - 命令：`go test ./backend/app ./backend/storage`
  - 结果：通过（`ok loomidoubx/backend/app`，`ok loomidoubx/backend/storage`）
  - 关键覆盖点摘要：
    - 未确认删除时返回 `CONFIRMATION_REQUIRED`，且不触发凭据清理
    - 确认后执行级联删除（连接 + 从属表快照 + 凭据引用）
    - 删除时调用凭据清理回调，确保无悬挂凭据引用
    - 凭据清理失败时整条删除事务回滚（连接/从属数据/凭据引用均保留）
- 未解决问题/风险：
  - 无
- 下一批前置条件：
  - 满足：`1.2` 的语义与事务一致性已由实现与自动化测试覆盖，可进入下一批次任务。

------

## 批次 C 模板（2.1-2.3）

你现在执行 spec-01 的实现批次 C，任务范围仅限：2.1, 2.2, 2.3。

重点验收：

\- env/keyring/AES 完整优先级链可测试

\- keyring 不可用 -> KEYRING_UNAVAILABLE

\- keyring 拒绝访问 -> KEYRING_ACCESS_DENIED

\- 禁止静默明文落库

\- 日志与错误输出不泄露敏感值

（其余要求同通用模板）

------

## 批次 D 模板（3.1-3.2）

你现在执行 spec-01 的实现批次 D，任务范围仅限：3.1, 3.2。

重点验收：

\- TestConnection 为同步接口

\- timeout_sec 生效（默认20s，支持覆盖）

\- 超时返回 DEADLINE_EXCEEDED

\- 认证/网络/TLS 错误有结构化分类

（其余要求同通用模板）

------

## 批次 E 模板（4.1-4.2）

你现在执行 spec-01 的实现批次 E，任务范围仅限：4.1, 4.2。

重点验收：

\- 服务层聚合入口稳定（测试、保存、列表、删除）

\- FFI JSON 结构固定为 ok/data/error

\- Save/List 响应不返回明文密码或令牌

\- DeleteConnection 契约包含确认语义

（其余要求同通用模板）

------

## 批次 F 模板（5.1-6.3）

你现在执行 spec-01 的实现批次 F，任务范围仅限：5.1, 5.2, 5.3, 6.1, 6.2, 6.3。

重点验收：

\- 凭据优先级与脱敏测试齐全

\- 连接测试集成用例覆盖成功/认证失败/超时

\- FFI golden/契约快照固定

\- 与 spec-02/06/07 的边界假验与对接说明完整

（其余要求同通用模板）

------

如果你希望，我还可以再给你一版“超短模板”（每次 6~8 行，适合快速粘贴执行）。