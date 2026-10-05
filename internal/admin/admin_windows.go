//go:build windows

package admin

// 管理员权限检测（TUN 必需）与弹窗提示。纯 Go 系统调用，无需 CGO。

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsAdmin 当前进程是否以管理员身份运行
func IsAdmin() bool {
	advapi32 := windows.NewLazySystemDLL("advapi32.dll")
	proc := advapi32.NewProc("CheckTokenMembership")
	var sid *windows.SID
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	var isMember int32
	ret, _, _ := proc.Call(0, uintptr(unsafe.Pointer(sid)), uintptr(unsafe.Pointer(&isMember)))
	return ret != 0 && isMember != 0
}

// Warn 弹出警告框
func Warn(title, text string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	const MB_ICONWARNING = 0x30
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), MB_ICONWARNING)
}
