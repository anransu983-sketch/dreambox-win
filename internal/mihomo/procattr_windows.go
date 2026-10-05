//go:build windows

package mihomo

import (
	"os/exec"
	"syscall"
)

// hideChildConsole 隐藏子进程的控制台窗口。
// mihomo.exe 是控制台子系统程序；当父进程为 windowsgui（无控制台）时，
// Windows 会为子进程弹出一个可见的黑窗口，必须显式隐藏。
func hideChildConsole(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
