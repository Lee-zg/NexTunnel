## 1. 使用的系统/方法
- Go 原生 `error` + `errors.New` 定义包级哨兵错误（sentinel errors），供调用方通过 `errors.Is` 判断。
- 使用 `fmt.Errorf("...: %w", err)` 对底层错误进行包装，保留错误链以便上层诊断。
- 在 HTTP Dashboard 层提供 `AuthMiddleware`，将认证错误统一转换为 JSON 响应并返回 `401`。
- 运行期异常通过结构化日志记录（`logger.Error(..., "error", err)`），而非 panic/recover 控制流。
- 测试中显式断言特定哨兵错误（如 `auth.ErrTokenExpired`）以验证错误语义。

## 2. 关键文件与包
- `pkg/protocol/errors.go` — 协议层共享哨兵错误：`ErrPayloadTooLarge`、`ErrUnknownMsgType`、`ErrConnClosed`。
- `desktop/internal/auth/token.go` — 桌面端鉴权哨兵错误：`ErrTokenExpired`、`ErrTokenInvalid`、`ErrTokenMalformed`。
- `server/internal/dashboard/auth.go` — Dashboard 的 `AuthManager` 与 `AuthMiddleware`，负责 token 校验及 HTTP 错误响应。
- `cli/internal/api/client.go` — CLI 远端 API 客户端，广泛使用 `%w` 包装网络/编解码错误，并在解析服务端错误体时还原为 `errors.New`。
- `cli/internal/configstore/store.go`、`cli/internal/desktop/control.go`、`cli/internal/system/process.go` — CLI 配置/进程/控制面交互的错误包装示例。
- `desktop/app.go`、`desktop/control_server.go`、`desktop/internal/tunnel/*.go`、`desktop/internal/p2p/mesh.go`、`desktop/internal/virtualnet/manager.go` — 大量 `logger.Error(..., "error", err)` 的结构化错误日志记录点。

## 3. 架构与约定
- **分层职责**
  - 基础库（`pkg/*`）仅暴露语义清晰的哨兵错误，不依赖具体框架；调用方用 `errors.Is` 做分支逻辑。
  - 业务组件（`desktop/*`、`server/*`）在边界处把底层错误用 `fmt.Errorf` 加上上下文信息再向上返回。
  - HTTP 边界（Dashboard）通过 `AuthMiddleware` 拦截认证失败，直接写回 JSON 错误体，避免上层 handler 重复处理。
- **错误传播策略**
  - 可恢复的业务错误 → 返回 error，由上层决定重试/降级。
  - 不可恢复的配置/启动错误（如 Dashboard 默认管理员初始化失败）→ 使用 `panic` 终止进程，交由外部进程管理器重启。
  - 运行时 IO/网络异常 → 记录结构化日志后继续或优雅关闭，不中断主流程。
- **日志规范**
  - 所有关键路径上的错误均通过 `logger.Error` 输出，附带 `"error"` 字段，便于集中采集与告警。
  - 未出现统一的错误码枚举或 `WithMessage` 类封装，错误消息即主要诊断信息。

## 4. 开发者应遵循的规则
1. **定义可检测的错误**：对跨模块需要按类型处理的错误，在对应包中以 `var ErrXxx = errors.New("package: ...")` 形式暴露，并使用小写注释说明触发条件。
2. **始终包装底层错误**：使用 `fmt.Errorf("...: %w", err)` 包裹来自 I/O、序列化、第三方库的错误，禁止吞掉原始错误。
3. **HTTP 层统一响应**：在 middleware/handler 顶层将业务错误转为 JSON `{"error":"..."}` 并设置合适的 HTTP 状态码；不要在内层 handler 里直接写响应体。
4. **日志优先于 panic**：仅在进程无法继续运行的初始化阶段使用 `panic`；其余场景一律记录结构化日志并返回 error。
5. **测试断言哨兵错误**：对关键错误分支编写 `errors.Is(err, pkg.ErrXxx)` 类型的断言，确保错误语义稳定。
6. **避免泄露敏感信息**：HTTP 错误消息不应包含密码、token 等敏感内容；Dashboard 的 `AuthMiddleware` 已体现该约束。