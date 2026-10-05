//go:build windows

package tray

// 系统托盘：独立 OS 线程跑自己的 GetMessage 循环，与 WebView2 主线程互不干扰。
// 纯 Go 实现（user32/shell32 直接调用），无 CGO。
//
// 右键菜单：显示主界面 / ── / TUN 开启(勾选) / 重启内核 / ── / 退出
// 左键单击（或双击）→ 显示主窗口。

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"dreambox-win/internal/icon"
	"golang.org/x/sys/windows"
)

var (
	modUser32 = windows.NewLazySystemDLL("user32.dll")
	modShell32 = windows.NewLazySystemDLL("shell32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW   = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW    = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW     = modUser32.NewProc("DefWindowProcW")
	procGetMessageW        = modUser32.NewProc("GetMessageW")
	procTranslateMessage   = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW   = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage    = modUser32.NewProc("PostQuitMessage")
	procShowWindow         = modUser32.NewProc("ShowWindow")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procGetCursorPos       = modUser32.NewProc("GetCursorPos")
	procPostMessageW       = modUser32.NewProc("PostMessageW")
	procDestroyWindow      = modUser32.NewProc("DestroyWindow")
	procCreatePopupMenu    = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW        = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu     = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu        = modUser32.NewProc("DestroyMenu")
	procGetModuleHandleW   = modKernel32.NewProc("GetModuleHandleW")
	procShellNotifyIconW   = modShell32.NewProc("Shell_NotifyIconW")
)

const (
	wmApp         = 0x8000
	wmNull        = 0x0000
	wmClose       = 0x0010
	wmDestroy     = 0x0002
	wmLButtonUp   = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp   = 0x0205
	wmTrayIcon    = wmApp + 1

	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	notifyIconVersion4 = 4

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	mfChecked   = 0x00000008

	tpmReturnCmd  = 0x0100
	tpmRightButton = 0x0002

	hwndMessage = uintptr(0xFFFFFFFFFFFFFFFD) // (HWND)-3
)

const (
	cmdShow    = 1001
	cmdTun     = 1002
	cmdRestart = 1003
	cmdQuit    = 1004
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msgT struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX      int32
	ptY      int32
	lPrivate uint32
}

type pointT struct {
	x int32
	y int32
}

type notifyIconData struct {
	cbSize            uint32
	hWnd              uintptr
	uID               uint32
	uFlags            uint32
	uCallbackMessage  uint32
	hIcon             uintptr
	szTip             [128]uint16
	dwState           uint32
	dwStateMask       uint32
	szInfo            [256]uint16
	uTimeoutOrVersion uint32
	szInfoTitle       [64]uint16
	dwInfoFlags       uint32
	guidItem          [16]byte
	hBalloonIcon      uintptr
}

// Callbacks 托盘事件回调（均在托盘线程上被调用，须线程安全、尽快返回）。
type Callbacks struct {
	OnShow      func()      // 显示主窗口
	OnToggleTun func()      // 切换 TUN 开关
	OnRestart   func()      // 重启内核
	OnQuit      func()      // 退出程序
	TunOn       func() bool // 查询 TUN 当前状态（菜单勾选态用）
}

var (
	mu        sync.Mutex
	started   bool
	cb        Callbacks
	trayHwnd  atomic.Uintptr
	wndProcCb uintptr // 防止 GC 回收窗口过程回调
	nid       notifyIconData
)

func safeCall(f func()) {
	if f == nil {
		return
	}
	defer func() { _ = recover() }()
	f()
}

// Start 启动托盘（独立 OS 线程）。成功返回 nil；失败返回 error，
// 调用方应记日志后继续运行（无托盘模式）。
func Start(c Callbacks) error {
	mu.Lock()
	if started {
		mu.Unlock()
		return errors.New("托盘已在运行")
	}
	started = true
	mu.Unlock()

	cb = c
	if _, err := icon.Load(); err != nil {
		mu.Lock()
		started = false
		mu.Unlock()
		return fmt.Errorf("加载托盘图标失败: %w", err)
	}

	initCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		if err := createAndRegister(); err != nil {
			initCh <- err
			return
		}
		initCh <- nil
		messageLoop()
	}()

	select {
	case err := <-initCh:
		if err != nil {
			mu.Lock()
			started = false
			mu.Unlock()
		}
		return err
	case <-time.After(10 * time.Second):
		mu.Lock()
		started = false
		mu.Unlock()
		return errors.New("托盘初始化超时")
	}
}

// Stop 移除托盘图标并退出托盘线程。
func Stop() {
	if hwnd := trayHwnd.Load(); hwnd != 0 {
		_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

// SetTooltip 更新托盘悬浮提示。
func SetTooltip(tip string) {
	if trayHwnd.Load() == 0 {
		return
	}
	var m notifyIconData
	m.cbSize = uint32(unsafe.Sizeof(m))
	m.hWnd = trayHwnd.Load()
	m.uID = 1
	m.uFlags = nifTip
	copy(m.szTip[:], utf16Trunc(tip, 127))
	_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&m)))
}

func utf16Trunc(s string, max int) []uint16 {
	u := windows.StringToUTF16(s)
	if len(u)-1 > max {
		u = append(append([]uint16{}, u[:max]...), 0)
	}
	return u
}

func createAndRegister() error {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := windows.StringToUTF16Ptr("DreamBoxTrayWindow")
	wndProcCb = windows.NewCallback(trayWndProc)

	var wc wndClassEx
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = wndProcCb
	wc.hInstance = hInst
	wc.lpszClassName = className
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return errors.New("RegisterClassExW 失败")
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("梦盒托盘"))),
		0, 0, 0, 0, 0,
		hwndMessage, 0, hInst, 0,
	)
	if hwnd == 0 {
		return errors.New("CreateWindowExW 失败")
	}
	trayHwnd.Store(hwnd)

	hIcon, _ := icon.Load()
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayIcon
	nid.hIcon = hIcon
	copy(nid.szTip[:], utf16Trunc("梦盒", 127))
	if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
		_, _, _ = procDestroyWindow.Call(hwnd)
		trayHwnd.Store(0)
		return errors.New("Shell_NotifyIconW(NIM_ADD) 失败")
	}
	// 启用 NOTIFYICON_VERSION_4 行为
	var ver notifyIconData
	ver.cbSize = uint32(unsafe.Sizeof(ver))
	ver.hWnd = hwnd
	ver.uID = 1
	ver.uTimeoutOrVersion = notifyIconVersion4
	_, _, _ = procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&ver)))
	return nil
}

func messageLoop() {
	var m msgT
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func trayWndProc(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmTrayIcon:
		switch uint32(lp) {
		case wmLButtonUp, wmLButtonDblClk:
			safeCall(cb.OnShow)
		case wmRButtonUp:
			showMenu(hwnd)
		}
		return 0
	case wmDestroy:
		_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		trayHwnd.Store(0)
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

func showMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	appendItem := func(flags uintptr, id uintptr, text string) {
		_, _, _ = procAppendMenuW.Call(hMenu, flags, id, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(text))))
	}
	appendItem(mfString, cmdShow, "显示主界面(&S)")
	appendItem(mfSeparator, 0, "")
	tunFlags := uintptr(mfString)
	if cb.TunOn != nil {
		func() {
			defer func() { _ = recover() }()
			if cb.TunOn() {
				tunFlags |= mfChecked
			}
		}()
	}
	appendItem(tunFlags, cmdTun, "TUN 开启(&T)")
	appendItem(mfString, cmdRestart, "重启内核(&R)")
	appendItem(mfSeparator, 0, "")
	appendItem(mfString, cmdQuit, "退出(&X)")

	var pt pointT
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(hMenu, tpmReturnCmd|tpmRightButton,
		uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
	_, _, _ = procPostMessageW.Call(hwnd, wmNull, 0, 0)

	switch cmd {
	case cmdShow:
		safeCall(cb.OnShow)
	case cmdTun:
		safeCall(cb.OnToggleTun)
	case cmdRestart:
		safeCall(cb.OnRestart)
	case cmdQuit:
		safeCall(cb.OnQuit)
	}
}
