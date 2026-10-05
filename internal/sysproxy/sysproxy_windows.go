//go:build windows

package sysproxy

// Windows 系统代理开关：改 HKCU\...\Internet Settings 并通知系统刷新。

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	regPath     = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	proxyServer = "127.0.0.1:7890"
)

// Enable 开启系统代理
func Enable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", proxyServer); err != nil {
		return err
	}
	notifyChanged()
	return nil
}

// Disable 关闭系统代理
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	notifyChanged()
	return nil
}

// IsEnabled 当前系统代理是否开启（且指向本机 7890）
func IsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || v == 0 {
		return false
	}
	srv, _, err := k.GetStringValue("ProxyServer")
	return err == nil && srv == proxyServer
}

// notifyChanged 通知 Windows 代理设置已变更
func notifyChanged() {
	wininet := windows.NewLazySystemDLL("wininet.dll")
	proc := wininet.NewProc("InternetSetOptionW")
	const (
		INTERNET_OPTION_SETTINGS_CHANGED = 39
		INTERNET_OPTION_REFRESH          = 37
	)
	_, _, _ = proc.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	_, _, _ = proc.Call(0, INTERNET_OPTION_REFRESH, 0, 0)
}
