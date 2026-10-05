package server

// 梦盒 Web 控制台：127.0.0.1:17890，提供单页 dashboard 与全部 JSON API。

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dreambox-win/internal/admin"
	"dreambox-win/internal/autostart"
	"dreambox-win/internal/linkparse"
	"dreambox-win/internal/manual"
	"dreambox-win/internal/mihomo"
	"dreambox-win/internal/panel"
	"dreambox-win/internal/paths"
	"dreambox-win/internal/stats"
	"dreambox-win/internal/sub"
	"dreambox-win/internal/sysproxy"
	"dreambox-win/internal/theme"
)

//go:embed web/index.html
var indexHTML []byte

// Version 程序版本
const Version = "v0.10.0"

// clipPending 待确认的剪贴板链接
type clipPending struct {
	Link   string `json:"link"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Server string `json:"server"`
	Port   int    `json:"port"`
}

// UpdateInfo 发现的新版本信息
type UpdateInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	Notes       string `json:"notes"`
}

// App 服务状态
type App struct {
	mu       sync.Mutex
	settings mihomo.Settings
	subs     []*sub.Subscription

	clipMu      sync.Mutex
	clipPending *clipPending

	updMu  sync.Mutex
	update *UpdateInfo // 检查更新发现的新版本（nil=无）

	// CloseToTrayHook 设置页切换"关闭到托盘"时的即时生效回调（main.go 设置）
	CloseToTrayHook func(bool)
}

// SetUpdateAvailable 记录检查更新发现的新版本
func (a *App) SetUpdateAvailable(u *UpdateInfo) {
	a.updMu.Lock()
	a.update = u
	a.updMu.Unlock()
}

// CloseToTrayEnabled 点 X 是否最小化到托盘（缺省 true）
func (a *App) CloseToTrayEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings.CloseToTrayEnabled()
}

// New 初始化服务状态
func New() *App {
	a := &App{}
	a.Reload()
	return a
}

// Reload 从磁盘重载设置与订阅
func (a *App) Reload() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings = mihomo.LoadSettings()
	if !theme.Exists(a.settings.Theme) {
		a.settings.Theme = "default"
		_ = mihomo.SaveSettings(a.settings)
	}
	subs, _ := sub.Load()
	a.subs = subs
}

// saveSettingsLocked 保存设置（调用者需持有锁）
func (a *App) saveSettingsLocked() error {
	return mihomo.SaveSettings(a.settings)
}

// AllProxies 订阅节点 + 手动节点合并（重名时手动优先），供生成配置用
func (a *App) AllProxies() []map[string]any {
	return manual.Merge(sub.FetchProxies())
}

// RebuildAndRestart 按当前订阅、手动节点与设置重新生成配置并重启内核
func (a *App) RebuildAndRestart() error {
	proxies := a.AllProxies()
	a.mu.Lock()
	st := a.settings
	a.mu.Unlock()

	if err := mihomo.WriteConfig(proxies, st); err != nil {
		return fmt.Errorf("写配置失败: %w", err)
	}
	if runtime.GOOS == "windows" {
		if err := mihomo.Restart(); err != nil {
			return fmt.Errorf("重启内核失败: %w", err)
		}
	}
	return nil
}

// ApplySwitches 按设置应用系统代理与开机自启
func (a *App) ApplySwitches() {
	a.mu.Lock()
	st := a.settings
	a.mu.Unlock()
	if st.SysProxy {
		_ = sysproxy.Enable()
	} else {
		_ = sysproxy.Disable()
	}
	if st.Autostart {
		_ = autostart.Enable(st.SilentStart)
	} else {
		_ = autostart.Disable()
	}
}

// Handler 返回全部路由
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/nodes", a.handleNodes)
	mux.HandleFunc("/api/node/select", a.handleNodeSelect)
	mux.HandleFunc("/api/nodes/delay", a.handleNodesDelay)
	mux.HandleFunc("/api/subs", a.handleSubs)
	mux.HandleFunc("/api/subs/update", a.handleSubUpdate)
	mux.HandleFunc("/api/mode", a.handleMode)
	mux.HandleFunc("/api/tun", a.handleTun)
	mux.HandleFunc("/api/sysproxy", a.handleSysProxy)
	mux.HandleFunc("/api/autostart", a.handleAutostart)
	mux.HandleFunc("/api/traffic", a.handleTraffic)
	mux.HandleFunc("/api/traffic/total", a.handleTrafficTotal)
	mux.HandleFunc("/api/connections", a.handleConnections)
	mux.HandleFunc("/api/outbound-ip", a.handleOutboundIP)
	mux.HandleFunc("/api/openui", a.handleOpenUI)
	mux.HandleFunc("/api/settings/reset", a.handleSettingsReset)
	mux.HandleFunc("/api/settings/closetotray", a.handleCloseToTray)
	mux.HandleFunc("/api/settings/silentstart", a.handleSilentStart)
	mux.HandleFunc("/api/settings/update", a.handleUpdateSettings)
	mux.HandleFunc("/api/settings/subua", a.handleSubUA)
	mux.HandleFunc("/api/update", a.handleUpdateInfo)
	mux.HandleFunc("/api/kernel/log", a.handleKernelLog)
	mux.HandleFunc("/api/kernel/status", a.handleKernelStatus)
	mux.HandleFunc("/api/kernel/restart", a.handleKernelRestart)
	mux.HandleFunc("/api/nodes/manual", a.handleManualNodes)     // GET 列表 / POST 解析预览
	mux.HandleFunc("/api/nodes/manual/add", a.handleManualAdd)   // POST 确认添加
	mux.HandleFunc("/api/nodes/manual/del", a.handleManualDel)   // DELETE?name= 删除
	mux.HandleFunc("/api/clipboard/pending", a.handleClipPending) // GET 待导入
	mux.HandleFunc("/api/clipboard/import", a.handleClipImport)   // POST 导入
	mux.HandleFunc("/api/clipboard/dismiss", a.handleClipDismiss) // POST 忽略
	mux.HandleFunc("/api/themes", a.handleThemes)                 // GET 主题列表
	mux.HandleFunc("/api/themes/upload", a.handleThemeUpload)     // POST multipart zip 安装
	mux.HandleFunc("/api/themes/enable", a.handleThemeEnable)     // POST {id} 启用
	mux.HandleFunc("/api/themes/", a.handleThemeDelete)           // DELETE /api/themes/<id>
	mux.HandleFunc("/api/panels", a.handlePanels)                 // GET 面板列表
	mux.HandleFunc("/api/panels/select", a.handlePanelSelect)     // POST {id} 切换面板
	mux.HandleFunc("/api/stats/today", a.handleStatsToday)        // GET 今日流量统计
	mux.HandleFunc("/api/stats/week", a.handleStatsWeek)          // GET 本周流量统计
	mux.HandleFunc("/api/stats/hosts", a.handleStatsHosts)        // GET 目标域名 Top
	// 主题静态文件（theme.css / 背景图），http.Dir 自带防目录穿越
	mux.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.Dir(paths.ThemesDir()))))
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
}

// GET /api/status
func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	st := a.settings
	subCount := len(a.subs)
	a.mu.Unlock()
	nodes, current, _ := mihomo.ListNodes()
	writeJSON(w, map[string]any{
		"ok": true, "version": Version,
		"admin":          admin.IsAdmin(),
		"mihomoRunning":  mihomo.Running(),
		"mihomoOnline":   mihomo.Online(),
		"tun":            st.Tun,
		"sysproxy":       sysproxy.IsEnabled(),
		"autostart":      autostart.IsEnabled(),
		"openUI":         st.OpenUIEnabled(),
		"closeToTray":    st.CloseToTrayEnabled(),
		"silentStart":    st.SilentStart,
		"checkUpdate":    st.CheckUpdate,
		"updateURL":      st.UpdateURL,
		"subUserAgent":   st.SubUA(),
		"mode":           mihomo.GetMode(),
		"theme":          st.Theme,
		"panel":          st.Panel,
		"panelName":      panelName(st.Panel),
		"panelURL":       fmt.Sprintf("http://127.0.0.1:%d/ui/", mihomo.CtrlPort),
		"nodeCount":      len(nodes),
		"currentNode":    current,
		"subCount":       subCount,
	})
}

// GET /api/nodes（节点来源：manual=手动添加，subscription=订阅）
func (a *App) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, current, err := mihomo.ListNodes()
	if err != nil {
		writeErr(w, err)
		return
	}
	manualNames := manual.Names()
	for i := range nodes {
		if manualNames[nodes[i].Name] {
			nodes[i].Source = "manual"
		} else {
			nodes[i].Source = "subscription"
		}
	}
	writeJSON(w, map[string]any{"ok": true, "current": current, "nodes": nodes})
}

// POST /api/node/select {"name": "..."}
func (a *App) handleNodeSelect(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v.Name == "" {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	if err := mihomo.SelectNode(v.Name); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/nodes/delay 全节点延迟测试（并发 6，单个超时 5s）
func (a *App) handleNodesDelay(w http.ResponseWriter, r *http.Request) {
	nodes, _, err := mihomo.ListNodes()
	if err != nil {
		writeErr(w, err)
		return
	}
	results := map[string]int{}
	var rmu sync.Mutex
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			d := mihomo.Delay(name, 5000)
			<-sem
			rmu.Lock()
			results[name] = d
			rmu.Unlock()
		}(n.Name)
	}
	wg.Wait()
	writeJSON(w, map[string]any{"ok": true, "results": results})
}

// /api/subs: GET 列表 / POST 新增 / DELETE 删除?id=
func (a *App) handleSubs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		subs, _ := sub.Load()
		if subs == nil {
			subs = []*sub.Subscription{}
		}
		writeJSON(w, map[string]any{"ok": true, "subs": subs})
	case http.MethodPost:
		var v struct {
			URL    string `json:"url"`
			Remark string `json:"remark"`
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v.URL == "" {
			writeErr(w, fmt.Errorf("订阅链接不能为空"))
			return
		}
		s, err := sub.AddWithUA(v.URL, v.Remark, a.subUA())
		if err != nil {
			writeErr(w, err)
			return
		}
		a.Reload()
		if err := a.RebuildAndRestart(); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "sub": s})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeErr(w, fmt.Errorf("缺少 id"))
			return
		}
		if err := sub.Delete(id); err != nil {
			writeErr(w, err)
			return
		}
		a.Reload()
		if err := a.RebuildAndRestart(); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// POST /api/subs/update?id= 手动更新
func (a *App) handleSubUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeErr(w, fmt.Errorf("缺少 id"))
		return
	}
	n, err := sub.UpdateWithUA(id, a.subUA())
	if err != nil {
		writeErr(w, err)
		return
	}
	a.Reload()
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": n})
}

// POST /api/mode {"mode":"rule|global|direct"}
func (a *App) handleMode(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	if v.Mode != "rule" && v.Mode != "global" && v.Mode != "direct" {
		writeErr(w, fmt.Errorf("未知模式"))
		return
	}
	// 先尝试热切换，失败则重建配置
	if err := mihomo.SetMode(v.Mode); err != nil {
		a.mu.Lock()
		a.settings.Mode = v.Mode
		_ = a.saveSettingsLocked()
		a.mu.Unlock()
		if err := a.RebuildAndRestart(); err != nil {
			writeErr(w, err)
			return
		}
	} else {
		a.mu.Lock()
		a.settings.Mode = v.Mode
		_ = a.saveSettingsLocked()
		a.mu.Unlock()
	}
	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/tun {"enable":true}
func (a *App) handleTun(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	a.mu.Lock()
	a.settings.Tun = v.Enable
	_ = a.saveSettingsLocked()
	a.mu.Unlock()
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/sysproxy {"enable":true}
func (a *App) handleSysProxy(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	var err error
	if v.Enable {
		err = sysproxy.Enable()
	} else {
		err = sysproxy.Disable()
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.mu.Lock()
	a.settings.SysProxy = v.Enable
	_ = a.saveSettingsLocked()
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// POST /api/autostart {"enable":true}
func (a *App) handleAutostart(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	var err error
	if v.Enable {
		a.mu.Lock()
		silent := a.settings.SilentStart
		a.mu.Unlock()
		err = autostart.Enable(silent)
	} else {
		err = autostart.Disable()
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.mu.Lock()
	a.settings.Autostart = v.Enable
	_ = a.saveSettingsLocked()
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// GET /api/traffic
func (a *App) handleTraffic(w http.ResponseWriter, r *http.Request) {
	up, down, at := mihomo.TrafficSnapshot()
	writeJSON(w, map[string]any{
		"ok": true, "up": up, "down": down,
		"at": at.Unix(),
	})
}

// GET /api/kernel/log 返回 mihomo.log 最后 200 行
func (a *App) handleKernelLog(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(paths.MihomoLog())
	if err != nil {
		writeJSON(w, map[string]any{"ok": true, "lines": []string{}})
		return
	}
	lines := strings.Split(string(data), "\n")
	// 去掉末尾空行，避免多一行空白
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	writeJSON(w, map[string]any{"ok": true, "lines": lines})
}

// GET /api/kernel/status 内核状态：是否在线、最后一次退出信息、
// 是否管理员身份、TUN 开关状态（供 Web 端横幅提示用；
// /api/status 已有 mihomoOnline/admin/tun 等字段，这里是排障专用精简视图）
func (a *App) handleKernelStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	tun := a.settings.Tun
	a.mu.Unlock()
	lt, lerr := mihomo.LastExit()
	v := map[string]any{
		"ok": true,
		"online":      mihomo.Online(),
		"is_admin":    admin.IsAdmin(),
		"tun_enabled": tun,
	}
	if lt.IsZero() {
		v["last_exit"] = nil
		v["last_exit_error"] = ""
	} else {
		v["last_exit"] = lt.Unix()
		if lerr != nil {
			v["last_exit_error"] = lerr.Error()
		} else {
			v["last_exit_error"] = "进程已退出（无错误信息）"
		}
	}
	writeJSON(w, v)
}

// GET /api/connections 活跃连接数（调内核 external-controller）
func (a *App) handleConnections(w http.ResponseWriter, r *http.Request) {
	n, err := mihomo.ConnectionCount()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "内核未运行"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": n})
}

// GET /api/outbound-ip 经代理检测出口 IP（超时 8s）
func (a *App) handleOutboundIP(w http.ResponseWriter, r *http.Request) {
	if !mihomo.Online() {
		writeJSON(w, map[string]any{"ok": false, "error": "内核未运行"})
		return
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", mihomo.MixedPort))
	c := &http.Client{
		Timeout:   8 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := c.Get("https://api.ip.sb/ip")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "检测失败"})
		return
	}
	defer resp.Body.Close()
	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	ip := strings.TrimSpace(string(body[:n]))
	if ip == "" || resp.StatusCode != 200 {
		writeJSON(w, map[string]any{"ok": false, "error": "检测失败"})
		return
	}
	out := map[string]any{"ok": true, "ip": ip}
	// 归属地/ISP：经同一代理查 ip-api，失败则只返回 ip，不报错
	func() {
		gc := &http.Client{
			Timeout:   8 * time.Second,
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		}
		gresp, err := gc.Get("http://ip-api.com/json/?fields=status,country,org,query")
		if err != nil {
			return
		}
		defer gresp.Body.Close()
		var g struct {
			Status  string `json:"status"`
			Country string `json:"country"`
			Org     string `json:"org"`
			Query   string `json:"query"`
		}
		if err := json.NewDecoder(gresp.Body).Decode(&g); err != nil {
			return
		}
		if g.Status == "success" {
			if g.Country != "" {
				out["country"] = g.Country
			}
			if g.Org != "" {
				out["org"] = g.Org
			}
		}
	}()
	writeJSON(w, out)
}

// /api/openui: GET 返回当前值；POST {"enable":bool} 设置启动是否自动打开控制台
func (a *App) handleOpenUI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var v struct {
			Enable bool `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			writeErr(w, fmt.Errorf("参数错误"))
			return
		}
		a.mu.Lock()
		a.settings.OpenUI = &v.Enable
		_ = a.saveSettingsLocked()
		a.mu.Unlock()
	}
	a.mu.Lock()
	enabled := a.settings.OpenUIEnabled()
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "enable": enabled})
}

// POST /api/settings/reset 恢复默认设置（保留订阅），重建配置并重启内核
func (a *App) handleSettingsReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.mu.Lock()
	a.settings = mihomo.DefaultSettings()
	_ = a.saveSettingsLocked()
	a.mu.Unlock()
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	a.ApplySwitches()
	writeJSON(w, map[string]any{"ok": true})
}

// GET /api/traffic/total 自启动以来的累计上传/下载总字节数
func (a *App) handleTrafficTotal(w http.ResponseWriter, r *http.Request) {
	up, down := mihomo.TrafficTotal()
	writeJSON(w, map[string]any{"ok": true, "up": up, "down": down})
}

// GET /api/stats/today 今日流量统计（小时粒度）
func (a *App) handleStatsToday(w http.ResponseWriter, r *http.Request) {
	rec := stats.Recorder()
	if rec == nil {
		writeJSON(w, map[string]any{"ok": true, "date": "", "total": map[string]any{"up": 0, "down": 0}, "hours": []any{}})
		return
	}
	t := rec.Today()
	writeJSON(w, map[string]any{"ok": true, "date": t.Date,
		"total": map[string]any{"up": t.Up, "down": t.Down}, "hours": t.Hours})
}

// GET /api/stats/week 本周流量统计（按日）
func (a *App) handleStatsWeek(w http.ResponseWriter, r *http.Request) {
	rec := stats.Recorder()
	if rec == nil {
		writeJSON(w, map[string]any{"ok": true, "days": []any{}})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "days": rec.Week()})
}

// GET /api/stats/hosts?days=1 目标域名 Top（按总量倒序，最多 20）
func (a *App) handleStatsHosts(w http.ResponseWriter, r *http.Request) {
	rec := stats.Recorder()
	if rec == nil {
		writeJSON(w, map[string]any{"ok": true, "hosts": []any{}})
		return
	}
	days := 1
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	writeJSON(w, map[string]any{"ok": true, "hosts": rec.TopHosts(days)})
}

// POST /api/kernel/restart 用当前配置重启内核
func (a *App) handleKernelRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if runtime.GOOS != "windows" {
		writeErr(w, fmt.Errorf("仅 Windows 支持"))
		return
	}
	if err := mihomo.Restart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// subUA 当前设置的订阅 UA
func (a *App) subUA() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings.SubUA()
}

// /api/settings/closetotray: GET 返回当前值；POST {"enable":bool} 设置
func (a *App) handleCloseToTray(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var v struct {
			Enable bool `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			writeErr(w, fmt.Errorf("参数错误"))
			return
		}
		a.mu.Lock()
		a.settings.CloseToTray = &v.Enable
		_ = a.saveSettingsLocked()
		hook := a.CloseToTrayHook
		a.mu.Unlock()
		if hook != nil {
			hook(v.Enable)
		}
	}
	a.mu.Lock()
	enabled := a.settings.CloseToTrayEnabled()
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "enable": enabled})
}

// /api/settings/silentstart: GET 返回当前值；POST {"enable":bool} 设置
func (a *App) handleSilentStart(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var v struct {
			Enable bool `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			writeErr(w, fmt.Errorf("参数错误"))
			return
		}
		a.mu.Lock()
		a.settings.SilentStart = v.Enable
		_ = a.saveSettingsLocked()
		// 若已开启自启，同步刷新注册表项
		if a.settings.Autostart {
			_ = autostart.Enable(v.Enable)
		}
		a.mu.Unlock()
	}
	a.mu.Lock()
	enabled := a.settings.SilentStart
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "enable": enabled})
}

// /api/settings/update: GET 返回当前值；POST {"checkUpdate":bool,"updateURL":string} 设置
func (a *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// 兼容只传其中一个字段
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeErr(w, fmt.Errorf("参数错误"))
			return
		}
		a.mu.Lock()
		if b, ok := raw["checkUpdate"]; ok {
			var cu bool
			if err := json.Unmarshal(b, &cu); err == nil {
				a.settings.CheckUpdate = cu
			}
		}
		if u, ok := raw["updateURL"]; ok {
			var uu string
			if err := json.Unmarshal(u, &uu); err == nil {
				a.settings.UpdateURL = strings.TrimSpace(uu)
			}
		}
		_ = a.saveSettingsLocked()
		a.mu.Unlock()
	}
	a.mu.Lock()
	cu, uu := a.settings.CheckUpdate, a.settings.UpdateURL
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "checkUpdate": cu, "updateURL": uu})
}

// /api/settings/subua: GET 返回当前值；POST {"ua":string} 设置
func (a *App) handleSubUA(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var v struct {
			UA string `json:"ua"`
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			writeErr(w, fmt.Errorf("参数错误"))
			return
		}
		a.mu.Lock()
		a.settings.SubUserAgent = strings.TrimSpace(v.UA)
		if a.settings.SubUserAgent == "" {
			a.settings.SubUserAgent = mihomo.DefaultSubUserAgent
		}
		_ = a.saveSettingsLocked()
		a.mu.Unlock()
	}
	writeJSON(w, map[string]any{"ok": true, "ua": a.subUA()})
}

// GET /api/update 检查更新发现的新版本（无则 available=false）
func (a *App) handleUpdateInfo(w http.ResponseWriter, r *http.Request) {
	a.updMu.Lock()
	u := a.update
	a.updMu.Unlock()
	if u == nil {
		writeJSON(w, map[string]any{"ok": true, "available": false})
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "available": true,
		"version": u.Version, "download_url": u.DownloadURL, "notes": u.Notes,
	})
}

// IsNewerVersion 判断 latest 是否比 cur 更新（"v0.7.0" 风格版本号）
func IsNewerVersion(cur, latest string) bool {
	parse := func(s string) []int {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "v")
		s = strings.TrimPrefix(s, "V")
		parts := strings.Split(s, ".")
		out := make([]int, 0, len(parts))
		for _, p := range parts {
			n := 0
			_, _ = fmt.Sscanf(strings.TrimSpace(p), "%d", &n)
			out = append(out, n)
		}
		return out
	}
	a, b := parse(cur), parse(latest)
	for i := 0; i < len(a) && i < len(b); i++ {
		if b[i] > a[i] {
			return true
		}
		if b[i] < a[i] {
			return false
		}
	}
	return len(b) > len(a)
}

// ToggleTun 切换 TUN 开关并返回新状态（供托盘菜单用）
func (a *App) ToggleTun() (bool, error) {
	a.mu.Lock()
	a.settings.Tun = !a.settings.Tun
	enable := a.settings.Tun
	_ = a.saveSettingsLocked()
	a.mu.Unlock()
	if err := a.RebuildAndRestart(); err != nil {
		return enable, err
	}
	return enable, nil
}

// TunEnabled 当前 TUN 开关状态
func (a *App) TunEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings.Tun
}

// OpenUIEnabled 启动是否自动打开控制台
func (a *App) OpenUIEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings.OpenUIEnabled()
}

// /api/nodes/manual: GET 手动节点列表；POST {"link"} 解析预览
func (a *App) handleManualNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var v struct {
			Link string `json:"link"`
		}
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil || strings.TrimSpace(v.Link) == "" {
			writeErr(w, fmt.Errorf("请粘贴代理链接"))
			return
		}
		res, err := linkparse.Parse(v.Link)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]any{
			"ok": true, "name": res.Name, "type": res.Type,
			"server": res.Server, "port": res.Port,
		})
		return
	}
	nodes, _ := manual.Load()
	if nodes == nil {
		nodes = []manual.ManualNode{}
	}
	writeJSON(w, map[string]any{"ok": true, "nodes": nodes})
}

// POST /api/nodes/manual/add {"link"} 确认添加手动节点
func (a *App) handleManualAdd(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil || strings.TrimSpace(v.Link) == "" {
		writeErr(w, fmt.Errorf("请粘贴代理链接"))
		return
	}
	n, err := manual.Add(v.Link)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": n.Name})
}

// DELETE /api/nodes/manual/del?name= 删除手动节点
func (a *App) handleManualDel(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeErr(w, fmt.Errorf("缺少 name"))
		return
	}
	if err := manual.Delete(name); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// OnClipboardLink 剪贴板发现新代理链接：预解析成功则记为 pending 等待用户确认
func (a *App) OnClipboardLink(link string) {
	res, err := linkparse.Parse(link)
	if err != nil {
		return // 解析失败不打扰用户
	}
	trimmed := strings.TrimSpace(link)
	a.clipMu.Lock()
	defer a.clipMu.Unlock()
	if a.clipPending != nil && a.clipPending.Link == trimmed {
		return
	}
	a.clipPending = &clipPending{
		Link: trimmed, Name: res.Name, Type: res.Type,
		Server: res.Server, Port: res.Port,
	}
}

// GET /api/clipboard/pending 待导入的剪贴板链接（无则为 null）
func (a *App) handleClipPending(w http.ResponseWriter, r *http.Request) {
	a.clipMu.Lock()
	p := a.clipPending
	a.clipMu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "pending": p})
}

// POST /api/clipboard/import 把 pending 导入为手动节点
func (a *App) handleClipImport(w http.ResponseWriter, r *http.Request) {
	a.clipMu.Lock()
	p := a.clipPending
	a.clipPending = nil
	a.clipMu.Unlock()
	if p == nil {
		writeErr(w, fmt.Errorf("没有待导入的链接"))
		return
	}
	n, err := manual.Add(p.Link)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": n.Name})
}

// POST /api/clipboard/dismiss 忽略待导入的剪贴板链接
func (a *App) handleClipDismiss(w http.ResponseWriter, r *http.Request) {
	a.clipMu.Lock()
	a.clipPending = nil
	a.clipMu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// GET /api/themes 主题列表
func (a *App) handleThemes(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	active := a.settings.Theme
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "themes": theme.List(active)})
}

// POST /api/themes/upload multipart zip 安装主题（表单字段 file）
func (a *App) handleThemeUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeErr(w, fmt.Errorf("解析上传失败"))
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, fmt.Errorf("请选择 zip 文件"))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 20<<20+1))
	if err != nil {
		writeErr(w, fmt.Errorf("读取文件失败"))
		return
	}
	th, err := theme.Upload(data)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "theme": th})
}

// POST /api/themes/enable {"id":"..."} 启用主题
func (a *App) handleThemeEnable(w http.ResponseWriter, r *http.Request) {
	var v struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v.ID == "" {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	if !theme.Exists(v.ID) {
		writeErr(w, fmt.Errorf("主题不存在: %s", v.ID))
		return
	}
	a.mu.Lock()
	a.settings.Theme = v.ID
	err := a.saveSettingsLocked()
	a.mu.Unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// DELETE /api/themes/<id> 删除主题（内置不可删；删的是当前主题则回退 default）
func (a *App) handleThemeDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/themes/")
	if err := theme.Delete(id); err != nil {
		writeErr(w, err)
		return
	}
	a.mu.Lock()
	if a.settings.Theme == id {
		a.settings.Theme = "default"
		_ = a.saveSettingsLocked()
	}
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// panelName 当前面板的显示名（非法 id 回退默认）
func panelName(id string) string {
	if p, err := panel.PanelByID(id); err == nil {
		return p.Name
	}
	if p, err := panel.PanelByID(panel.DefaultID); err == nil {
		return p.Name
	}
	return "面板"
}

// GET /api/panels 面板列表（含版本与当前选中）
func (a *App) handlePanels(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	cur := a.settings.Panel
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "panels": panel.Panels(), "active": cur})
}

// POST /api/panels/select {"id":"yacd"} 切换面板：释放面板文件 + 重写配置 + 重启内核
func (a *App) handlePanelSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var v struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v.ID == "" {
		writeErr(w, fmt.Errorf("参数错误"))
		return
	}
	p, err := panel.PanelByID(v.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := panel.EnsurePanel(p.ID); err != nil {
		writeErr(w, err)
		return
	}
	a.mu.Lock()
	a.settings.Panel = p.ID
	err = a.saveSettingsLocked()
	a.mu.Unlock()
	if err != nil {
		writeErr(w, err)
		return
	}
	// external-ui 路径不变（仍指向 panel 目录），重启内核使新面板生效
	if err := a.RebuildAndRestart(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "panel": p.ID, "panelName": p.Name})
}

// Start 启动 Web 服务
func Start(addr string, h http.Handler) error {
	return http.ListenAndServe(addr, h)
}
