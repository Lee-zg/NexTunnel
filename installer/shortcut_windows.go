//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

const (
	wscriptShellProgramID = "WScript.Shell"
	comSFalse             = uint32(0x00000001)
	comRPCChangedMode     = uint32(0x80010106)
)

func createShellLink(shortcutPath string, targetPath string, workingDir string, description string) error {
	if err := os.MkdirAll(filepath.Dir(shortcutPath), 0o755); err != nil {
		return fmt.Errorf("创建快捷方式目录：%w", err)
	}

	return runInCOMApartment(func() error {
		return saveShellLink(shortcutPath, targetPath, workingDir, description)
	})
}

func runInCOMApartment(operation func() error) error {
	result := make(chan error, 1)
	go func() {
		// COM 对象必须在同一系统线程初始化、使用并释放；同时隔离 Wails UI 线程的 COM 模型。
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		uninitialize, err := initializeCOMApartment()
		if err != nil {
			result <- err
			return
		}
		defer uninitialize()
		result <- operation()
	}()
	return <-result
}

func initializeCOMApartment() (func(), error) {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err == nil {
		return ole.CoUninitialize, nil
	}

	var oleError *ole.OleError
	if !errors.As(err, &oleError) {
		return nil, fmt.Errorf("初始化 COM：%w", err)
	}
	// HRESULT 是 32 位值；显式截断可兼容 syscall 在 32/64 位上的返回宽度。
	switch uint32(oleError.Code()) {
	case comSFalse:
		// S_FALSE 仍代表初始化成功，并要求用 CoUninitialize 配对释放。
		return ole.CoUninitialize, nil
	case comRPCChangedMode:
		// 当前线程已使用另一种 COM 模型初始化，可以继续使用，但不能由这里反初始化。
		return func() {}, nil
	default:
		return nil, fmt.Errorf("初始化 COM：%w", err)
	}
}

func saveShellLink(shortcutPath string, targetPath string, workingDir string, description string) error {
	unknown, err := oleutil.CreateObject(wscriptShellProgramID)
	if err != nil {
		return fmt.Errorf("创建 WScript.Shell COM 对象：%w", err)
	}
	defer unknown.Release()

	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return fmt.Errorf("获取 WScript.Shell 接口：%w", err)
	}
	defer shell.Release()

	shortcutValue, err := oleutil.CallMethod(shell, "CreateShortcut", shortcutPath)
	if err != nil {
		return fmt.Errorf("创建快捷方式 %s：%w", shortcutPath, err)
	}
	if shortcutValue == nil || shortcutValue.ToIDispatch() == nil {
		if shortcutValue != nil {
			_ = shortcutValue.Clear()
		}
		return fmt.Errorf("创建快捷方式 %s：COM 未返回快捷方式对象", shortcutPath)
	}
	defer shortcutValue.Clear()
	shortcut := shortcutValue.ToIDispatch()

	properties := []struct {
		name  string
		value string
	}{
		{name: "TargetPath", value: targetPath},
		{name: "WorkingDirectory", value: workingDir},
		{name: "Description", value: description},
		{name: "IconLocation", value: targetPath},
	}
	for _, property := range properties {
		if err := putCOMProperty(shortcut, property.name, property.value); err != nil {
			return fmt.Errorf("设置快捷方式 %s 的 %s：%w", shortcutPath, property.name, err)
		}
	}
	if err := callCOMMethod(shortcut, "Save"); err != nil {
		return fmt.Errorf("保存快捷方式 %s：%w", shortcutPath, err)
	}
	return nil
}

func putCOMProperty(dispatch *ole.IDispatch, name string, value string) error {
	result, err := oleutil.PutProperty(dispatch, name, value)
	if result != nil {
		_ = result.Clear()
	}
	return err
}

func callCOMMethod(dispatch *ole.IDispatch, name string) error {
	result, err := oleutil.CallMethod(dispatch, name)
	if result != nil {
		_ = result.Clear()
	}
	return err
}
