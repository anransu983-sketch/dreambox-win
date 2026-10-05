package mihomo

// 用户设置与 mihomo config.yaml 生成。

import (
	"encoding/json"
	"fmt"
	"os"

	"dreambox-win/internal/panel"
	"dreambox-win/internal/paths"

	"github.com/goccy/go-yaml"
)

// Settings 用户设置，存 settings.json
type Settings struct {
	Tun          bool   `json:"tun"`          // TUN 全局开关
	SysProxy     bool   `json:"sysproxy"`     // 系统代理开关
	Autostart    bool   `json:"autostart"`    // 开机自启
	Mode         string `json:"mode"`         // rule / global / direct
	OpenUI       *bool  `json:"openUI"`       // 启动自动打开控制台；nil 视为 true（兼容老配置）
	Theme        string `json:"theme"`        // 当前主题 id，默认 default
	CloseToTray  *bool  `json:"closeToTray"`  // 点 X 最小化到托盘；nil 视为 true（兼容老配置）
	SilentStart  bool   `json:"silentStart"`  // 开机自启时不显示窗口
	SubUserAgent string `json:"subUserAgent"` // 订阅下载 UA，默认 Chrome
	CheckUpdate  bool   `json:"checkUpdate"`  // 启动时自动检查更新
	UpdateURL    string `json:"updateURL"`    // version.json 地址，空=未配置
	Panel        string `json:"panel"`        // 专业面板 id：metacubexd / yacd / zashboard，默认 metacubexd
}

func boolPtr(b bool) *bool { return &b }

// DefaultSettings 默认设置：TUN 开、规则模式、启动自动打开控制台、默认深色主题、
// 关闭到托盘开、订阅 UA 为 Chrome
func DefaultSettings() Settings {
	return Settings{
		Tun: true, SysProxy: false, Autostart: false, Mode: "rule",
		OpenUI: boolPtr(true), Theme: "default",
		CloseToTray: boolPtr(true), SilentStart: false,
		SubUserAgent: DefaultSubUserAgent, CheckUpdate: false, UpdateURL: "",
		Panel: panel.DefaultID,
	}
}

// DefaultSubUserAgent 订阅下载默认 UA（Chrome）
const DefaultSubUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// OpenUIEnabled 启动是否自动打开控制台（缺省 true）
func (s Settings) OpenUIEnabled() bool {
	return s.OpenUI == nil || *s.OpenUI
}

// CloseToTrayEnabled 点 X 是否最小化到托盘（缺省 true）
func (s Settings) CloseToTrayEnabled() bool {
	return s.CloseToTray == nil || *s.CloseToTray
}

// SubUA 订阅下载 UA（空则回退默认）
func (s Settings) SubUA() string {
	if s.SubUserAgent == "" {
		return DefaultSubUserAgent
	}
	return s.SubUserAgent
}

// LoadSettings 读取设置，不存在则返回默认
func LoadSettings() Settings {
	st := DefaultSettings()
	data, err := os.ReadFile(paths.SettingsFile())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	if st.Mode != "rule" && st.Mode != "global" && st.Mode != "direct" {
		st.Mode = "rule"
	}
	if st.Theme == "" {
		st.Theme = "default"
	}
	if st.SubUserAgent == "" {
		st.SubUserAgent = DefaultSubUserAgent
	}
	if !panel.ValidID(st.Panel) {
		st.Panel = panel.DefaultID
	}
	return st
}

// SaveSettings 保存设置
func SaveSettings(st Settings) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.SettingsFile(), data, 0o644)
}

// 端口约定
const (
	MixedPort   = 7890 // 混合代理端口
	CtrlPort    = 9090 // external-controller 端口
	WebUIPort   = 17890 // 梦盒 Web 控制台端口
	SelectGroup = "梦盒"  // 主选择组名
	AutoGroup   = "自动选择" // url-test 组名
)

// BuildConfig 根据订阅节点与设置生成 config.yaml 内容。
// 零订阅（proxies 为空）时不生成 proxy-groups，rules 退化为 MATCH,DIRECT，
// 保证内核能正常启动；Web 端提示用户添加订阅。
func BuildConfig(proxies []map[string]any, st Settings) ([]byte, error) {
	names := dedupeNames(proxies)

	cfg := map[string]any{
		"mixed-port":          MixedPort,
		"allow-lan":           false,
		"ipv6":                false,
		"mode":                st.Mode,
		"log-level":           "warning",
		"external-controller": fmt.Sprintf("127.0.0.1:%d", CtrlPort),
		"external-ui":         paths.PanelDir(), // 专业面板（三选一：metacubexd/yacd/zashboard），经 http://127.0.0.1:9090/ui/ 访问
		"dns": map[string]any{
			"enable":             true,
			"ipv6":               false,
			"enhanced-mode":      "fake-ip",
			"fake-ip-range":      "198.18.0.1/16",
			"default-nameserver": []string{"223.5.5.5", "119.29.29.29"},
			"nameserver":         []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"},
		},
		"tun": map[string]any{
			"enable":               st.Tun,
			"stack":                "system",
			"dns-hijack":           []string{"any:53", "tcp://any:53"},
			"auto-route":           true,
			"auto-detect-interface": true,
		},
		"proxies": proxies,
	}
	if len(names) > 0 {
		cfg["proxy-groups"] = []any{
			map[string]any{"name": SelectGroup, "type": "select", "proxies": names},
			map[string]any{
				"name": AutoGroup, "type": "url-test", "proxies": names,
				"url": "http://www.gstatic.com/generate_204", "interval": 300,
			},
		}
		cfg["rules"] = []string{"MATCH," + SelectGroup}
	} else {
		// 无节点：空代理组会导致 mihomo 配置校验失败直接退出，这里直接全直连
		cfg["rules"] = []string{"MATCH,DIRECT"}
	}
	return yaml.Marshal(cfg)
}

// WriteConfig 生成并写入 config.yaml
func WriteConfig(proxies []map[string]any, st Settings) error {
	data, err := BuildConfig(proxies, st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigFile(), data, 0o644)
}

// dedupeNames 提取节点名并去重（重名加序号后缀）
func dedupeNames(proxies []map[string]any) []string {
	seen := map[string]int{}
	names := make([]string, 0, len(proxies))
	for _, p := range proxies {
		name, _ := p["name"].(string)
		if name == "" {
			continue
		}
		if n, ok := seen[name]; ok {
			seen[name] = n + 1
			name = fmt.Sprintf("%s (%d)", name, n+1)
			p["name"] = name
		} else {
			seen[name] = 1
		}
		names = append(names, name)
	}
	return names
}
