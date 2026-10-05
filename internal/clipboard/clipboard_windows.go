//go:build windows

package clipboard

// Windows 剪贴板轮询：每 1.5s 用 OpenClipboard/GetClipboardData(CF_UNICODETEXT)
// 读取文本；新出现的代理链接触发 onLink。所有错误静默跳过，不打扰主流程。

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cfUnicodeText = 13

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard      = user32.NewProc("OpenClipboard")
	procCloseClipboard     = user32.NewProc("CloseClipboard")
	procGetClipboardData   = user32.NewProc("GetClipboardData")
	procGlobalLock         = kernel32.NewProc("GlobalLock")
	procGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	procGetClipboardSeqNum = user32.NewProc("GetClipboardSequenceNumber")
)

// StartWatcher 启动剪贴板嗅探 goroutine；onLink 在新代理链接出现时被调用。
func StartWatcher(onLink func(link string)) {
	go func() {
		var lastSeq uint32
		var lastLink string
		for {
			time.Sleep(1500 * time.Millisecond)
			seq := clipboardSeq()
			if seq == lastSeq {
				continue
			}
			lastSeq = seq
			text := readText()
			if text == "" {
				continue
			}
			link := ExtractLink(text)
			if link == "" || link == lastLink {
				continue
			}
			lastLink = link
			onLink(link)
		}
	}()
}

func clipboardSeq() uint32 {
	r, _, _ := procGetClipboardSeqNum.Call()
	return uint32(r)
}

// readText 读取当前剪贴板文本；失败返回 ""
func readText() string {
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)

	// UTF-16 以 0 结尾，逐字读取
	var buf []uint16
	for i := 0; ; i++ {
		c := *(*uint16)(unsafe.Pointer(p + uintptr(i*2)))
		if c == 0 {
			break
		}
		buf = append(buf, c)
		if len(buf) > 4096 { // 限长，避免超长文本
			break
		}
	}
	return windows.UTF16ToString(buf)
}
