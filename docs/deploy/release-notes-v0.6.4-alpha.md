# NexTunnel v0.6.4-alpha Release Notes

## Summary

v0.6.4-alpha 将 macOS System TUN 推进到可实机验收的 alpha 状态。macOS 不再依赖普通桌面进程直接创建 utun 或交互式 sudo；新增 signed/notarized pkg 预期安装的 LaunchDaemon helper，由 root helper 创建 utun、通过 Unix fd passing 交给桌面端，并受控应用/清理路由。

本版本不能声明 macOS System TUN 已生产通过。只有安装 signed/notarized pkg，并在 macOS 实机归档 `dist/verification/tun-macos-latest.json` 后，才能把该能力升级为“真实环境功能验收通过”。

## Capability Status

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| macOS System TUN helper | 开发完成 | `nextunnel-helper`、LaunchDaemon、Unix socket 协议、fd passing、helper-backed route applier 已接入。 |
| macOS System TUN 本地门禁 | 本地测试通过 | helper 请求校验、默认路由拒绝、peer credential 授权、桌面集成和验证器构建已通过本地测试。 |
| macOS System TUN 真实环境 | 外部阻塞 | 仍需 Developer ID 签名/公证、pkg 实机安装、LaunchDaemon 自启动、utun 创建、路由注入/清理和 JSON 报告归档。 |
| macOS DMG | P2P/Relay-only | DMG 继续提供普通安装形态，不声明 System TUN 生产可用。 |
| macOS PKG | 验收入口 | `.pkg` 安装 App、helper 和 LaunchDaemon，是 System TUN 验收路径。 |
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

v0.6.4-alpha 可以作为公开 alpha 发布，重点是让用户和 CI 拿到 macOS helper/pkg 验收入口。它不是 beta：没有真实 macOS pkg 验收报告、Dashboard 公网 HTTPS 报告、eBPF 压测报告和真实多地域拓扑报告前，不应发布为 `v0.7.0-beta`。
