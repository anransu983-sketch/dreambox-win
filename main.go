// 梦盒 Windows 版 v0.10.0
// 桌面壳 + mihomo 内核：TUN 全局代理、订阅管理、WebView2 原生应用窗口、系统托盘。
//
// 用法：
//  1. 右键「以管理员身份运行」（TUN 必需）
//  2. 直接弹出原生「梦盒」窗口（内嵌仪表盘），托盘常驻
//  3. 点 X 默认最小化到托盘（设置可改）；退出走托盘右键菜单
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dreambox-win/internal/admin"
	"dreambox-win/internal/clipboard"
	"dreambox-win/internal/mihomo"
	"dreambox-win/internal/panel"
	"dreambox-win/internal/paths"
	"dreambox-win/internal/server"
	"dreambox-win/internal/theme"
	"dreambox-win/internal/tray"
	"dreambox-win/internal/winapp"
)

func main() {
	minimized := flag.Bool("minimized", false, "最小化启动（开机自启用，不自动打开浏览器）")
	console := flag.Bool("console", false, "保留参数：排障用控制台构建请去掉 -H=windowsgui 重新编译")
	flag.Parse()
	_ = console

	fmt.Println("=================================")
	fmt.Println("  梦盒 Windows 版", server.Version)
	fmt.Println("=================================")

	// 1. 数据目录
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		fmt.Println("创建数据目录失败:", err)
		os.Exit(1)
	}
	fmt.Println("数据目录:", paths.DataDir())

	// 2. 释放内核
	if err := mihomo.EnsureBins(); err != nil {
		fmt.Println("释放内核失败:", err)
		os.Exit(1)
	}
	fmt.Println("内核就绪")

	// 2.5 释放内置主题
	if err := theme.EnsureBuiltin(); err != nil {
		fmt.Println("释放内置主题失败:", err)
	}
	fmt.Println("主题就绪")

// 2.6 释放专业面板（三选一，默认 metacubexd）
	st := mihomo.LoadSettings()
	if err := panel.EnsurePanel(st.Panel); err != nil {
		fmt.Println("释放面板失败:", err)
	}
	fmt.Println("面板就绪:", st.Panel)

	// 3. 加载状态，生成配置（订阅节点 + 手动节点）
	app := server.New()
	proxies := app.AllProxies()
	fmt.Printf("已加载 %d 个节点（订阅+手动）\n", len(proxies))
	if err := mihomo.WriteConfig(proxies, st); err != nil {
		fmt.Println("生成配置失败:", err)
		os.Exit(1)
	}

	if runtime.GOOS == "windows" {
		runWindowsGUI(app, minimized)
		return
	}

	// 非 Windows：保持原有控制台 + Web 逻辑（开发机调试用）
	mihomo.StartTrafficSampler()
	mihomo.StartHistory() // 流量历史：小时桶落盘 + 域名聚合
	addr := fmt.Sprintf("127.0.0.1:%d", mihomo.WebUIPort)
	fmt.Printf("控制台: http://%s\n", addr)
	fmt.Println("按 Ctrl+C 退出")
	go func() {
		if err := server.Start(addr, app.Handler()); err != nil {
			fmt.Println("Web 服务异常:", err)
			os.Exit(1)
		}
	}()
	go checkUpdate(app)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\n正在退出…")
	_ = mihomo.Stop()
}

// setupAppLog 把自身日志同时写入 %APPDATA%\DreamBox\app.log
func setupAppLog() {
	f, err := os.OpenFile(paths.AppLog(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// runWindowsGUI：Windows 下的启动流程（v0.4.0 起主界面为 WebView2 原生窗口，v0.7.0 起托盘常驻）
func runWindowsGUI(app *server.App, minimized *bool) {
	setupAppLog()
	log.Printf("梦盒 %s 启动", server.Version)

	// 管理员检查（TUN 必需）
	if !admin.IsAdmin() {
		log.Println("警告: 当前不是管理员身份，TUN 全局模式将无法工作")
	}

	// 启动内核
	if err := mihomo.Start(); err != nil {
		log.Println("内核启动失败:", err)
		admin.Warn("梦盒", "内核启动失败：\n"+err.Error())
		os.Exit(1)
	}
	log.Println("内核已启动")

	// 等内核 API 就绪后应用开关
	go func() {
		for i := 0; i < 30; i++ {
			if mihomo.Online() {
				break
			}
			time.Sleep(time.Second)
		}
		app.ApplySwitches()
	}()

	mihomo.StartTrafficSampler()
	mihomo.StartHistory() // 流量历史：小时桶落盘 + 域名聚合

	// 剪贴板代理链接嗅探（Windows 真实现，其他平台空实现）
	clipboard.StartWatcher(app.OnClipboardLink)

	// Web 服务：原生窗口的数据源（仅监听本机 127.0.0.1）
	addr := fmt.Sprintf("127.0.0.1:%d", mihomo.WebUIPort)
	go func() {
		log.Printf("Web 服务: http://%s", addr)
		if err := server.Start(addr, app.Handler()); err != nil {
			log.Println("Web 服务异常:", err)
		}
	}()

	// 启动后检查更新（设置开启且配了地址才查）
	go checkUpdate(app)

	quitOnce := &sync.Once{}
	quitCh := make(chan struct{})
	requestQuit := func() {
		quitOnce.Do(func() { close(quitCh) })
	}

	// 系统托盘（独立线程；失败则无托盘继续跑，关闭行为强制为退出）
	trayOK := false
	if err := tray.Start(tray.Callbacks{
		OnShow: func() {
			showMainWindow(app, addr, &trayOK)
		},
		OnToggleTun: func() {
			if on, err := app.ToggleTun(); err != nil {
				log.Println("托盘切换 TUN 失败:", err)
			} else {
				log.Printf("托盘切换 TUN: %v", on)
			}
		},
		OnRestart: func() {
			if err := mihomo.Restart(); err != nil {
				log.Println("托盘重启内核失败:", err)
				admin.Warn("梦盒", "重启内核失败：\n"+err.Error())
			} else {
				log.Println("托盘触发内核重启")
			}
		},
		OnQuit: func() {
			log.Println("托盘请求退出")
			requestQuit()
			winapp.Quit()
		},
		TunOn: func() bool { return app.TunEnabled() },
	}); err != nil {
		log.Println("托盘初始化失败，将无托盘运行:", err)
	} else {
		trayOK = true
		defer tray.Stop()
		tray.SetTooltip("梦盒 - 运行中")
	}

	// 设置页"关闭到托盘"开关即时生效；托盘不可用时强制按退出处理
	effectiveCloseToTray := func() bool { return app.CloseToTrayEnabled() && trayOK }
	app.CloseToTrayHook = func(b bool) {
		winapp.SetCloseToTray(b && trayOK)
	}
	winapp.SetCloseToTray(effectiveCloseToTray())

	if *minimized {
		// 开机自启：不自动建窗口，仅后台运行内核 + Web 服务 + 托盘
		log.Println("--minimized：不创建窗口，后台运行（托盘可打开主界面）")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		select {
		case <-quitCh:
		case <-sig:
		}
	} else {
		// 等 Web 服务就绪再开窗，避免首屏白屏；主线程跑 WebView2 消息循环
		waitForServer(addr, 15*time.Second)
		markWindowStarted()
		url := fmt.Sprintf("http://127.0.0.1:%d/", mihomo.WebUIPort)
		winapp.SetDashboardURL(url) // 面板页"返回梦盒"按钮的目标
		if err := winapp.Run("梦盒", url, paths.WebViewDataDir(), 1280, 860, effectiveCloseToTray()); err != nil {
			// WebView2 不可用：弹窗说明，回退用浏览器打开控制台
			log.Println("原生窗口不可用:", err)
			admin.Warn("梦盒", "无法创建原生窗口（可能未安装 WebView2 Runtime），将用浏览器打开控制台。")
			openBrowser(url)
			waitForExit(quitCh)
		}
		// 窗口关闭（非托盘隐藏）或托盘退出 → Run 返回
	}

	log.Println("正在退出…")
	_ = mihomo.Stop()
}

// windowStarted 标记主窗口是否已创建过（--minimized 下托盘首次"显示主界面"时创建）
var windowStarted atomic.Bool

func markWindowStarted() { windowStarted.Store(true) }

// showMainWindow 托盘"显示主界面"/左键单击：窗口已存在则显示，否则创建。
// 非主线程调用时（如托盘线程），窗口跑在独立锁定的 OS 线程上。
func showMainWindow(app *server.App, addr string, trayOK *bool) {
	if windowStarted.Load() {
		winapp.Show()
		return
	}
	if !windowStarted.CompareAndSwap(false, true) {
		winapp.Show()
		return
	}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		waitForServer(addr, 15*time.Second)
		url := fmt.Sprintf("http://127.0.0.1:%d/", mihomo.WebUIPort)
		winapp.SetDashboardURL(url) // 面板页"返回梦盒"按钮的目标
		log.Println("托盘打开主界面")
		if err := winapp.Run("梦盒", url, paths.WebViewDataDir(), 1280, 860, app.CloseToTrayEnabled() && *trayOK); err != nil {
			log.Println("原生窗口不可用:", err)
			admin.Warn("梦盒", "无法创建原生窗口（可能未安装 WebView2 Runtime），将用浏览器打开控制台。")
			openBrowser(url)
			windowStarted.Store(false) // 允许下次重试
		}
		// 窗口真正关闭（非隐藏到托盘）后重置标记，下次可重建
		windowStarted.Store(false)
	}()
}

// checkUpdate 启动后检查更新：设置开启且配了 version.json 地址才查，
// 有新版本则记到 App，前端首页横幅提示。
func checkUpdate(app *server.App) {
	st := mihomo.LoadSettings()
	u := strings.TrimSpace(st.UpdateURL)
	if !st.CheckUpdate || u == "" {
		return
	}
	time.Sleep(10 * time.Second) // 等网络就绪
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(u)
	if err != nil {
		log.Println("检查更新失败:", err)
		return
	}
	defer resp.Body.Close()
	var v struct {
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		Notes       string `json:"notes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		log.Println("更新信息解析失败:", err)
		return
	}
	if v.Version == "" || !server.IsNewerVersion(server.Version, v.Version) {
		log.Println("当前已是最新版本")
		return
	}
	log.Printf("发现新版本 %s", v.Version)
	app.SetUpdateAvailable(&server.UpdateInfo{
		Version:     v.Version,
		DownloadURL: v.DownloadURL,
		Notes:       v.Notes,
	})
}

// waitForServer 等待 TCP 端口可连接（WebView2 导航前避免白屏）
func waitForServer(addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// waitForExit 等待退出信号（Ctrl+C）或托盘退出请求
func waitForExit(quitCh <-chan struct{}) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-quitCh:
	}
}

// openBrowser 用默认浏览器打开 URL（仅 Windows 下调用）
func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
