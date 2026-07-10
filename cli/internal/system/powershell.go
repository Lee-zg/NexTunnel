package system

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"unicode/utf16"
)

const windowsLogFilesEnvironment = "NEXTUNNEL_LOG_FILES_JSON"

func newWindowsTailLogsCommand(files []string, follow bool) (*exec.Cmd, error) {
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return nil, fmt.Errorf("编码 Windows 日志路径：%w", err)
	}
	script := `$pathsJson = [Environment]::GetEnvironmentVariable('NEXTUNNEL_LOG_FILES_JSON')
if ([string]::IsNullOrWhiteSpace($pathsJson)) { exit 2 }
[string[]]$paths = ConvertFrom-Json -InputObject $pathsJson
Get-Content -LiteralPath $paths -Tail 200`
	if follow {
		script += " -Wait"
	}

	command := exec.Command("powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-EncodedCommand",
		encodeWindowsPowerShellCommand(script),
	)
	// 路径通过环境变量传递，避免日志目录中的特殊字符被解释为 PowerShell 语法。
	command.Env = append(command.Environ(), windowsLogFilesEnvironment+"="+string(filesJSON))
	return command, nil
}

func encodeWindowsPowerShellCommand(script string) string {
	encodedWords := utf16.Encode([]rune(script))
	encodedBytes := make([]byte, len(encodedWords)*2)
	for index, word := range encodedWords {
		binary.LittleEndian.PutUint16(encodedBytes[index*2:], word)
	}
	return base64.StdEncoding.EncodeToString(encodedBytes)
}
