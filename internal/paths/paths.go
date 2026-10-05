package paths

// 数据目录与常用路径。Windows 下使用 %APPDATA%/DreamBox；
// Linux 下使用 ~/.dreambox（仅用于开发机调试 Web UI）。

import (
	"os"
	"path/filepath"
)

// DataDir 返回数据根目录
func DataDir() string {
	if dir := os.Getenv("APPDATA"); dir != "" {
		return filepath.Join(dir, "DreamBox")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dreambox")
}

// BinDir 内核二进制目录
func BinDir() string { return filepath.Join(DataDir(), "bin") }

// MihomoExe 内核主程序路径
func MihomoExe() string { return filepath.Join(BinDir(), "mihomo.exe") }

// WintunDll wintun 驱动路径（TUN system 栈需要）
func WintunDll() string { return filepath.Join(BinDir(), "wintun.dll") }

// ConfigFile 生成的 mihomo 配置文件
func ConfigFile() string { return filepath.Join(DataDir(), "config.yaml") }

// MihomoLog 内核 stdout/stderr 日志文件（每次启动截断）
func MihomoLog() string { return filepath.Join(DataDir(), "mihomo.log") }

// AppLog 桌面壳自身日志
func AppLog() string { return filepath.Join(DataDir(), "app.log") }

// SubsFile 订阅存档
func SubsFile() string { return filepath.Join(DataDir(), "subs.json") }

// ManualFile 手动添加的节点存档
func ManualFile() string { return filepath.Join(DataDir(), "manual.json") }

// SettingsFile 用户设置存档
func SettingsFile() string { return filepath.Join(DataDir(), "settings.json") }

// WebViewDataDir WebView2 用户数据目录（缓存/Cookie），与 exe 同名默认目录区分开
func WebViewDataDir() string { return filepath.Join(DataDir(), "webview2") }

// ThemesDir 主题根目录
func ThemesDir() string { return filepath.Join(DataDir(), "themes") }

// ThemeDir 单个主题目录（id 需先经 theme.ValidID 校验）
func ThemeDir(id string) string { return filepath.Join(ThemesDir(), id) }

// PanelDir 专业面板（三选一：metacubexd/yacd/zashboard）释放目录，mihomo external-ui 指向这里
func PanelDir() string { return filepath.Join(DataDir(), "panel") }

// HistoryFile 流量历史落盘文件（小时粒度，保留 90 天）
func HistoryFile() string { return filepath.Join(DataDir(), "traffic_history.json") }
