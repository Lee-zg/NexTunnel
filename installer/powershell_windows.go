//go:build windows

package main

import (
	"encoding/base64"
	"encoding/binary"
	"os/exec"
	"unicode/utf16"
)

func newEncodedPowerShellCommand(script string, hidden bool) *exec.Cmd {
	arguments := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}
	if hidden {
		arguments = append(arguments, "-WindowStyle", "Hidden")
	}
	arguments = append(arguments, "-EncodedCommand", encodePowerShellCommand(script))
	return exec.Command("powershell.exe", arguments...)
}

func encodePowerShellCommand(script string) string {
	encodedWords := utf16.Encode([]rune(script))
	encodedBytes := make([]byte, len(encodedWords)*2)
	for index, word := range encodedWords {
		binary.LittleEndian.PutUint16(encodedBytes[index*2:], word)
	}
	return base64.StdEncoding.EncodeToString(encodedBytes)
}
