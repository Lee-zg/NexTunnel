package virtualnet

// Route 是控制面下发给客户端的虚拟网络路由条目。
type Route struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Interface   string `json:"interface"`
	Metric      int    `json:"metric"`
}

// Config 是节点接入虚拟网络所需的最小配置。
type Config struct {
	NodeID    string  `json:"node_id"`
	VirtualIP string  `json:"virtual_ip"`
	Subnet    string  `json:"subnet"`
	Gateway   string  `json:"gateway"`
	Interface string  `json:"interface"`
	MTU       int     `json:"mtu"`
	Routes    []Route `json:"routes"`
}

// State 记录当前本机虚拟网络应用状态，供桌面端 UI 展示和回滚。
type State struct {
	Applied      bool     `json:"applied"`
	Interface    string   `json:"interface"`
	VirtualIP    string   `json:"virtual_ip"`
	Subnet       string   `json:"subnet"`
	Gateway      string   `json:"gateway"`
	MTU          int      `json:"mtu"`
	Routes       []Route  `json:"routes"`
	LastError    string   `json:"last_error"`
	LastCommands []string `json:"last_commands"`
}

// CommandRunner 抽象系统命令执行，便于单元测试验证命令构建而不修改真实路由表。
type CommandRunner interface {
	Run(name string, args ...string) error
}

// NetworkInterfaceChecker 在执行系统路由命令前确认目标网卡已经被操作系统识别。
type NetworkInterfaceChecker interface {
	InterfaceExists(name string) (bool, error)
}

// PrivilegedApplier lets a platform-specific helper apply network state without
// exposing arbitrary system command execution to the desktop process.
type PrivilegedApplier interface {
	ApplyVirtualNetwork(cfg Config) (State, error)
	ResetVirtualNetwork(state State) (State, error)
}
