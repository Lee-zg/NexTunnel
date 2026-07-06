# NexTunnel v0.6.4-alpha Release Notes

## Summary

v0.6.4-alpha 将 macOS System TUN 调整为更适合开源项目的可选 helper 形态。macOS 桌面端默认普通权限运行；用户可在网络页授权安装官方内置 LaunchDaemon helper，或通过 `sudo nextunnel helper install` 透明安装，由 root helper 创建 utun、通过 Unix fd passing 交给桌面端，并受控应用/清理路由。

本版本不能声明 macOS System TUN 已生产通过。signed/notarized PKG 保留为体验增强通道，不再是开源默认能力前置；只有在授权 macOS 实机安装 helper，并归档 `dist/verification/tun-macos-latest.json` 后，才能把该能力升级为“真实环境功能验收通过”。

## Capability Status

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| macOS System TUN helper | 开发完成 | `nextunnel-helper`、LaunchDaemon、Unix socket 协议、fd passing、helper-backed route applier 已接入。 |
| macOS System TUN 本地门禁 | 本地测试通过 | helper 请求校验、默认路由拒绝、peer credential 授权、桌面集成和验证器构建已通过本地测试。 |
| macOS System TUN 真实环境 | 外部阻塞 | 仍需授权实机安装 helper、确认 LaunchDaemon 自启动、utun 创建、路由注入/清理和 JSON 报告归档。 |
| macOS DMG | 开源默认入口 | DMG 内置 helper 资源和网络页安装按钮，用户主动输入管理员密码后启用 System TUN。 |
| macOS CLI/Homebrew | 开源透明入口 | `sudo nextunnel helper install/status/restart/uninstall` 管理固定官方 helper。 |
| macOS signed/notarized PKG | 可选增强 | 一键安装体验最好，但需要 Apple Developer Program，不阻塞源码、Homebrew 或 unsigned DMG 路径。 |
| 桌面端网络页 | 已修复 | 补齐 Naive UI Dialog Provider，避免进入网络页时因 helper 安装确认框上下文缺失导致黑屏，并验证可继续切换到日志/设置页。 |
| Windows System TUN | 实机验收待补 | 打包链路支持随包注入官方 `wintun.dll`，仍需 Windows 实机管理员权限验收报告。 |
| Dashboard HTTPS | 外部阻塞 | API/SSH 隧道验证可用，公网 HTTPS 仍需域名、证书和反向代理复验。 |
| eBPF/Edge | 功能/演练通过 | eBPF 功能挂载和 Edge 远端注册演练已通过，生产压测和多地域拓扑仍需真实资源。 |

## macOS System TUN Notes

- helper 安装路径：`/Library/PrivilegedHelperTools/nextunnel-helper`
- LaunchDaemon：`/Library/LaunchDaemons/com.nextunnel.helper.plist`
- socket：`/var/run/nextunnel/helper.sock`
- socket 权限：`root:admin 0660`
- helper 接口：`status`、`create_tun`、`apply_routes`、`reset_routes`
- 安全边界：不提供任意 shell；默认拒绝 `0.0.0.0/0` 和 `::/0` 默认路由。
- 安装方式：网络页管理员授权安装，或 `sudo nextunnel helper install`；不支持第三方动态 root 插件。

## Verification

发布前基础门禁：

```bash
go test ./...
go vet ./...
go build ./...
cd desktop/frontend && npm run lint && npm run build
cd server/web && npm run build
cd docs && npm run docs:build
bash -n scripts/package-macos.sh
plutil -lint packaging/macos/com.nextunnel.helper.plist
```

macOS System TUN 真实验收：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-p2p-tun.ps1 `
  -MacHost "mac.example.com" `
  -MacUser "<ssh-user>" `
  -MacUseHelper `
  -ReportPath "dist/verification/tun-macos-latest.json"
```

验收报告不得出现 `macos_helper_missing`、`macos_helper_unreachable`、`privilege_required`、`wintun_dll_missing` 等阻塞项。

## Release Boundary

v0.6.4-alpha 可以作为公开 alpha 发布，重点是让用户和 CI 拿到 macOS helper 验收入口。它不是 beta：没有真实 macOS helper/TUN 验收报告、Dashboard 公网 HTTPS 报告、eBPF 压测报告和真实多地域拓扑报告前，不应发布为 `v0.7.0-beta`。
