# NexTunnel v0.7.0-beta Release Notes

## Summary

v0.7.0-beta 是面向公开 Beta / 自部署可用性的收口版本，核心补齐“本地 HTTP 服务发布为稳定公开 URL”的主路径。Relay 新增 Public HTTP Gateway，Dashboard、桌面端和 CLI 增加 Public Endpoint 创建与查看入口，并把 Endpoint 访问策略、请求级观测和发布验证脚本纳入同一条验收链路。

本版本仍保持自部署优先：域名和证书由部署者提供，ACME、文件证书或外部反代可以按环境选择。旧 TCP 隧道和旧客户端字段保持兼容，未完成真实环境 JSON 验收的 P2P/TUN、eBPF 压测和多地域 Edge 能力不写入生产通过声明。

## Capability Status

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| Public HTTP Gateway | Beta | Relay 可按 Host 路由公开 HTTP Endpoint，TCP 隧道继续保持远端端口模型。 |
| Endpoint 域名与公开 URL | Beta | 支持 `domain_suffix`、子域名冲突校验、`host_header` 和 Relay 返回 `public_url`。 |
| Endpoint 访问策略 | Beta | 支持 Basic Auth、Bearer Token、IP allow/deny、时间窗、限流、并发上限和过期时间。 |
| HTTP 请求观测 | Beta | 记录方法、Host、Path、状态码、延迟、字节数、客户端 IP、策略结果和隧道名；默认不保存 body。 |
| Dashboard Endpoint 页 | Beta | 新增 Endpoint、策略和请求日志运营视图，支持复制公开 URL、筛选请求和查看拒绝原因。 |
| 桌面端 publish | Beta | 新增本地发布入口和 `nextunnel desktop publish` 快捷命令，返回最终 URL 与策略状态。 |
| 发布验证脚本 | Beta | 新增 `scripts/verify-public-endpoint.ps1`，输出可归档 JSON 报告作为发布依据。 |
| P2P/TUN、eBPF、Edge | 报告门禁 | 本轮不扩大功能面；真实通过仍以 `dist/verification/*.json` 归档报告为准。 |

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
.\scripts\package-desktop.ps1 -Version v0.7.0-beta -WintunMode bundled
.\scripts\package-cli.ps1 -Version v0.7.0-beta
.\scripts\package-server.ps1 -Version v0.7.0-beta
```

Public Endpoint 自部署验收：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-public-endpoint.ps1 `
  -GatewayUrl "https://demo.example.com" `
  -ExpectedContains "ok" `
  -BasicUsername "demo" `
  -BasicPassword "<password>" `
  -ReportPath "dist/verification/public-endpoint-latest.json"
```

macOS 发布包需要在 macOS runner 或 macOS 本机执行：

```bash
bash scripts/package-macos.sh --version v0.7.0-beta
```

## Release Boundary

v0.7.0-beta 可以声明 Public Endpoint 主干进入 Beta：Relay Gateway、Endpoint Policy、请求日志、Dashboard/桌面端/CLI 创建入口和本地验证脚本已形成闭环。生产可用性声明必须基于归档 JSON 报告；没有真实域名 HTTPS、macOS/Windows TUN、eBPF 压测和多地域 Edge 报告前，不把这些能力标成生产通过。
