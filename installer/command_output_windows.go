//go:build windows

package main

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
)

func decodeWindowsCommandOutput(output []byte) string {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return ""
	}
	if bytes.HasPrefix(trimmed, []byte{0xff, 0xfe}) {
		if decoded, err := textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM).NewDecoder().Bytes(trimmed); err == nil {
			return strings.TrimSpace(string(decoded))
		}
	}
	if utf8.Valid(trimmed) && !bytes.Contains(trimmed, []byte{0}) {
		return strings.TrimSpace(string(trimmed))
	}

	candidates := make([]string, 0, 2)
	if len(trimmed)%2 == 0 {
		if decoded, err := textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM).NewDecoder().Bytes(trimmed); err == nil {
			candidates = append(candidates, string(decoded))
		}
	}
	// Windows 本地化命令在中文系统上常通过 ANSI/OEM 代码页输出到管道。
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(trimmed)
	if err == nil && utf8.Valid(decoded) {
		candidates = append(candidates, string(decoded))
	}
	if best := bestDecodedCommandOutput(candidates); best != "" {
		return best
	}
	return strings.TrimSpace(string(bytes.ToValidUTF8(trimmed, []byte("�"))))
}

func bestDecodedCommandOutput(candidates []string) string {
	bestScore := -1 << 30
	bestValue := ""
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		score := decodedCommandOutputScore(trimmed)
		if score > bestScore {
			bestScore = score
			bestValue = trimmed
		}
	}
	if bestScore <= 0 {
		return ""
	}
	return bestValue
}

func decodedCommandOutputScore(value string) int {
	score := 0
	for _, char := range value {
		switch {
		case char == utf8.RuneError:
			score -= 20
		case char == '\r' || char == '\n' || char == '\t':
			score++
		case unicode.IsControl(char):
			score -= 20
		case unicode.IsPrint(char):
			score += 2
			if unicode.IsLetter(char) || unicode.IsNumber(char) || unicode.Is(unicode.Han, char) {
				score++
			}
		default:
			score -= 6
		}
	}
	return score
}
