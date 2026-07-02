# P2P网络系统

<cite>
**本文档引用的文件**
- [README.md](file://README.md)
- [app.go](file://desktop/app.go)
- [main.go](file://desktop/main.go)
- [engine.go](file://desktop/internal/p2p/engine.go)
- [ice.go](file://desktop/internal/p2p/ice.go)
- [mesh.go](file://desktop/internal/p2p/mesh.go)
- [wireguard.go](file://desktop/internal/p2p/wireguard.go)
- [punch.go](file://desktop/internal/p2p/punch.go)
- [detect.go](file://desktop/internal/nat/detect.go)
- [stun.go](file://desktop/internal/nat/stun.go)
- [types.go](file://desktop/internal/nat/types.go)
- [manager.go](file://desktop/internal/tunnel/manager.go)
- [message.go](file://pkg/protocol/message.go)
- [keys.go](file://pkg/crypto/keys.go)
- [types.go](file://pkg/types/types.go)
- [tunnel.ts](file://desktop/frontend/src/stores/tunnel.ts)
- [StatusView.vue](file://desktop/frontend/src/views/StatusView.vue)
</cite>

## 更新摘要
**变更内容**
- 更新了P2P引擎的完整实现状态，包括WebRTC ICE代理、UDP打洞、WireGuard隧道等核心组件
- 新增了NAT检测器的详细实现和RFC 3489标准算法
- 完善了Mesh网络路由器的功能描述和路由管理机制
- 增强了协议消息流和加密密钥管理的详细说明
- 更新了性能考虑和故障排除指南

## 目录
1. [简介](#简介)
2. [项目结构](#项目结构)
3. [核心组件](#核心组件)
4. [架构概览](#架构概览)
5. [详细组件分析](#详细组件分析)
6. [依赖关系分析](#依赖关系分析)
7. [性能考虑](#性能考虑)
8. [故障排除指南](#故障排除指南)
9. [结论](#结论)

## 简介

NexTunnel是一个开源的内网穿透和P2P直连网络工具，采用Go + Vue 3 + Wails技术栈构建。该项目旨在超越传统的FRP/NPS等"客户端→中转服务器"的TCP转发模式，打造下一代智能组网方案。

### 核心特性
- **P2P优先**：数据直连传输，不经过中继服务器，降低延迟与带宽成本
- **智能链路**：自动检测网络环境，选择最优传输路径  
- **安全零信任**：端到端加密，WireGuard级安全保障
- **自动降级**：P2P不可达时自动切换至中继，保证连通性
- **可视化桌面端**：基于Wails的原生桌面应用，屏蔽技术细节，一键连接
- **跨平台**：覆盖Windows/macOS/Linux

### 技术栈
- 客户端核心：Go 1.25
- 桌面框架：Wails v2
- 前端：Vue 3 + Vite + TypeScript
- 状态管理：Pinia
- 本地存储：SQLite（modernc.org/sqlite，纯Go实现）
- STUN：pion/stun v2
- WireGuard：wireguard-go
- 中继传输：QUIC
- 服务端：Go + Docker
- CI/CD：GitHub Actions

## 项目结构

```mermaid
graph TB
subgraph "桌面端 (desktop/)"
A[main.go<br/>应用入口]
B[app.go<br/>Wails应用结构]
C[frontend/<br/>Vue 3前端]
D[internal/<br/>内部模块]
end
subgraph "P2P引擎 (internal/p2p/)"
E[engine.go<br/>P2P引擎]
F[ice.go<br/>ICE协议]
G[mesh.go<br/>Mesh网络]
H[wireguard.go<br/>WireGuard隧道]
I[punch.go<br/>UDP打洞]
J[nat/detect.go<br/>NAT检测]
end
subgraph "公共包 (pkg/)"
K[protocol/<br/>协议定义]
L[crypto/<br/>加密工具]
M[types/<br/>共享类型]
end
subgraph "服务端 (server/)"
N[cmd/<br/>服务入口]
O[internal/<br/>业务逻辑]
end
A --> B
B --> C
B --> D
D --> E
E --> F
E --> G
E --> H
E --> I
E --> J
K --> E
L --> E
M --> E
```

**图表来源**
- [main.go:1-37](file://desktop/main.go#L1-L37)
- [app.go:17-24](file://desktop/app.go#L17-L24)
- [engine.go:56-71](file://desktop/internal/p2p/engine.go#L56-L71)

**章节来源**
- [README.md:39-96](file://README.md#L39-L96)
- [README.md:163-195](file://README.md#L163-L195)

## 核心组件

### P2P引擎 (Engine)
P2P引擎是整个系统的核心协调器，负责完整的P2P连接流程：NAT检测 → ICE协商 → 打洞 → WireGuard隧道建立。

```mermaid
classDiagram
class Engine {
+EngineConfig config
+NATResult natResult
+string wgPrivKey
+string wgPubKey
+sync.Map sessions
+atomic.Value state
+GetState() SessionState
+GetNATType() string
+DetectNAT(ctx) (*NATResult, error)
+InitiateP2P(ctx, peerID) (Transport, error)
+HandleP2POffer(ctx, offer) (Transport, error)
+HandleP2PAnswer(ctx, answer) error
+Close() void
}
class Session {
+string ID
+string PeerID
+atomic.Value State
+Agent iceAgent
+PunchEngine punchEng
+WGTunnel wgTunnel
+p2pTransport transport
}
class Transport {
<<interface>>
+Read([]byte) (int, error)
+Write([]byte) (int, error)
+Close() error
+LocalAddr() net.Addr
+RemoteAddr() net.Addr
}
Engine --> Session : "管理多个会话"
Session --> Transport : "返回传输接口"
```

**图表来源**
- [engine.go:56-82](file://desktop/internal/p2p/engine.go#L56-L82)
- [engine.go:109-125](file://desktop/internal/p2p/engine.go#L109-L125)

### ICE协议实现 (Agent)
ICE（Interactive Connectivity Establishment）协议实现负责候选地址收集和连通性检查。

```mermaid
classDiagram
class Agent {
+AgentConfig config
+atomic.Value state
+net.UDPConn udpConn
+STUNClient stunClient
+[]Candidate localCandidates
+[]Candidate remoteCandidates
+[]CandidatePair checklist
+map[TransactionID]chan *stun.Message pendingChecks
+atomic.Value selectedPair
+GatherCandidates(ctx) ([]Candidate, error)
+AddRemoteCandidate(Candidate) void
+StartChecks(ctx) error
+GetSelectedPair() *CandidatePair
+GetUDPConn() *net.UDPConn
+Close() error
}
class Candidate {
+string ID
+CandidateType Type
+net.UDPAddr Addr
+uint32 Priority
+string Foundation
}
class CandidatePair {
+Candidate Local
+Candidate Remote
+PairState State
+uint64 Priority
+bool Nominated
+time.Time LastCheckAt
}
Agent --> Candidate : "管理本地/远程候选"
Candidate --> CandidatePair : "组合成配对"
Agent --> CandidatePair : "检查连通性"
```

**图表来源**
- [ice.go:98-127](file://desktop/internal/p2p/ice.go#L98-L127)
- [ice.go:29-76](file://desktop/internal/p2p/ice.go#L29-L76)

### Mesh网络路由器 (MeshRouter)
Mesh路由器管理多个P2P对等连接，形成网状网络拓扑。

```mermaid
classDiagram
class MeshRouter {
+MeshConfig config
+atomic.Value state
+map[string]*PeerInfo peers
+map[string]*RouteEntry routes
+string localSubnet
+ConnectToPeer(peerID) void
+HandleMeshPeerList(msg) void
+HandleMeshPing(fromID) void
+HandleMeshPong(fromID) void
+AddPeerRoute(peerID, transport, subnet) void
+RemovePeerRoute(peerID) void
+LookupRoute(targetPeerID) *RouteEntry
+Stats() MeshStats
}
class PeerInfo {
+string ClientID
+string NATType
+string WGPubKey
+bool Connected
+string Subnet
+time.Time LastSeen
}
class RouteEntry {
+string PeerID
+Transport Transport
+string Subnet
+time.Time AddedAt
+atomic.Int64 BytesIn
+atomic.Int64 BytesOut
}
MeshRouter --> PeerInfo : "跟踪对等节点"
MeshRouter --> RouteEntry : "维护路由表"
```

**图表来源**
- [mesh.go:65-79](file://desktop/internal/p2p/mesh.go#L65-L79)
- [mesh.go:25-43](file://desktop/internal/p2p/mesh.go#L25-L43)

**章节来源**
- [engine.go:56-107](file://desktop/internal/p2p/engine.go#L56-L107)
- [ice.go:98-143](file://desktop/internal/p2p/ice.go#L98-L143)
- [mesh.go:65-94](file://desktop/internal/p2p/mesh.go#L65-L94)

## 架构概览

### 系统架构

```mermaid
graph TB
subgraph "客户端A"
A1[Wails应用]
A2[Tunnel Manager]
A3[P2P Engine]
A4[NAT检测器]
A5[ICE Agent]
A6[WireGuard隧道]
A7[Mesh路由器]
end
subgraph "中继服务器"
R1[控制平面]
R2[中继节点]
R3[NAT检测服务]
end
subgraph "客户端B"
B1[Wails应用]
B2[Tunnel Manager]
B3[P2P Engine]
B4[NAT检测器]
B5[ICE Agent]
B6[WireGuard隧道]
B7[Mesh路由器]
end
A1 --> A2
A2 --> A3
A3 --> A4
A3 --> A5
A3 --> A6
A3 --> A7
A4 --> R3
A5 --> R1
A6 --> R2
A7 --> R1
R1 --> B1
R2 --> B6
R3 --> B4
A7 --> B7
B7 --> A7
```

**图表来源**
- [README.md:100-130](file://README.md#L100-L130)
- [engine.go:127-143](file://desktop/internal/p2p/engine.go#L127-L143)
- [mesh.go:138-165](file://desktop/internal/p2p/mesh.go#L138-L165)

### 链路调度策略

```mermaid
flowchart TD
Start([开始连接]) --> DetectNAT["检测NAT类型"]
DetectNAT --> EvaluateRTT["评估RTT/丢包率"]
EvaluateRTT --> ChoosePath{"选择最优路径"}
ChoosePath --> |Full Cone NAT| UDPDirect["UDP直连"]
ChoosePath --> |Restricted NAT| QUICP2P["QUIC P2P打洞"]
ChoosePath --> |Symmetric NAT| TCPP2P["TCP P2P尝试"]
ChoosePath --> |P2P全部失败| NearbyRelay["就近中继"]
ChoosePath --> |全部不可用| GlobalRelay["全球中继"]
UDPDirect --> Success([连接成功])
QUICP2P --> Success
TCPP2P --> Success
NearbyRelay --> Success
GlobalRelay --> Success
```

**图表来源**
- [README.md:150-159](file://README.md#L150-L159)
- [detect.go:29-137](file://desktop/internal/nat/detect.go#L29-L137)

## 详细组件分析

### P2P连接建立流程

```mermaid
sequenceDiagram
participant ClientA as 客户端A
participant EngineA as P2P引擎A
participant ClientB as 客户端B
participant EngineB as P2P引擎B
ClientA->>EngineA : InitiateP2P(目标客户端ID)
EngineA->>EngineA : DetectNAT()
EngineA->>EngineA : GatherCandidates()
EngineA->>ClientB : 发送Offer(包含候选地址)
ClientB->>EngineB : 接收Offer
EngineB->>EngineB : DetectNAT()
EngineB->>EngineB : GatherCandidates()
EngineB->>EngineB : AddRemoteCandidate(来自Offer)
EngineB->>ClientA : 发送Answer(接受/拒绝)
ClientA->>EngineA : 接收Answer
EngineA->>EngineA : AddRemoteCandidate(来自Answer)
EngineA->>EngineA : StartChecks()
EngineA->>EngineA : 选择最佳候选对
EngineA->>EngineA : PunchEngine.Punch()
EngineB->>EngineB : PunchEngine.Punch()
EngineA->>EngineA : NewWGTunnel()
EngineB->>EngineB : NewWGTunnel()
EngineA->>ClientA : 返回Transport接口
EngineB->>ClientB : 返回Transport接口
```

**图表来源**
- [engine.go:145-192](file://desktop/internal/p2p/engine.go#L145-L192)
- [engine.go:194-296](file://desktop/internal/p2p/engine.go#L194-L296)
- [engine.go:298-372](file://desktop/internal/p2p/engine.go#L298-L372)

### UDP打洞机制

```mermaid
sequenceDiagram
participant Initiator as 发起方
participant Responder as 响应方
participant NAT1 as 发起方NAT
participant NAT2 as 响应方NAT
Initiator->>NAT1 : 发送SYN包
NAT1->>NAT2 : 转发SYN包
NAT2->>Responder : 到达SYN包
Responder->>NAT2 : 发送SYNACK包
NAT2->>NAT1 : 转发SYNACK包
NAT1->>Initiator : 到达SYNACK包
Initiator->>NAT1 : 发送ACK包
NAT1->>NAT2 : 转发ACK包
NAT2->>Responder : 到达ACK包
Note over Initiator,Responder : 打洞完成，建立双向通道
```

**图表来源**
- [punch.go:81-131](file://desktop/internal/p2p/punch.go#L81-L131)
- [punch.go:143-201](file://desktop/internal/p2p/punch.go#L143-L201)

### WireGuard隧道实现

```mermaid
classDiagram
class WGTunnel {
+WGConfig config
+Device dev
+netTun tun
+atomic.Value status
+Start(bind) error
+TUN() *netTun
+Close() error
+Status() WGState
}
class netBind {
+net.UDPConn udpConn
+net.UDPAddr endpoint
+atomic.Bool closed
+Open(port) ([]ReceiveFunc, uint16, error)
+Send(bufs, endpoint) error
+Close() error
}
class netEP {
+net.UDPAddr addr
+netip.Addr addrIP
+DstIP() netip.Addr
+DstToString() string
}
WGTunnel --> netBind : "使用"
netBind --> netEP : "封装"
```

**图表来源**
- [wireguard.go:37-57](file://desktop/internal/p2p/wireguard.go#L37-L57)
- [wireguard.go:114-123](file://desktop/internal/p2p/wireguard.go#L114-L123)
- [wireguard.go:173-185](file://desktop/internal/p2p/wireguard.go#L173-L185)

### NAT检测器实现

```mermaid
classDiagram
class Detector {
+string serverAddr
+string altServerAddr
+STUNClient stunClient
+Detect(ctx) (*NATResult, error)
}
class STUNClient {
+BindingRequest(ctx, serverAddr, localConn) (*STUNBinding, error)
+BindingRequestFromAlt(ctx, serverAddr, localConn) (*STUNBinding, error)
}
class NATResult {
+NATType Type
+string PublicAddr
+uint16 MappedPort
+string LocalAddr
+IsP2PPossible(peerNAT) bool
}
Detector --> STUNClient : "使用"
Detector --> NATResult : "返回"
```

**图表来源**
- [detect.go:29-137](file://desktop/internal/nat/detect.go#L29-L137)
- [stun.go:65-114](file://desktop/internal/nat/stun.go#L65-L114)
- [types.go:18-37](file://desktop/internal/nat/types.go#L18-L37)

**章节来源**
- [engine.go:145-372](file://desktop/internal/p2p/engine.go#L145-L372)
- [punch.go:58-137](file://desktop/internal/p2p/punch.go#L58-L137)
- [wireguard.go:37-112](file://desktop/internal/p2p/wireguard.go#L37-L112)
- [detect.go:29-137](file://desktop/internal/nat/detect.go#L29-L137)

## 依赖关系分析

### 协议消息流

```mermaid
graph LR
subgraph "控制通道消息"
A[TypeAuth<br/>认证请求]
B[TypeAuthResp<br/>认证响应]
C[TypeNewProxy<br/>创建代理]
D[TypeNewProxyResp<br/>代理响应]
E[TypeStartWorkConn<br/>开始工作连接]
F[TypeWorkConn<br/>工作连接]
G[TypeHeartbeat<br/>心跳]
H[TypeHeartbeatResp<br/>心跳响应]
end
subgraph "P2P信令消息"
I[TypeNATDetectReq<br/>NAT检测请求]
J[TypeNATDetectResp<br/>NAT检测响应]
K[TypeP2POffer<br/>P2P提议]
L[TypeP2PAnswer<br/>P2P应答]
M[TypeP2PClose<br/>P2P关闭]
end
subgraph "Mesh网络消息"
N[TypeMeshJoin<br/>加入Mesh]
O[TypeMeshPeerList<br/>对等列表]
P[TypeMeshLeave<br/>离开Mesh]
Q[TypeMeshPing<br/>Mesh Ping]
R[TypeMeshPong<br/>Mesh Pong]
end
A --> B
C --> D
E --> F
G --> H
I --> J
K --> L
L --> M
N --> O
O --> P
Q --> R
```

**图表来源**
- [message.go:6-33](file://pkg/protocol/message.go#L6-L33)
- [message.go:95-144](file://pkg/protocol/message.go#L95-L144)
- [message.go:146-184](file://pkg/protocol/message.go#L146-L184)

### 加密密钥管理

```mermaid
flowchart TD
Start([生成密钥对]) --> GenPriv["生成随机私钥<br/>32字节"]
GenPriv --> Clamp["Clamp私钥<br/>符合Curve25519规范"]
Clamp --> ComputePub["计算公钥<br/>X25519(private, Basepoint)"]
ComputePub --> Encode["Base64编码<br/>私钥和公钥"]
Encode --> Store["存储到配置<br/>用于WireGuard"]
Store --> PSK["生成预共享密钥<br/>32字节随机数"]
PSK --> PSKEncode["Base64编码PSK"]
PSKEncode --> PSKStore["存储PSK<br/>增强安全性"]
PSKStore --> End([完成])
```

**图表来源**
- [keys.go:11-32](file://pkg/crypto/keys.go#L11-L32)
- [keys.go:34-42](file://pkg/crypto/keys.go#L34-L42)
- [keys.go:44-59](file://pkg/crypto/keys.go#L44-L59)

**章节来源**
- [message.go:6-432](file://pkg/protocol/message.go#L6-L432)
- [keys.go:11-60](file://pkg/crypto/keys.go#L11-L60)

## 性能考虑

### NAT类型检测算法

系统实现了RFC 3489标准的NAT类型检测算法，通过四个测试步骤确定NAT行为：

1. **测试I (Basic Binding)**：验证STUN服务器可达性
2. **测试II (Alternate Server)**：检查是否能从备用IP访问
3. **测试III (Different Server)**：验证映射地址随服务器变化
4. **综合判断**：根据测试结果确定NAT类型

### ICE候选地址优先级

候选地址的优先级计算公式：
```
优先级 = TypePreference << 24 + LocalPreference << 8 + (256 - ComponentID)
```

不同类型候选的TypePreference值：
- Host候选：126
- Server Reflexive候选：100  
- Relay候选：0

### 连接池优化

Mesh路由器实现了连接池管理，包括：
- 最大对等节点数量限制
- 健康检查循环（默认15秒间隔）
- 超时处理（默认60秒超时）
- 自动路由清理

## 故障排除指南

### 常见问题诊断

#### P2P连接失败
1. **检查NAT类型**：不同NAT类型影响P2P成功率
2. **验证STUN服务器**：确保STUN服务器可达
3. **检查防火墙设置**：确认UDP端口未被阻断
4. **查看ICE候选**：确认候选地址收集正常

#### WireGuard隧道问题
1. **验证密钥格式**：确保Base64编码正确
2. **检查IP地址配置**：确认子网分配合理
3. **验证端点可达性**：确认对等节点网络连通

#### Mesh网络异常
1. **检查路由表**：确认路由条目正确
2. **验证健康检查**：确认Ping/Pong消息正常
3. **监控连接状态**：查看对等节点连接状态

### 日志分析

系统提供了详细的日志记录，包括：
- NAT检测过程日志
- ICE候选收集日志  
- 打洞过程日志
- WireGuard隧道状态日志
- Mesh网络路由日志

**章节来源**
- [detect.go:29-137](file://desktop/internal/nat/detect.go#L29-L137)
- [ice.go:300-357](file://desktop/internal/p2p/ice.go#L300-L357)
- [wireguard.go:59-94](file://desktop/internal/p2p/wireguard.go#L59-L94)
- [mesh.go:389-441](file://desktop/internal/p2p/mesh.go#L389-L441)

## 结论

NexTunnel项目展现了现代P2P网络技术的完整实现，通过以下关键技术实现了智能化的网络连接：

### 技术优势
1. **完整的P2P栈**：从NAT检测到WireGuard隧道的全栈实现
2. **智能路由选择**：基于网络条件的动态路径选择
3. **安全加密**：端到端加密和密钥管理
4. **可视化管理**：直观的桌面界面和实时状态监控

### 架构特点
- **模块化设计**：清晰的组件分离和接口定义
- **可扩展性**：支持Mesh网络和多对等节点连接
- **容错机制**：自动降级到中继连接
- **性能优化**：高效的候选地址管理和连接池

### 发展方向
项目按照阶段规划逐步演进：
- **Phase 1**：基础隧道功能
- **Phase 2**：P2P直连和Mesh网络
- **Phase 3**：智能调度和多中继节点
- **Phase 4**：全球加速和SD-WAN

该系统为内网穿透和P2P网络应用提供了坚实的技术基础，具有良好的扩展性和实用性。