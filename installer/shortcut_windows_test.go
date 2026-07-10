//go:build windows

package main

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestCreateWindowsShortcutWithSpecialCharacters(t *testing.T) {
	root := t.TempDir()
	workingDir := filepath.Join(root, "Program Files & Tools", "NexTunnel's App")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(workingDir, "NexTunnel.exe")
	if err := os.WriteFile(targetPath, []byte("test executable placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	shortcutPath := filepath.Join(root, "Public Desktop", "NexTunnel.lnk")

	if err := createWindowsShortcut(shortcutPath, targetPath, workingDir, "NexTunnel 桌面客户端's shortcut"); err != nil {
		t.Fatalf("createWindowsShortcut: %v", err)
	}
	info, err := os.Stat(shortcutPath)
	if err != nil {
		t.Fatalf("expected shortcut to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected shortcut to contain shell link data")
	}
}

func TestEncodePowerShellCommandUsesUTF16LE(t *testing.T) {
	script := "Write-Output '卸载 NexTunnel'"
	decodedBytes, err := base64.StdEncoding.DecodeString(encodePowerShellCommand(script))
	if err != nil {
		t.Fatalf("decode command: %v", err)
	}
	if len(decodedBytes)%2 != 0 {
		t.Fatalf("encoded command has odd byte length: %d", len(decodedBytes))
	}
	words := make([]uint16, len(decodedBytes)/2)
	for index := range words {
		words[index] = binary.LittleEndian.Uint16(decodedBytes[index*2:])
	}
	if decoded := string(utf16.Decode(words)); decoded != script {
		t.Fatalf("decoded=%q want=%q", decoded, script)
	}
}

func TestEncodedPowerShellCommandReadsSpecialCharactersFromEnvironment(t *testing.T) {
	const environmentName = "NEXTUNNEL_POWERSHELL_TEST_VALUE"
	value := `C:\Program Files & Tools\NexTunnel's App`
	script := `[Console]::OutputEncoding = [Text.Encoding]::UTF8
[Console]::Write([Environment]::GetEnvironmentVariable('NEXTUNNEL_POWERSHELL_TEST_VALUE'))`
	command := newEncodedPowerShellCommand(script, false)
	command.Env = append(command.Environ(), environmentName+"="+value)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run encoded PowerShell command: %v: %s", err, decodeWindowsCommandOutput(output))
	}
	if decoded := strings.TrimSpace(decodeWindowsCommandOutput(output)); decoded != value {
		t.Fatalf("decoded=%q want=%q", decoded, value)
	}
}

func TestDirectoryRemovalPowerShellScriptRejectsMissingEnvironment(t *testing.T) {
	command := newEncodedPowerShellCommand(directoryRemovalPowerShellScript, false)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("expected missing removal environment to be rejected")
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 2 {
		t.Fatalf("unexpected script result: %v: %s", err, decodeWindowsCommandOutput(output))
	}
}
