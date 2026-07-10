package system

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestActiveLinuxServicesUsesDefaultPrefix(t *testing.T) {
	paths := Paths{EnvPath: filepath.Join(t.TempDir(), "missing.env")}

	services := activeLinuxServices(paths)

	want := []string{
		"nextunnel-control-plane.service",
		"nextunnel-relay.service",
		"nextunnel-nat-detector.service",
		"nextunnel-dashboard.service",
	}
	assertStringSliceEqual(t, services, want)
}

func TestActiveLinuxServicesUsesConfiguredPrefixAndDashboardFlag(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "server.env")
	content := "NEXTUNNEL_SERVICE_PREFIX=nextunnel-wsltest\nDASHBOARD_ENABLED=false\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write env: %v", err)
	}

	services := activeLinuxServices(Paths{EnvPath: envPath})

	want := []string{
		"nextunnel-wsltest-control-plane.service",
		"nextunnel-wsltest-relay.service",
		"nextunnel-wsltest-nat-detector.service",
	}
	assertStringSliceEqual(t, services, want)
}

func TestWindowsTailLogsCommandKeepsPathsOutOfScript(t *testing.T) {
	files := []string{
		`C:\Program Files & Tools\NexTunnel's Logs\control-plane.log`,
		`C:\日志\relay-server.log`,
	}
	command, err := newWindowsTailLogsCommand(files, true)
	if err != nil {
		t.Fatalf("newWindowsTailLogsCommand: %v", err)
	}
	if len(command.Args) == 0 || command.Args[0] != "powershell.exe" {
		t.Fatalf("unexpected command: %v", command.Args)
	}
	joinedArguments := strings.Join(command.Args, " ")
	for _, file := range files {
		if strings.Contains(joinedArguments, file) {
			t.Fatalf("path leaked into PowerShell command text: %q", file)
		}
	}

	encodedScript := command.Args[len(command.Args)-1]
	scriptBytes, err := base64.StdEncoding.DecodeString(encodedScript)
	if err != nil {
		t.Fatalf("decode PowerShell command: %v", err)
	}
	words := make([]uint16, len(scriptBytes)/2)
	for index := range words {
		words[index] = binary.LittleEndian.Uint16(scriptBytes[index*2:])
	}
	script := string(utf16.Decode(words))
	if !strings.Contains(script, "-LiteralPath $paths -Tail 200 -Wait") {
		t.Fatalf("unexpected PowerShell script: %q", script)
	}

	var encodedFiles []string
	for _, environment := range command.Env {
		if strings.HasPrefix(environment, windowsLogFilesEnvironment+"=") {
			value := strings.TrimPrefix(environment, windowsLogFilesEnvironment+"=")
			if err := json.Unmarshal([]byte(value), &encodedFiles); err != nil {
				t.Fatalf("decode log files environment: %v", err)
			}
		}
	}
	assertStringSliceEqual(t, encodedFiles, files)
}

func TestWindowsTailLogsCommandReadsSpecialCharacterPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell execution is only available on Windows")
	}
	logDir := filepath.Join(t.TempDir(), "Program Files & Tools", "NexTunnel's Logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		filepath.Join(logDir, "control-plane.log"),
		filepath.Join(logDir, "relay-server.log"),
	}
	contents := []string{"nextunnel-log-1", "nextunnel-log-2"}
	for index, file := range files {
		if err := os.WriteFile(file, []byte(contents[index]), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	command, err := newWindowsTailLogsCommand(files, false)
	if err != nil {
		t.Fatalf("newWindowsTailLogsCommand: %v", err)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run Windows tail command: %v: %s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"nextunnel-log-1", "nextunnel-log-2"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output does not contain %q: %q", expected, text)
		}
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("got[%d] = %q, want %q; got=%v", index, got[index], want[index], got)
		}
	}
}
