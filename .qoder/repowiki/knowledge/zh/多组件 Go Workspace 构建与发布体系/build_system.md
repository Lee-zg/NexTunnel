## 1. 系统概览

NexTunnel 采用 **Go Workspace + 顶层 Makefile + PowerShell/Shell 脚本 + GitHub Actions** 的多层构建体系：
- `go.work` 聚合 `cli / desktop / installer / pkg / server` 五个子模块，统一 Go 版本为 1.25.0；
- 顶层 `Makefile` 提供 `build / package-* / lint / test / verify-* / clean` 等统一入口；
- 各产物打包逻辑下沉到 `scripts/*.ps1`（Windows）与 `scripts/package-macos.sh`（macOS），由 CI 或本地调用；
- `.github/workflows/ci.yml` 负责 PR 合并前的 lint/build/test，`.github/workflows/release.yml` 在打 tag 时并行产出桌面端、CLI、服务端、文档站点并上传 GitHub Releases。

## 2. 关键文件与职责

| 文件 | 职责 |
|---|---|
| `go.work` | 声明 workspace 根与五个子模块路径 |
| `Makefile` | 统一开发/构建/测试/验证入口，封装各平台打包脚本 |
| `scripts/package-desktop.ps1` | Windows 桌面端 NSIS 安装器 + zip 包生成，含 wintun.dll 来源校验与架构检查 |
| `scripts/package-macos.sh` | macOS DMG 打包（需 Xcode 工具链） |
| `scripts/package-cli.ps1` | CLI 跨平台交叉编译（linux-amd64/arm64, windows-amd64）并归档 |
| `scripts/package-server.ps1` | 服务端全二进制 + Dashboard Web 静态资源 + 部署脚本打包 |
| `scripts/verify-*.ps1/.sh` | TUN/P2P/Dashboard/eBPF 端到端验证脚本 |
| `.github/workflows/ci.yml` | 触发 main/develop 与 PR，执行 golangci-lint、ESLint、Go build/vet/test、前端构建 |
| `.github/workflows/release.yml` | 按 tag 并行构建桌面端（win/mac）、CLI、服务端、docs，并上传 Release Assets 与 Pages |
| `deploy/server/install.{sh,ps1}` | 服务端一键安装/升级/卸载脚本，支持 systemd 服务注册与环境变量覆盖 |
| `docker-compose.yml` | 本地一键拉起 Control Plane + Relay + NAT Detector + Dashboard 的集成环境 |

## 3. 架构与约定

### 3.1 版本与产物命名
- 版本号通过 `-X main.version=<normalized>` ldflags 注入 CLI，并在所有 MANIFEST.txt 中记录；
- 产物命名遵循 `<component>-<version>-<os>-<arch>.<ext>` 模式，如 `nextunnel-cli-v0.6.3-alpha-linux-arm64.tar.gz`；
- 每个压缩包自动生成同目录 `.sha256` 校验文件，release 流程强制上传。

### 3.2 依赖与缓存
- Go 构建缓存被重定向到仓库根 `.gocache-release` / `.gocache-test*` 目录，CI 可复用；
- Node 依赖使用 npm ci / pnpm install --frozen-lockfile，lock 文件分别位于 `desktop/frontend/package-lock.json`、`server/web/package-lock.json`、`installer/frontend/package.json`；
- Wails CLI 固定版本 `v2.12.0`，NSIS 通过 choco 安装。

### 3.3 安全与完整性
- Windows 桌面端打包强制校验 wintun.dll 的 SHA256 与 PE Machine 架构，防止混入错误架构 DLL；
- 服务端安装脚本支持 `--package-url` + `--sha256` 指定离线包与校验值，默认从 GitHub Releases 下载；
- 安装脚本以 `set -Eeuo pipefail` 运行，参数经正则白名单校验。

### 3.4 验证流水线
- `make verify-dashboard[-ssh]` 通过 SSH 隧道或直接访问 Dashboard API 进行生产级冒烟测试；
- `make verify-tun / verify-p2p-tun` 在真实网卡上验证 TUN 路由下发与 P2P 连通性；
- `make verify-ebpf-linux` 在 Linux 内核上编译并加载 XDP 转发程序。

## 4. 开发者应遵守的规则

1. **统一入口**：本地开发与 CI 均通过 `make <target>` 驱动，新增目标请同步更新 `help` 输出。
2. **版本来源单一**：版本号仅通过 Makefile 顶层 `VERSION ?= v0.6.3-alpha` 与 release workflow 的 `RELEASE_VERSION` 注入，不要在子模块硬编码。
3. **交叉编译开关 CGO**：CLI/Server 打包脚本显式设置 `CGO_ENABLED=0`，避免引入平台相关 C 依赖导致交叉编译失败。
4. **wintun.dll 不可绕过**：桌面端打包若启用 bundled/download 模式，必须提供 `WintunSha256`，否则脚本直接抛错。
5. **前端构建前置**：任何涉及 dashboard 前端的构建（package-server、CI build-check）必须先执行对应 `npm ci && npm run build`。
6. **验证脚本参数化**：新增验证场景应在 `scripts/` 下新增独立脚本，并通过 Makefile target 暴露，保持 CI 与本地一致。
7. **Release 产物规范**：新组件打包需在 `release.yml` 添加并行 job，并遵循现有 `dist/<name>.tar.gz(.zip)` + `.sha256` + `MANIFEST.txt` 三件套约定。