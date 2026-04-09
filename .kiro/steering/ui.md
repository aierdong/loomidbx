# UI Design Guidelines

LoomiDBX 的用户界面设计遵循专业数据库工具的风格（如 Navicat、DBeaver），提供清晰、高效的操作体验。

## Design Principles

- **专业性**：借鉴成熟数据库工具的界面风格，降低用户认知障碍
- **清晰性**：层次分明，信息组织合理，关键操作易于发现
- **高效性**：支持批量操作、多任务并行，减少重复点击
- **一致性**：统一的视觉语言和交互模式

## Key UI Components

### Layout Structure
- 三栏布局：左侧连接树 + 中间工作区 + 底部状态栏
- 多 Tab 工作区：支持同时配置多个表
- 可调整的面板位置：生成器配置面板可在下方或右侧

### Navigation Tree
- 层级结构：连接 → 数据库 → 表
- 支持多选（同一数据库下的表）
- 快捷操作按钮：新建连接、扫描 Schema、生成数据

### Table Configuration
- 表级配置：生成数量、Truncate 选项、关联关系
- 字段列表：完整显示字段属性（类型、约束、注释等）
- 生成器配置：按字段类型提供不同的生成策略选择

### Data Generation Wizard
- 显示选中的表及其生成数量
- 支持设置倍数范围（如 1-3 倍关系）
- 自动检测生成顺序
- 显示预估时间

## Visual Theme

- **主色调**：专业蓝灰系（Primary: #2563EB）
- **字体**：Inter (Sans), JetBrains Mono (Code), PingFang SC (中文)
- **间距系统**：基于 4px 基数的层级间距
- **响应式**：最小支持 1024×768，推荐 1280×800+

## Animation Guidelines

- 页面加载：400ms ease-out
- 树节点展开：200ms ease-out
- Tab 切换：250ms ease-out
- 按钮交互：150ms hover, 100ms press
- 对话框：300ms open, 200ms close

## Implementation Platform

- **前端框架**：Flutter (支持 Windows、MacOS、Linux)
- **调用方式**：Dart FFI 调用 Go 后端动态库
- **状态管理**：Provider 或 Riverpod
- **持久化**：本地 JSON 文件存储配置

---

## Detailed Design Documents

完整的 UI 设计文档请参阅：
- [UI 设计详细文档](ui/UI_DESIGN.md) - 包含布局、组件、配色、字体、动画等完整规范
- [主题样式](ui/loomidbx_ui_theme.css) - CSS 变量定义和组件样式
- [交互原型](ui/loomidbx_ui_design.html) - 可在浏览器查看的原型示例