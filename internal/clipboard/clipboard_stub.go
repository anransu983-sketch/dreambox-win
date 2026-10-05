//go:build !windows

package clipboard

// StartWatcher 非 Windows 空实现（开发机调试用）
func StartWatcher(onLink func(link string)) {}
