//go:build !windows

package winapp

import "errors"

// Run 非 Windows 下的占位实现（保证 Linux 构建通过；实际不调用）。
func Run(title, url, dataDir string, width, height uint, closeToTray bool) error {
	return errors.New("winapp 仅支持 Windows")
}

// Show 空操作。
func Show() {}

// Hide 空操作。
func Hide() {}

// Quit 空操作。
func Quit() {}

// SetCloseToTray 空操作。
func SetCloseToTray(b bool) {}

// SetDashboardURL 空操作（非 Windows 下无面板返回按钮）。
func SetDashboardURL(u string) {}

// Navigate 空操作。
func Navigate(url string) {}
