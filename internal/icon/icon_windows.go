//go:build windows

package icon

// 梦盒图标：内嵌 PNG → HICON，供原生窗口（winapp）与系统托盘（tray）共用。
// 纯 Go 实现（DIB Section + CreateIconIndirect），无 CGO。

import (
	"bytes"
	_ "embed"
	"errors"
	"image"
	_ "image/png"
	"image/draw"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed icon.png
var iconPNG []byte

var (
	modUser32           = windows.NewLazySystemDLL("user32.dll")
	modGdi32            = windows.NewLazySystemDLL("gdi32.dll")
	procCreateIconIndirect = modUser32.NewProc("CreateIconIndirect")
	procCreateDIBSection   = modGdi32.NewProc("CreateDIBSection")
	procCreateBitmap       = modGdi32.NewProc("CreateBitmap")
	procDestroyIcon        = modUser32.NewProc("DestroyIcon")
)

type iconInfo struct {
	fIcon    uint32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

var (
	cachedIcon uintptr
	cachedOnce sync.Once
	cachedErr  error
)

// Load 返回梦盒图标的 HICON（进程内缓存，多次调用返回同一句柄）。
func Load() (uintptr, error) {
	cachedOnce.Do(func() {
		cachedIcon, cachedErr = FromPNG(iconPNG)
	})
	return cachedIcon, cachedErr
}

// FromPNG 把 PNG 字节解码为 32bpp DIB，再经 CreateIconIndirect 生成 HICON。
// 调用方负责在不再需要时用 Free 释放（Load 缓存的句柄请勿释放）。
func FromPNG(png []byte) (uintptr, error) {
	src, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return 0, err
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || sw > 256 || sh > 256 {
		return 0, errors.New("图标尺寸非法")
	}
	rgba := image.NewRGBA(image.Rect(0, 0, sw, sh))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)

	var bi bitmapInfoHeader
	bi.biSize = uint32(unsafe.Sizeof(bi))
	bi.biWidth = int32(sw)
	bi.biHeight = -int32(sh) // 自上而下
	bi.biPlanes = 1
	bi.biBitCount = 32

	var bits unsafe.Pointer
	hColor, _, _ := procCreateDIBSection.Call(
		0,
		uintptr(unsafe.Pointer(&bi)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hColor == 0 || bits == nil {
		return 0, errors.New("CreateDIBSection 失败")
	}
	// 按 BGRA 写入像素（非预乘 alpha）
	dst := unsafe.Slice((*byte)(bits), sw*sh*4)
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			c := rgba.RGBAAt(x, y)
			var r, g, bl, a uint8
			if c.A == 0 {
				r, g, bl, a = 0, 0, 0, 0
			} else {
				// 反预乘
				r = uint8(uint16(c.R) * 255 / uint16(c.A))
				g = uint8(uint16(c.G) * 255 / uint16(c.A))
				bl = uint8(uint16(c.B) * 255 / uint16(c.A))
				a = c.A
			}
			o := (y*sw + x) * 4
			dst[o+0] = bl
			dst[o+1] = g
			dst[o+2] = r
			dst[o+3] = a
		}
	}
	// 全 0 掩码：透明度完全由 alpha 通道决定
	hMask, _, _ := procCreateBitmap.Call(uintptr(sw), uintptr(sh), 1, 1, 0)
	if hMask == 0 {
		return 0, errors.New("CreateBitmap 失败")
	}
	ii := iconInfo{fIcon: 1, hbmMask: hMask, hbmColor: hColor}
	hIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if hIcon == 0 {
		return 0, errors.New("CreateIconIndirect 失败")
	}
	return hIcon, nil
}

// Free 释放 FromPNG 生成的 HICON（Load 缓存的句柄不要调这个）。
func Free(hIcon uintptr) {
	if hIcon != 0 {
		_, _, _ = procDestroyIcon.Call(hIcon)
	}
}
