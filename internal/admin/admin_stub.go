//go:build !windows

package admin

// Linux stub：本机调试用，视为管理员、不弹窗。

func IsAdmin() bool              { return true }
func Warn(title, text string)    {}
