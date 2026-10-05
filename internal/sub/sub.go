package sub

// 订阅管理：增删改查、手动更新、本地 JSON 存档。
// 解析 Clash YAML 的 proxies 段，原样透传给 mihomo（原生支持
// vless/vmess/trojan/ss/hysteria2 等全部类型）。

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"dreambox-win/internal/paths"
	"dreambox-win/internal/singbox"

	"github.com/goccy/go-yaml"
)

// Subscription 单条订阅
type Subscription struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Remark    string    `json:"remark"`
	UpdatedAt int64     `json:"updatedAt"`
	Count     int       `json:"count"`
	UserInfo  *UserInfo `json:"userInfo,omitempty"` // 订阅流量用量/到期（来自 subscription-userinfo 响应头）
}

// UserInfo 订阅流量信息：upload/download/total 为字节数，expire 为 Unix 时间戳
type UserInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}

// parseUserInfo 解析 subscription-userinfo 响应头
// 格式：upload=123; download=456; total=1073741824; expire=1712345678
func parseUserInfo(header string) *UserInfo {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	ui := &UserInfo{}
	found := false
	for _, part := range strings.Split(header, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			continue
		}
		found = true
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "upload":
			ui.Upload = v
		case "download":
			ui.Download = v
		case "total":
			ui.Total = v
		case "expire":
			ui.Expire = v
		}
	}
	if !found {
		return nil
	}
	return ui
}

type store struct {
	Subs []*Subscription `json:"subs"`
}

// Load 读取全部订阅
func Load() ([]*Subscription, error) {
	data, err := os.ReadFile(paths.SubsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Subs, nil
}

// save 写回
func save(subs []*Subscription) error {
	data, err := json.MarshalIndent(store{Subs: subs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(paths.SubsFile(), data, 0o644)
}

// DefaultUserAgent 订阅下载默认 UA（Chrome，可在设置中修改）
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

func newID(url string) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d", url, time.Now().UnixNano())))
	return hex.EncodeToString(h[:])[:12]
}

// Add 新增订阅（会立即抓取一次以校验与计数）
func Add(url, remark string) (*Subscription, error) {
	return AddWithUA(url, remark, DefaultUserAgent)
}

// AddWithUA 用指定 UA 新增订阅
func AddWithUA(url, remark, ua string) (*Subscription, error) {
	proxies, info, err := FetchWithUAInfo(url, ua)
	if err != nil {
		return nil, err
	}
	subs, _ := Load()
	s := &Subscription{
		ID:        newID(url),
		URL:       url,
		Remark:    remark,
		UpdatedAt: time.Now().Unix(),
		Count:     len(proxies),
		UserInfo:  info,
	}
	subs = append(subs, s)
	if err := save(subs); err != nil {
		return nil, err
	}
	return s, nil
}

// Delete 删除订阅
func Delete(id string) error {
	subs, _ := Load()
	kept := subs[:0]
	for _, s := range subs {
		if s.ID != id {
			kept = append(kept, s)
		}
	}
	return save(kept)
}

// Update 手动更新一条订阅，返回最新节点数
func Update(id string) (int, error) {
	return UpdateWithUA(id, DefaultUserAgent)
}

// UpdateWithUA 用指定 UA 手动更新一条订阅，返回最新节点数
func UpdateWithUA(id, ua string) (int, error) {
	subs, _ := Load()
	for _, s := range subs {
		if s.ID == id {
			proxies, info, err := FetchWithUAInfo(s.URL, ua)
			if err != nil {
				return 0, err
			}
			s.Count = len(proxies)
			s.UpdatedAt = time.Now().Unix()
			s.UserInfo = info
			if err := save(subs); err != nil {
				return 0, err
			}
			return len(proxies), nil
		}
	}
	return 0, fmt.Errorf("订阅不存在")
}

// FetchProxies 抓取全部订阅的节点（供生成配置用）
func FetchProxies() []map[string]any {
	subs, _ := Load()
	var all []map[string]any
	for _, s := range subs {
		proxies, err := Fetch(s.URL)
		if err != nil {
			continue // 某条订阅失效不影响其他
		}
		all = append(all, proxies...)
	}
	return all
}

// Fetch 下载并解析订阅，返回 proxies 列表
func Fetch(rawURL string) ([]map[string]any, error) {
	return FetchWithUA(rawURL, DefaultUserAgent)
}

// FetchWithUA 用指定 UA 下载订阅并解析
func FetchWithUA(rawURL, ua string) ([]map[string]any, error) {
	proxies, _, err := FetchWithUAInfo(rawURL, ua)
	return proxies, err
}

// FetchWithUAInfo 用指定 UA 下载订阅，返回 proxies 列表与流量信息（无该响应头则为 nil）
func FetchWithUAInfo(rawURL, ua string) ([]map[string]any, *UserInfo, error) {
	if ua == "" {
		ua = DefaultUserAgent
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("下载订阅失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("订阅服务器返回 %s", resp.Status)
	}
	info := parseUserInfo(resp.Header.Get("Subscription-Userinfo"))
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 最多 8MB
	if err != nil {
		return nil, nil, err
	}
	format := detectFormat(body)
	log.Printf("订阅 %s: 识别为 %s 格式", rawURL, format)
	proxies, err := ParseProxies(body)
	if err != nil {
		return nil, nil, err
	}
	return proxies, info, nil
}

// detectFormat 识别订阅内容格式：sing-box JSON / Clash YAML
func detectFormat(data []byte) string {
	if looksLikeSingBox(data) {
		return "sing-box JSON"
	}
	return "Clash YAML"
}

// looksLikeSingBox 内容 trim 后以 { 开头且能解析出 outbounds 数组
func looksLikeSingBox(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var probe struct {
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return false
	}
	return probe.Outbounds != nil
}

// ParseProxies 解析订阅：sing-box JSON 走转换，Clash YAML 解析 proxies 段
func ParseProxies(data []byte) ([]map[string]any, error) {
	if looksLikeSingBox(data) {
		proxies, err := singbox.Convert(data)
		if err != nil {
			return nil, fmt.Errorf("sing-box 订阅解析失败: %w", err)
		}
		return proxies, nil
	}
	return parseClashYAML(data)
}

// parseClashYAML 解析 Clash YAML 中的 proxies 段
func parseClashYAML(data []byte) ([]map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("YAML 解析失败: %w", err)
	}
	raw, ok := doc["proxies"].([]any)
	if !ok {
		return nil, fmt.Errorf("订阅中没有 proxies 段")
	}
	var out []map[string]any
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		typ, _ := m["type"].(string)
		if name == "" || typ == "" {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		ni, _ := out[i]["name"].(string)
		nj, _ := out[j]["name"].(string)
		return ni < nj
	})
	return out, nil
}
