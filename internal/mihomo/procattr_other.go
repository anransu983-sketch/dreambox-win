//go:build !windows

package mihomo

import "os/exec"

// hideChildConsole 非 Windows 下无操作（Linux 调试用）。
func hideChildConsole(c *exec.Cmd) {}
