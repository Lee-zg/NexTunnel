package macoshelperctl

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type recordingRunner struct {
	commands []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.commands = append(r.commands, name+" "+joinArgsForTest(args))
	return "", nil
}

func TestValidateAction(t *testing.T) {
	for _, action := range []string{ActionInstall, ActionStatus, ActionRestart, ActionUninstall} {
		if !ValidateAction(action) {
			t.Fatalf("expected %s to be allowed", action)
		}
	}
	if ValidateAction("shell") {
		t.Fatal("unexpected dynamic helper action accepted")
	}
}

func TestResolveResourcesFromResourceDirectory(t *testing.T) {
	resourceDir := writeTestResources(t)
	resources, err := (Manager{Resources: Resources{ResourceDir: resourceDir}}).ResolveResources()
	if err != nil {
		t.Fatalf("ResolveResources: %v", err)
	}
	if resources.HelperBinary != filepath.Join(resourceDir, HelperExecutableName) {
		t.Fatalf("unexpected helper path: %s", resources.HelperBinary)
	}
	if resources.LaunchDaemonPlist != filepath.Join(resourceDir, ResourcePlistName) {
		t.Fatalf("unexpected plist path: %s", resources.LaunchDaemonPlist)
	}
}

func TestResolveResourcesFromExecutableDirectory(t *testing.T) {
	root := t.TempDir()
	resourceDir := filepath.Join(root, ResourceDirectoryName)
	writeTestResourcesAt(t, resourceDir)
	manager := Manager{
		ExecutablePath: func() (string, error) {
			return filepath.Join(root, "nextunnel"), nil
		},
	}
	resources, err := manager.ResolveResources()
	if err != nil {
		t.Fatalf("ResolveResources: %v", err)
	}
	if resources.ResourceDir != resourceDir {
		t.Fatalf("unexpected resource dir: %s", resources.ResourceDir)
	}
}

func TestRequireRoot(t *testing.T) {
	manager := Manager{EUID: func() int { return 501 }}
	if err := manager.requireRoot(); err != ErrRequiresRoot {
		t.Fatalf("expected ErrRequiresRoot, got %v", err)
	}
	manager.EUID = func() int { return 0 }
	if err := manager.requireRoot(); err != nil {
		t.Fatalf("root should be accepted: %v", err)
	}
}

func TestInstallUsesFixedLaunchctlSequence(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("install execution is darwin-only")
	}
	root := t.TempDir()
	resourceDir := writeTestResources(t)
	runner := &recordingRunner{}
	manager := Manager{
		Resources: Resources{ResourceDir: resourceDir},
		Runner:    runner,
		EUID:      func() int { return 0 },
		chown:     func(string, int, int) error { return nil },
		paths: targetPaths{
			HelperPath:       filepath.Join(root, "Library", "PrivilegedHelperTools", HelperExecutableName),
			LaunchDaemonPath: filepath.Join(root, "Library", "LaunchDaemons", ResourcePlistName),
			SocketDirectory:  filepath.Join(root, "var", "run", "nextunnel"),
			SocketPath:       filepath.Join(root, "var", "run", "nextunnel", "helper.sock"),
		},
	}
	result, err := manager.Install(context.Background())
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.Action != ActionInstall {
		t.Fatalf("unexpected action: %s", result.Action)
	}
	want := []string{
		"launchctl bootout system/com.nextunnel.helper",
		"launchctl bootstrap system " + manager.paths.LaunchDaemonPath,
		"launchctl enable system/com.nextunnel.helper",
		"launchctl kickstart -k system/com.nextunnel.helper",
		"launchctl print system/com.nextunnel.helper",
	}
	if len(runner.commands) != len(want) {
		t.Fatalf("commands=%v want=%v", runner.commands, want)
	}
	for index := range want {
		if runner.commands[index] != want[index] {
			t.Fatalf("command[%d]=%q want %q", index, runner.commands[index], want[index])
		}
	}
}

func TestStatusUnsupportedOffDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("unsupported status is non-darwin behavior")
	}
	status := (Manager{}).Status(context.Background())
	if status.Supported {
		t.Fatal("expected non-darwin status to be unsupported")
	}
}

func writeTestResources(t *testing.T) string {
	t.Helper()
	resourceDir := filepath.Join(t.TempDir(), ResourceDirectoryName)
	writeTestResourcesAt(t, resourceDir)
	return resourceDir
}

func writeTestResourcesAt(t *testing.T, resourceDir string) {
	t.Helper()
	if err := os.MkdirAll(resourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, HelperExecutableName), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, ResourcePlistName), []byte("<plist version=\"1.0\"></plist>\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func joinArgsForTest(args []string) string {
	if len(args) == 0 {
		return ""
	}
	result := args[0]
	for _, arg := range args[1:] {
		result += " " + arg
	}
	return result
}
