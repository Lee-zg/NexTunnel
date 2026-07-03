# NexTunnel 项目进度跟踪

> **最后更新时间**：2026-07-02（v0.6.4-alpha 发布准备：macOS System TUN helper、pkg 打包和版本口径同步）
> **当前口径**：按"可验收 MVP / 原型验证 / 占位设计 / 未接入生产链路"统计，避免把单元测试通过等同于生产完成。
> **本次更新重点**：版本口径更新到 v0.6.4-alpha，已同步桌面端、Dashboard、CLI、服务端打包脚本、前端包元数据、README、部署手册、发布说明和生产验证文档；macOS System TUN 新增 LaunchDaemon helper、pkg 安装链路、预检状态和 `-MacUseHelper` 验证参数。真实生产外部验收清单继续保留，Dashboard HTTPS、Windows/macOS 真实 TUN、eBPF 吞吐/延迟基准和真实多地域 Edge/Anycast 均需外部资源满足后复验。

---

## 总体进度概览

```
Phase 1 [基础隧道]  ████████████████████  98%  MVP+安全基线+mTLS+审计已就绪
Phase 2 [P2P直连]   █████████████████░░░  85%  P2P直连已验证，真实TUN仍需root/驱动验证
Phase 3 [智能调度]  ███████████████░░░░░  75%  调度器数据面闭环已验证，真实弱网/多路径压测待执行
Phase 4 [全球加速]  ████████████████████  98%  eBPF XDP与Edge远端演练通过，Dashboard HTTPS待域名证书修复
─────────────────────────────────────────────
总体开发进度        ████████████████████  100%
生产验证进度        ███████████████████░  94%
```

---

## 阶段里程碑状态

| 阶段 | 名称 | 当前状态 | 完成度 | 说明 |
|:---:|:---|:---:|:---:|:---|
| Phase 1 | 基础隧道 | ✅ MVP+安全基线 | 98% | TCP Relay、客户端配置、SQLite、重连和桌面基础流程可用；安全配置、持久化、mTLS、审计已就绪 |
| Phase 2 | P2P 直连 | ✅ 开发闭环 | 85% | NAT/STUN/ICE/WireGuard 编排有测试；TUNDevice 抽象、控制面 IPAM 自动分配、路由下发已完成；真实 TUN 需权限验证 |
| Phase 3 | 智能调度 | ✅ 数据面闭环 | 75% | 调度器+RelayManager+Migrator 三接口驱动真实路径切换已验证；QUIC work stream E2E 就绪；真实弱网/多路径压测待执行 |
| Phase 4 | 全球加速 | ✅ 生产验证推进 | 98% | 安全、持久化、CI、QUIC E2E、TUN抽象、调度闭环、Edge↔CP、Dashboard、GeoIP、eBPF XDP 代码已完成；eBPF Linux 真实网卡验证和 Edge 远端 Control Plane 演练已通过；Dashboard HTTPS 仍待域名/证书修复 |

---

## 当前已落地事实

| 领域 | 已实现 | 当前限制 |
|:---|:---|:---|
| Relay TCP 数据面 | 控制连接、工作连接、代理监听、会话桥接、基础统计 | 生产部署仍需完整认证策略、限流、审计、配置管理 |
| Relay QUIC 数据面 | QUIC listener 可启动，work stream 握手后接入既有会话桥接；客户端 QUIC WorkConn 已完成 E2E，默认 TLS 1.3 且不跳过证书校验 | 生产证书信任链、超时/限流策略和真实网络压测待验证 |
| 控制面 | HTTP API 支持节点注册、心跳、Peer 查询、ACL、密钥、自动 IPAM、节点路由下发；支持 Bearer Token、mTLS、SQLite 持久化恢复、审计日志 | 尚未实现 gRPC/proto、OIDC、实时 ACL 下发 |
| Dashboard 后端/前端 | REST API、bcrypt 密码、可配置 CORS、启动期弱配置拒绝、SQLiteDashboardStore、Vue Dashboard 登录/节点/流量/ACL/告警界面；`cmd/dashboard` 独立进程、静态资源托管、Release 打包和一键部署已接入；服务器二 API 端到端验证通过；受限环境支持 SSH 隧道验证且默认拒绝非本机 HTTP 管理员密码传输 | HTTPS 反向代理仍受域名证书阻塞：`lee97.top` 证书已过期且 Let's Encrypt HTTP-01 被 DNSPod webblock 拦截；`sslip.io` 临时域名在服务器二公网侧被阿里云 ICP 拦截 |
| 桌面端 | 连接/断开服务器、创建/删除配置、启动/停止单隧道、状态刷新 | 仍需更完整的错误可视化、Relay/STUN 配置持久化、P2P 操作流 |
| P2P/TUN | NAT/STUN/ICE/WireGuard/Mesh 测试原型；TUN 平台抽象、虚拟 IP 分配和路由下发已完成；Windows Wintun 打包链路和 macOS LaunchDaemon helper 链路已接入；预检已输出 Windows Wintun、macOS helper/LaunchDaemon、Linux CAP_NET_ADMIN 等环境方案 | 真实 OS TUN 仍需 Windows/macOS 授权实机验收、签名/公证和 JSON 报告归档 |
| 调度器 | scheduler/relay/migration setter 已替换为强类型接口，路径切换闭环有集成测试 | 真实弱网和多路径迁移压测待执行 |
| 质量门禁 | Go 版本统一到 1.25.0；Go 测试/构建/vet 过滤 `frontend/node_modules` 与 `web/node_modules`；前端补 `test` 脚本 | 当前 shell 无可用 `npm`，已用本地 `vue-tsc` CLI 完成等价验证 |

---

## Phase 4 模块详细状态（本次迭代重点）

### P4-T01 边缘节点部署（`server/internal/edge/`）

| 子任务 | 状态 | 说明 |
|:---|:---:|:---|
| P4-T01-1 边缘节点数据模型（EdgeNode/Region/Status） | ✅ 已实现 | `node.go` — EdgeNode 结构体、Region/Status 枚举、原子状态管理、失败计数 |
| P4-T01-2 边缘节点注册表（Registry CRUD + 按区域索引） | ✅ 已实现 | `registry.go` — Register/Deregister/Get/List/ListByRegion/ListHealthy/CountHealthy/Regions，带回调 |
| P4-T01-3 健康检查器（心跳探测、自动摘除） | ✅ 已实现 | `health.go` — TCP 探测、并发检查、连续失败阈值判定、自动摘除与恢复、超时自动注销 |
| P4-T01-4 部署自动化（Docker Compose 模板 + 部署清单） | ✅ 已实现 | `deploy.go` — Go template 生成 Docker Compose、DeployManifest 部署指引 |
| P4-T01-5 Control Plane 集成（边缘节点自动注册） | ✅ 已实现 | `controlplane.go` — ControlPlaneClient HTTP 注册/心跳、ConnectRegistry 回调接入、StartHeartbeatLoop/StopAll |
| P4-T01-x 单元测试 | ✅ 已实现 | `node_test.go`（362 行）+ `controlplane_test.go`（223 行）覆盖 EdgeNode 状态机、Registry CRUD、健康检查、CP 注册/心跳全流程 |

**结论**：后端骨架完整，核心数据模型、注册表、健康检查、部署模板、Control Plane 集成均已到位；2026-06-18 已完成本地 3 区域演练和服务器二真实 Control Plane 远端注册/心跳/清理演练，后续若进入商用生产仍需真实多地域节点和观测指标压测。

### P4-T02 Anycast 路由（`server/internal/anycast/`）

| 子任务 | 状态 | 说明 |
|:---|:---:|:---|
| P4-T02-1 Anycast 路由器核心逻辑 | ✅ 已实现 | `router.go` — Haversine 距离计算、SelectNearest/SelectNearestWithFailover/SelectByRegion |
| P4-T02-2 GeoDNS 解析引导 | ✅ 已实现 | `router.go` + `geoip.go` — GeoIPProvider 接口、MaxMindAdapter（CIDR 匹配 + 静态映射）、GeoIPRouter 封装 |
| P4-T02-3 故障自动切换 | ✅ 已实现 | SelectNearestWithFailover + GeoIPRouter.SelectNearestWithFailoverForIP 支持多节点故障转移 |
| P4-T02-x 单元测试 | ✅ 已实现 | `router_test.go`（220 行）覆盖距离计算、故障转移、GeoDNS 解析 |

**结论**：路由核心逻辑和 GeoDNS 原型完整，GeoIPProvider 接口已抽象、MaxMindAdapter 支持 CIDR 匹配；生产环境需加载真实 .mmdb 数据库文件。

### P4-T03 eBPF 网络加速（`server/internal/ebpf/`）

| 子任务 | 状态 | 说明 |
|:---|:---:|:---|
| P4-T03-1 eBPF 程序加载器 | ✅ 开发完成 | `loader_linux.go` — 接入 `github.com/cilium/ebpf`，支持加载 XDP ELF 对象、`link.AttachXDP` 挂载、`RequireKernelMode` 强制内核态和失败降级 |
| P4-T03-2 XDP 转发程序 | ✅ 开发完成 | `xdp_forwarder.c` + `kernel_rules.go` — 实现 IPv4 TCP/UDP 目标端口级 PASS/DROP/REDIRECT fast path，复杂 CIDR/源端口/优先级遮挡规则保留用户态 |
| P4-T03-3 优雅降级机制 | ✅ 已实现 | `loader_other.go`（非 Linux）+ Linux 内核态加载失败自动回落 ModeUserspace；强制内核态配置下返回明确错误 |
| P4-T03-4 性能监控 | ✅ 开发完成 | StartStats 定期日志、用户态原子计数与 `xdp_stats_map` 内核统计合并读取 |
| P4-T03-x 单元测试 | ✅ 已实现 | `loader_test.go` + `kernel_rules_test.go` 覆盖创建、加载、模式切换、统计、XDP L4 规则编码和优先级遮挡 |

**结论**：eBPF XDP 内核态加载路径、L4 fast path 规则同步、用户态降级和统计读取已完成开发；2026-06-18 已在服务器二 Linux 6.8、`eth0`、`skb` 模式完成真实挂载、DROP 规则同步、统计读取和卸载验证。当前报告为功能验收，吞吐/延迟压力基准仍需隔离维护窗口补充。

### P4-T04 SD-WAN 流量策略（`server/internal/sdwan/`）

| 子任务 | 状态 | 说明 |
|:---|:---:|:---|
| P4-T04-1 流量分类器 | ✅ 已实现 | `classifier.go` — Classifier 基于端口/协议映射 AppType（HTTP/SSH/RDP/DNS 等） |
| P4-T04-2 策略引擎 | ✅ 已实现 | `policy.go` — PolicyEngine 规则 CRUD、优先级排序、Evaluate 匹配（AppType/Protocol/SrcNode/DstPort） |
| P4-T04-3 QoS 管理器 | ✅ 已实现 | `qos.go` — 8 级 heap 优先级队列、TokenBucket 令牌桶限速、QoSManager 入队/出队/批量/统计 |
| P4-T04-4 策略热更新 | ✅ 已实现 | UpdateRule 原子替换 + rebuildSorted，规则修改即时生效 |
| P4-T04-x 单元测试 | ✅ 已实现 | `policy_test.go`（227 行）覆盖规则增删改、优先级匹配、Evaluate 全流程 |

**结论**：策略引擎、分类器、QoS 队列和令牌桶限速全部完成；规则 CRUD、匹配、优先级队列、流量整形均已实现。

### P4-T05 管理控制台（`server/internal/dashboard/` + `server/web/`）

| 子任务 | 状态 | 说明 |
|:---|:---:|:---|
| P4-T05-1 HTTP API 服务器 | ✅ 已实现 | `server.go` — 路由注册、CORS 白名单中间件、JSON 响应封装、优雅启停 |
| P4-T05-2 管理员认证 | ✅ 已实现 | `auth.go` — JWT 签发/验证、bcrypt 密码、Login/Refresh/角色、弱密码拒绝 |
| P4-T05-3 节点管理 API | ✅ 已实现 | GET/GET-by-ID/DELETE /api/v1/nodes |
| P4-T05-4 流量统计 API | ✅ 已实现 | GET /api/v1/stats、GET /api/v1/stats/{node_id} |
| P4-T05-5 ACL 管理 API | ✅ 已实现 | GET/POST/DELETE /api/v1/acl |
| P4-T05-6 告警通知 API | ✅ 已实现 | `alerts.go` — AlertEngine 规则引擎、Evaluate/冷却/节点过滤、LogNotifier + WebhookNotifier、POST /api/v1/metrics 指标摄入、POST /api/v1/alert-rules CRUD |
| P4-T05-x Web 前端 | ✅ 开发完成 | `server/web/src/App.vue` 接入登录、健康检查、节点地图、流量图表、节点表、ACL 新增/删除、告警确认；拆分 `api.ts`、`types.ts`、`formatters.ts` |
| P4-T05-y 单元测试 | ✅ 已实现 | `server_test.go`（246 行）覆盖登录、节点/ACL/告警 CRUD、CORS、认证中间件 |

**结论**：后端 API 完整，认证、节点、ACL、统计、告警规则引擎、Webhook 通知、指标摄入端点全部可用；前端 Dashboard 已从静态原型升级为可操作控制台。独立 `cmd/dashboard`、SQLiteDashboardStore 启动链路、静态资源托管、Release 打包和一键部署管理已补齐；2026-06-18 服务器二通过 SSH 加密通道完成健康检查、登录、401、CORS、节点、统计、ACL、告警和静态入口验证。公网 HTTPS 反代验收仍待域名和证书条件恢复。

---

## Phase 4 迭代开发计划（2026-06-08 启动）

基于 Phase 3 验收通过和 Phase 4 当前模块状态，制定以下迭代路线：

> **完成确认（2026-06-18）**：迭代 1「生产基线加固」、迭代 2「数据面闭环」、迭代 3「生产能力完善」均已完成开发验收；I3-02 eBPF XDP、I3-04 Dashboard 前端和 Dashboard 生产启动链路已补齐。服务端/客户端最小连接已在真实服务器测试成功；服务器二已通过 Dashboard API、eBPF XDP 真实网卡挂载和 Edge 远端 Control Plane 演练。剩余为 Dashboard HTTPS 域名证书修复、真实 TUN 环境补齐和弱网/多路径压力基准。

### 迭代 1：生产基线加固（P0，预计 2 周）

| 编号 | 任务 | 验收标准 | 涉及模块 |
|:---:|:---|:---|:---|
| I1-01 | Control Plane 持久化 | Store 接口接入 SQLite 或 PostgreSQL；MemoryStore 仅用于测试 | `server/internal/controlplane/` |
| I1-02 | Relay 安全基线 | 控制/工作连接必须配置认证 token；生产环境禁止空 token | `server/internal/relay/` |
| I1-03 | Dashboard 安全加固 | 启动期拒绝弱密码和空 secret；密码 bcrypt 存储；CORS 白名单 | `server/internal/dashboard/` |
| I1-04 | 质量门禁 CI 化 | `go test ./...`、`go vet ./...`、`go build ./...` 在 CI Pipeline 稳定通过 | `.github/workflows/` |

### 迭代 2：数据面闭环（P1，预计 3 周）

| 编号 | 任务 | 验收标准 | 涉及模块 |
|:---:|:---|:---|:---|
| I2-01 | QUIC Relay E2E | 客户端通过 QUIC work stream 完成一条 TCP 隧道端到端转发 | `desktop/internal/quic/` + `server/internal/relay/` |
| I2-02 | P2P/TUN 平台抽象 | 引入真实 TUN 接口或 userspace netstack MVP，明确 Win/Mac/Linux 实现计划 | `desktop/internal/p2p/` |
| I2-03 | 调度器数据面闭环 | PathManager/RelaySelector/MigrationController 能真实触发 TCP Relay/QUIC/P2P 路径切换，并有集成测试 | `desktop/internal/scheduler/` + `p2p/engine.go` |
| I2-04 | Edge↔CP 集成 | 边缘节点 Registry 回调接入 Control Plane HTTP API，实现自动注册/注销 | `server/internal/edge/` + `controlplane/` |

### 迭代 3：生产能力完善（P2，预计 4 周）

| 编号 | 任务 | 验收标准 | 涉及模块 |
|:---:|:---|:---|:---|
| I3-01 | Dashboard 持久化 | 用户/token/节点/ACL/告警接入 SQLite/PostgreSQL；补 RBAC 测试 | `server/internal/dashboard/` |
| I3-02 | eBPF XDP 内核态 | ✅ 开发完成：eBPF C 程序 + cilium/ebpf 加载 + XDP L4 fast path + 用户态降级；Linux 生产基准待部署环境执行 | `server/internal/ebpf/` |
| I3-03 | GeoIP 数据库 | MaxMindAdapter 加载真实 .mmdb 文件，替换静态映射 | `server/internal/anycast/` |
| I3-04 | Dashboard 前端 | ✅ 开发完成：App.vue 接入节点地图、流量图表、节点表、ACL 编辑表单、告警确认和 API 客户端 | `server/web/` |
| I3-05 | 网络场景测试 | localhost 双节点 Docker、Relay 降级、连接中断重连、QUIC 证书失败路径 | 测试套件 |

---

## 近期 TODO

| 优先级 | 任务 | 验收口径 |
|:---:|:---|:---|
| P0 | 标准质量门禁固化 | ✅ 已完成：Makefile/CI 已过滤前端 node_modules；Go test/vet/build 和前端 vue-tsc 已通过 |
| P1 | Dashboard 端到端部署联调 | API 验证已通过；已提供 SSH 隧道安全验证入口；HTTPS 反代仍需使用备案/可用域名修复证书后复验 |
| P1 | eBPF Linux 生产验证 | ✅ 功能验收已通过：服务器二 `eth0`/`skb` 模式挂载、规则同步、统计读取、卸载成功；吞吐/延迟压力基准待补充 |
| P1 | 多地域边缘部署演练 | ✅ 本地演练和服务器二真实 Control Plane 注册/心跳/清理已通过；商用生产仍需真实多地域节点压测 |
| P2 | P2P/TUN 生产化 | IPAM、路由下发、Windows Wintun 打包链路和 macOS LaunchDaemon helper 已完成；真实 OS TUN 与跨平台系统路由仍需实机权限验证和报告归档 |

---

## 真实生产外部验收待办（2026-07-02）

以下事项不是当前代码阻塞，而是需要外部域名、证书、驱动、系统权限、隔离压测窗口或多地域资源。满足条件后再执行对应验证脚本，并把 JSON 报告归档到 `dist/verification/`。

| 优先级 | 生产验收项 | 当前状态 | 所需外部条件 | 执行入口 | 通过标准 |
|:---:|:---|:---|:---|:---|:---|
| P0 | Dashboard 公网 HTTPS / CORS / 反向代理 | 未执行最终 HTTPS 验收；SSH 隧道 API 验证已通过 | 备案或公网可访问域名；DNS 控制权；80/443 入站放行；可签发并续期的证书（DNS-01 或可用 HTTP-01）；Nginx/OpenResty 反代配置；Dashboard 管理员凭据；精确 CORS 白名单 | `scripts/verify-dashboard.ps1` | `https://` 健康检查、登录、无效 token 401、CORS 允许/拒绝、节点/ACL/告警 API、静态入口全部通过；不得通过非本机 HTTP 传输管理员密码 |
| P0 | Windows 真实 TUN / Wintun | 未执行真实系统 TUN 生产验收 | Windows amd64 实机或虚拟机；管理员权限；官方且架构匹配的 `wintun.dll`；桌面安装包随包携带或通过 `NEXTUNNEL_WINTUN_DLL` / `-WintunDllPath` 注入；测试路由不与办公/VPN 网段冲突；安全软件允许创建网卡 | `scripts/verify-tun.ps1`、`scripts/verify-p2p-tun.ps1` | Wintun 设备创建成功且不是 `netTun` 回退；路由注入与清理成功；报告无 `wintun_dll_missing`、`privilege_required` 等阻塞项 |
| P0 | macOS 真实 TUN / 系统路由 | helper 链路已开发完成；未执行 signed/notarized pkg 实机验收 | macOS 测试机；Developer ID Application/Installer 证书；notarization 账号；pkg 安装权限；LaunchDaemon 可 bootstrap；允许创建 utun；测试路由不冲突；远端 SSH 临时公钥接入；防火墙允许验证流量 | `scripts/verify-p2p-tun.ps1 -MacUseHelper` | helper reachable；utun 创建成功且不是用户态回退；路由应用和清理成功；Windows/macOS 双端候选交换和连通性通过；报告归档为 `dist/verification/tun-macos-latest.json`；无临时 SSH 公钥、临时文件、临时路由残留 |
| P1 | eBPF Linux 吞吐 / 延迟 / CPU 基准 | 已通过真实网卡功能验收；未执行压力基准 | 隔离 Linux 节点或维护窗口；root 或等价 `CAP_BPF`、`CAP_NET_ADMIN`；Linux 6.x 内核；目标网卡名和 XDP 模式确认；`clang`、`llc`、`bpftool`、Go 工具链；可控流量发生器和基线用户态对照 | `scripts/verify-ebpf-linux.sh` 加压测参数或配套压测脚本 | 用户态与 XDP 模式分别记录吞吐、延迟、CPU、规则同步、统计读取和卸载清理；XDP 卸载后网卡无残留程序；报告可支撑 Beta 发布说明 |
| P1 | 真实多地域 Edge / Anycast | 本地演练和单真实 Control Plane 注册已通过；未执行商用多地域压测 | 至少 2-3 个不同地域节点；各节点公网 IP、时钟同步和安全组放行；真实 Control Plane 地址和 token；GeoIP 数据或静态映射；观测指标采集；可模拟节点故障和恢复 | `scripts/verify-edge-rehearsal.ps1 -RegisterRemote` | 多地域节点注册、心跳、最近节点选择、故障切换、路由偏移和清理全部通过；输出真实拓扑报告 |
| P1 | 发布包实物验收 | 本地质量门禁通过；需生成并在目标平台安装验证 | Git tag `v0.6.4-alpha`；Release 构建环境；Windows/macOS/Linux 目标机；代码签名/公证资源（如启用）；SHA256 校验产物；官方 Wintun 来源 | `scripts/package-server.ps1`、`scripts/package-cli.ps1`、`scripts/package-desktop.ps1`、`scripts/package-macos.sh` | server、cli、Windows desktop、macOS DMG 产物带 `v0.6.4-alpha` 版本；安装后可启动、连接、查看 Dashboard 状态；校验文件与发布说明一致 |

---

## 验证记录

| 日期 | 验证项 | 结果 | 备注 |
|:---:|:---|:---:|:---|
| 2026-06-18 | Dashboard 服务器二 API 端到端验证 | ✅ 通过 | 通过 SSH 本地端口转发访问服务器二 `127.0.0.1:8080`，避免管理员密码走公网 HTTP；验证健康检查、CORS 白名单/拒绝、登录、无效 token 401、节点、统计、ACL 创建/删除、告警规则、指标摄入和静态入口全部通过。报告：`dist/verification/dashboard-server2-ssh-script-report.json` |
| 2026-06-18 | Dashboard HTTPS 反向代理核查 | ⚠️ 阻塞 | 服务器一 `lee97.top -> 150.158.18.55`，但证书已于 2025-08-10 过期；certbot HTTP-01 续签被 DNSPod webblock 拦截，Let's Encrypt 看到 `https://dnspod.qcloud.com/static/webblock.html?d=lee97.top`，无法完成 HTTPS 验收。服务器二按 IP 访问 443 触发 SNI/TLS 错误，暂无可用 Dashboard HTTPS 域名 |
| 2026-06-18 | 受限环境阻塞项工程化处理 | ✅ 已实现 | Dashboard 验证脚本默认拒绝向非本机 HTTP 发送管理员密码，新增 `scripts/verify-dashboard-ssh.ps1` 和 `make verify-dashboard-ssh`；服务端发布包包含 SSH 验证脚本；桌面打包支持 `NEXTUNNEL_WINTUN_DLL`/`-WintunDllPath` 随包复制 `wintun.dll`；TUN 预检新增 Windows/macOS/Linux 环境方案提示 |
| 2026-06-18 | eBPF Linux 真实网卡功能验证 | ✅ 通过 | 服务器二 Linux 6.8，`clang/llc/bpftool` 可用；`INTERFACE_NAME=eth0 XDP_MODE=skb` 编译 `xdp_forwarder.c` 并挂载 XDP，`load_xdp mode=kernel`、DROP 规则同步、`stats_read packets=17 dropped=1`、卸载成功且无残留 XDP。报告：`dist/verification/ebpf-linux-server2-report.json` |
| 2026-06-18 | Edge/Anycast 本地与远端 Control Plane 演练 | ✅ 通过 | 本地 3 区域注册、Anycast 最近节点、故障切换、GeoIP 路由偏移通过；服务器二真实 Control Plane `http://47.116.218.140:9090` 远端注册 3 个临时节点、心跳等待和清理通过。报告：`dist/verification/edge-rehearsal-local-report.json`、`dist/verification/edge-rehearsal-server2-remote-report.json` |
| 2026-06-10 | 剩余 25% 开发收尾验证 | ✅ 通过 | 新增 Control Plane 虚拟网络管理器、注册自动分配 `virtual_ip`、`GET /api/v1/nodes/{id}/routes`、节点删除/过期释放 IP、SQLite 节点/ACL/IPAM 恢复；QUIC WorkConn 默认 TLS 1.3 且不默认 `InsecureSkipVerify`；Makefile/CI 过滤 `web/node_modules`。验证：过滤后 desktop/server/pkg Go test 全 PASS，Go vet/build 全 PASS，desktop/server 前端 `vue-tsc --noEmit` 通过 |
| 2026-06-10 | Beta 发布就绪核查 | ✅ 通过 | 42/42 关键功能核查全 PASS；三模块 go vet 零警告、go test 全 PASS、go build 成功；部署配置（.env.example + install.sh/ps1 + CI/Release workflow）就绪；跨平台编译（Linux/Darwin/Windows）已验证 |
| 2026-06-10 | 最终迭代开发完成 | ✅ 通过 | mTLS双向认证(pkg/tlsutil+CP+Relay+客户端)+审计日志(pkg/audit+CP+Dashboard)+RBAC策略(admin/operator/viewer)+Dashboard HTTPS/TLS+P2P/TUN OS TUN平台抽象(Linux/macOS/Windows)+IPAM路由管理(pkg/ipam+CP集成)。三模块go vet零警告、go test全PASS、go build成功。新增12个源文件+15个测试文件，跨平台编译验证通过 |
| 2026-06-09 | 服务端/客户端真实连接验证 | ✅ 通过 | 腾讯云服务器部署服务端后，客户端连接使用测试成功；当前可进入服务端部署连接使用阶段 |
| 2026-06-09 | Dashboard 生产启动链路补齐 | ✅ 通过 | 新增 `cmd/dashboard`、SQLiteDashboardStore 启动接入、静态资源托管、Linux/Windows 一键部署管理和 Release 打包；`go test ./...`、四个服务端入口 `go build`、Dashboard `vue-tsc`、Vite build、`install.ps1` 解析/配置生成、Git Bash `bash -n deploy/server/install.sh` 均通过 |
| 2026-06-09 | Phase 4 I3-02/I3-04 补齐验证 | ✅ 通过 | `go mod tidy`；`go test ./internal/ebpf ./internal/dashboard ./internal/anycast`；`GOOS=linux GOARCH=amd64 go test -c ./internal/ebpf`；`vue-tsc --noEmit`；ESLint；Vite production build。Windows 环境缺 `clang/llc`，未生成/挂载 BPF 对象 |
| 2026-06-09 | 服务端 Release 包部署脚本验证 | ✅ 通过 | `install.ps1` 语法解析通过；`install.ps1 -Action config -NonInteractive` 使用工作区临时目录生成 `server.env` 和连接信息通过；Git Bash `bash -n deploy/server/install.sh` 通过；腾讯云服务器部署后客户端连接使用测试成功 |
| 2026-06-08 | Phase 4 迭代3 生产能力完善 | ✅ 通过 | Dashboard SQLite持久化(5测试)+GeoIP真实加载(23测试)+网络场景(7测试)=35新测试全PASS。全量回归desktop(11)+server(8)+pkg(2)零失败。go vet+build零警告 |
| 2026-06-08 | Phase 4 迭代2 数据面闭环 | ✅ 通过 | QUIC E2E 1测试+TUN抽象 3测试+调度器闭环 1测试+Edge↔CP 6测试=11新测试全PASS。全量回归desktop(11)+server(8)+pkg(2)零失败。go vet+build零警告 |
| 2026-06-08 | Phase 4 迭代1 生产基线加固 | ✅ 通过 | SQLite Store 5测试+Relay安全 5测试+Dashboard安全 4测试=14新测试全PASS。全量回归desktop(11)+server(8)+pkg(2)零失败。go vet+build零警告。CI新增desktop build+go vet步骤 |
| 2026-06-08 | Phase 3 系统性测试验证 | ✅ 通过 | 6模块37个单元测试全PASS，全量回归零失败，go vet零警告，go build三模块成功。修复edge模块flaky test（改用原子安全的ConsecutiveOKs，新增DirectProbe确定性测试） |
| 2026-06-04 | `go test ./...`（server） | ✅ 通过 | 包含 controlplane、dashboard、relay 等 |
| 2026-06-04 | `go test ./...`（pkg） | ✅ 通过 | protocol、crypto 通过 |
| 2026-06-04 | `go test`（desktop，过滤 `frontend/node_modules`，工作区 GOCACHE） | ✅ 通过 | 默认 GOCACHE 受沙箱限制，已改用工作区缓存验证 |
| 2026-06-04 | `node node_modules/vue-tsc/bin/vue-tsc.js --noEmit` | ✅ 通过 | 等价于当前 `npm run test` 脚本；普通沙箱无法启动 node，已提权运行本地 CLI |
| 2026-06-04 | `node node_modules/eslint/bin/eslint.js . --ext .vue,.js,.jsx,.ts,.tsx` | ✅ 通过 | 0 error，仍有 Vue 模板格式 warning |

---

## 进度更新日志

| 日期 | 更新内容 | 操作人 |
|:---:|:---|:---:|
| 2026-07-02 | **v0.6.4-alpha 发布准备**：统一桌面端、Dashboard、CLI、服务端打包脚本、前端包元数据、部署示例、文档站、发布说明和生产验证口径到 `v0.6.4-alpha` / `0.6.4-alpha`；新增 macOS LaunchDaemon helper、pkg 打包、网络页 helper 状态和 `-MacUseHelper` 验证路径；真实生产通过仍需 signed/notarized pkg 实机报告 | Agent |
| 2026-06-24 | **v0.6.2-alpha 发布更新**：统一桌面端、Dashboard、CLI、服务端打包脚本、前端包元数据、部署示例、文档站、发布说明和生产验证口径到 `v0.6.2-alpha` / `0.6.2-alpha`；保留 `v0.6.0-beta` 作为历史生产收口记录 | Agent |
| 2026-06-23 | **v0.6.0-beta 发布准备**：统一桌面端、Dashboard、CLI、服务端打包脚本、部署示例、文档站和发布说明版本入口到 `v0.6.0-beta` / `0.6.0-beta`；保留 `v0.5.3-beta` 作为历史更新记录 | Agent |
| 2026-06-23 | **补充真实生产外部验收条件**：在进度文档中新增 Dashboard 公网 HTTPS、Windows 真实 TUN、macOS 真实 TUN、eBPF 性能基准、真实多地域 Edge/Anycast 和发布包实物验收的外部条件、执行入口和通过标准；这些事项在外部资源未满足前不计入代码阻塞 | Agent |
| 2026-06-23 | **v0.6.0-beta 生产收口推进**：Relay Admin API 与 Dashboard 客户端监控补齐在线客户端列表、断开入口、运行配置状态和审计查询；部署脚本同步 `DASHBOARD_RELAY_ADMIN_TOKEN`；桌面端新增服务端实例检测，按 Relay、Control Plane、STUN 返回可执行修复建议。Dashboard HTTPS、Windows/macOS 真实 TUN 和 eBPF 性能基准仍需在具备外部资源的环境复验 | Agent |
| 2026-06-18 | **生产验证推进基线**：两台公网服务器当前均部署 NexTunnel 四服务；服务器二 Dashboard API 通过 SSH 隧道完成全接口验证，公网 HTTP 健康和静态入口可达；服务器一 Dashboard 本机 8088 健康正常，但 `lee97.top` 证书过期且 DNSPod webblock 导致 Let's Encrypt 续签失败，`sslip.io` 临时域名在阿里云公网侧被 ICP 拦截，HTTPS 验收需换备案/可用域名。服务器二 eBPF XDP 在真实 `eth0` 以 `skb` 模式完成挂载/规则/统计/卸载；Edge/Anycast 完成本地与真实 Control Plane 远端注册演练；已补齐 Dashboard SSH 安全验证、Wintun 随包路径和 TUN 环境方案提示 | Agent |
| 2026-06-10 | **剩余 25% 开发收尾完成**：补齐 Control Plane IPAM 自动分配和节点路由下发，新增 `VirtualNetworkManager`、`virtual_ip` 持久化字段、`GET /api/v1/nodes/{id}/routes`、`DELETE /api/v1/nodes/{id}` 释放 IP，启动时恢复 SQLite 节点/ACL/IPAM 状态；收紧 QUIC WorkConn 默认 TLS 配置，不再默认跳过证书校验；Makefile/CI 过滤前端 node_modules 内 Go 包；补充 IPAM、Control Plane、QUIC TLS 安全默认值测试 | Agent |
| 2026-06-09 | **服务端 Release 包部署与 1Panel 教程补齐**：将 `deploy/server/install.sh` 从 Docker Compose 源码构建改为 GitHub Release 包下载、SHA256 可选校验、二进制安装、`/etc/nextunnel/server.env` 配置生成和 systemd 三服务管理；`install.ps1` 改为下载 Windows Release 包并以 PID/日志管理本地进程；`.env.example` 增加 `NEXTUNNEL_PACKAGE_URL`、`NEXTUNNEL_VERSION`、`NEXTUNNEL_RELEASE_BASE_URL`、`NEXTUNNEL_PACKAGE_SHA256` 等多源配置；`docker-compose.yml` 去除源码 build，保留为后续官方镜像和 1Panel 容器编排模板；新增 `.github/workflows/release.yml` 在 `v*` tag 生成服务端 linux-amd64、linux-arm64、windows-amd64 发布包和校验文件；README 补齐命令行、环境变量、配置文件、离线包、1Panel 终端/文件/计划任务/容器编排多种部署方式 | Agent |
| 2026-06-09 | **服务端一键部署能力补齐**：新增 `deploy/server/` 生产部署目录，提供 Linux `install.sh`、Windows/PowerShell `install.ps1`、`.env.example`、Docker Compose 编排和部署说明；支持 install/up/down/restart/status/logs/health/update/uninstall，多方式配置 Relay、Control Plane、NAT Detector。修正服务端 Dockerfile 和根 Compose 构建上下文，保证 `server` 依赖 `../pkg` 时可在干净环境构建；公网 Relay 默认强制认证 token | Agent |
| 2026-06-09 | **Phase 4 剩余开发任务补齐**：I3-02 eBPF XDP 内核态转发完成（cilium/ebpf 加载、XDP attach、L4 fast path 规则编码、优先级遮挡保护、内核统计读取、用户态降级、xdp_forwarder.c）。I3-04 Dashboard 前端完成（登录、健康检查、节点地图、流量图表、节点表、ACL 新增/删除、告警确认、API/types/formatters 模块拆分）。验证通过 Go 测试、Linux 目标编译、vue-tsc、ESLint、Vite build；Linux 真实网卡基准待部署环境执行 | Agent |
| 2026-06-08 | **Phase 4 迭代3 生产能力完善完成**：I3-01 Dashboard持久化(SQLiteDashboardStore接口+实现，Node/ACL/Alert/User CRUD，5测试PASS)。I3-03 GeoIP数据库(MaxMindAdapter接入oschwald/maxminddb-golang纯Go库，真实.mdb文件加载+CIDR匹配+回退映射，23测试PASS)。I3-05 网络场景测试(QUIC证书失败+TCP不可达+Relay降级+重连，7测试PASS)。新增文件：store_sqlite.go、store_sqlite_test.go、network_scenario_test.go。修改文件：geoip.go、go.mod。全量回归零失败 | Agent |
| 2026-06-08 | **Phase 4 迭代2 数据面闭环完成**：I2-01 QUIC Relay E2E(WorkConnOpener接口+QUICWorkConnOpener实现+QUIC stream转发，E2E测试PASS)。I2-02 P2P/TUN抽象(TUNDevice接口+TUNConfig+PlatformCapabilities+netTun实现ReadPacket/WritePacket，3测试PASS)。I2-03 调度器数据面闭环(PathManager+RelaySelector+MigrationController三接口驱动真实路径切换，集成测试PASS)。I2-04 Edge↔CP集成(ControlPlaneClient+Registry回调自动注册/注销+HeartbeatLoop，6测试PASS)。修复Manager日志初始化nil writer bug。新增文件：workconn.go、workconn_quic.go、tun.go、tun_test.go、quic_relay_e2e_test.go、scheduler_integration_test.go。修改文件：manager.go、tunnel.go、net_tun.go。全量回归零失败 | Agent |
| 2026-06-08 | **Phase 4 迭代1 生产基线加固完成**：I1-01 Control Plane SQLite持久化(store_sqlite.go+factory+config 5个新测试全PASS)。I1-02 Relay安全基线(非localhost部署强制要求auth-token，5个配置验证测试全PASS)。I1-03 Dashboard安全加固验证(bcrypt密码+弱密码拒绝+空密钥拒绝+CORS白名单，4个安全测试全PASS)。I1-04 CI质量门禁(添加desktop Go构建+go vet全三模块)。全量回归desktop(11)+server(8)+pkg(2)全PASS，go vet零警告，go build成功。新增文件：store_sqlite.go、store_factory.go、store_sqlite_test.go。修改文件：config.go、main.go(control-plane+relay)、server_test.go(relay)、server_test.go(dashboard)、ci.yml | Agent |
| 2026-06-08 | **Phase 3 系统性验收 + Phase 4 迭代计划启动**：逐模块运行Phase 3全部6个功能模块测试（QUIC 5+Probe 6+Scheduler 9+Relay 4+Migration 5+ControlPlane 8=37个单元测试全PASS）。全量回归desktop(11模块)+server(7模块)+pkg(2模块)零失败。go vet三模块零警告，go build三模块成功。集成验证：协议7种Phase 3消息类型完整，Engine强类型接口(PathManager/RelaySelector/MigrationController)集成正常，P2P全链路(ICE+Punch+WireGuard)测试PASS。UAT验收对照task-plan.md需求全覆盖。修复edge HealthChecker时序flaky test。更新进度跟踪文档，制定Phase 4迭代开发计划 | Agent |
| 2026-06-04 | **Phase 4 实现迭代**：新增 SD-WAN QoS 队列+令牌桶（qos.go）、Dashboard AlertEngine+Webhook 通知（alerts.go）、Edge ControlPlane 注册/心跳集成（controlplane.go）、Anycast GeoIPProvider 接口+MaxMind 适配器（geoip.go）、eBPF 转发规则+用户态转发（forwarder.go）；全部模块新增单元测试 | Agent |
| 2026-06-04 | **Phase 4 专项迭代**：逐模块校验 edge/anycast/ebpf/sdwan/dashboard 代码实现状态，新增 Phase 4 详细子任务跟踪表；近期 TODO 增加 Edge↔Control Plane 集成、eBPF XDP 真实转发、SD-WAN QoS 队列、Anycast GeoIP、Dashboard 前端 5 项 P2 任务 | Agent |
| 2026-06-04 | 核心代码与架构审查后修正进度口径：Phase 1 为 MVP 可用，Phase 2-4 为原型/概念验证；补质量门禁、安全基线、控制面 HTTP API、Relay QUIC work stream、桌面端连接/启停流程 | Codex |
| 2026-06-03 | 原记录显示 Phase 3/4 全完成，经代码审查后调整为“模块原型与单元测试完成”，不再作为生产验收完成依据 | - |
| 2026-06-01 | 原记录显示 Phase 2 全完成，经代码审查后调整为“P2P 核心链路原型完成”，真实 TUN/IPAM/路由仍待实现 | - |
| 2026-05-29 | Phase 1 基础 TCP Relay 原型、SQLite 配置、桌面基础 UI 与集成测试形成 MVP 基线 | - |
