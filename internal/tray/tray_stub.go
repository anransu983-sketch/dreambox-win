//go:build !windows

package tray

import "errors"

// Callbacks 托盘事件回调（非 Windows 空实现）。
type Callbacks struct {
	OnShow      func()
	OnToggleTun func()
	OnRestart   func()
	OnQuit      func()
	TunOn       func() bool
}

// Start 在非 Windows 平台上始终返回错误。
func Start(c Callbacks) error {
	return errors.New("系统托盘仅支持 Windows")
}

// Stop 空操作。
func Stop() {}

// SetTooltip 空操作。
func SetTooltip(tip string) {}
