//go:build darwin && desktop

package main

/*
#cgo LDFLAGS: -framework UniformTypeIdentifiers
*/
import "C"

// macOSLinkUniformTypeIdentifiers 确保 macOS 15+ SDK 下 Wails 依赖的 UTType 符号可被链接。
func macOSLinkUniformTypeIdentifiers() {}
