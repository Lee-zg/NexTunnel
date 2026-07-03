# 更新日志

## v0.6.4-alpha

- 版本入口统一升级到 `v0.6.4-alpha` / `0.6.4-alpha`，同步桌面端、Dashboard、CLI、服务端打包脚本、前端包元数据、部署示例、发布流程和文档站口径。
- macOS System TUN 完成 LaunchDaemon helper 开发接入：`nextunnel-helper` 以 root 运行，监听 `/var/run/nextunnel/helper.sock`，通过受控协议创建 utun、fd passing 交给桌面端，并应用/清理路由。
- macOS helper 限制 socket 权限为 `root:admin 0660`，校验 peer credential，拒绝任意 shell 执行和 `0.0.0.0/0`、`::/0` 默认路由。
- 桌面端、P2P/TUN 预检、网络页和验证器接入 macOS helper 状态；helper 可用时不再把 macOS System TUN 标为 `privilege_required`。
- `scripts/package-macos.sh` 新增 helper 构建、LaunchDaemon plist、System TUN `.pkg`、签名/公证参数和 SHA256 产物；DMG 继续作为 P2P/Relay-only 安装形态。
- `scripts/verify-p2p-tun.ps1` 新增 `-MacUseHelper`，生产验证口径改为 signed/notarized pkg 安装并归档 `tun-macos-latest.json` 后，才能声明 macOS System TUN 真实环境功能验收通过。
- 发布边界：本版本可声明 macOS System TUN helper 链路开发完成、本地测试通过；缺少 Developer ID 签名/公证、pkg 实机安装和 JSON 报告前，不声明生产通过。

## v0.6.3-alpha

- 版本入口统一升级到 `v0.6.3-alpha` / `0.6.3-alpha`，同步桌面端、Dashboard、CLI、服务端打包脚本、前端包元数据、部署示例、发布流程和文档站口径。
- 新增 `scripts/verify-scripts-static.ps1`，在 CI 中解析 PowerShell 脚本、检查关键脚本参数契约和 Bash 脚本语法，并把 JSON 报告上传为 `dist/verification/` artifact。
- 验证报告默认命名统一为 `dashboard-https-latest.json`、`dashboard-ssh-latest.json`、`tun-*.json`、`ebpf-benchmark-latest.json`、`edge-rehearsal-latest.json`，便于发布归档。
- macOS System TUN 新增 LaunchDaemon helper 链路：`nextunnel-helper` 通过 `/var/run/nextunnel/helper.sock` 创建 utun、应用/清理路由，DMG 保持 P2P/Relay-only，PKG 用于安装 System TUN helper。
- 明亮主题下新增专用 `logo-light.png` 图标，侧边栏和关于页按最终解析主题选择图标，暗色主题继续使用原 `logo.png`。
- 补齐明亮主题标题栏、侧边栏和 logo 容器样式，提升浅色模式下的视觉一致性和对比度。
- 修正 GitHub Release workflow 的桌面打包参数，使其匹配当前自定义 Wails 安装器打包脚本。
- 发布边界继续沿用 v0.6.2-alpha 的生产验收口径：真实系统 TUN、Dashboard 公网 HTTPS、eBPF 压力基准和真实多地域拓扑仍需外部资源复验。

## v0.6.2-alpha

- 版本入口统一升级到 `v0.6.2-alpha` / `0.6.2-alpha`，同步桌面端、Dashboard、CLI、服务端打包脚本、前端包元数据和文档站发布口径。
- Windows 桌面安装器切换为独立 `installer/` Wails 项目，Vue3 绘制安装界面，Go 后端负责 payload 校验、安全解压、升级替换、回滚、快捷方式和卸载信息。
- 桌面发布脚本移除 NSIS 构建路径，改为先生成桌面 payload，再构建内置 WebView2 bootstrapper 的自定义 Wails 安装器，Release 资产名保持不变。
- 发布说明、部署示例、生产验证手册和 README 切换到 `v0.6.2-alpha`，便于通过 GitHub tag 生成一致的 Release 产物。
- 保留 v0.6.0-beta 的生产收口能力边界：Relay Admin API、Dashboard 客户端监控、审计日志、运行配置状态、桌面服务端实例检测和真实生产外部验收清单继续作为本版本验收基线。

## v0.6.0-beta

- 版本入口统一升级到 `v0.6.0-beta` / `0.6.0-beta`，桌面端、Dashboard、CLI、服务端脚本和文档站发布口径同步更新。
- 延续 v0.5.3-beta 的生产收口成果：Relay Admin API、Dashboard 客户端监控、审计日志、运行配置状态和桌面服务端实例检测作为 Beta 发布基线。
- 进度文档补齐真实生产外部验收清单，明确 Dashboard HTTPS、Windows/macOS 真实 TUN、eBPF 性能基准、真实多地域 Edge/Anycast 和发布包实物验收所需条件。
- 发布说明和部署示例切换到 `v0.6.0-beta`，便于通过 GitHub tag 触发统一 Release 产物生成。

## v0.5.3-beta

- Relay Admin API 与 Dashboard 客户端监控进入 Beta 收口：客户端列表、断开、配置状态、审计查询和运行态错误提示形成闭环。
- Dashboard 新增审计日志页和运行配置状态展示，覆盖 HTTPS、Relay Admin API、CORS、审计日志、存储路径和版本信息。
- 桌面端新增服务端实例检测，按 Relay、Control Plane、STUN 返回可执行修复建议，并写入本地活动日志。
- 部署脚本补齐 `DASHBOARD_RELAY_ADMIN_TOKEN`，默认兼容复用 `RELAY_ADMIN_TOKEN`，同时更新 Compose、安装脚本和文档说明。
- `make.ps1 test-go` 与 CI 一致过滤前端 `node_modules` 内的 Go 包，避免本地 Go 测试误扫前端依赖。
- 版本入口统一升级到 `v0.5.3-beta` / `0.5.3-beta`，发布文档同步更新 Release 资产名与打包命令。

## v0.5.3-alpha

- 连接设置页改为服务端实例列表管理，移除顶部内联配置表单，新增与编辑统一通过弹窗完成。
- 服务端实例列表支持卡片/表格视图、搜索、选择当前连接目标、单项编辑和删除，保存后继续实时同步到本地配置。
- 端口扫描弹窗采用分步流程，首次扫描和重新扫描先展示居中雷达动画，并在扫描完成后进入开放端口列表。
- 桌面端新增符合主题的内容区滚动条样式，默认弱化显示，鼠标悬浮或键盘聚焦内容区域时显示青紫渐变滑块。
- 版本入口统一升级到 `v0.5.3-alpha` / `0.5.3`，发布文档同步更新 Release 资产名与打包命令。

## v0.5.0-alpha

- Windows 桌面端新增 Wails 官方 NSIS 安装包，安装器使用可审计的 NSIS 原生自定义界面，支持自定义安装位置、桌面快捷方式、完成后立即运行和 Wintun 组件页。
- Wintun 改为内置优先策略：发布脚本下载官方 Wintun ZIP、校验 SHA256、抽取匹配架构 DLL 打进安装包；安装时离线复制，联网下载只作为兜底路径。
- 桌面网络页新增 Wintun 状态与修复入口，zip 包或旧安装缺失 `wintun.dll` 时可下载官方包、校验后修复，并在权限不足时请求管理员模式修复。
- macOS 桌面端新增 `.app + .dmg` 打包方案，DMG 内置 Applications 拖拽入口、安装说明和未签名 alpha 标记；脚本预留 Developer ID 签名和 notarization 钩子。
- 发布流程新增 Windows installer、Windows zip 和 macOS DMG 资产，所有安装包生成 SHA256 校验文件和 manifest。
- 桌面发布脚本增加 `-Installer`、`-WintunMode`、`-WintunDownloadUrl`、`-WintunSha256`、`-SkipZip` 参数；zip 便携包继续支持通过 `NEXTUNNEL_WINTUN_DLL` 或 `-WintunDllPath` 随包放置官方 DLL。
- Release workflow 拆分 Windows 与 macOS 桌面包构建，上传最终安装器、压缩包和校验文件，避免发布中间目录、缓存、旧版本 exe 和临时资源。
- 版本入口统一升级到 `v0.5.0-alpha` / `0.5.0`，发布文档补充 Wintun、管理员权限、签名/公证和资源精简说明。

## v0.4.1-alpha

- 新增生产验证手册，覆盖 Dashboard HTTPS/CORS/鉴权、Windows/macOS P2P/TUN、Linux eBPF XDP 和多地域 Edge/Anycast 演练。
- 发布包补充 Dashboard、Edge rehearsal、eBPF verify 等验证入口，服务端包内包含验证脚本和 `xdp_forwarder.c`。
- Dashboard 生产部署联调脚本可验证健康检查、登录、token、CORS、节点、ACL 和告警接口。
- Dashboard 验证脚本默认拒绝向非本机 HTTP 发送管理员密码；新增 SSH 隧道验证脚本，域名/证书受限时可通过加密通道完成 API 验收。
- Windows/macOS P2P/TUN 验证脚本支持临时 SSH 公钥接入、双端候选交换、直连优先、Relay 降级和 JSON 报告汇总。
- 桌面端新增真实 TUN 生产预检，明确提示 Windows `wintun.dll` 缺失、管理员权限不足、macOS sudo/root 权限不足和 Linux `/dev/net/tun` 缺失。
- 桌面打包支持通过 `NEXTUNNEL_WINTUN_DLL` 或 `-WintunDllPath` 随包复制匹配架构的官方 `wintun.dll`，TUN 预检补充 Windows/macOS/Linux 环境修复建议。
- P2P/TUN 验证器改为真实内核 TUN 创建，不再用用户态 `netTun` 回退掩盖生产问题。
- 修复 macOS utun 控制器结构和数据包读写方式，为真实 utun 创建与收发验证打基础。
- 发布前已确认 P2P 直连链路可用；真实系统路由 TUN 仍需在 Windows 提供匹配架构 `wintun.dll`，并在 macOS 使用可用的 sudo/root 权限完成最终实机验收。

## v0.3.3-alpha

- CLI 增强服务端安装参数透传，支持 `--service-prefix`、自定义安装目录、端口、下载镜像、token 和 Dashboard 初始化参数。
- CLI 本机服务管理读取安装脚本生成的 `NEXTUNNEL_SERVICE_PREFIX`，同机多套 systemd 部署时可正确查看状态、启动、停止、重启和读取日志。
- CLI 远端命令补充上下文地址校验，缺少 Control Plane 或 Dashboard 配置时输出明确修复指引。
- Linux 一键安装脚本优化 WSL / Ubuntu 26.04 验证路径，安装后写入包内 `.env`，并增强健康检查、日志输出、端口占用诊断和 CLI 软链接安装。
- README 精简项目状态和路线规划内容，保留面向使用者的能力说明与入口。
- 文档站新增 CLI 使用指南，补充服务端管理、同机隔离测试、远端上下文、桌面端控制和排障流程。

## v0.3.1-alpha

- 桌面端新增实时流量图表，按刷新周期展示上传和下载增量。
- 实时流量图表切换为 `uPlot` Canvas 时序图，适配高频实时反馈场景。
- 总览页减重，仅保留连接控制、核心指标、实时流量图和最近日志。
- 左侧菜单改为 Lucide 图标导航，并增加滑动指示条和轻量切换动画。
- 新增桌面端持久化运行日志，支持级别/分类筛选、刷新和清空。
- 隧道列表新增连接类型，区分 P2P 直连、中继和待机。
- 新增本机端口扫描，限制回环地址并支持常用端口管理。
- 设置界面重构为连接、网络、端口、安全、外观和关于分类。
- 顶部品牌移除 `titleLogoImage`，改用与默认 logo 文字风格一致的字标；侧栏使用默认 `logo.png`。
- 新增 VitePress 文档站和发布流程说明，Release 同步发布一键安装脚本。
- 一键安装脚本新增 GitHub 下载代理配置，并补充腾讯云 COS/CDN 镜像源安装示例。
- Linux 一键安装脚本补齐 `nextunnel` CLI 安装和 `/usr/local/bin/nextunnel` 软链接，并在安装后执行健康检查与连接排障提示。
