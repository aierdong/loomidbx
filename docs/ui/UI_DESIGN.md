# LoomiDBX UI 设计文档

## 概述

本文档描述了 LoomiDBX 数据库模拟数据生成工具的用户界面设计。设计风格参考了 Navicat、DBeaver 等专业数据库工具，适用于桌面端应用（Windows、MacOS、Linux）。

## 设计文件

- **主题样式**: `loomidbx_ui_theme.css` - 包含完整的颜色、字体、间距等设计规范
- **界面原型**: `loomidbx_ui_design.html` - 可在浏览器中查看的交互式设计原型

## 布局结构

### 整体布局

```
┌─────────────────────────────────────────────┐
│ Titlebar (32px)                             │
├─────────────────────────────────────────────┤
│ Menubar (32px)                              │
├──────────────┬──────────────────────────────┤
│              │                              │
│   Sidebar    │      Workspace               │
│   (280px)    │      (Flex)                  │
│              │                              │
├──────────────┴──────────────────────────────┤
│ Statusbar (24px)                            │
└─────────────────────────────────────────────┘
```

### 主要区域说明

1. **Titlebar (标题栏)**
   - 高度: 32px
   - 显示应用名称和窗口控制按钮
   - 支持拖拽移动窗口

2. **Menubar (菜单栏)**
   - 高度: 32px
   - 菜单项: File, Edit, Tools, Generate, Help

3. **Sidebar (左侧面板)**
   - 宽度: 280px
   - 包含操作按钮区和连接树
   - 支持多选表（复选框）

4. **Workspace (工作区)**
   - 多 Tab 布局
   - 每个 Tab 显示一个表的配置
   - 包含表信息、关联关系、字段列表、生成器配置

5. **Statusbar (状态栏)**
   - 高度: 24px
   - 显示连接状态、选中表数量等信息

## 配色方案

### 主色调
- **Primary (主色)**: `#2563EB` - 用于强调、按钮、选中状态
- **Secondary (辅色)**: `#64748B` - 用于次要信息
- **Accent (强调色)**: `#10B981` - 用于成功、生成按钮
- **Warning (警告)**: `#F59E0B` - 用于警告提示
- **Error (错误)**: `#EF4444` - 用于错误、删除操作

### 背景色
- **Background**: `#F8FAFC` - 主背景
- **Surface**: `#FFFFFF` - 卡片、面板
- **Surface-Hover**: `#F1F5F9` - 悬停状态
- **Border**: `#E2E8F0` - 边框颜色

### 文字色
- **Text-Primary**: `#0F172A` - 主要文字
- **Text-Secondary**: `#475569` - 次要文字
- **Text-Tertiary**: `#94A3B8` - 辅助文字

## 字体系统

- **Sans-Serif**: Inter, SF Pro, Segoe UI, system-ui
- **Monospace**: JetBrains Mono, Fira Code, Consolas
- **中文**: PingFang SC, Microsoft YaHei

### 字号
- XS: 11px (状态栏、标签)
- SM: 12px (表格内容、次要文字)
- Base: 14px (正文、按钮)
- LG: 16px (标题、Tab)
- XL: 18px (页面标题)
- 2XL: 24px (对话框标题)

## 间距系统

- XS: 4px (紧密间距)
- SM: 8px (小间距)
- MD: 12px (中等间距)
- LG: 16px (大间距)
- XL: 24px (超大间距)
- 2XL: 32px (区块间距)

## 核心组件

### 1. 连接树 (Tree Component)

**功能**:
- 显示数据库连接、数据库、表的层级结构
- 支持展开/折叠
- 支持多选（复选框）
- 同一数据库下的表可多选

**交互**:
- 单击选中节点
- 双击展开/折叠或打开表详情
- 复选框用于批量操作

### 2. 多 Tab 工作区

**功能**:
- 同时打开多个表进行配置
- Tab 可关闭
- 支持 Tab 切换动画

**布局**:
- Tab 栏在顶部
- Tab 内容区域包含表配置

### 3. 表配置面板

**包含内容**:
- 表基本信息（名称、注释）
- 生成选项（数量、Truncate）
- 关联关系（可展开区域）
  - Foreign Keys (物理外键)
  - Logical Keys (逻辑外键)
  - Referenced By (被引用)
- 字段列表（表格形式）
- 生成器配置面板（位置可选：下方或右侧）

### 4. 字段列表表格

**列定义**:
- 复选框 (是否参与生成)
- Name (字段名 + 注释)
- Type (数据类型)
- Key (约束: PK/FK/UNI)
- Null (是否允许 NULL)
- Default (默认值)
- Generator (生成器类型)

**交互**:
- 点击行选中字段
- 选中后在生成器配置面板显示详细配置

### 5. 生成器配置面板

**位置选项**:
- 选项 A: 字段列表下方（默认）
- 选项 B: 字段列表右侧
- 用户可通过按钮切换

**内容**:
- 生成器类型下拉选择
- 生成器特定配置项
- 约束配置（Unique, Allow Null, Null Rate）
- 预览按钮

### 6. 生成数据向导

**触发方式**:
- 点击工具栏"Generate Data"按钮
- 或右键菜单选择

**对话框内容**:
```
┌─ Generate Data Wizard ──────────────────────┐
│ Selected Tables:                            │
│                                             │
│ ☑ users          [10000] rows               │
│ ☑ orders         [30000] rows  (3x)         │
│ ☑ order_items    [60000] rows  (2x)         │
│                  Range: [1] - [3] per order │
│                                             │
│ Generation Order: (Auto-detected)           │
│ 1. users                                    │
│ 2. orders                                   │
│ 3. order_items                              │
│                                             │
│ Options:                                    │
│ [✓] Truncate before insert                  │
│ [✓] Use transaction                         │
│ [✓] Stop on error                           │
│                                             │
│ Estimated time: ~2 minutes                  │
│                                             │
│           [Cancel]  [Generate]              │
└─────────────────────────────────────────────┘
```

**关键特性**:
- 显示选中的表
- 为每个表设置生成数量
- 对于有外键关系的表，设置倍数范围（如 1-3 倍）
- 自动检测生成顺序
- 显示预估时间
- 若本次任务涉及 HTTP/SQL 外部数据源，点击 Generate 后先弹出“出网确认框”

### 6.1 出网确认框（外部数据源）

当本次生成任务包含外部数据源（HTTP/SQL）字段时，必须先展示确认框：

```
┌─ 外部访问确认 ─────────────────────────────────┐
│ 本次生成任务将访问外部数据源。                    │
│                                                  │
│ 连接: crm-demo                                   │
│ 类型: HTTP / SQL                                 │
│ 脱敏端点预览: https://api.partner.local/dataset  │
│                                                  │
│ ⚠ URL query 中若包含密钥将存在泄露风险。          │
│                                                  │
│ [ ] 对此连接不再提示                              │
│                                                  │
│                 [取消] [继续生成]                 │
└──────────────────────────────────────────────────┘
```

交互规则：
- 未勾选“对此连接不再提示”时，每次任务都弹窗确认
- 勾选后仅对当前连接生效，后续任务不再重复弹窗
- 该设置可在连接配置页重新开启

## 动画规范

### 页面加载
- App Launch: 400ms ease-out, opacity 0→1, Y+20→0

### 树形导航
- Node Expand: 200ms ease-out, height 0→auto, rotate 0→90deg
- Node Collapse: 150ms ease-in
- Node Hover: 150ms
- Node Select: 200ms, scale 1→1.02→1

### Tab 切换
- Tab Switch: 250ms ease-out, opacity 0→1, X+10→0
- Tab Close: 200ms ease-in
- Tab Indicator: 200ms ease-out, X position slide

### 表格交互
- Row Hover: 150ms, Y 0→-1px
- Row Select: 200ms, border-left 0→3px

### 按钮交互
- Button Hover: 150ms, scale 1→1.02
- Button Press: 100ms, scale 1.02→0.98
- Ripple: 400ms, scale 0→2, opacity 0.3→0

### 对话框
- Dialog Open: 300ms ease-out, scale 0.95→1, opacity 0→1
- Dialog Close: 200ms ease-in
- Overlay Fade: 250ms

### 进度指示
- Progress Bar: 300ms ease-out
- Spinner: 1000ms linear infinite, rotate 360deg
- Success Check: 500ms bounce, scale 0→1.2→1

## 响应式考虑

虽然这是桌面应用，但需要考虑不同屏幕尺寸：

- **最小窗口尺寸**: 1024×768
- **推荐尺寸**: 1280×800 或更大
- **侧边栏**: 可折叠以节省空间
- **工作区**: 自适应宽度

## 可访问性

- 所有交互元素支持键盘导航
- 使用语义化 HTML 标签
- 提供适当的 ARIA 标签
- 确保足够的颜色对比度
- 支持屏幕阅读器

## Flutter 实现建议

### 推荐的 Flutter 组件

1. **布局**:
   - `Row`, `Column`, `Expanded` 用于基本布局
   - `SplitView` 或自定义 `Resizable` 组件用于可调整大小的面板

2. **树形组件**:
   - 使用 `TreeView` package 或自定义实现
   - 支持懒加载大量节点

3. **表格**:
   - `DataTable` 或 `flutter_data_table` package
   - 支持排序、筛选、虚拟滚动

4. **Tab**:
   - `TabBar` + `TabBarView`
   - 自定义 Tab 样式以匹配设计

5. **对话框**:
   - `showDialog` + 自定义 `Dialog` widget

6. **主题**:
   - 使用 `ThemeData` 定义全局主题
   - 自定义 `ColorScheme` 和 `TextTheme`

### 状态管理建议

- 使用 `Provider` 或 `Riverpod` 管理应用状态
- 连接信息、表配置等持久化到本地存储
- 使用 `ChangeNotifier` 或 `StateNotifier` 管理复杂状态

### 性能优化

- 使用 `ListView.builder` 实现虚拟滚动
- 大量数据使用分页加载
- 图标使用 SVG 格式（`flutter_svg`）
- 避免不必要的 widget 重建

## 后续迭代方向

1. **暗色主题**: 提供暗色模式选项
2. **自定义主题**: 允许用户自定义颜色方案
3. **快捷键**: 定义常用操作的快捷键
4. **工作区布局**: 支持保存和恢复工作区布局
5. **插件系统**: 支持自定义生成器插件

## 参考资源

- 设计原型: `loomidbx_ui_design.html`
- 主题样式: `loomidbx_ui_theme.css`
- 产品需求: `product-outline.md`
