//go:build !windows

package icon

import "errors"

// Load 在非 Windows 平台上不可用。
func Load() (uintptr, error) {
	return 0, errors.New("图标仅支持 Windows")
}

// FromPNG 在非 Windows 平台上不可用。
func FromPNG(png []byte) (uintptr, error) {
	return 0, errors.New("图标仅支持 Windows")
}

// Free 在非 Windows 平台上是空操作。
func Free(hIcon uintptr) {}
