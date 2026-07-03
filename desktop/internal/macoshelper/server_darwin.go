//go:build darwin

package macoshelper

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/nextunnel/desktop/internal/p2p"
	"github.com/nextunnel/desktop/internal/virtualnet"
	"golang.org/x/sys/unix"
)

type ServerOptions struct {
	SocketPath string
	Version    string
	Signed     bool
}

func RunServer(ctx context.Context, opts ServerOptions) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("nextunnel-helper must run as root")
	}
	socketPath := opts.SocketPath
	if strings.TrimSpace(socketPath) == "" {
		socketPath = DefaultSocketPath
	}
	adminGID := lookupAdminGID()
	if err := prepareSocketPath(socketPath, adminGID); err != nil {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socketPath)
	_ = os.Chmod(socketPath, 0660)
	_ = os.Chown(socketPath, 0, int(adminGID))

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go handleConnection(conn, opts, adminGID)
	}
}

func prepareSocketPath(socketPath string, adminGID uint32) error {
	if err := os.MkdirAll(HelperSocketDirectory, 0770); err != nil {
		return err
	}
	_ = os.Chown(HelperSocketDirectory, 0, int(adminGID))
	_ = os.Chmod(HelperSocketDirectory, 0770)
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func lookupAdminGID() uint32 {
	group, err := user.LookupGroup("admin")
	if err != nil {
		return 80
	}
	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil {
		return 80
	}
	return uint32(gid)
}

func handleConnection(conn *net.UnixConn, opts ServerOptions, adminGID uint32) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	cred, err := peerCredential(conn)
	if err != nil {
		writeJSON(conn, errorResponse("", err))
		return
	}
	if !authorizedPeer(cred, adminGID) {
		writeJSON(conn, errorResponse("", fmt.Errorf("peer uid=%d is not authorized", cred.Uid)))
		return
	}

	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		writeJSON(conn, errorResponse("", err))
		return
	}
	if req.ProtocolVersion != ProtocolVersion {
		writeJSON(conn, errorResponse(req.Action, fmt.Errorf("protocol mismatch: got %s want %s", req.ProtocolVersion, ProtocolVersion)))
		return
	}
	switch req.Action {
	case actionStatus:
		writeJSON(conn, response{
			OK:              true,
			Action:          req.Action,
			ProtocolVersion: ProtocolVersion,
			Version:         opts.Version,
			Signed:          opts.Signed,
			Message:         "nextunnel macOS helper is running",
		})
	case actionCreateTUN:
		handleCreateTUN(conn, req)
	case actionApplyRoute:
		handleApplyRoutes(conn, req)
	case actionResetRoute:
		handleResetRoutes(conn, req)
	default:
		writeJSON(conn, errorResponse(req.Action, fmt.Errorf("unsupported action: %s", req.Action)))
	}
}

func peerCredential(conn *net.UnixConn) (*unix.Xucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Xucred
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		cred, controlErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return nil, err
	}
	if controlErr != nil {
		return nil, controlErr
	}
	return cred, nil
}

func authorizedPeer(cred *unix.Xucred, adminGID uint32) bool {
	if cred == nil {
		return false
	}
	if cred.Uid == 0 {
		return true
	}
	for index := int16(0); index < cred.Ngroups && index < int16(len(cred.Groups)); index++ {
		if cred.Groups[index] == adminGID {
			return true
		}
	}
	return false
}

func handleCreateTUN(conn *net.UnixConn, req request) {
	if req.CreateTUN == nil {
		writeJSON(conn, errorResponse(req.Action, fmt.Errorf("create_tun payload is required")))
		return
	}
	if err := ValidateCreateTUNRequest(*req.CreateTUN); err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
		return
	}
	cfg := p2p.TUNConfig{
		Name:    req.CreateTUN.Name,
		MTU:     req.CreateTUN.MTU,
		LocalIP: net.ParseIP(req.CreateTUN.LocalIP),
		PeerIP:  net.ParseIP(req.CreateTUN.PeerIP),
	}
	_, subnet, err := net.ParseCIDR(req.CreateTUN.Subnet)
	if err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
		return
	}
	cfg.Subnet = subnet
	file, name, mtu, err := p2p.CreateDarwinKernelTUNFile(cfg)
	if err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
		return
	}
	defer file.Close()
	resp := response{
		OK:              true,
		Action:          req.Action,
		ProtocolVersion: ProtocolVersion,
		CreateTUN:       &CreateTUNResult{Interface: name, MTU: mtu},
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
		return
	}
	rights := unix.UnixRights(int(file.Fd()))
	if _, _, err := conn.WriteMsgUnix(payload, rights, nil); err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
	}
}

func handleApplyRoutes(conn *net.UnixConn, req request) {
	if req.VirtualNetwork == nil {
		writeJSON(conn, errorResponse(req.Action, fmt.Errorf("virtual_network payload is required")))
		return
	}
	cfg := *req.VirtualNetwork
	if err := ValidateVirtualNetworkConfig(cfg); err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
		return
	}
	commands := buildDarwinApplyCommands(cfg)
	executed, err := runCommands(commands)
	if err != nil {
		state := stateFromConfig(cfg, false, executed)
		state.LastError = err.Error()
		writeJSON(conn, response{OK: false, Action: req.Action, ProtocolVersion: ProtocolVersion, Error: err.Error(), State: &state})
		return
	}
	state := stateFromConfig(cfg, true, executed)
	writeJSON(conn, response{OK: true, Action: req.Action, ProtocolVersion: ProtocolVersion, State: &state})
}

func handleResetRoutes(conn *net.UnixConn, req request) {
	if req.State == nil {
		writeJSON(conn, errorResponse(req.Action, fmt.Errorf("state payload is required")))
		return
	}
	state := *req.State
	if err := validateResetState(state); err != nil {
		writeJSON(conn, errorResponse(req.Action, err))
		return
	}
	commands := buildDarwinResetCommands(state)
	executed, err := runCommands(commands)
	state.LastCommands = executed
	if err != nil {
		state.LastError = err.Error()
		writeJSON(conn, response{OK: false, Action: req.Action, ProtocolVersion: ProtocolVersion, Error: err.Error(), State: &state})
		return
	}
	state.Applied = false
	state.LastError = ""
	writeJSON(conn, response{OK: true, Action: req.Action, ProtocolVersion: ProtocolVersion, State: &state})
}

func writeJSON(conn *net.UnixConn, resp response) {
	_ = json.NewEncoder(conn).Encode(resp)
}

func errorResponse(action string, err error) response {
	return response{OK: false, Action: action, ProtocolVersion: ProtocolVersion, Error: err.Error()}
}

type command struct {
	name string
	args []string
}

func (c command) String() string {
	if len(c.args) == 0 {
		return c.name
	}
	return c.name + " " + strings.Join(c.args, " ")
}

func buildDarwinApplyCommands(cfg virtualnet.Config) []command {
	commands := []command{
		{name: "ifconfig", args: []string{cfg.Interface, "mtu", fmt.Sprintf("%d", cfg.MTU)}},
		{name: "ifconfig", args: []string{cfg.Interface, "inet", cfg.VirtualIP, cfg.Gateway, "netmask", maskFromCIDR(cfg.Subnet), "up"}},
	}
	for _, route := range cfg.Routes {
		commands = append(commands, command{name: "route", args: []string{"add", "-net", route.Destination, route.Gateway}})
	}
	return commands
}

func buildDarwinResetCommands(state virtualnet.State) []command {
	commands := make([]command, 0, len(state.Routes))
	for _, route := range state.Routes {
		if strings.TrimSpace(route.Destination) == "" {
			continue
		}
		commands = append(commands, command{name: "route", args: []string{"delete", "-net", route.Destination}})
	}
	return commands
}

func runCommands(commands []command) ([]string, error) {
	executed := make([]string, 0, len(commands))
	for _, command := range commands {
		output, err := exec.Command(command.name, command.args...).CombinedOutput()
		executed = append(executed, command.String())
		if err != nil {
			return executed, fmt.Errorf("%s failed: %w: %s", command.String(), err, strings.TrimSpace(string(output)))
		}
	}
	return executed, nil
}

func maskFromCIDR(cidr string) string {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil || !prefix.Addr().Is4() {
		return "255.255.255.0"
	}
	bits := prefix.Bits()
	mask := uint32(0xffffffff) << uint32(32-bits)
	return fmt.Sprintf("%d.%d.%d.%d", byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask))
}
