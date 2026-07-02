## 1. 使用的系统与工具链
- 框架与构建：两个前端子项目（desktop/frontend、installer/frontend）均采用 Vue 3 + TypeScript + Vite 技术栈，通过 vue-tsc --noEmit && vite build 进行类型检查与打包。
- UI 组件库：桌面客户端使用 Naive UI (naive-ui) 作为主组件库，安装器前端为极简实现，仅依赖 Vue 自身。
- 图标与可视化：桌面端使用 lucide-vue-next 提供线性图标集；数据图表使用 uplot。
- 状态管理：桌面端使用 Pinia 管理应用状态。
- 国际化：通过 vue-i18n 实现中英文切换。
- 样式方案：不使用 Tailwind/PostCSS/SCSS/Less，采用原生 CSS + CSS 自定义属性（CSS Variables）+ 组件内 <style> 块的组织方式。

## 2. 核心文件与位置
- 桌面客户端入口与全局样式：desktop/frontend/src/App.vue（包含完整的主题变量定义、布局样式、滚动条定制、动画等）
- 安装器前端样式：installer/frontend/src/styles.css（独立的全局样式，定义品牌色板、字体、窗口布局等）
- 桌面端视图组件：desktop/frontend/src/views/StatusView.vue、NetworkView.vue、LogsView.vue、SettingsView.vue
- 桌面端公共组件：desktop/frontend/src/components/UpdatePrompt.vue
- 国际化资源：desktop/frontend/src/i18n.ts
- 包配置：desktop/frontend/package.json、installer/frontend/package.json

## 3. 架构与设计约定
### 3.1 设计令牌（Design Tokens）体系
两个前端均通过 :root CSS 变量集中定义设计令牌，形成统一的品牌视觉基线：
- 品牌色系：--nex-cyan（青色）、--tunnel-violet（紫色）、--data-blue（蓝色）构成主色调
- 中性色阶：--text-main、--text-dim、--text-muted 三级文字层次
- 语义色：--success、--warning、--danger 用于状态反馈
- 表面层：--surface-bg、--surface-strong、--line-soft 控制卡片、分割线等层级关系
- 动效参数：--ease-standard、--duration-small/medium 等缓动与时长变量

### 3.2 主题系统（Theme System）
- 双主题支持：通过 .theme-light 类名切换亮/暗主题，桌面端默认深色主题
- 动态主题覆盖：通过 Naive UI 的 themeOverrides 配置对象，在运行时注入用户自定义强调色（accent color），实现个性化主题
- 系统主题跟随：监听 prefers-color-scheme 媒体查询，支持系统级主题同步
- 无障碍支持：遵循 prefers-reduced-motion 媒体查询，自动禁用或减弱动画效果

### 3.3 布局架构
- 桌面客户端：采用经典的「标题栏 + 侧边栏 + 内容区」三栏布局，使用 CSS Grid 和 Flexbox 组合实现响应式适配
- 安装器前端：左右分栏布局（40%/60%），左侧展示品牌信息，右侧显示操作面板
- Wails 集成：通过 --wails-draggable 属性标记可拖拽区域，实现原生窗口体验

### 3.4 样式组织模式
- 单文件组件样式：每个 Vue 组件的样式直接写在 <style> 块中，避免外部 CSS 文件碎片化
- BEM 风格命名：使用 .app-shell、.sidebar-nav、.nav-button 等语义化类名
- CSS 模块化：通过 CSS 变量实现跨组件样式共享，减少重复代码

## 4. 开发者规范与约束
1. 优先使用 CSS 变量：新增颜色、间距、圆角等设计元素时，应先在 :root 中定义变量，而非硬编码具体值
2. 主题兼容性：所有新增样式必须同时考虑亮/暗两种主题下的可读性和对比度
3. 组件样式隔离：组件内部样式使用 <style scoped> 或 BEM 命名空间，避免全局污染
4. 动画性能：使用 CSS 变量定义的缓动函数和时长，确保动画一致性；尊重用户的运动偏好设置
5. 无第三方样式框架：禁止引入 Tailwind、Bootstrap 等样式框架，保持样式体系的统一性
6. Naive UI 主题覆盖：需要调整组件外观时，通过 themeOverrides 配置而非直接覆盖组件样式
7. 字体回退策略：使用 local() 优先加载系统字体，确保在不同平台上的最佳显示效果