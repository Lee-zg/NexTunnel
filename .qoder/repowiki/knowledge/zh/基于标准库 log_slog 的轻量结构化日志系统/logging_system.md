## 1. 使用的系统与框架
- 仅使用 Go 标准库 log/slog，未引入任何第三方日志库（如 zap、logrus、zerolog）。
- 输出格式为纯文本（slog.NewTextHandler），默认级别 LevelInfo，目标为 os.Stdout。
- 通过依赖注入将 *slog.Logger 作为参数传入各组件，而非全局单例。

## 2. 关键文件与位置
- desktop/app.go：唯一发现集中使用 slog 的位置，负责创建 logger、注入到虚拟网卡管理器、NAT 客户端/探测器、控制 API 等子模块。
- 其他子模块（server、cli、installer、pkg/*）在当前搜索范围内未发现显式 slog 调用，可能尚未接入统一日志或仍使用 fmt/log 包。

## 3. 架构与约定
- Logger 生命周期：在桌面端 App 启动时构造一次 slog.Logger，随后通过字段和构造函数选项（如 nat.WithLogger、SetLogger）向下传递。
- 降级策略：当某函数未收到 logger 时回退到 slog.Default()，避免 nil 指针导致崩溃。
- 错误日志安全：活动审计写入 SQLite 失败时只落 slog.Error，注释明确说明“避免递归触发错误日志”。
- 结构化字段：使用 slog 的键值对形式记录上下文，如 "error"、"level" 等，但尚未形成统一的字段命名规范文档。

## 4. 开发者应遵循的规则
- 优先使用 *slog.Logger 作为依赖注入参数，不要直接引用 slog.Default() 以外的全局状态。
- 所有可观测性事件使用 slog 的 Debug/Info/Warn/Error 级别，并附带结构化键值对（至少包含 "error" 字段）。
- 若组件需要独立日志实例，应在其入口通过 With("component", "...") 派生子 logger，保持来源可区分。
- 目前仅有桌面端实现了完整链路；server/cli/installer 模块如需接入，应复用相同的注入模式，并在各自 main 中初始化 logger。