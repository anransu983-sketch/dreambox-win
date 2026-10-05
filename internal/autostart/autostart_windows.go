//go:build windows

package autostart

// 开机自启：写 HKCU\Software\Microsoft\Windows\CurrentVersion\Run。

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	regPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueKey = "DreamBox"
)

// Enable 开启开机自启；silent 为 true 时带 --minimized（不显示窗口后台跑）
func Enable(silent bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	val := `"` + exe + `"`
	if silent {
		val += " --minimized"
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(valueKey, val)
}

// Disable 关闭开机自启
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	_ = k.DeleteValue(valueKey) // 不存在也算成功
	return nil
}

// IsEnabled 是否已设置开机自启
func IsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueKey)
	return err == nil
}
