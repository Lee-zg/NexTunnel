## 系统概述

NexTunnel 仓库采用**各子模块各自管理自身配置**的分散式策略，没有统一的集中式配置框架。CLI、桌面客户端、服务端与安装器分别通过各自的入口文件加载配置，主要依赖环境变量（`.env`/`server.env`）与命令行参数，辅以少量 YAML/TOML 配置文件。

## 关键组件与约定

### CLI 配置存储（configstore + paths）
- `cli/internal/configstore/`：提供上下文（Context）持久化，默认路径由 `cli/internal/system/paths.go` 计算，位于用户配置目录下的 `nextunnel` 子目录。
- `cli/internal/command/config.go`：暴露 `nexctl config` 子命令，支持查看配置路径、设置/切换/列出 Context。
- `cli/internal/system/paths.go`：根据运行平台解析 `EnvPath`（默认 `~/.nextunnel/server.env`），并支持从源码树 `deploy/server/.env` 读取开发期默认值。
- `cli/internal/envfile/`：轻量 `.env` 文件读写工具，供 CLI 在启动本地服务端时注入环境变量。

### 服务端配置
- 服务端通过 `deploy/server/.env.example` 提供环境变量模板，实际运行时使用 `server.env`（由安装脚本生成）。CLI 的 `server` 子命令支持 `--env-file` 指定自定义路径。
- 服务进程启动时由 `cli/internal/system/process.go` 负责读取 `server.env` 并决定是否启用 Dashboard 等子服务。

### 桌面端与安装器
- 桌面端（Wails）与安装器各自维护独立的配置结构体与加载逻辑，遵循 Go 标准库 `os.Getenv` + 结构体 tag 的模式，未引入第三方配置库。
- 桌面端偏好设置通过 Wails 内置机制持久化，不经过全局 `server.env`。

### 部署与容器编排
- `docker-compose.yml` 与 `deploy/server/docker-compose.yml` 通过环境变量注入配置，配合 `install.sh` / `install.ps1` 生成 `server.env`。
- 安装脚本将用户输入写入 `server.env`，后续由 CLI 与服务端共享该文件。

## 架构决策与约束

1. **无统一配置中心**：每个二进制自行解析其所需的环境变量，避免跨模块强耦合。
2. **环境变量优先**：所有可配置项均以 `NEXTUNNEL_*` 或特定前缀的环境变量形式暴露，便于容器化与 CI 注入。
3. **分层默认值**：源码树内 `deploy/server/.env` 提供开发默认值；安装脚本覆盖为生产值；`--env-file` 允许运行时覆盖。
4. **CLI 作为配置入口**：用户通过 `nexctl config` 和 `nexctl server --env-file` 交互，而非直接编辑配置文件。
5. **安全敏感信息**：密钥类字段建议通过外部密钥管理服务注入，不在 `server.env` 中明文存放。

## 开发者应遵循的规则

- 新增配置项时，先在对应模块的结构体中添加字段与 `os.Getenv` 解析逻辑。
- 若该配置影响多个组件，应在 `deploy/server/.env.example` 中补充说明，并在 CLI 帮助文本中体现。
- 不要引入新的配置格式（JSON/YAML/TOML），除非有充分理由；优先使用环境变量。
- 测试时应通过临时目录与 `t.TempDir()` 构造隔离的 `server.env`，参考现有 `*_test.go` 中的写法。
