# NexTunnel 项目规划文档

## 0. v0.6.4-alpha 发布更新状态（2026-07-02）

当前代码开发按 v0.6.4-alpha 口径发布更新：macOS System TUN LaunchDaemon helper 已完成代码接入，Relay Admin API、Dashboard 客户端监控、运行配置状态、审计查询、桌面服务端实例检测、明亮主题 logo 兼容和部署脚本变量继续作为验收路径。剩余事项不再是继续堆功能，而是把真实生产依赖拆成可验证、可回收、可后续迭代的外部条件：域名/证书、Windows 驱动 DLL 实机验收、macOS signed/notarized pkg 实机验收、eBPF 压测窗口和真实多地域资源。

### 0.1 当前结论

| 方向 | 状态 | 结论 |
|:---|:---:|:---|
| Dashboard API | ✅ 已通过 | 服务器二通过 SSH 隧道完成健康检查、登录、401、CORS、节点、统计、ACL、告警和静态入口验证，报告位于 `dist/verification/dashboard-server2-ssh-script-report.json`。 |
| Dashboard HTTPS | ⚠️ 外部阻塞 | `lee97.top` 证书过期且 DNSPod webblock 阻断 HTTP-01；`*.sslip.io` 在服务器二公网侧被 ICP 拦截。生产验收需备案/可用域名和有效证书。 |
| Windows/macOS P2P | ✅ 直连已验证 | 局域网双端候选交换和直连链路已通过；真实系统 TUN 仍依赖平台驱动和授权。 |
| Windows 真实 TUN | ⚠️ 实机验收待补 | 打包链路已支持随包注入官方、匹配架构 `wintun.dll` 并做架构校验；生产通过前仍需 Windows 实机管理员权限创建设备、注入路由和清理验证报告。 |
| macOS 真实 TUN | ⚠️ helper 已实现，实机验收待补 | `nextunnel-helper` LaunchDaemon 已接入桌面端、预检、验证器和 pkg 打包；生产通过前仍需 Developer ID 签名/公证、pkg 安装、utun 创建、路由应用/清理和 `tun-macos-latest.json`。 |
| Linux eBPF XDP | ✅ 功能验收通过 | 服务器二 `eth0`/`skb` 模式完成 BPF 编译、XDP 挂载、规则同步、统计读取和卸载；吞吐/延迟基准仍需隔离窗口。 |
| Edge/Anycast | ✅ 演练通过 | 本地 3 区域和服务器二真实 Control Plane 注册/心跳/清理通过；商用生产仍需真实多地域节点压测。 |

### 0.2 已执行的阻塞处理方案

| 阻塞项 | 已执行方案 | 后续边界 |
|:---|:---|:---|
| 无可用公网 HTTPS 域名 | `scripts/verify-dashboard.ps1` 默认拒绝向非本机 HTTP 发送管理员密码；新增 `scripts/verify-dashboard-ssh.ps1` 通过 SSH 本地端口转发读取远端配置并完成 API 验证。 | HTTPS 最终验收仍必须在备案/可用域名和有效证书下复验。 |
| Windows 缺少 `wintun.dll` | `scripts/package-desktop.ps1` 支持 `NEXTUNNEL_WINTUN_DLL`/`-WintunDllPath` 自动复制官方 DLL，并校验 PE 架构。 | 未随包携带 DLL 或未完成管理员权限实机验证时，不声明 Windows 真实 TUN 生产可用。 |
| macOS 无免密提权 | 新增 `nextunnel-helper` LaunchDaemon、受控 Unix socket 协议、fd passing 创建 utun、helper-backed route applier、pkg 打包和 `-MacUseHelper` 验证参数。 | 只有 signed/notarized pkg 安装且 `tun-macos-latest.json` 验收通过后，才能声明 macOS System TUN 真实环境功能验收通过。 |
| 真实 TUN 前置条件不清晰 | `desktop/internal/p2p` 预检新增 `EnvironmentHints`，向前端输出 Windows/macOS/Linux 的生产修复建议。 | 后续应用端可把这些提示接入更细的安装向导。 |
| 发布包遗漏验证入口 | 服务端打包清单加入 `scripts/verify-dashboard-ssh.ps1`，Release 文档同步 SSH 隧道验证流程。 | 发布前必须确认新增脚本已纳入 Git 跟踪。 |

### 0.3 v0.6.4-alpha 发布验收口径

| 优先级 | 任务 | 验收状态 |
|:---:|:---|:---|
| P0 | 质量门禁稳定化 | ✅ Go 测试、PowerShell 脚本解析、前端类型检查已通过；发布包可生成。 |
| P0 | 安全基线 | ✅ Relay/Control Plane/Dashboard 强配置、bcrypt、CORS 白名单、mTLS、审计和 RBAC 已实现。 |
| P1 | Dashboard 端到端部署联调 | ✅ API 通过；⚠️ HTTPS 受外部域名/证书阻塞，已提供 SSH 安全验证替代路径。 |
| P1 | eBPF Linux 生产验证 | ✅ 真实网卡功能验收通过；⚠️ 性能压力基准待隔离窗口补充。 |
| P1 | 多地域边缘部署演练 | ✅ 本地和服务器二远端 Control Plane 演练通过；⚠️ 商用多地域压测待资源准备。 |
| P2 | P2P/TUN 生产化 | ✅ 直连链路、IPAM、路由下发、Windows Wintun 打包链路和 macOS helper 链路已实现；⚠️ 真实 OS TUN 仍需 Windows/macOS 实机验收报告。 |

### 0.4 后续迭代候选

| 优先级 | 方向 | 说明 |
|:---:|:---|:---|
| P1 | Dashboard HTTPS 复验 | 更换备案/可访问域名，配置 Nginx/OpenResty HTTPS 反代、CORS 白名单和证书续期后重跑 Dashboard 验证。 |
| P1 | 桌面 TUN 安装体验 | Windows 安装器随附官方 `wintun.dll`；macOS 通过 signed/notarized pkg 安装 LaunchDaemon helper，补齐升级/卸载与实机报告。 |
| P1 | eBPF 压测 | 在隔离 Linux 节点记录用户态和 XDP 模式吞吐、延迟、CPU、统计读取和卸载清理数据。 |
| P2 | 多地域生产拓扑 | 接入真实多地域节点、GeoIP 数据源和观测指标，补充故障切换与路由偏移报告。 |
| P2 | 应用端提示迭代 | 将 `EnvironmentHints` 显示为更清晰的安装/修复向导，并保留 P2P-only/Relay-only 可用路径。 |

---

## 一、项目概述

### 1.1 项目定位

NexTunnel 是一款**开源内网穿透 + P2P 直连**的现代网络工具，旨在超越传统 FRP/NPS 等"客户端→中转服务器"的 TCP 转发模式，打造下一代智能组网方案。

### 1.2 项目愿景

让内网穿透从"能连上"进化为"智能直连"——用户无需理解端口、NAT、UDP、Tunnel 等底层概念，设备自动发现、自动组网、自动加速、自动直连。

### 1.3 核心价值

| 价值维度 | 描述 |
|---------|------|
| **P2P 优先** | 数据直连传输，不经过中继服务器，降低延迟与带宽成本 |
| **智能链路** | 自动检测网络环境，选择最优传输路径 |
| **安全零信任** | 端到端加密，WireGuard 级安全保障，OIDC 认证 |
| **自动降级** | P2P 不可达时自动切换至中继，保证连通性 |
| **全球加速** | 多中继节点 + Anycast，边缘网络加速 |
| **跨平台** | Wails 桌面客户端，覆盖 Windows/macOS/Linux |

---

## 二、总体架构设计

### 2.1 系统架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                        NexTunnel 系统全景                            │
└─────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐                              ┌──────────────┐
  │  Client A    │                              │  Client B    │
  │ (Wails App)  │                              │ (Wails App)  │
  └──────┬───────┘                              └──────┬───────┘
         │                                             │
         │  ←─────── P2P Direct (WireGuard/QUIC) ───→ │
         │                                             │
         │         ┌─────────────────────┐             │
         ├────────→│   Control Plane     │←────────────┤
         │         │  (Go + gRPC)        │             │
         │         │  - 节点注册/认证     │             │
         │         │  - 密钥交换          │             │
         │         │  - ACL/路由下发      │             │
         │         └─────────────────────┘             │
         │                                             │
         │         ┌─────────────────────┐             │
         ├────────→│   STUN/TURN 服务    │←────────────┤
         │         │  (coturn)           │             │
         │         │  - NAT 类型检测      │             │
         │         │  - 打洞协助          │             │
         │         └─────────────────────┘             │
         │                                             │
         │         ┌─────────────────────┐             │
         └────────→│   Relay 中继节点    │←────────────┘
                   │  (QUIC Relay)       │
                   │  - 降级中继          │
                   │  - 全球多节点        │
                   └─────────────────────┘
```

### 2.2 客户端架构分层

```
┌─────────────────────────────────────────┐
│          Presentation Layer             │
│       Vue 3 + Vite (Desktop UI)         │
├─────────────────────────────────────────┤
│          Application Layer              │
│    Wails Bridge (Go ↔ Frontend IPC)     │
├─────────────────────────────────────────┤
│          Business Logic Layer           │
│  ┌───────────┬───────────┬───────────┐  │
│  │  Tunnel   │  Device   │  Config   │  │
│  │  Manager  │  Manager  │  Manager  │  │
│  └───────────┴───────────┴───────────┘  │
├─────────────────────────────────────────┤
│          P2P Engine Layer               │
│  ┌───────────┬───────────┬───────────┐  │
│  │ WireGuard │  WebRTC   │   Link    │  │
│  │  Tunnel   │  DataCh   │ Scheduler │  │
│  └───────────┴───────────┴───────────┘  │
├─────────────────────────────────────────┤
│          Transport Layer                │
│  ┌───────────┬───────────┬───────────┐  │
│  │   QUIC    │    UDP    │    TCP    │  │
│  │ Transport │ Transport │ Transport │  │
│  └───────────┴───────────┴───────────┘  │
├─────────────────────────────────────────┤
│          NAT Traversal Layer            │
│  ┌───────────┬───────────┬───────────┐  │
│  │   ICE     │   STUN    │   TURN    │  │
│  │  Agent    │  Client   │  Client   │  │
│  └───────────┴───────────┴───────────┘  │
├─────────────────────────────────────────┤
│          Relay Fallback Layer           │
│  ┌───────────┬───────────────────────┐  │
│  │   QUIC    │     DERP-like         │  │
│  │   Relay   │     Relay             │  │
│  └───────────┴───────────────────────┘  │
├─────────────────────────────────────────┤
│          Storage Layer                  │
│          SQLite (Local Config)          │
└─────────────────────────────────────────┘
```

### 2.3 服务端架构分层

```
┌─────────────────────────────────────────┐
│            API Gateway                  │
│        (gRPC + REST + WebSocket)        │
├──────────┬──────────┬───────────────────┤
│ Auth     │ Device   │ Route             │
│ Service  │ Registry │ Manager           │
├──────────┴──────────┴───────────────────┤
│          Core Services                  │
│  ┌──────────┬──────────┬─────────────┐  │
│  │ Key      │ ACL      │ NAT         │  │
│  │ Exchange │ Engine   │ Detection   │  │
│  └──────────┴──────────┴─────────────┘  │
├─────────────────────────────────────────┤
│          Infrastructure                 │
│  ┌──────────┬──────────┬─────────────┐  │
│  │PostgreSQL│  Redis   │ Prometheus  │  │
│  │(持久化)  │ (缓存)   │ (监控)      │  │
│  └──────────┴──────────┴─────────────┘  │
├─────────────────────────────────────────┤
│          STUN/TURN    │  Relay Nodes    │
│          (coturn)     │  (QUIC Relay)   │
└───────────────────────┴─────────────────┘
```

### 2.4 数据流向说明

#### 建立连接流程

```
1. Client A → Control Plane: 注册节点，上报 NAT 类型
2. Client A → Control Plane: 请求连接 Client B
3. Control Plane → Client A/B: 下发对方 Endpoint + 公钥
4. Client A ↔ STUN: 获取公网映射地址
5. Client A ↔ Client B: ICE 协商，尝试 UDP 打洞
6. 成功 → WireGuard P2P 直连
7. 失败 → 降级至 QUIC Relay 中继
```

#### 数据传输流程

```
应用数据 → TUN 接口 → WireGuard 加密 → UDP/QUIC 传输 → 对端解密 → TUN 接口 → 目标应用
```

#### 链路调度决策流程

```
检测 NAT 类型 → 评估 RTT/丢包 → 选择最优路径:
  ├── Full Cone NAT     → UDP P2P (直连)
  ├── Restricted NAT    → QUIC P2P (打洞)
  ├── Symmetric NAT     → TCP P2P (尝试)
  ├── P2P 全部失败      → Nearby Relay (就近中继)
  └── 全部不可用        → Global Relay (全球中继)
```

---

## 三、功能模块划分

### 3.1 模块清单

| 模块 | 职责 | 所属组件 |
|------|------|---------|
| **tunnel-core** | TCP/UDP/HTTP 隧道核心实现 | Client |
| **p2p-engine** | P2P 连接引擎（WireGuard + WebRTC） | Client |
| **nat-traversal** | NAT 穿透（ICE/STUN/TURN 集成） | Client |
| **link-scheduler** | 智能链路调度与切换 | Client |
| **relay-client** | Relay 中继客户端 | Client |
| **config-store** | 本地配置与状态持久化（SQLite） | Client |
| **desktop-ui** | 桌面端 Vue 3 界面 | Desktop |
| **wails-bridge** | Go ↔ Vue IPC 桥接层 | Desktop |
| **control-plane** | 节点管理、认证、ACL、密钥交换 | Server |
| **relay-server** | QUIC Relay 中继服务 | Server |
| **nat-detector** | NAT 类型检测服务 | Server |
| **stun-turn** | STUN/TURN 基础设施 | Server |

### 3.2 模块职责与接口

#### tunnel-core

- **职责**：实现基础隧道功能，支持 TCP Tunnel、HTTP Tunnel、UDP Tunnel
- **边界**：仅关注数据传输通道建立与维护，不涉及打洞逻辑
- **接口**：
  - `CreateTunnel(config TunnelConfig) → Tunnel`
  - `Tunnel.Forward(conn net.Conn) → error`
  - `Tunnel.Close() → error`

#### p2p-engine

- **职责**：管理 P2P 连接生命周期，集成 WireGuard 和 WebRTC
- **边界**：调用 nat-traversal 完成打洞，调用 link-scheduler 决定路径
- **接口**：
  - `Connect(peerID string) → P2PConnection`
  - `P2PConnection.Send(data []byte) → error`
  - `P2PConnection.OnData(handler func([]byte))`

#### nat-traversal

- **职责**：NAT 类型检测、ICE 协商、STUN 绑定、TURN 分配
- **边界**：提供候选地址，不负责最终传输
- **接口**：
  - `DetectNATType() → NATType`
  - `GatherCandidates() → []Candidate`
  - `Negotiate(remoteCandidates []Candidate) → SelectedPair`

#### link-scheduler

- **职责**：基于网络指标动态选择最优链路，执行路径切换
- **边界**：消费网络探测数据，输出路由决策
- **接口**：
  - `Evaluate(metrics LinkMetrics) → LinkDecision`
  - `SwitchPath(decision LinkDecision) → error`
  - `OnPathChange(handler func(oldPath, newPath Path))`

#### relay-client

- **职责**：连接 Relay 中继节点，作为 P2P 失败时的降级路径
- **边界**：被 link-scheduler 调度启用
- **接口**：
  - `ConnectRelay(relayAddr string) → RelayConn`
  - `RelayConn.Forward(data []byte) → error`

#### control-plane（服务端）

- **职责**：节点注册与认证（OIDC）、设备管理、ACL 规则下发、WireGuard 密钥交换、路由表管理
- **边界**：不转发用户数据，仅传递控制信息
- **接口**（gRPC）：
  - `RegisterNode(request) → NodeInfo`
  - `ExchangeKeys(request) → KeyPair`
  - `GetPeers(nodeID) → []PeerInfo`
  - `UpdateACL(rules) → ACLResult`

#### relay-server

- **职责**：为无法 P2P 的节点提供 QUIC 中继转发
- **边界**：纯数据转发，不解密用户数据内容
- **接口**：
  - `HandleRelaySession(stream quic.Stream)`
  - `ReportMetrics() → RelayMetrics`

### 3.3 模块依赖关系图

```
                    ┌────────────┐
                    │ desktop-ui │
                    └─────┬──────┘
                          │
                    ┌─────▼──────┐
                    │wails-bridge│
                    └─────┬──────┘
                          │
              ┌───────────▼───────────┐
              │      p2p-engine       │
              └───┬───────┬───────┬───┘
                  │       │       │
         ┌────────▼┐  ┌──▼─────┐ │
         │tunnel-  │  │  link- │ │
         │  core   │  │scheduler│ │
         └─────────┘  └──┬─────┘ │
                          │       │
              ┌───────────▼┐  ┌───▼────────┐
              │nat-traversal│  │relay-client│
              └─────────────┘  └────────────┘
                          │
                    ┌─────▼──────┐
                    │config-store│
                    └────────────┘

         ── 服务端 ──

    ┌──────────────┐    ┌────────────┐
    │control-plane │    │relay-server│
    └──────┬───────┘    └────────────┘
           │
    ┌──────▼───────┐    ┌────────────┐
    │ nat-detector │    │  stun-turn │
    └──────────────┘    └────────────┘
```

---

## 四、技术选型详细说明

### 4.1 语言与框架

| 组件 | 语言/框架 | 选型理由 |
|------|----------|---------|
| 客户端核心 | Go | 网络库成熟，协程模型优秀，跨平台编译简单 |
| 桌面 UI | Vue 3 + Vite | 组件化开发，生态丰富，HMR 开发体验好 |
| 桌面框架 | Wails v2 | Go + Web 技术栈封装为原生桌面应用，体积小性能好 |
| 服务端 | Go + gRPC | 高并发处理能力，gRPC 强类型接口适合控制面 |
| 本地存储 | SQLite | 嵌入式零部署，适合客户端本地配置与状态存储 |
| 服务端存储 | PostgreSQL + Redis | PostgreSQL 关系型数据持久化，Redis 缓存热点数据 |

### 4.2 关键库

| 功能领域 | 库 | 版本要求 | 用途 |
|---------|---|---------|------|
| QUIC 传输 | `github.com/quic-go/quic-go` | latest | QUIC 协议实现，用于 P2P 传输和 Relay |
| WebRTC | `github.com/pion/webrtc/v3` | v3.x | ICE/STUN/TURN 集成，DataChannel |
| WireGuard | `golang.zx2c4.com/wireguard` | latest | WireGuard 协议实现，P2P 加密隧道 |
| TUN 接口 | `github.com/songgao/water` | latest | 创建虚拟网络接口 |
| STUN | `github.com/pion/stun` | latest | STUN 协议客户端 |
| gRPC | `google.golang.org/grpc` | latest | 控制面 RPC 通信 |
| SQLite | `github.com/mattn/go-sqlite3` | latest | 本地 SQLite 驱动 |
| 日志 | `go.uber.org/zap` | latest | 高性能结构化日志 |
| 配置 | `github.com/spf13/viper` | latest | 多格式配置文件管理 |

### 4.3 关键协议说明

#### WireGuard

- **用途**：节点间 P2P 加密隧道
- **特点**：极简代码量（~4000行），使用 Noise Protocol Framework，仅 UDP，固定头开销小
- **在 NexTunnel 中的角色**：作为数据面主传输协议，所有 P2P 流量经 WireGuard 加密
- **密钥管理**：Curve25519 密钥对，通过 Control Plane 交换公钥

#### QUIC

- **用途**：替代 TCP 的现代传输协议，用于 Relay 中继和 HTTP3 隧道
- **特点**：基于 UDP、多路复用、0-RTT 连接恢复、连接迁移（网络切换不断线）
- **在 NexTunnel 中的角色**：Relay 传输层协议 + P2P 备选传输 + HTTP3 Tunnel

#### ICE/STUN/TURN

- **ICE（Interactive Connectivity Establishment）**：自动协商最佳连接路径的框架
- **STUN（Session Traversal Utilities for NAT）**：发现 NAT 映射地址，辅助打洞
- **TURN（Traversal Using Relays around NAT）**：当 P2P 不可达时提供中继
- **在 NexTunnel 中的角色**：NAT 穿透核心组件，由 pion 库集成

#### gRPC

- **用途**：Control Plane 服务间通信
- **特点**：强类型 Protobuf 接口定义、双向流、高效二进制传输
- **在 NexTunnel 中的角色**：客户端与控制面通信（注册、认证、密钥交换、ACL 同步）

---

## 五、开发里程碑与阶段规划

### 5.1 总览

| 阶段 | 名称 | 目标 | 预估时间 |
|------|------|------|---------|
| Phase 1 | 基础隧道 | TCP/HTTP Tunnel + Relay 中继 | 6-8 周 |
| Phase 2 | P2P 直连 | UDP 打洞 + WireGuard Mesh | 8-10 周 |
| Phase 3 | 智能调度 | QUIC 传输 + 智能路由 + 多 Relay | 6-8 周 |
| Phase 4 | 全球加速 | 边缘节点 + eBPF + SD-WAN | 10-12 周 |

### 5.2 Phase 1：基础隧道（6-8 周）

**目标**：实现最小可用产品，客户端可通过中继服务器穿透内网访问服务。

**交付物**：
- Wails 桌面客户端（基础 UI）
- TCP Tunnel 功能
- HTTP Tunnel 功能
- Relay 中继服务
- 基础认证（用户名/密码）
- SQLite 本地配置存储

**里程碑节点**：
- Week 2：项目骨架搭建完成，Wails 项目可运行
- Week 4：TCP Tunnel 端到端打通
- Week 6：HTTP Tunnel + Relay 完成
- Week 8：桌面 UI 集成，基础功能可用

### 5.3 Phase 2：P2P 直连（8-10 周）

**目标**：实现 NAT 穿透和 P2P 直连，大幅降低延迟和带宽成本。

**前置依赖**：Phase 1 完成

**交付物**：
- NAT 类型检测
- ICE/STUN 集成
- UDP P2P 打洞
- WireGuard 加密隧道
- 基础 Mesh 组网（多节点互联）
- OIDC 认证集成

**里程碑节点**：
- Week 2：NAT 检测 + STUN 集成
- Week 4：UDP 打洞验证
- Week 6：WireGuard 隧道端到端
- Week 8：Mesh 多节点组网
- Week 10：OIDC 认证 + UI 完善

### 5.4 Phase 3：智能调度（6-8 周）

**目标**：实现智能链路选择，根据网络状况自动选择最优传输路径。

**前置依赖**：Phase 2 完成

**交付物**：
- QUIC 传输层
- 链路质量探测（RTT、丢包、带宽）
- 智能路由决策引擎
- 多 Relay 节点管理与自动选择
- 连接迁移（网络切换不断线）
- Control Plane gRPC 服务

**里程碑节点**：
- Week 2：QUIC 传输集成
- Week 4：链路探测 + 决策引擎
- Week 6：多 Relay + 自动选择
- Week 8：连接迁移 + Control Plane 完善

### 5.5 Phase 4：全球加速（10-12 周）

**目标**：构建全球边缘网络，实现企业级 SD-WAN 能力。

**前置依赖**：Phase 3 完成

**交付物**：
- 全球边缘节点部署方案
- Anycast 路由
- eBPF 网络加速（Linux）
- SD-WAN 流量策略
- 高级 ACL 与流量审计
- 管理控制台（Web Dashboard）

**里程碑节点**：
- Week 3：边缘节点架构 + 部署自动化
- Week 6：Anycast + eBPF 加速
- Week 9：SD-WAN 策略引擎
- Week 12：管理控制台 + 全面测试

---

## 六、任务分解

### 6.1 Phase 1 任务分解

#### P1-T01：项目初始化与骨架搭建

- **目标**：创建项目目录结构，初始化 Wails 项目，配置开发工具链
- **输入**：技术选型文档、目录结构设计
- **输出**：可编译运行的 Wails 空项目 + Go module 初始化
- **验收标准**：
  - `wails dev` 可正常启动桌面窗口
  - Go 后端 + Vue 前端通信正常
  - CI lint 通过
- **预估工时**：3 天
- **优先级**：P0（阻塞所有后续任务）

#### P1-T02：SQLite 本地存储模块

- **目标**：实现本地配置持久化，包括隧道配置、设备信息、认证凭据
- **输入**：数据模型设计
- **输出**：config-store 模块，提供 CRUD API
- **验收标准**：
  - 支持隧道配置的增删改查
  - 支持数据库迁移
  - 单元测试覆盖率 > 80%
- **预估工时**：3 天
- **优先级**：P0
- **并行关系**：可与 P1-T03 并行

#### P1-T03：TCP Tunnel 核心实现

- **目标**：实现基础 TCP 端口转发隧道
- **输入**：隧道协议设计（控制连接 + 数据连接）
- **输出**：tunnel-core 模块（TCP 部分）
- **验收标准**：
  - 客户端可将本地 TCP 端口暴露至远端
  - 远端可通过指定端口访问内网服务
  - 支持多路复用
  - 断线自动重连
- **预估工时**：5 天
- **优先级**：P0
- **并行关系**：可与 P1-T02 并行

#### P1-T04：HTTP Tunnel 实现

- **目标**：实现 HTTP/HTTPS 反向代理隧道，支持域名路由
- **输入**：TCP Tunnel 基础
- **输出**：tunnel-core 模块（HTTP 部分）
- **验收标准**：
  - 支持基于域名的请求路由
  - 支持 WebSocket 透传
  - 支持自定义 Host Header
  - 支持 HTTPS（TLS 终止）
- **预估工时**：4 天
- **优先级**：P1
- **依赖**：P1-T03

#### P1-T05：Relay 中继服务实现

- **目标**：实现独立部署的 Relay 中继服务端
- **输入**：中继协议设计
- **输出**：server/ 目录下的 relay-server 服务
- **验收标准**：
  - 支持客户端注册与认证
  - 支持多客户端同时中继
  - 支持流量统计
  - 支持优雅关闭
- **预估工时**：5 天
- **优先级**：P0
- **依赖**：P1-T03

#### P1-T06：基础认证模块

- **目标**：实现简单的 Token 认证，保护中继服务
- **输入**：认证流程设计
- **输出**：auth 中间件 + token 管理
- **验收标准**：
  - 客户端使用 Token 连接 Relay
  - 无效 Token 被拒绝
  - Token 支持过期与刷新
- **预估工时**：3 天
- **优先级**：P1
- **并行关系**：可与 P1-T04、P1-T05 并行

#### P1-T07：桌面 UI 基础界面

- **目标**：实现 Wails 桌面端基础管理界面
- **输入**：UI 设计稿 / 线框图
- **输出**：Vue 3 前端页面（隧道管理、连接状态、设置）
- **验收标准**：
  - 可查看/创建/删除隧道配置
  - 实时显示连接状态
  - 显示流量统计
  - 支持系统托盘
- **预估工时**：5 天
- **优先级**：P1
- **依赖**：P1-T01

#### P1-T08：端到端集成测试

- **目标**：验证 Phase 1 所有功能协同工作
- **输入**：所有 Phase 1 模块
- **输出**：集成测试用例 + 测试报告
- **验收标准**：
  - TCP Tunnel E2E 通过
  - HTTP Tunnel E2E 通过
  - Relay 降级场景通过
  - 异常断线恢复通过
- **预估工时**：3 天
- **优先级**：P0
- **依赖**：P1-T03 ~ P1-T07 全部完成

---

### 6.2 Phase 2 任务分解

#### P2-T01：NAT 类型检测模块

- **目标**：实现客户端本地 NAT 类型检测（Full Cone / Restricted / Port Restricted / Symmetric）
- **输入**：RFC 3489 / RFC 5389 STUN 规范
- **输出**：nat-traversal 模块（检测部分）
- **验收标准**：
  - 准确识别 4 种 NAT 类型
  - 检测耗时 < 3 秒
  - 支持多网卡环境
- **预估工时**：4 天
- **优先级**：P0

#### P2-T02：STUN 客户端集成

- **目标**：集成 pion/stun，获取 NAT 映射地址
- **输入**：STUN 服务器地址列表
- **输出**：STUN binding 模块
- **验收标准**：
  - 可获取 Server Reflexive Candidate
  - 支持多 STUN 服务器冗余
  - 支持 IPv4/IPv6
- **预估工时**：3 天
- **优先级**：P0
- **并行关系**：可与 P2-T01 并行

#### P2-T03：ICE 协商引擎

- **目标**：实现 ICE Candidate 收集、交换与连接检测
- **输入**：pion/ice 库
- **输出**：ICE Agent 封装模块
- **验收标准**：
  - 支持 Host / Server Reflexive / Relay 三类 Candidate
  - 支持 Candidate Pair 优先级排序
  - 连通性检测正常工作
  - 支持 ICE Restart
- **预估工时**：5 天
- **优先级**：P0
- **依赖**：P2-T01、P2-T02

#### P2-T04：UDP P2P 打洞实现

- **目标**：基于 ICE 协商结果实现 UDP 直连打洞
- **输入**：ICE 协商模块
- **输出**：P2P 连接建立功能
- **验收标准**：
  - Full Cone NAT 环境 100% 打洞成功
  - Restricted NAT 环境 > 80% 打洞成功
  - 打洞超时自动降级到 Relay
  - 打洞耗时 < 5 秒
- **预估工时**：5 天
- **优先级**：P0
- **依赖**：P2-T03

#### P2-T05：WireGuard 隧道集成

- **目标**：在 P2P 连接之上建立 WireGuard 加密隧道
- **输入**：wireguard-go 库 + TUN 接口
- **输出**：WireGuard tunnel 模块
- **验收标准**：
  - P2P 连接上可建立 WireGuard 隧道
  - 数据加密传输，Wireshark 无法解密
  - 支持密钥轮换
  - TUN 接口正常收发数据包
- **预估工时**：7 天
- **优先级**：P0
- **依赖**：P2-T04

#### P2-T06：Mesh 网络基础

- **目标**：支持多节点 Mesh 组网，任意两节点可互通
- **输入**：WireGuard 隧道模块 + Control Plane 路由表
- **输出**：Mesh 路由管理模块
- **验收标准**：
  - 3+ 节点可自动组成 Mesh 网络
  - 新节点加入自动发现并建立连接
  - 节点离线自动从路由表移除
  - 支持子网路由
- **预估工时**：7 天
- **优先级**：P1
- **依赖**：P2-T05

#### P2-T07：OIDC 认证集成

- **目标**：集成 OpenID Connect 认证，支持第三方 IdP
- **输入**：OIDC 规范
- **输出**：auth 模块（OIDC 部分）
- **验收标准**：
  - 支持标准 OIDC 流程（Authorization Code Flow）
  - 支持 Google / GitHub / 自建 IdP
  - Token 刷新正常
  - 设备授权流程（Device Authorization Grant）
- **预估工时**：5 天
- **优先级**：P1
- **并行关系**：可与 P2-T05、P2-T06 并行

#### P2-T08：NAT 检测服务端

- **目标**：部署独立的 NAT 检测服务供客户端使用
- **输入**：STUN 规范
- **输出**：nat-detector 服务
- **验收标准**：
  - 支持高并发检测请求
  - 准确返回 NAT 类型
  - 多地域部署支持
- **预估工时**：3 天
- **优先级**：P1
- **并行关系**：可与 P2-T04 并行

---

### 6.3 Phase 3 任务分解

#### P3-T01：QUIC 传输层实现

- **目标**：基于 quic-go 实现 QUIC 传输通道
- **输入**：quic-go 库
- **输出**：QUIC transport 模块
- **验收标准**：
  - 支持 QUIC 双向流
  - 支持 0-RTT 连接恢复
  - 支持连接迁移
  - 多路复用性能达标
- **预估工时**：5 天
- **优先级**：P0

#### P3-T02：链路质量探测

- **目标**：实现实时网络质量探测，采集 RTT、丢包率、可用带宽等指标
- **输入**：探测协议设计
- **输出**：link-probe 模块
- **验收标准**：
  - RTT 探测精度 < 1ms
  - 丢包率统计窗口可配置
  - 带宽估算误差 < 20%
  - 探测开销 < 总带宽的 1%
- **预估工时**：5 天
- **优先级**：P0
- **并行关系**：可与 P3-T01 并行

#### P3-T03：智能路由决策引擎

- **目标**：基于探测数据实现自动路径选择与切换
- **输入**：链路质量数据 + 路由策略配置
- **输出**：link-scheduler 决策模块
- **验收标准**：
  - 支持 5 级优先级链路选择（UDP P2P → QUIC P2P → TCP P2P → Nearby Relay → Global Relay）
  - 路径切换耗时 < 500ms
  - 切换过程不丢包（或丢包 < 3 个）
  - 支持手动锁定路径
- **预估工时**：7 天
- **优先级**：P0
- **依赖**：P3-T01、P3-T02

#### P3-T04：多 Relay 节点管理

- **目标**：支持多个 Relay 节点，客户端自动选择最优节点
- **输入**：Relay 节点列表 + 探测数据
- **输出**：relay-manager 模块
- **验收标准**：
  - 自动探测所有可用 Relay 节点延迟
  - 自动选择 RTT 最低的节点
  - 节点故障时自动切换（< 2 秒）
  - 支持地理位置就近选择
- **预估工时**：4 天
- **优先级**：P1
- **依赖**：P3-T01

#### P3-T05：连接迁移（Network Handoff）

- **目标**：网络环境变化时（WiFi → 4G）连接不断线
- **输入**：QUIC 连接迁移特性
- **输出**：connection-migration 模块
- **验收标准**：
  - WiFi → 4G 切换时连接保持
  - 切换期间丢包 < 5 个
  - 切换恢复时间 < 1 秒
- **预估工时**：5 天
- **优先级**：P1
- **依赖**：P3-T01

#### P3-T06：Control Plane gRPC 服务完善

- **目标**：完善控制面服务，支持完整的设备管理与 ACL
- **输入**：gRPC proto 定义
- **输出**：完整 Control Plane 服务
- **验收标准**：
  - 支持 1000+ 节点注册
  - ACL 规则实时下发（< 1 秒）
  - 密钥轮换自动化
  - 支持 RBAC 权限模型
- **预估工时**：7 天
- **优先级**：P1
- **并行关系**：可与 P3-T03 并行

---

### 6.4 Phase 4 任务分解

#### P4-T01：边缘节点部署架构

- **目标**：设计并实现全球边缘节点部署方案
- **输入**：全球机房分布 + 网络拓扑
- **输出**：边缘节点部署自动化脚本 + 架构文档
- **所属模块**：`server/internal/edge/`
- **子任务分解**：
  - P4-T01-1：边缘节点数据模型（EdgeNode 结构、Region/Status 枚举）
  - P4-T01-2：边缘节点注册表（Registry 增删查改、按区域索引）
  - P4-T01-3：健康检查器（心跳探测、延迟探测、自动摘除不健康节点）
  - P4-T01-4：部署自动化（Docker Compose 模板生成、一键部署脚本）
  - P4-T01-5：Control Plane 集成（边缘节点自动注册至控制面）
- **验收标准**：
  - 支持一键部署新边缘节点
  - 节点自动注册至 Control Plane
  - 健康检查与自动摘除（< 5秒检测失败节点）
  - 单元测试覆盖率 > 80%
- **预估工时**：7 天
- **优先级**：P0

#### P4-T02：Anycast 路由实现

- **目标**：通过 Anycast 将用户引导至最近的边缘节点
- **输入**：BGP Anycast 方案 + 边缘节点注册表
- **输出**：Anycast 配置 + GeoDNS 方案
- **所属模块**：`server/internal/anycast/`
- **子任务分解**：
  - P4-T02-1：Anycast 路由器核心逻辑（节点距离计算、最近节点选择）
  - P4-T02-2：GeoDNS 解析引导（基于客户端IP地理位置的智能解析）
  - P4-T02-3：故障自动切换（节点不健康时自动切换至次近节点）
- **验收标准**：
  - 用户请求自动路由至最近节点
  - 节点故障时自动切换至次近节点（< 3秒）
  - 全球延迟降低 > 30%
  - 单元测试覆盖率 > 80%
- **预估工时**：5 天
- **优先级**：P1
- **依赖**：P4-T01

#### P4-T03：eBPF 网络加速

- **目标**：利用 eBPF 实现内核态数据转发加速（Linux）
- **输入**：eBPF 程序设计 + Relay 服务器架构
- **输出**：eBPF 加速模块
- **所属模块**：`server/internal/ebpf/`
- **子任务分解**：
  - P4-T03-1：eBPF 程序加载器（BPF 程序加载、映射管理、条件编译）
  - P4-T03-2：XDP 转发程序（内核态快速路径转发、UDP 报文处理）
  - P4-T03-3：优雅降级机制（无 eBPF 支持时自动回退用户态）
  - P4-T03-4：性能监控（吞吐量、延迟、CPU 使用率统计）
- **验收标准**：
  - Relay 转发吞吐提升 > 50%
  - 转发延迟降低 > 30%
  - 不影响系统稳定性
  - 优雅降级（无 eBPF 时走用户态）
  - 仅 Linux 平台，条件编译 `//go:build linux`
- **预估工时**：10 天
- **优先级**：P2
- **并行关系**：可与 P4-T02 并行

#### P4-T04：SD-WAN 流量策略

- **目标**：实现基于应用/协议/目的地的流量策略路由
- **输入**：策略规则引擎设计 + 流量分类需求
- **输出**：SD-WAN policy engine
- **所属模块**：`server/internal/sdwan/`
- **子任务分解**：
  - P4-T04-1：流量分类器（基于端口/协议/DPI 识别应用类型）
  - P4-T04-2：策略引擎（规则匹配、优先级评估、动作执行）
  - P4-T04-3：QoS 管理器（优先级队列、带宽限制、流量整形）
  - P4-T04-4：策略热更新（运行时规则修改、实时生效）
- **验收标准**：
  - 支持基于应用（HTTP/SSH/RDP等）的策略路由
  - 支持 QoS 优先级（8级优先级队列）
  - 支持带宽限制（per-rule 粒度）
  - 策略实时生效（< 1 秒）
  - 单元测试覆盖率 > 80%
- **预估工时**：8 天
- **优先级**：P1
- **依赖**：P4-T01

#### P4-T05：管理控制台（Web Dashboard）

- **目标**：为管理员提供全局可视化管理界面后端 API
- **输入**：UI 设计 + Control Plane API
- **输出**：Web 管理控制台后端服务
- **所属模块**：`server/internal/dashboard/`
- **子任务分解**：
  - P4-T05-1：HTTP API 服务器（路由注册、中间件、CORS）
  - P4-T05-2：管理员认证（JWT 登录、角色权限验证）
  - P4-T05-3：节点管理 API（列表/详情/状态/删除）
  - P4-T05-4：流量统计 API（实时带宽、历史趋势、节点维度）
  - P4-T05-5：ACL 管理 API（规则 CRUD、批量操作）
  - P4-T05-6：告警通知 API（告警规则、通知渠道、历史记录）
- **验收标准**：
  - 节点状态实时监控
  - 流量统计与可视化 API
  - ACL 规则管理（CRUD + 批量操作）
  - 用户/设备管理
  - 告警通知
  - 单元测试覆盖率 > 80%
- **预估工时**：10 天
- **优先级**：P1
- **并行关系**：可与 P4-T03、P4-T04 并行
- **备注**：前端 Web Dashboard 为独立项目，本任务仅实现后端 RESTful API

---

## 七、质量保障策略

### 7.1 测试策略

#### 单元测试

| 维度 | 要求 |
|------|------|
| 覆盖率目标 | 核心模块 > 80%，工具模块 > 60% |
| 测试框架 | Go: `testing` + `testify`；Vue: `vitest` + `@vue/test-utils` |
| 运行时机 | 每次 commit 前 + CI Pipeline |
| 关注点 | 协议解析、状态机转换、边界条件、并发安全 |

#### 集成测试

| 维度 | 要求 |
|------|------|
| 范围 | 模块间交互、网络协议栈、数据库操作 |
| 环境 | Docker Compose 模拟多节点 |
| 关注点 | 隧道建立全流程、打洞成功率、Relay 降级、重连机制 |
| 运行时机 | PR 合并前 + 每日定时 |

#### E2E 测试

| 维度 | 要求 |
|------|------|
| 范围 | 完整用户场景：创建隧道 → 数据传输 → 断线恢复 |
| 环境 | 多容器模拟不同 NAT 环境（iptables 模拟） |
| 工具 | 自研测试框架 + Docker 网络隔离 |
| 关注点 | 跨 NAT 连通性、性能达标、UI 操作正确性 |

#### 网络模拟测试

| 维度 | 要求 |
|------|------|
| 工具 | `tc`（traffic control）/ `netem` |
| 模拟场景 | 高延迟、高丢包、带宽限制、网络抖动、NAT 类型变化 |
| 目的 | 验证智能路由决策正确性、降级策略有效性 |

### 7.2 CI/CD 规划

```
┌─────────────────────────────────────────────────────┐
│                    CI Pipeline                       │
├─────────────────────────────────────────────────────┤
│  Trigger: Push / PR                                 │
│                                                     │
│  1. Lint (golangci-lint + eslint)                   │
│  2. Unit Test (go test + vitest)                    │
│  3. Build Check (go build + vite build)             │
│  4. Integration Test (docker-compose)               │
│  5. Security Scan (govulncheck + npm audit)         │
│  6. Coverage Report                                 │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│                    CD Pipeline                       │
├─────────────────────────────────────────────────────┤
│  Trigger: Tag / Release                             │
│                                                     │
│  1. Cross-compile (Windows/macOS/Linux)             │
│  2. Wails Package (Desktop Installer)              │
│  3. Docker Image Build (Server)                     │
│  4. E2E Test (Staging)                              │
│  5. Release Artifacts (GitHub Releases)             │
│  6. Docker Registry Push                            │
└─────────────────────────────────────────────────────┘
```

**CI/CD 工具选型**：
- CI 平台：GitHub Actions
- 容器化：Docker + Docker Compose
- 代码质量：golangci-lint、ESLint、Prettier
- 安全扫描：govulncheck、npm audit、trivy（容器镜像扫描）
- 发布管理：GoReleaser（Go 多平台构建）+ Wails CLI（桌面打包）

### 7.3 性能基准

| 指标 | Phase 1 目标 | Phase 2 目标 | Phase 3 目标 |
|------|-------------|-------------|-------------|
| TCP Tunnel 吞吐 | > 500 Mbps | > 800 Mbps | > 1 Gbps |
| HTTP Tunnel QPS | > 5000 | > 8000 | > 10000 |
| P2P 连接建立时间 | - | < 5 秒 | < 3 秒 |
| Relay 延迟开销 | < 50ms | < 30ms | < 20ms |
| 内存占用（客户端） | < 50MB | < 80MB | < 100MB |
| CPU 占用（空闲） | < 1% | < 1% | < 1% |
| 打洞成功率 | - | > 70% | > 85% |
| 连接迁移恢复时间 | - | - | < 1 秒 |

---

## 八、风险与挑战

### 8.1 技术风险

| 风险点 | 严重程度 | 可能性 | 影响 | 缓解策略 |
|--------|---------|--------|------|---------|
| **Symmetric NAT 穿透困难** | 高 | 高 | 企业网络 P2P 失败率高 | 1. 完善 TURN Relay 降级<br>2. TCP 打洞尝试<br>3. 端口预测算法 |
| **运营商 UDP 限制/丢弃** | 高 | 中 | 部分网络 QUIC/WireGuard 不可用 | 1. UDP/TCP 自动切换<br>2. QUIC over TCP 伪装<br>3. 多协议探测 |
| **WireGuard 内核态依赖** | 中 | 中 | 部分系统无 TUN 设备权限 | 1. 用户态 WireGuard 实现<br>2. gVisor netstack 方案<br>3. 管理员权限引导 |
| **跨平台 TUN 接口差异** | 中 | 高 | Windows/macOS/Linux 行为不一致 | 1. 平台抽象层<br>2. 各平台独立测试<br>3. 条件编译 |
| **QUIC 库稳定性** | 中 | 低 | quic-go 某些边界场景有 bug | 1. 固定版本<br>2. 集成测试覆盖<br>3. 关注上游 issue |
| **移动网络连接迁移** | 中 | 中 | WiFi↔4G 切换时连接中断 | 1. QUIC Connection Migration<br>2. 智能重连<br>3. 会话保持 |
| **密钥管理安全** | 高 | 低 | 密钥泄露导致数据被截获 | 1. 密钥定期轮换<br>2. 硬件安全模块(HSM)<br>3. 零知识证明 |

### 8.2 工程风险

| 风险点 | 缓解策略 |
|--------|---------|
| 模块耦合度过高 | 严格定义模块接口，通过 interface 解耦，依赖注入 |
| 并发安全问题 | Go race detector 持续运行，channel 优先于锁 |
| 性能退化 | 建立性能基准，CI 中持续跑 benchmark，regression 告警 |
| 第三方库变更 | go.sum 锁定版本，定期评估升级，fork 关键库 |
| 测试环境不足 | Docker 模拟各种 NAT，云主机多地域测试 |

### 8.3 产品风险

| 风险点 | 缓解策略 |
|--------|---------|
| 用户体验复杂 | UI 屏蔽技术细节，一键连接，状态可视化 |
| 安装权限门槛 | 提供免安装模式（用户态网络），平台适配文档 |
| 竞品追赶 | 差异化功能（智能路由可视化），开源社区生态 |

---

## 附录 A：目录结构规划

```
NexTunnel/
├── desktop/                    # Wails 桌面客户端
│   ├── app.go                  # Wails 应用入口
│   ├── main.go                 # 主程序入口
│   ├── frontend/               # Vue 3 前端
│   │   ├── src/
│   │   │   ├── components/     # UI 组件
│   │   │   ├── views/          # 页面视图
│   │   │   ├── stores/         # Pinia 状态管理
│   │   │   ├── api/            # Wails binding 调用
│   │   │   └── assets/         # 静态资源
│   │   ├── index.html
│   │   ├── vite.config.ts
│   │   └── package.json
│   ├── internal/               # 内部模块
│   │   ├── tunnel/             # 隧道核心
│   │   ├── p2p/                # P2P 引擎
│   │   ├── nat/                # NAT 穿透
│   │   ├── relay/              # Relay 客户端
│   │   ├── scheduler/          # 链路调度
│   │   ├── config/             # SQLite 配置
│   │   └── auth/               # 认证模块
│   ├── wails.json
│   └── go.mod
├── server/                     # 独立服务端
│   ├── cmd/
│   │   ├── control-plane/      # 控制面入口
│   │   ├── relay/              # Relay 服务入口
│   │   └── nat-detector/       # NAT 检测服务入口
│   ├── internal/
│   │   ├── controlplane/       # 控制面逻辑
│   │   ├── relay/              # Relay 逻辑
│   │   ├── natdetect/          # NAT 检测逻辑
│   │   └── common/             # 公共组件
│   ├── proto/                  # gRPC Proto 定义
│   ├── migrations/             # 数据库迁移
│   ├── Dockerfile
│   └── go.mod
├── pkg/                        # 公共包（客户端与服务端共享）
│   ├── protocol/               # 协议定义
│   ├── crypto/                 # 加密工具
│   └── types/                  # 共享类型
├── docs/                       # 项目文档
├── scripts/                    # 构建与部署脚本
├── .github/                    # GitHub Actions CI/CD
│   └── workflows/
├── docker-compose.yml          # 本地开发环境
├── Makefile                    # 构建命令
└── README.md
```

---

## 附录 B：关键协议交互时序

### B.1 P2P 连接建立时序

```
Client A          Control Plane         STUN Server         Client B
   │                    │                    │                  │
   │── Register ───────→│                    │                  │
   │                    │                    │    ←── Register ──│
   │── Connect(B) ────→│                    │                  │
   │                    │── Notify(A wants)─────────────────────→│
   │←── B's PublicKey ──│                    │                  │
   │                    │── A's PublicKey ──────────────────────→│
   │── STUN Binding ───────────────────────→│                  │
   │←── Mapped Addr ───────────────────────│                  │
   │                    │                    │── STUN Binding ──→│
   │                    │                    │←── Mapped Addr ──│
   │── ICE Candidates → Control Plane → ICE Candidates ───────→│
   │←── ICE Candidates ← Control Plane ← ICE Candidates ──────│
   │                                                            │
   │◄══════════════ UDP P2P Punch Through ════════════════════►│
   │◄══════════════ WireGuard Handshake ══════════════════════►│
   │◄══════════════ Encrypted Data Transfer ══════════════════►│
   │                                                            │
```

### B.2 Relay 降级时序

```
Client A          Link Scheduler        Relay Server         Client B
   │                    │                    │                  │
   │── P2P Failed ─────→│                    │                  │
   │                    │── Select Relay ───→│                  │
   │←── Use Relay ──────│                    │                  │
   │── Relay Connect ──────────────────────→│                  │
   │                    │                    │←── Relay Connect──│
   │── Data (encrypted) ───────────────────→│── Forward ──────→│
   │←── Data (encrypted) ──────────────────│←── Forward ──────│
   │                                                            │
   │  [Background: Keep trying P2P]                            │
   │── P2P Retry ──────────────────────────────────────────────→│
   │── P2P Success! ───→│                    │                  │
   │                    │── Switch to P2P ──→│                  │
   │◄══════════════ P2P Direct ═══════════════════════════════►│
```

---

## 附录 C：技术对标参考

| 能力 | Tailscale | NetBird | NexTunnel（目标） |
|------|-----------|---------|------------------|
| P2P 直连 | ✓ | ✓ | ✓ |
| WireGuard 加密 | ✓ | ✓ | ✓ |
| NAT 穿透 | DERP | ICE | ICE + QUIC |
| 中继节点 | DERP | coturn | QUIC Relay |
| 智能路由 | 有限 | 基础 | 多维度智能调度 |
| QUIC 传输 | ✗ | ✗ | ✓ |
| 连接迁移 | 部分 | ✗ | ✓ |
| 开源程度 | 客户端开源 | 全开源 | 全开源 |
| 桌面 UI | 有 | 有 | Wails 原生 |
| eBPF 加速 | ✗ | ✗ | ✓（Phase 4） |
| SD-WAN | ✗ | ✗ | ✓（Phase 4） |
