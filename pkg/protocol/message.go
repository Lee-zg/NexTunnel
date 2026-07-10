// Package protocol defines the wire protocol for NexTunnel control channel communication.
package protocol

import "encoding/json"

// MsgType identifies the type of a protocol message.
type MsgType uint8

const (
	TypeAuth          MsgType = 0x01
	TypeAuthResp      MsgType = 0x02
	TypeNewProxy      MsgType = 0x03
	TypeNewProxyResp  MsgType = 0x04
	TypeCloseProxy    MsgType = 0x05
	TypeStartWorkConn MsgType = 0x06
	TypeWorkConn      MsgType = 0x07
	TypeHeartbeat     MsgType = 0x08
	TypeHeartbeatResp MsgType = 0x09

	// P2P signaling message types (Phase 2)
	TypeNATDetectReq  MsgType = 0x0A
	TypeNATDetectResp MsgType = 0x0B
	TypeP2POffer      MsgType = 0x0C
	TypeP2PAnswer     MsgType = 0x0D
	TypeP2PClose      MsgType = 0x10

	// Mesh network message types (Phase 2 - P2-T06)
	TypeMeshJoin     MsgType = 0x11
	TypeMeshPeerList MsgType = 0x12
	TypeMeshLeave    MsgType = 0x13
	TypeMeshPing     MsgType = 0x14
	TypeMeshPong     MsgType = 0x15

	// Phase 3 message types (Intelligent Scheduling)
	TypeQUICOffer       MsgType = 0x16
	TypeQUICAnswer      MsgType = 0x17
	TypeSchedulerSwitch MsgType = 0x18
	TypeSchedulerLock   MsgType = 0x19
	TypeProbeRequest    MsgType = 0x1A
	TypeProbeResponse   MsgType = 0x1B
	TypeRelayQUICOffer  MsgType = 0x1C
)

// ProtocolVersion is the current protocol version.
const ProtocolVersion uint8 = 1

// Message represents a raw protocol frame with type and payload.
type Message struct {
	Type    MsgType
	Payload []byte
}

// --- Typed payload structs ---

// AuthMessage is the payload for TypeAuth.
type AuthMessage struct {
	Version  uint8  `json:"version"`
	ClientID string `json:"client_id"`
	// AuthToken 用于 Relay 控制通道的共享密钥认证，避免仅凭 client_id 接入。
	AuthToken string `json:"auth_token,omitempty"`
}

// AuthRespMessage is the payload for TypeAuthResp.
type AuthRespMessage struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// NewProxyMessage is the payload for TypeNewProxy.
type NewProxyMessage struct {
	ProxyName      string `json:"proxy_name"`
	ProxyType      string `json:"proxy_type"`
	LocalAddr      string `json:"local_addr"`
	RemotePort     uint16 `json:"remote_port"`
	Domain         string `json:"domain,omitempty"`
	HostHeader     string `json:"host_header,omitempty"`
	UseHTTPS       bool   `json:"use_https,omitempty"`
	PublicURL      string `json:"public_url,omitempty"`
	AccessPolicyID string `json:"access_policy_id,omitempty"`
	InspectEnabled bool   `json:"inspect_enabled,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

// NewProxyRespMessage is the payload for TypeNewProxyResp.
type NewProxyRespMessage struct {
	ProxyName  string `json:"proxy_name"`
	Success    bool   `json:"success"`
	RemotePort uint16 `json:"remote_port"`
	PublicURL  string `json:"public_url,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CloseProxyMessage is the payload for TypeCloseProxy.
type CloseProxyMessage struct {
	ProxyName string `json:"proxy_name"`
}

// StartWorkConnMessage is the payload for TypeStartWorkConn.
type StartWorkConnMessage struct {
	ProxyName string `json:"proxy_name"`
	SessionID string `json:"session_id"`
}

// WorkConnMessage is the payload for TypeWorkConn.
type WorkConnMessage struct {
	ProxyName string `json:"proxy_name"`
	SessionID string `json:"session_id"`
	// AuthToken 让工作连接与控制连接使用同一认证凭据，避免未授权连接抢占会话。
	AuthToken string `json:"auth_token,omitempty"`
}

// --- P2P signaling payload structs (Phase 2) ---

// ICECandidateJSON represents a single ICE candidate for P2P negotiation.
type ICECandidateJSON struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // "host", "srflx", "relay"
	Addr       string `json:"addr"` // ip:port
	Priority   uint32 `json:"priority"`
	Foundation string `json:"foundation"`
}

// NATDetectReqMessage is the payload for TypeNATDetectReq.
type NATDetectReqMessage struct {
	ClientID string `json:"client_id"`
}

// NATDetectRespMessage is the payload for TypeNATDetectResp.
type NATDetectRespMessage struct {
	NATType    string `json:"nat_type"`
	PublicAddr string `json:"public_addr"`
	MappedPort uint16 `json:"mapped_port"`
}

// P2POfferMessage is the payload for TypeP2POffer.
type P2POfferMessage struct {
	SessionID    string             `json:"session_id"`
	FromClientID string             `json:"from_client_id"`
	ToClientID   string             `json:"to_client_id"`
	Candidates   []ICECandidateJSON `json:"candidates"`
	NATType      string             `json:"nat_type"`
	WGPublicKey  string             `json:"wg_public_key"`
}

// P2PAnswerMessage is the payload for TypeP2PAnswer.
type P2PAnswerMessage struct {
	SessionID    string             `json:"session_id"`
	FromClientID string             `json:"from_client_id"`
	ToClientID   string             `json:"to_client_id"`
	Candidates   []ICECandidateJSON `json:"candidates"`
	NATType      string             `json:"nat_type"`
	WGPublicKey  string             `json:"wg_public_key"`
	Accept       bool               `json:"accept"`
	Reason       string             `json:"reason,omitempty"`
}

// P2PCloseMessage is the payload for TypeP2PClose.
type P2PCloseMessage struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

// --- Mesh network payload structs (Phase 2 - P2-T06) ---

// MeshPeerJSON describes a peer in the mesh network.
type MeshPeerJSON struct {
	ClientID string `json:"client_id"`
	NATType  string `json:"nat_type"`
	WGPubKey string `json:"wg_pub_key"`
	Subnet   string `json:"subnet,omitempty"`
}

// MeshJoinMessage is the payload for TypeMeshJoin.
type MeshJoinMessage struct {
	ClientID string `json:"client_id"`
	WGPubKey string `json:"wg_pub_key"`
	NATType  string `json:"nat_type"`
	Subnet   string `json:"subnet,omitempty"`
}

// MeshPeerListMessage is the payload for TypeMeshPeerList.
type MeshPeerListMessage struct {
	Peers []MeshPeerJSON `json:"peers"`
}

// MeshLeaveMessage is the payload for TypeMeshLeave.
type MeshLeaveMessage struct {
	ClientID string `json:"client_id"`
}

// MeshPingMessage is the payload for TypeMeshPing.
type MeshPingMessage struct {
	FromClientID string `json:"from_client_id"`
	ToClientID   string `json:"to_client_id"`
}

// MeshPongMessage is the payload for TypeMeshPong.
type MeshPongMessage struct {
	FromClientID string `json:"from_client_id"`
	ToClientID   string `json:"to_client_id"`
}

// --- Phase 3 payload structs (Intelligent Scheduling) ---

// QUICOfferMessage is the payload for TypeQUICOffer.
type QUICOfferMessage struct {
	SessionID    string `json:"session_id"`
	FromClientID string `json:"from_client_id"`
	ToClientID   string `json:"to_client_id"`
	ListenAddr   string `json:"listen_addr"`
	ALPN         string `json:"alpn"`
}

// QUICAnswerMessage is the payload for TypeQUICAnswer.
type QUICAnswerMessage struct {
	SessionID    string `json:"session_id"`
	FromClientID string `json:"from_client_id"`
	ToClientID   string `json:"to_client_id"`
	ListenAddr   string `json:"listen_addr"`
	Accept       bool   `json:"accept"`
	Reason       string `json:"reason,omitempty"`
}

// SchedulerSwitchMessage notifies a peer of a path switch.
type SchedulerSwitchMessage struct {
	SessionID   string `json:"session_id"`
	OldPathType string `json:"old_path_type"`
	NewPathType string `json:"new_path_type"`
}

// SchedulerLockMessage notifies a peer of a path lock/unlock.
type SchedulerLockMessage struct {
	SessionID string `json:"session_id"`
	PathType  string `json:"path_type"`
	Locked    bool   `json:"locked"`
}

// ProbeRequestMessage is the payload for TypeProbeRequest.
type ProbeRequestMessage struct {
	FromClientID string `json:"from_client_id"`
	ToClientID   string `json:"to_client_id"`
	PathType     string `json:"path_type"`
	SeqNum       uint16 `json:"seq_num"`
	TimestampNs  int64  `json:"timestamp_ns"`
}

// ProbeResponseMessage is the payload for TypeProbeResponse.
type ProbeResponseMessage struct {
	FromClientID string `json:"from_client_id"`
	ToClientID   string `json:"to_client_id"`
	PathType     string `json:"path_type"`
	SeqNum       uint16 `json:"seq_num"`
	TimestampNs  int64  `json:"timestamp_ns"`
}

// RelayQUICOfferMessage is the payload for TypeRelayQUICOffer.
type RelayQUICOfferMessage struct {
	SessionID string `json:"session_id"`
	RelayAddr string `json:"relay_addr"`
	ProxyName string `json:"proxy_name"`
	ClientID  string `json:"client_id"`
}

// --- Factory constructors ---

// NewAuthMessage creates a TypeAuth message.
func NewAuthMessage(clientID string) (*Message, error) {
	return NewAuthMessageWithToken(clientID, "")
}

// NewAuthMessageWithToken creates a TypeAuth message with an optional auth token.
func NewAuthMessageWithToken(clientID, authToken string) (*Message, error) {
	return marshalMessage(TypeAuth, &AuthMessage{Version: ProtocolVersion, ClientID: clientID, AuthToken: authToken})
}

// NewAuthRespMessage creates a TypeAuthResp message.
func NewAuthRespMessage(success bool, errMsg string) (*Message, error) {
	return marshalMessage(TypeAuthResp, &AuthRespMessage{Success: success, Error: errMsg})
}

// NewNewProxyMessage creates a TypeNewProxy message.
func NewNewProxyMessage(name, proxyType, localAddr string, remotePort uint16) (*Message, error) {
	return marshalMessage(TypeNewProxy, &NewProxyMessage{ProxyName: name, ProxyType: proxyType, LocalAddr: localAddr, RemotePort: remotePort})
}

// NewHTTPProxyMessage creates a TypeNewProxy message with HTTP-specific fields.
func NewHTTPProxyMessage(name, localAddr string, remotePort uint16, domain, hostHeader string, useHTTPS bool) (*Message, error) {
	return marshalMessage(TypeNewProxy, &NewProxyMessage{ProxyName: name, ProxyType: "http", LocalAddr: localAddr, RemotePort: remotePort, Domain: domain, HostHeader: hostHeader, UseHTTPS: useHTTPS})
}

// NewHTTPProxyMessageWithEndpoint creates a TypeNewProxy message with public endpoint metadata.
func NewHTTPProxyMessageWithEndpoint(name, localAddr string, remotePort uint16, domain, hostHeader string, useHTTPS bool, publicURL, accessPolicyID string, inspectEnabled bool, expiresAt string) (*Message, error) {
	return marshalMessage(TypeNewProxy, &NewProxyMessage{
		ProxyName:      name,
		ProxyType:      "http",
		LocalAddr:      localAddr,
		RemotePort:     remotePort,
		Domain:         domain,
		HostHeader:     hostHeader,
		UseHTTPS:       useHTTPS,
		PublicURL:      publicURL,
		AccessPolicyID: accessPolicyID,
		InspectEnabled: inspectEnabled,
		ExpiresAt:      expiresAt,
	})
}

// NewNewProxyRespMessage creates a TypeNewProxyResp message.
func NewNewProxyRespMessage(name string, success bool, remotePort uint16, errMsg string) (*Message, error) {
	return marshalMessage(TypeNewProxyResp, &NewProxyRespMessage{ProxyName: name, Success: success, RemotePort: remotePort, Error: errMsg})
}

// NewNewProxyRespMessageWithURL creates a TypeNewProxyResp message including the public URL.
func NewNewProxyRespMessageWithURL(name string, success bool, remotePort uint16, publicURL, errMsg string) (*Message, error) {
	return marshalMessage(TypeNewProxyResp, &NewProxyRespMessage{ProxyName: name, Success: success, RemotePort: remotePort, PublicURL: publicURL, Error: errMsg})
}

// NewCloseProxyMessage creates a TypeCloseProxy message.
func NewCloseProxyMessage(name string) (*Message, error) {
	return marshalMessage(TypeCloseProxy, &CloseProxyMessage{ProxyName: name})
}

// NewStartWorkConnMessage creates a TypeStartWorkConn message.
func NewStartWorkConnMessage(proxyName, sessionID string) (*Message, error) {
	return marshalMessage(TypeStartWorkConn, &StartWorkConnMessage{ProxyName: proxyName, SessionID: sessionID})
}

// NewWorkConnMessage creates a TypeWorkConn message.
func NewWorkConnMessage(proxyName, sessionID string) (*Message, error) {
	return NewWorkConnMessageWithToken(proxyName, sessionID, "")
}

// NewWorkConnMessageWithToken creates a TypeWorkConn message with an optional auth token.
func NewWorkConnMessageWithToken(proxyName, sessionID, authToken string) (*Message, error) {
	return marshalMessage(TypeWorkConn, &WorkConnMessage{ProxyName: proxyName, SessionID: sessionID, AuthToken: authToken})
}

// NewHeartbeat creates a TypeHeartbeat message with empty payload.
func NewHeartbeat() *Message { return &Message{Type: TypeHeartbeat} }

// NewHeartbeatResp creates a TypeHeartbeatResp message with empty payload.
func NewHeartbeatResp() *Message { return &Message{Type: TypeHeartbeatResp} }

// --- P2P factory constructors (Phase 2) ---

func NewNATDetectReqMessage(clientID string) (*Message, error) {
	return marshalMessage(TypeNATDetectReq, &NATDetectReqMessage{ClientID: clientID})
}

func NewNATDetectRespMessage(natType, publicAddr string, mappedPort uint16) (*Message, error) {
	return marshalMessage(TypeNATDetectResp, &NATDetectRespMessage{NATType: natType, PublicAddr: publicAddr, MappedPort: mappedPort})
}

func NewP2POfferMessage(sessionID, fromClientID, toClientID, natType, wgPubKey string, candidates []ICECandidateJSON) (*Message, error) {
	return marshalMessage(TypeP2POffer, &P2POfferMessage{SessionID: sessionID, FromClientID: fromClientID, ToClientID: toClientID, Candidates: candidates, NATType: natType, WGPublicKey: wgPubKey})
}

func NewP2PAnswerMessage(sessionID, fromClientID, toClientID, natType, wgPubKey string, candidates []ICECandidateJSON, accept bool, reason string) (*Message, error) {
	return marshalMessage(TypeP2PAnswer, &P2PAnswerMessage{SessionID: sessionID, FromClientID: fromClientID, ToClientID: toClientID, Candidates: candidates, NATType: natType, WGPublicKey: wgPubKey, Accept: accept, Reason: reason})
}

func NewP2PCloseMessage(sessionID, reason string) (*Message, error) {
	return marshalMessage(TypeP2PClose, &P2PCloseMessage{SessionID: sessionID, Reason: reason})
}

// --- Mesh factory constructors (Phase 2) ---

func NewMeshJoinMessage(clientID, wgPubKey, natType, subnet string) (*Message, error) {
	return marshalMessage(TypeMeshJoin, &MeshJoinMessage{ClientID: clientID, WGPubKey: wgPubKey, NATType: natType, Subnet: subnet})
}

func NewMeshPeerListMessage(peers []MeshPeerJSON) (*Message, error) {
	return marshalMessage(TypeMeshPeerList, &MeshPeerListMessage{Peers: peers})
}

func NewMeshLeaveMessage(clientID string) (*Message, error) {
	return marshalMessage(TypeMeshLeave, &MeshLeaveMessage{ClientID: clientID})
}

func NewMeshPingMessage(fromClientID, toClientID string) (*Message, error) {
	return marshalMessage(TypeMeshPing, &MeshPingMessage{FromClientID: fromClientID, ToClientID: toClientID})
}

func NewMeshPongMessage(fromClientID, toClientID string) (*Message, error) {
	return marshalMessage(TypeMeshPong, &MeshPongMessage{FromClientID: fromClientID, ToClientID: toClientID})
}

// --- Phase 3 factory constructors ---

func NewQUICOfferMessage(sessionID, fromClientID, toClientID, listenAddr, alpn string) (*Message, error) {
	return marshalMessage(TypeQUICOffer, &QUICOfferMessage{SessionID: sessionID, FromClientID: fromClientID, ToClientID: toClientID, ListenAddr: listenAddr, ALPN: alpn})
}

func NewQUICAnswerMessage(sessionID, fromClientID, toClientID, listenAddr string, accept bool, reason string) (*Message, error) {
	return marshalMessage(TypeQUICAnswer, &QUICAnswerMessage{SessionID: sessionID, FromClientID: fromClientID, ToClientID: toClientID, ListenAddr: listenAddr, Accept: accept, Reason: reason})
}

func NewSchedulerSwitchMessage(sessionID, oldPath, newPath string) (*Message, error) {
	return marshalMessage(TypeSchedulerSwitch, &SchedulerSwitchMessage{SessionID: sessionID, OldPathType: oldPath, NewPathType: newPath})
}

func NewSchedulerLockMessage(sessionID, pathType string, locked bool) (*Message, error) {
	return marshalMessage(TypeSchedulerLock, &SchedulerLockMessage{SessionID: sessionID, PathType: pathType, Locked: locked})
}

func NewProbeRequestMessage(fromClientID, toClientID, pathType string, seqNum uint16, timestampNs int64) (*Message, error) {
	return marshalMessage(TypeProbeRequest, &ProbeRequestMessage{FromClientID: fromClientID, ToClientID: toClientID, PathType: pathType, SeqNum: seqNum, TimestampNs: timestampNs})
}

func NewProbeResponseMessage(fromClientID, toClientID, pathType string, seqNum uint16, timestampNs int64) (*Message, error) {
	return marshalMessage(TypeProbeResponse, &ProbeResponseMessage{FromClientID: fromClientID, ToClientID: toClientID, PathType: pathType, SeqNum: seqNum, TimestampNs: timestampNs})
}

func NewRelayQUICOfferMessage(sessionID, relayAddr, proxyName, clientID string) (*Message, error) {
	return marshalMessage(TypeRelayQUICOffer, &RelayQUICOfferMessage{SessionID: sessionID, RelayAddr: relayAddr, ProxyName: proxyName, ClientID: clientID})
}

// DecodePayload decodes the message payload into the appropriate typed struct.
func (m *Message) DecodePayload() (any, error) {
	switch m.Type {
	case TypeAuth:
		var v AuthMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeAuthResp:
		var v AuthRespMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeNewProxy:
		var v NewProxyMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeNewProxyResp:
		var v NewProxyRespMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeCloseProxy:
		var v CloseProxyMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeStartWorkConn:
		var v StartWorkConnMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeWorkConn:
		var v WorkConnMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeHeartbeat, TypeHeartbeatResp:
		return nil, nil
	case TypeNATDetectReq:
		var v NATDetectReqMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeNATDetectResp:
		var v NATDetectRespMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeP2POffer:
		var v P2POfferMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeP2PAnswer:
		var v P2PAnswerMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeP2PClose:
		var v P2PCloseMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeMeshJoin:
		var v MeshJoinMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeMeshPeerList:
		var v MeshPeerListMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeMeshLeave:
		var v MeshLeaveMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeMeshPing:
		var v MeshPingMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeMeshPong:
		var v MeshPongMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeQUICOffer:
		var v QUICOfferMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeQUICAnswer:
		var v QUICAnswerMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeSchedulerSwitch:
		var v SchedulerSwitchMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeSchedulerLock:
		var v SchedulerLockMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeProbeRequest:
		var v ProbeRequestMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeProbeResponse:
		var v ProbeResponseMessage
		return &v, json.Unmarshal(m.Payload, &v)
	case TypeRelayQUICOffer:
		var v RelayQUICOfferMessage
		return &v, json.Unmarshal(m.Payload, &v)
	default:
		return nil, ErrUnknownMsgType
	}
}

func marshalMessage(t MsgType, v any) (*Message, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &Message{Type: t, Payload: data}, nil
}
