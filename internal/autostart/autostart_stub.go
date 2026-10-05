//go:build !windows

package autostart

// Linux stub：本机调试用，什么都不做。

func Enable(silent bool) error { return nil }
func Disable() error           { return nil }
func IsEnabled() bool          { return false }
