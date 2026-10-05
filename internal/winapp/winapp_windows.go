//go:build windows

package winapp

// 梦盒原生应用窗口：基于 WebView2，把本地 Web 仪表盘装进原生窗口里。
//
// 关闭行为由 closeToTray 控制：
//   - true：拦截 WM_CLOSE，隐藏窗口到托盘（内核继续跑）
//   - false：正常关闭，Run 返回后由调用方停内核退出
//
// Show/Hide/Quit 供托盘回调调用；Terminate 跨线程安全。

import (
	"fmt"
	"sync"
	"sync/atomic"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"dreambox-win/internal/icon"
)

var (
	modUser32             = windows.NewLazySystemDLL("user32.dll")
	procSendMessageW      = modUser32.NewProc("SendMessageW")
	procShowWindow        = modUser32.NewProc("ShowWindow")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procCallWindowProcW   = modUser32.NewProc("CallWindowProcW")
	procGetWindowLongPtrW = modUser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = modUser32.NewProc("SetWindowLongPtrW")
)

const (
	wmSetIcon   = 0x0080
	wmClose     = 0x0010
	iconSmall   = 0
	iconBig     = 1
	swHide      = 0
	swShow      = 5
	gwlpWndproc = ^uintptr(3) // (LONG_PTR)-4
)

var (
	mu            sync.Mutex
	curW          webview2.WebView
	curHwnd       uintptr
	origWndProc   uintptr
	subclassCb    uintptr // 防止 GC
	closeToTrayAt int32   // atomic bool
	dashboardURL  string  // 梦盒主界面地址，用于面板页"返回梦盒"按钮
)

// SetDashboardURL 设置主界面地址（面板页悬浮"返回梦盒"按钮跳回这里）。
// 需在 Run 之前调用。
func SetDashboardURL(u string) {
	mu.Lock()
	dashboardURL = u
	mu.Unlock()
}

// Navigate 让当前窗口跳转到指定 URL（可在任意线程调用）。
func Navigate(url string) {
	mu.Lock()
	w := curW
	mu.Unlock()
	if w == nil {
		return
	}
	w.Dispatch(func() { w.Navigate(url) })
}

// backButtonJS 注入到每个页面的初始化脚本：在 metacubexd 面板页（/ui/）
// 加一个悬浮"← 返回梦钩"按钮。Init 保证在 window.onload 之前执行。
func backButtonJS() string {
	mu.Lock()
	dash := dashboardURL
	mu.Unlock()
	if dash == "" {
		return ""
	}
	return `(function(){
try{
if(location.href.indexOf('/ui/')===-1)return;
var css='#dreambox-back{position:fixed;top:12px;left:12px;z-index:2147483647;padding:8px 16px;border-radius:10px;border:1px solid rgba(255,255,255,.25);background:rgba(16,185,129,.93);color:#fff;font-size:14px;font-weight:600;cursor:pointer;box-shadow:0 4px 14px rgba(0,0,0,.35);font-family:inherit}';
function add(){
if(document.getElementById('dreambox-back'))return;
var st=document.createElement('style');st.textContent=css;
(document.head||document.documentElement).appendChild(st);
var b=document.createElement('button');b.id='dreambox-back';b.textContent='← 返回梦盒';
b.onclick=function(){location.href='` + dash + `';};
document.documentElement.appendChild(b);
}
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',add);else add();
}catch(e){}
})();`
}

// SetCloseToTray 运行时切换关闭行为（设置页开关即时生效）。
func SetCloseToTray(b bool) {
	if b {
		atomic.StoreInt32(&closeToTrayAt, 1)
	} else {
		atomic.StoreInt32(&closeToTrayAt, 0)
	}
}

// Run 创建原生窗口并进入消息循环，阻塞直到窗口关闭或 Quit 被调用。
// dataDir 为 WebView2 用户数据目录。返回非 nil 错误表示 WebView2 不可用。
func Run(title, url, dataDir string, width, height uint, closeToTray bool) error {
	SetCloseToTray(closeToTray)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  dataDir,
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  width,
			Height: height,
			Center: true,
		},
	})
	if w == nil {
		return fmt.Errorf("WebView2 初始化失败（可能未安装 WebView2 Runtime）")
	}
	defer w.Destroy()

	mu.Lock()
	curW = w
	if hw := w.Window(); hw != nil {
		curHwnd = uintptr(hw)
	}
	mu.Unlock()
	defer func() {
		mu.Lock()
		curW = nil
		curHwnd = 0
		mu.Unlock()
	}()

	setWindowIcon(w) // 尽力而为：失败则保留默认图标
	subclassForCloseTray(curHwnd)
	if js := backButtonJS(); js != "" {
		w.Init(js) // 每个页面加载前执行：面板页注入"返回梦盒"悬浮按钮
	}
	w.Navigate(url)
	w.Run() // 窗口关闭（WM_CLOSE）或 Quit 时返回
	return nil
}

// Show 显示主窗口（托盘"显示主界面"/左键单击用），可在任意线程调用。
func Show() {
	mu.Lock()
	w, hwnd := curW, curHwnd
	mu.Unlock()
	if w == nil || hwnd == 0 {
		return
	}
	w.Dispatch(func() {
		_, _, _ = procShowWindow.Call(hwnd, swShow)
		_, _, _ = procSetForegroundWindow.Call(hwnd)
	})
}

// Hide 隐藏主窗口，可在任意线程调用。
func Hide() {
	mu.Lock()
	w, hwnd := curW, curHwnd
	mu.Unlock()
	if w == nil || hwnd == 0 {
		return
	}
	w.Dispatch(func() {
		_, _, _ = procShowWindow.Call(hwnd, swHide)
	})
}

// Quit 终止消息循环（托盘"退出"用），Run 随即返回。可在任意线程调用。
func Quit() {
	mu.Lock()
	w := curW
	mu.Unlock()
	if w != nil {
		w.Terminate() // 跨线程安全
	}
}

// setWindowIcon 把内嵌图标设为窗口图标（标题栏/任务栏）。
func setWindowIcon(w webview2.WebView) {
	defer func() { _ = recover() }()
	hIcon, err := icon.Load()
	if err != nil || hIcon == 0 {
		return
	}
	hwnd := w.Window()
	if hwnd == nil {
		return
	}
	w.Dispatch(func() {
		h := uintptr(hwnd)
		procSendMessageW.Call(h, wmSetIcon, iconBig, hIcon)
		procSendMessageW.Call(h, wmSetIcon, iconSmall, hIcon)
	})
}

// subclassForCloseTray 子类化窗口过程，拦截 WM_CLOSE 实现"关闭到托盘"。
func subclassForCloseTray(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	defer func() { _ = recover() }()
	orig, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlpWndproc)
	if orig == 0 {
		return
	}
	origWndProc = orig
	subclassCb = windows.NewCallback(closeInterceptProc)
	_, _, _ = procSetWindowLongPtrW.Call(hwnd, gwlpWndproc, subclassCb)
}

// closeInterceptProc 子类化后的窗口过程：closeToTray 开启时把关闭转为隐藏。
func closeInterceptProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmClose && atomic.LoadInt32(&closeToTrayAt) == 1 {
		// WM_CLOSE 是 Send 消息，当前即 UI 线程，直接隐藏
		_, _, _ = procShowWindow.Call(hwnd, swHide)
		return 0
	}
	r, _, _ := procCallWindowProcW.Call(origWndProc, hwnd, msg, wp, lp)
	return r
}
