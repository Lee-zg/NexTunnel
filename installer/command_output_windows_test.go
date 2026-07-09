//go:build windows

package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestDecodeWindowsCommandOutputGB18030(t *testing.T) {
	message := "错误: 没有找到进程 NexTunnel.exe"
	encoded, err := simplifiedchinese.GB18030.NewEncoder().String(message)
	if err != nil {
		t.Fatalf("encode GB18030: %v", err)
	}

	decoded := decodeWindowsCommandOutput([]byte(encoded))
	if decoded != message {
		t.Fatalf("decoded=%q want=%q", decoded, message)
	}
	if !isTaskkillProcessNotFound(decoded) {
		t.Fatalf("expected taskkill not-found message to be ignored: %q", decoded)
	}
}

func TestDecodeWindowsCommandOutputUTF16LE(t *testing.T) {
	message := "安装器需要管理员权限"
	decoded := decodeWindowsCommandOutput(utf16LEBytes(message))
	if decoded != message {
		t.Fatalf("decoded=%q want=%q", decoded, message)
	}
}

func TestDecodeWindowsCommandOutputUTF8(t *testing.T) {
	message := "failed to create shortcut"
	decoded := decodeWindowsCommandOutput([]byte("  " + message + "\r\n"))
	if strings.TrimSpace(decoded) != message {
		t.Fatalf("decoded=%q want=%q", decoded, message)
	}
}

func utf16LEBytes(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	output := make([]byte, len(encoded)*2)
	for index, word := range encoded {
		binary.LittleEndian.PutUint16(output[index*2:], word)
	}
	return output
}
