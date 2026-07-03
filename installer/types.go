package main

const (
	appName                  = "NexTunnel"
	appExecutableName        = "NexTunnel.exe"
	installerExecutableName  = "NexTunnelInstaller.exe"
	defaultRequiredSpaceMB   = 512
	progressEventName        = "installer:progress"
	installPhasePreparing    = "preparing"
	installPhaseValidating   = "validating"
	installPhaseExtracting   = "extracting"
	installPhaseReplacing    = "replacing"
	installPhaseIntegrating  = "integrating"
	installPhaseComplete     = "complete"
	installPhaseRollback     = "rollback"
	installPhaseUninstalling = "uninstalling"
)

// AppVersion 由发布脚本通过 -ldflags 注入；默认值用于本地开发和测试。
var AppVersion = "0.6.4-alpha"

type InstallOptions struct {
	InstallDir              string `json:"install_dir"`
	CreateDesktopShortcut   bool   `json:"create_desktop_shortcut"`
	CreateStartMenuShortcut bool   `json:"create_start_menu_shortcut"`
	LaunchAfterInstall      bool   `json:"launch_after_install"`
}

type InstallProgress struct {
	Phase   string `json:"phase"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

type InstallResult struct {
	Success bool   `json:"success"`
	Version string `json:"version"`
	AppPath string `json:"app_path"`
	Error   string `json:"error"`
}

type InstallPlan struct {
	Version         string `json:"version"`
	Target          string `json:"target"`
	DefaultDir      string `json:"default_dir"`
	PayloadReady    bool   `json:"payload_ready"`
	PayloadSHA256   string `json:"payload_sha256"`
	AppExecutable   string `json:"app_executable"`
	RequiresAdmin   bool   `json:"requires_admin"`
	IsAdmin         bool   `json:"is_admin"`
	WebView2Ready   bool   `json:"webview2_ready"`
	WebView2Mode    string `json:"webview2_mode"`
	RequiredSpaceMB int    `json:"required_space_mb"`
	WintunIncluded  bool   `json:"wintun_included"`
	Signing         string `json:"signing"`
	Error           string `json:"error"`
}

type PayloadManifest struct {
	Version              string `json:"version"`
	Target               string `json:"target"`
	PayloadFile          string `json:"payload_file"`
	PayloadSHA256        string `json:"payload_sha256"`
	AppExecutable        string `json:"app_executable"`
	RequiredSpaceMB      int    `json:"required_space_mb"`
	WintunIncluded       bool   `json:"wintun_included"`
	WebView2Bootstrapper string `json:"webview2_bootstrapper"`
	Signing              string `json:"signing"`
}

type ProgressReporter func(InstallProgress)
