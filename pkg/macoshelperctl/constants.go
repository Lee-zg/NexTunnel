package macoshelperctl

const (
	HelperLabel           = "com.nextunnel.helper"
	HelperExecutableName  = "nextunnel-helper"
	HelperSocketDirectory = "/var/run/nextunnel"
	DefaultSocketPath     = HelperSocketDirectory + "/helper.sock"
	DefaultHelperPath     = "/Library/PrivilegedHelperTools/" + HelperExecutableName
	LaunchDaemonPath      = "/Library/LaunchDaemons/" + HelperLabel + ".plist"
	SystemLaunchLabel     = "system/" + HelperLabel

	ResourceDirectoryName = "macos-helper"
	ResourcePlistName     = HelperLabel + ".plist"
	InstallScriptName     = "install-helper.sh"

	DefaultAppBundlePath      = "/Applications/NexTunnel.app"
	HomebrewIntelResourceDir  = "/usr/local/share/nextunnel/" + ResourceDirectoryName
	HomebrewAppleResourceDir  = "/opt/homebrew/share/nextunnel/" + ResourceDirectoryName
	EnvResourceDirectory      = "NEXTUNNEL_MACOS_HELPER_DIR"
	AdminGroupFallbackGID     = 80
	rootUserID                = 0
	wheelGroupID              = 0
	helperExecutableMode      = 0755
	launchDaemonMode          = 0644
	helperSocketDirectoryMode = 0770
)

const (
	ActionInstall   = "install"
	ActionStatus    = "status"
	ActionRestart   = "restart"
	ActionUninstall = "uninstall"
)

// ValidateAction 限制提权入口只能执行固定 helper 管理动作，避免外部注入任意命令。
func ValidateAction(action string) bool {
	switch action {
	case ActionInstall, ActionStatus, ActionRestart, ActionUninstall:
		return true
	default:
		return false
	}
}
