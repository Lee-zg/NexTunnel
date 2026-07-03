# 发布流程

v0.6.4-alpha 使用统一版本号发布桌面端安装器、CLI、服务端包、一键安装脚本、验证工具和 VitePress 文档站。

本版本发布说明见 [v0.6.4-alpha Release Notes](./release-notes-v0.6.4-alpha.md)。其中 macOS System TUN 只能声明 helper 链路开发完成和本地测试通过；没有 signed/notarized pkg 实机报告前，不能标注生产通过。

## 本地打包

```bash
make package-desktop VERSION=v0.6.4-alpha
make package-macos VERSION=v0.6.4-alpha
make package-cli VERSION=v0.6.4-alpha
make package-server VERSION=v0.6.4-alpha
```

Windows PowerShell：

```powershell
.\scripts\package-desktop.ps1 -Version v0.6.4-alpha -WintunMode bundled
.\scripts\package-cli.ps1 -Version v0.6.4-alpha
.\scripts\package-server.ps1 -Version v0.6.4-alpha
```

macOS DMG 只能在 macOS 本机或 macOS runner 构建：

```bash
bash scripts/package-macos.sh --version v0.6.4-alpha
```

## Windows Wintun 打包

Windows 自定义 Wails 安装器默认使用 `-WintunMode bundled`：

- 下载官方 Wintun 0.14.1 ZIP。
- 校验 SHA256。
- 抽取匹配架构 `wintun.dll`。
- 打入安装器内置 payload。
- 安装时离线解压到 `NexTunnel.exe` 同目录。

官方 ZIP SHA256：

```text
07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51
```

显式指定：

```powershell
.\scripts\package-desktop.ps1 `
  -Version v0.6.4-alpha `
  -WintunMode bundled `
  -WintunSha256 "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
```

本地已校验 DLL：

```powershell
.\scripts\package-desktop.ps1 `
  -Version v0.6.4-alpha `
  -WintunDllPath "D:\path\to\wintun.dll"
```

zip 便携包缺少 DLL 时，桌面端网络页会显示 Wintun 状态，并提供修复入口。

## GitHub Release

推送 `v0.6.4-alpha` 标签会触发 `.github/workflows/release.yml`。

发布资产：

```text
nextunnel-v0.6.4-alpha-windows-amd64-installer.exe
nextunnel-v0.6.4-alpha-windows-amd64-installer.exe.sha256
nextunnel-v0.6.4-alpha-windows-amd64-installer.MANIFEST.txt
nextunnel-v0.6.4-alpha-windows-amd64.zip
nextunnel-v0.6.4-alpha-windows-amd64.zip.sha256
nextunnel-v0.6.4-alpha-darwin-universal.dmg
nextunnel-v0.6.4-alpha-darwin-universal.dmg.sha256
nextunnel-v0.6.4-alpha-darwin-universal.MANIFEST.txt
nextunnel-v0.6.4-alpha-darwin-universal.pkg
nextunnel-v0.6.4-alpha-darwin-universal.pkg.sha256
nextunnel-cli-v0.6.4-alpha-linux-amd64.tar.gz
nextunnel-cli-v0.6.4-alpha-linux-arm64.tar.gz
nextunnel-cli-v0.6.4-alpha-windows-amd64.zip
nextunnel-server-linux-amd64.tar.gz
nextunnel-server-linux-arm64.tar.gz
nextunnel-server-windows-amd64.zip
nextunnel-docs-v0.6.4-alpha.tar.gz
install.sh
install.ps1
*.sha256
```

不上传 unpacked 目录、构建缓存、旧版本 exe、日志或临时下载资源。

## 文档站发布

文档站使用 VitePress：

```bash
cd docs
npm run docs:build
```

Release workflow 会打包 `nextunnel-docs-v0.6.4-alpha.tar.gz`，并同步发布到 GitHub Pages。首次启用前需要在仓库 Pages 设置中选择 `GitHub Actions` 发布模式。

站点地址：

```text
https://lee-zg.github.io/NexTunnel/
```

## 发布前检查

必跑：

```bash
go test ./...
cd desktop/frontend && npm run build
cd installer/frontend && npm run build
cd server/web && npm run build
cd docs && npm run docs:build
```

Windows PowerShell 可使用：

```powershell
.\make.ps1 test-go
cd desktop\frontend; npm run build; cd ..\..
cd installer\frontend; npm run build; cd ..\..
cd server\web; npm run build; cd ..\..
cd docs; npm run docs:build; cd ..
```

生产验证按 [生产验证手册](./production-verification.md) 执行：

```bash
make verify-scripts-static
make verify-edge
make verify-tun
make verify-p2p-tun MAC_HOST=mac.example.com MAC_USER=<ssh-user>
DASHBOARD_URL=https://dashboard.example.com DASHBOARD_PASSWORD=<password> make verify-dashboard
DASHBOARD_HOST=server.example.com DASHBOARD_USER=root DASHBOARD_IDENTITY=~/.ssh/id_ed25519 make verify-dashboard-ssh
sudo INTERFACE_NAME=eth0 make verify-ebpf-linux
```

真实 TUN、eBPF 和路由验证会修改系统网络状态，只能在授权实机或隔离节点执行。

所有验证报告必须保留在 `dist/verification/`。CI 会上传脚本静态校验报告，真实生产验收报告需要在执行验证的机器上作为 Release 附件或发布归档保存。

发布说明中的能力状态只使用“开发完成”“本地测试通过”“真实环境功能验收通过”“生产压测通过”“外部阻塞”。没有 JSON 报告的真实 TUN、Dashboard HTTPS、eBPF 压测或多地域拓扑，不能写成生产通过。

## 发布后检查

- Release 页面存在所有安装器、压缩包、manifest 和 SHA256。
- Windows 自定义 Wails 安装器可提权启动，显示安装位置、桌面快捷方式、payload/WebView2/Wintun 状态和完成后立即运行选项。
- Windows zip 包缺少 DLL 时，网络页能显示 Wintun 状态和修复入口。
- macOS DMG 可挂载，包含 `NexTunnel.app`、Applications 链接、README 和 manifest。
- `install.sh` 和 `install.ps1` 可从 Release 下载。
- Linux 一键安装后 `nextunnel server health` 通过。
- Dashboard HTTPS 或 SSH 隧道验证通过。
- 文档站可访问，导航显示 `v0.6.4-alpha`。

## 能力边界

Release notes 必须明确：

- 支持自部署 Relay、Control Plane、Dashboard。
- 支持桌面端 TCP/HTTP 隧道和客户端监控。
- Windows TUN 需要 Wintun 和管理员权限。
- macOS DMG 只声明 P2P/Relay；System TUN 只有 signed/notarized pkg 安装 `com.nextunnel.helper` 且有 `tun-macos-latest.json` 报告后，才能标注真实环境功能验收通过。
- Dashboard 生产 HTTPS 需要可用域名、证书和反向代理。
- eBPF 压测需要隔离 Linux 节点或维护窗口。
- Edge/Anycast 多地域生产拓扑需要真实多地域节点和可归档验证报告。
