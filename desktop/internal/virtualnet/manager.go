package virtualnet

import (
	"fmt"
	"log/slog"
	"net/netip"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	windowsInterfaceCheckAttempts = 6
	windowsInterfaceCheckDelay    = 250 * time.Millisecond
)

// Manager 负责将控制面虚拟网络配置应用到本机，并维护可展示状态。
type Manager struct {
	runner           CommandRunner
	interfaceChecker NetworkInterfaceChecker
	privileged       PrivilegedApplier
	logger           *slog.Logger
	mu               sync.Mutex
	state            State
}

// NewManager 创建虚拟网络管理器，runner 为空时使用真实系统命令执行器。
func NewManager(runner CommandRunner, logger *slog.Logger) *Manager {
	return NewManagerWithPrivilegedApplier(runner, logger, nil)
}

// NewManagerWithPrivilegedApplier creates a manager that delegates apply/reset
// to a constrained privileged helper when one is configured.
func NewManagerWithPrivilegedApplier(runner CommandRunner, logger *slog.Logger, privileged PrivilegedApplier) *Manager {
	var interfaceChecker NetworkInterfaceChecker
	if runner == nil {
		execRunner := ExecRunner{}
		runner = execRunner
		interfaceChecker = execRunner
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{runner: runner, interfaceChecker: interfaceChecker, privileged: privileged, logger: logger}
}

// State 返回当前虚拟网络状态副本。
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Apply 按当前平台应用虚拟网络配置。
func (m *Manager) Apply(cfg Config) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateConfig(cfg); err != nil {
		m.state.LastError = err.Error()
		return m.state, err
	}

	if m.privileged != nil {
		state, err := m.privileged.ApplyVirtualNetwork(cfg)
		if err != nil {
			m.state = stateFromConfig(cfg)
			m.state.Applied = false
			m.state.LastError = err.Error()
			m.logger.Error("privileged virtual network apply failed", "interface", cfg.Interface, "error", err)
			return m.state, err
		}
		m.state = state
		m.state.LastError = ""
		m.logger.Info("virtual network applied by privileged helper", "interface", state.Interface, "ip", state.VirtualIP)
		return m.state, nil
	}

	if err := m.ensureApplyPreconditions(runtime.GOOS, cfg); err != nil {
		m.state = stateFromConfig(cfg)
		m.state.Applied = false
		m.state.LastError = err.Error()
		m.state.LastCommands = nil
		m.logger.Error("virtual network preflight failed", "interface", cfg.Interface, "error", err)
		return m.state, err
	}

	commands, err := buildApplyCommands(runtime.GOOS, cfg)
	if err != nil {
		m.state.LastError = err.Error()
		return m.state, err
	}

	executed := make([]string, 0, len(commands))
	for _, command := range commands {
		if err := m.runner.Run(command.name, command.args...); err != nil {
			m.state = stateFromConfig(cfg)
			m.state.Applied = false
			m.state.LastError = err.Error()
			m.state.LastCommands = append(executed, command.String())
			m.logger.Error("apply virtual network command failed", "command", command.String(), "error", err)
			return m.state, err
		}
		executed = append(executed, command.String())
	}

	m.state = stateFromConfig(cfg)
	m.state.Applied = true
	m.state.LastCommands = executed
	m.state.LastError = ""
	m.logger.Info("virtual network applied", "interface", cfg.Interface, "ip", cfg.VirtualIP)
	return m.state, nil
}

// Reset 回滚当前虚拟网络路由配置；没有应用状态时直接返回当前状态。
func (m *Manager) Reset() (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.state.Applied {
		return m.state, nil
	}

	if m.privileged != nil {
		state, err := m.privileged.ResetVirtualNetwork(m.state)
		if err != nil {
			m.state.LastError = err.Error()
			m.logger.Error("privileged virtual network reset failed", "interface", m.state.Interface, "error", err)
			return m.state, err
		}
		m.state = state
		m.state.Applied = false
		m.state.LastError = ""
		m.logger.Info("virtual network reset by privileged helper", "interface", m.state.Interface)
		return m.state, nil
	}

	commands, err := buildResetCommands(runtime.GOOS, m.state)
	if err != nil {
		m.state.LastError = err.Error()
		return m.state, err
	}

	executed := make([]string, 0, len(commands))
	for _, command := range commands {
		if err := m.runner.Run(command.name, command.args...); err != nil {
			m.state.LastError = err.Error()
			m.state.LastCommands = append(executed, command.String())
			m.logger.Error("reset virtual network command failed", "command", command.String(), "error", err)
			return m.state, err
		}
		executed = append(executed, command.String())
	}

	m.state.Applied = false
	m.state.LastError = ""
	m.state.LastCommands = executed
	m.logger.Info("virtual network reset", "interface", m.state.Interface)
	return m.state, nil
}

func (m *Manager) ensureApplyPreconditions(goos string, cfg Config) error {
	if goos != "windows" || m.interfaceChecker == nil {
		return nil
	}
	return ensureWindowsInterfaceAvailable(m.interfaceChecker, cfg.Interface, windowsInterfaceCheckAttempts, windowsInterfaceCheckDelay)
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.NodeID) == "" {
		return fmt.Errorf("node_id is required")
	}
	if strings.TrimSpace(cfg.VirtualIP) == "" {
		return fmt.Errorf("virtual_ip is required")
	}
	if _, err := netip.ParseAddr(cfg.VirtualIP); err != nil {
		return fmt.Errorf("virtual_ip is invalid: %w", err)
	}
	if strings.TrimSpace(cfg.Subnet) == "" {
		return fmt.Errorf("subnet is required")
	}
	if _, err := netip.ParsePrefix(cfg.Subnet); err != nil {
		return fmt.Errorf("subnet is invalid: %w", err)
	}
	if strings.TrimSpace(cfg.Gateway) != "" {
		if _, err := netip.ParseAddr(cfg.Gateway); err != nil {
			return fmt.Errorf("gateway is invalid: %w", err)
		}
	}
	if strings.TrimSpace(cfg.Interface) == "" {
		return fmt.Errorf("interface is required")
	}
	if cfg.MTU < 576 || cfg.MTU > 9000 {
		return fmt.Errorf("mtu must be between 576 and 9000")
	}
	for index, route := range cfg.Routes {
		if strings.TrimSpace(route.Destination) == "" {
			return fmt.Errorf("route[%d].destination is required", index)
		}
		if _, err := netip.ParsePrefix(route.Destination); err != nil {
			return fmt.Errorf("route[%d].destination is invalid: %w", index, err)
		}
		if strings.TrimSpace(route.Gateway) == "" {
			return fmt.Errorf("route[%d].gateway is required", index)
		}
		if _, err := netip.ParseAddr(route.Gateway); err != nil {
			return fmt.Errorf("route[%d].gateway is invalid: %w", index, err)
		}
		if route.Metric < 0 {
			return fmt.Errorf("route[%d].metric must not be negative", index)
		}
	}
	return nil
}

func ensureWindowsInterfaceAvailable(checker NetworkInterfaceChecker, interfaceName string, attempts int, delay time.Duration) error {
	if attempts <= 0 {
		attempts = 1
	}
	normalizedInterfaceName := strings.TrimSpace(interfaceName)
	for attempt := 1; attempt <= attempts; attempt++ {
		exists, err := checker.InterfaceExists(normalizedInterfaceName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		if attempt < attempts && delay > 0 {
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("未找到 Windows 网络接口 %q。请先确认 wintun.dll 已就绪，并以管理员身份启动 NexTunnel 创建 Wintun 适配器；如果适配器名称不同，请同步 Control Plane 下发的 virtual_interface 后再应用路由", normalizedInterfaceName)
}

func stateFromConfig(cfg Config) State {
	return State{
		Interface: cfg.Interface,
		VirtualIP: cfg.VirtualIP,
		Subnet:    cfg.Subnet,
		Gateway:   cfg.Gateway,
		MTU:       cfg.MTU,
		Routes:    append([]Route(nil), cfg.Routes...),
	}
}
