package command

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nextunnel/cli/internal/system"
)

func TestBuildServerInstallerArgsForLinux(t *testing.T) {
	options := serverInstallerOptions{
		version:           "v0.6.4-alpha",
		packageURL:        "/tmp/nextunnel-server-linux-amd64.tar.gz",
		packageSHA256:     "abc123",
		releaseBaseURL:    "https://cos.example.com/nextunnel/v0.6.4-alpha",
		publicHost:        "example.com",
		relayPort:         "27000",
		relayQuicPort:     "27443",
		controlPlanePort:  "29090",
		dashboardPort:     "28080",
		natPort:           "23478",
		relayToken:        "relay-token",
		controlToken:      "control-token",
		dashboardPassword: "dashboard-password",
		servicePrefix:     "nextunnel-test",
		cliLink:           "none",
		dashboardDisabled: true,
		nonInteractive:    true,
		force:             true,
	}
	paths := system.Paths{
		InstallDir: "/opt/nextunnel-test",
		ConfigDir:  "/etc/nextunnel-test",
		DataDir:    "/var/lib/nextunnel-test",
	}

	args, err := buildServerInstallerArgs(options, paths, "linux")
	if err != nil {
		t.Fatalf("buildServerInstallerArgs: %v", err)
	}

	want := []string{
		"--version", "v0.6.4-alpha",
		"--package-url", "/tmp/nextunnel-server-linux-amd64.tar.gz",
		"--sha256", "abc123",
		"--release-base-url", "https://cos.example.com/nextunnel/v0.6.4-alpha",
		"--install-dir", "/opt/nextunnel-test",
		"--config-dir", "/etc/nextunnel-test",
		"--data-dir", "/var/lib/nextunnel-test",
		"--public-host", "example.com",
		"--relay-port", "27000",
		"--relay-quic-port", "27443",
		"--control-plane-port", "29090",
		"--dashboard-port", "28080",
		"--nat-port", "23478",
		"--relay-token", "relay-token",
		"--control-token", "control-token",
		"--dashboard-password", "dashboard-password",
		"--dashboard-disabled",
		"--non-interactive",
		"--force",
		"--service-prefix", "nextunnel-test",
		"--cli-link", "none",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args mismatch:\n got=%v\nwant=%v", args, want)
	}
}

func TestBuildServerInstallerArgsRejectsLinuxOnlyFlagsOnWindows(t *testing.T) {
	options := serverInstallerOptions{servicePrefix: "nextunnel-test"}

	_, err := buildServerInstallerArgs(options, system.Paths{}, "windows")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "仅支持 Linux") {
		t.Fatalf("unexpected error: %v", err)
	}
}
