# 生产验证手册

本文档覆盖 Beta 发布前剩余的生产验收项。所有命令都会生成 JSON 报告到 `dist/verification/`，失败时返回非零退出码。

## 报告与状态口径

发布前必须归档 `dist/verification/` 下的报告，并在 CI artifact 或 Release 附件中保留。默认报告命名如下：

| 验证项 | 默认报告 |
| --- | --- |
| 脚本解析与参数契约静态校验 | `dist/verification/verification-scripts-static-latest.json` |
| Dashboard 公网 HTTPS | `dist/verification/dashboard-https-latest.json` |
| Dashboard SSH 隧道 | `dist/verification/dashboard-ssh-latest.json` |
| 本机 TUN | `dist/verification/tun-local-latest.json` |
| Windows/macOS P2P/TUN | `dist/verification/tun-windows-macos-latest.json` |
| eBPF 功能/压测 | `dist/verification/ebpf-benchmark-latest.json` |
| Edge/Anycast 演练 | `dist/verification/edge-rehearsal-latest.json` |

Release notes 必须使用以下状态，不得把外部条件未满足的能力写成默认生产可用：

| 状态 | 含义 |
| --- | --- |
| 开发完成 | 代码、配置入口和本地测试已完成，但未进入真实环境验收。 |
| 本地测试通过 | 单元/集成/构建在本地或 CI 通过。 |
| 真实环境功能验收通过 | 在授权实机或服务器上完成脚本验证，有 JSON 报告。 |
| 生产压测通过 | 有隔离压测数据，包含吞吐、延迟、CPU 和清理结果。 |
| 外部阻塞 | 域名、证书、驱动、权限、资源或维护窗口尚未满足。 |

## v0.6.4-alpha 验证进度

截至 2026-07-02，工程侧已完成生产验证入口、报告结构、故障前置检查和 macOS LaunchDaemon helper 链路，并在两台公网服务器上推进了 Dashboard、eBPF 和 Edge 演练。剩余实机验收以域名证书、系统驱动、管理员权限、签名公证和授权环境准备为主：

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| Dashboard 端到端部署联调 | API 已通过，HTTPS 阻塞 | 服务器二通过 SSH 加密通道完成健康检查、登录、token、401、CORS、节点、ACL、告警和静态入口验证；公网 HTTPS 仍阻塞于 `lee97.top` 证书过期及 DNSPod webblock，需修复域名/证书后复验。 |
| Windows/macOS P2P 直连 | 已通过局域网双端验证 | Mac 回调 Windows 不可达时，验证器可由 Windows 主动推/拉候选完成直连链路。 |
| Windows 真实 TUN | 环境阻塞 | 当前环境缺少匹配架构 `wintun.dll`；生产可用前必须随包或安装器提供，并以管理员权限运行。 |
| macOS 真实 TUN | helper 链路已实现，待实机验收 | signed/notarized pkg 会安装 `nextunnel-helper` 和 `com.nextunnel.helper` LaunchDaemon；生产通过前仍需 macOS 实机安装 pkg 并归档 `tun-macos-latest.json`。 |
| Linux eBPF XDP | 功能验收已通过 | 服务器二 Linux 6.8、`eth0`、`skb` 模式完成 BPF 对象编译、XDP 挂载、DROP 规则同步、统计读取和卸载；吞吐/延迟压力基准仍需隔离窗口补充。 |
| 多地域 Edge/Anycast | 远端 Control Plane 演练已通过 | 本地 3 区域演练和服务器二真实 Control Plane 注册/心跳/清理均通过；商用生产仍需真实多地域节点与观测指标压测。 |

发布边界：v0.6.4-alpha 可以声明生产验证工具链、Relay Admin API、Dashboard 客户端监控和故障诊断能力已齐备，P2P 直连链路、Dashboard API、eBPF XDP 功能挂载和 Edge/Anycast 远端注册链路已验证；真实系统 TUN、Dashboard 公网 HTTPS、eBPF 压力基准和真实多地域拓扑仍需在具备权限和依赖的生产或隔离环境完成最终验收。README 与用户手册只按这个边界描述能力，不把外部条件未满足的能力写成默认生产可用。

阻塞项最佳处理方案：

- Dashboard HTTPS：生产验收必须使用备案且能正常解析到服务器的域名，配置 Nginx/OpenResty 反向代理和有效证书。当前 `lee97.top` 被 DNSPod webblock，`*.sslip.io` 被阿里云 ICP 拦截，均不适合作为 HTTPS 验收域名。
- Dashboard 受限环境验证：在域名/证书不可用时，使用 `scripts/verify-dashboard-ssh.ps1` 通过 SSH 隧道验证 API；`scripts/verify-dashboard.ps1` 默认拒绝向非本机 HTTP 发送管理员密码。
- Windows TUN：发布包或安装器应随附官方、匹配架构的 `wintun.dll`，放在 EXE 同目录；也可通过 `NEXTUNNEL_WINTUN_DLL` 或 `-WintunDllPath` 指定来源后重新打包/验证。
- macOS TUN：生产使用 signed/notarized pkg 安装 LaunchDaemon helper 创建 utun 并注入路由；验证环境仍可配置 `sudo -n` 后使用 `-MacUseSudo`。
- eBPF 压测：功能验收已通过，吞吐/延迟压力基准应放在隔离节点或维护窗口执行。

最新报告：

- `dist/verification/dashboard-server2-ssh-script-report.json`
- `dist/verification/ebpf-linux-server2-report.json`
- `dist/verification/edge-rehearsal-local-report.json`
- `dist/verification/edge-rehearsal-server2-remote-report.json`

## Dashboard 端到端部署联调

前置条件：

- Dashboard 已部署到真实 HTTP/HTTPS 地址。
- 已配置强 `DASHBOARD_SECRET_KEY`、管理员密码和 CORS 白名单。
- 若通过反向代理提供 HTTPS，外部访问地址应使用 HTTPS URL。

执行：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-dashboard.ps1 `
  -BaseUrl "https://dashboard.example.com" `
  -Username "admin" `
  -Password "<强管理员密码>" `
  -AllowedOrigin "https://dashboard.example.com" `
  -ReportPath "dist/verification/dashboard-https-latest.json"
```

没有可用 HTTPS 域名时，使用 SSH 隧道验证，避免管理员密码经过公网 HTTP：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-dashboard-ssh.ps1 `
  -SshHost "47.116.218.140" `
  -User "root" `
  -IdentityFile "$env:USERPROFILE\.ssh\id_ed25519" `
  -RemoteDashboardPort 8080 `
  -AllowedOrigin "http://47.116.218.140:8080" `
  -ReportPath "dist/verification/dashboard-ssh-latest.json"
```

验收点：

- `/api/v1/health` 返回 `ok`。
- 管理员可登录，合法 token 能访问节点、统计、ACL、告警接口。
- 无效 token 返回 401，前端应清理本地 token 并回到登录态。
- CORS 只允许白名单 Origin。
- 验证脚本会创建并删除临时 ACL 与告警规则。

## P2P/TUN 生产化验证

前置条件：

- Windows 使用管理员权限并安装/允许 Wintun。
- macOS 用户 `lizhigang` 先完成一次临时公钥接入。
- 执行前确认验证路由不会与当前办公/VPN 网段冲突。
- Windows 缺少 System32 或应用目录 `wintun.dll` 时，通过 `NEXTUNNEL_WINTUN_DLL` 或 `-WintunDllPath` 指向官方 DLL；桌面发布包会在打包时自动复制该 DLL。
- macOS 真实 utun 和路由验证优先使用 pkg 安装的 LaunchDaemon helper；过渡环境可用 `sudo -n`。没有 helper 或 sudo 时只能验证 P2P 候选交换，不应宣称系统路由 TUN 已生产可用。

执行：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-p2p-tun.ps1 `
  -MacHost "10.160.166.44" `
  -MacUser "lizhigang" `
  -MacUseHelper `
  -WintunDllPath "D:\path\to\wintun.dll"
```

未安装 pkg helper 的过渡验证环境，可把 `-MacUseHelper` 换成 `-MacUseSudo`，并确保远端 `sudo -n true` 可用。

首次运行若 Mac 还没有临时公钥，会输出一次性 bootstrap 命令。先在本机执行该命令把公钥写入 Mac 的 `authorized_keys`，再重新运行上面的验证脚本。

验收点：

- Windows 侧真实 Wintun 创建成功，设备名不能是 `netTun`。
- Windows 侧路由注入成功，结束后无残留路由。
- macOS 侧 utun 创建成功，路由应用与清理成功。
- 双端 ICE 候选交换成功，直连优先，必要时可回落 Relay。
- 临时 SSH 公钥、远端临时文件和临时路由全部清理。
- 报告会输出 Windows 与 macOS 两端的 JSON 汇总到 `dist/verification/`。
- 若报告出现 `wintun_dll_missing`、`privilege_required`、`linux_dev_net_tun_missing` 等阻塞项，应先修复环境，不要用 `netTun` 回退结果替代生产验收。

验证完成后清理临时 SSH 公钥和远端临时目录：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-p2p-tun.ps1 `
  -MacHost "10.160.166.44" `
  -MacUser "lizhigang" `
  -CleanupOnly
```

## eBPF Linux 生产验证

前置条件：

- 只能在 Linux 节点执行。
- 使用 root 或等价 `CAP_BPF`、`CAP_NET_ADMIN` 权限。
- 已安装 `clang` 和 Go。
- 在隔离节点或维护窗口执行，避免 XDP 挂载影响生产流量。

执行：

```bash
sudo INTERFACE_NAME=eth0 XDP_MODE=skb REPORT_PATH=dist/verification/ebpf-benchmark-latest.json \
  bash scripts/verify-ebpf-linux.sh
```

验收点：

- `load_xdp` 显示 `mode=kernel`。
- 临时 DROP 规则可以同步到内核 map。
- `stats_read` 能读取内核/用户态统计。
- `unload_xdp` 成功卸载，执行后确认网卡无残留 XDP 程序。

## 多地域边缘部署演练

本地模拟演练：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-edge-rehearsal.ps1
```

连真实 Control Plane 演练：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-edge-rehearsal.ps1 `
  -ControlUrl "https://control.example.com" `
  -ControlToken "<control-plane-token>" `
  -RegisterRemote
```

验收点：

- Edge Registry 能注册 3 个区域节点。
- Anycast 能按客户端坐标选择最近节点。
- 节点故障后能切换到备用节点。
- GeoIP 静态映射能触发路由偏移。
- 真实 Control Plane 模式会注册、心跳并清理临时节点。

## 发布前清理

发布前执行：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-scripts-static.ps1
git status --short --branch
Get-FileHash -Algorithm MD5 desktop/frontend/package.json
Get-Content desktop/frontend/package.json.md5
```

`desktop/frontend/package.json.md5` 必须等于当前 `package.json` 的 MD5 小写值。若没有修改 `package.json`，不要单独提交无来源的 md5 变化。
