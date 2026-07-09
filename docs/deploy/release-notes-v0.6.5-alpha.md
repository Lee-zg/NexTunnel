# NexTunnel v0.6.5-alpha Release Notes

## Summary

v0.6.5-alpha 是一次重新发布版本，重点修复 Windows 自定义安装器在中文系统上安装失败时的乱码错误，并把 macOS 开发机已处理的 System TUN 改动纳入最新 release 资源重新打包。

Windows 安装器现在会正确解码本地化命令输出，避免 `taskkill.exe` 或 PowerShell 返回 GB18030/UTF-16 文本时在界面中显示乱码；同时把“没有找到旧进程”视为可继续状态，避免无旧版本运行时误判为安装失败。

## Capability Status

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| Windows 自定义安装器 | 已修复 | 本地化命令输出统一解码为 UTF-8 展示，中文错误不再乱码。 |
| Windows 升级安装 | 已修复 | `taskkill.exe` 返回“未找到/没有找到进程”时不再中断安装。 |
| Windows Wintun payload | 发布打包 | 继续按官方 Wintun ZIP SHA256 校验后随 Windows payload 打包。 |
| macOS System TUN | 重新打包 | macOS 开发机已处理的 TUN 改动随本版本重新发布；真实生产声明仍以归档 JSON 验收报告为准。 |
| macOS DMG/PKG | 发布打包 | DMG 默认 unsigned alpha；签名、公证和 PKG 仍由 CI secrets 决定。 |
| CLI / Server / Docs | 发布打包 | 默认版本、文档入口、Release workflow 和包元数据同步到 `v0.6.5-alpha`。 |

## Verification

发布前基础门禁：

```bash
go test ./...
cd desktop/frontend && npm run build
cd installer/frontend && npm run build
cd server/web && npm run build
cd docs && npm run docs:build
```

Windows 发布包：

```powershell
.\scripts\package-desktop.ps1 -Version v0.6.5-alpha -WintunMode bundled
.\scripts\package-cli.ps1 -Version v0.6.5-alpha
.\scripts\package-server.ps1 -Version v0.6.5-alpha
```

macOS 发布包需要在 macOS runner 或 macOS 本机执行：

```bash
bash scripts/package-macos.sh --version v0.6.5-alpha
```

macOS System TUN 真实验收仍建议归档：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-p2p-tun.ps1 `
  -MacHost "mac.example.com" `
  -MacUser "<ssh-user>" `
  -MacUseHelper `
  -ReportPath "dist/verification/tun-macos-latest.json"
```

## Release Boundary

v0.6.5-alpha 可以作为公开 alpha 重新发布，修复 Windows 安装器中文乱码和升级误判问题，并发布最新 macOS TUN 处理结果。没有新的 `tun-macos-latest.json`、Dashboard HTTPS、eBPF 压测和真实多地域拓扑报告前，仍不把对应能力描述为生产通过。
