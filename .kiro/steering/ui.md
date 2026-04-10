---
name: ui
description: LoomiDBX Flutter UI 设计规范 - 布局结构、组件设计、主题配色、动画规范
type: reference
---

# UI Design Guidelines

LoomiDBX 的用户界面设计遵循专业数据库工具风格（参考 Navicat、DBeaver），使用 Flutter 开发桌面端应用。

## Design Principles

- **专业性**：借鉴成熟数据库工具界面，降低认知障碍
- **清晰性**：层次分明，信息组织合理，关键操作易于发现
- **高效性**：支持批量操作、多任务并行，减少重复点击
- **一致性**：统一的视觉语言和交互模式

## Layout Structure

三栏布局结构：

```
+---------------------------------------------+
| Titlebar (32px) - App name + window controls|
+---------------------------------------------+
| Menubar (32px) - File, Edit, Tools...       |
+-------------+-------------------------------+
| Sidebar     | Workspace (Flex)             |
| (280px)     | + Multi-Tab Layout            |
| + Tree Nav  | + Table Configuration         |
+-------------+-------------------------------+
| Statusbar (24px) - Connection status        |
+---------------------------------------------+
```

**区域说明**：
- **Sidebar**：连接树 → 数据库 → 表层级，支持多选
- **Workspace**：多 Tab 工作区，每 Tab 显示一个表配置
- **Statusbar**：连接状态、选中数量、操作提示

## Theme Configuration

### Color Palette

| Token | Value | Usage |
|-------|-------|-------|
| `--primary` | `#2563EB` | 主色、选中、按钮 |
| `--accent` | `#10B981` | 成功、生成操作 |
| `--warning` | `#F59E0B` | 警告提示 |
| `--error` | `#EF4444` | 错误、删除 |
| `--background` | `#F8FAFC` | 主背景 |
| `--surface` | `#FFFFFF` | 卡片、面板 |
| `--border` | `#E2E8F0` | 边框 |

### Typography

```css
--font-sans: 'Inter', 'SF Pro', 'Segoe UI', system-ui;
--font-mono: 'JetBrains Mono', 'Fira Code', monospace;
--font-cn: 'PingFang SC', 'Microsoft YaHei', sans-serif;

/* Font Sizes */
--font-xs: 11px;  /* Status, tags */
--font-sm: 12px;  /* Table content, secondary */
--font-base: 14px; /* Body, buttons */
--font-lg: 16px;  /* Titles, tabs */
```

### Spacing System

基于 4px 基数：`xs:4, sm:8, md:12, lg:16, xl:24, 2xl:32`

## Animation Guidelines

| Animation | Duration | Easing |
|-----------|----------|--------|
| Page Load | 400ms | ease-out |
| Tree Expand | 200ms | ease-out |
| Tab Switch | 250ms | ease-out |
| Button Hover | 150ms | ease-out |
| Button Press | 100ms | ease-out |
| Dialog Open | 300ms | ease-out |
| Dialog Close | 200ms | ease-in |

## Flutter Implementation

### Recommended Widgets

```dart
// 主布局结构
class MainLayout extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        _buildTitlebar(),    // 32px
        _buildMenubar(),     // 32px
        Expanded(
          child: Row(
            children: [
              _buildSidebar(),   // 280px, resizable
              Expanded(child: _buildWorkspace()),
            ],
          ),
        ),
        _buildStatusbar(),   // 24px
      ],
    );
  }
}

// Tree Navigation
TreeView(
  controller: treeController,
  nodes: connectionNodes,
  onNodeTap: (node) => _selectTable(node),
)

// Multi-Tab Workspace
TabBarView(
  controller: tabController,
  children: openTabs.map((table) => TableConfigPanel(table: table)).toList(),
)

// Theme Definition
ThemeData(
  colorScheme: ColorScheme.light(
    primary: Color(0xFF2563EB),
    secondary: Color(0xFF64748B),
    error: Color(0xFFEF4444),
  ),
  textTheme: TextTheme(
    bodyMedium: TextStyle(fontSize: 14, fontFamily: 'Inter'),
  ),
)
```

### State Management

- **Provider/Riverpod**：管理应用状态
- **持久化**：本地 JSON 存储配置
- **性能**：`ListView.builder` 实现虚拟滚动

## Core Components

### Tree Navigation
- 层级：Connection → Database → Table
- 支持：展开/折叠、多选（复选框）、懒加载

### Table Configuration Panel
- 表信息：名称、注释
- 生成选项：数量、Truncate 选项
- 关联关系：外键、逻辑键、被引用
- 字段列表：复选框、名称、类型、约束、生成器

### Generator Panel
- 位置：字段列表下方或右侧（可切换）
- 内容：类型选择、配置项、约束、预览

### Generation Wizard Dialog
- 显示选中表及生成数量
- 外键表设置倍数范围（如 1-3x）
- 自动检测生成顺序
- 预估时间显示

## Responsive Requirements

- **最小窗口**：1024x768
- **推荐尺寸**：1280x800+
- **侧边栏**：可折叠节省空间

## References

- 完整设计规范：`docs/ui/UI_DESIGN.md`
- 主题样式：`docs/ui/loomidbx_ui_theme.css`
- 交互原型：`docs/ui/loomidbx_ui_design.html`