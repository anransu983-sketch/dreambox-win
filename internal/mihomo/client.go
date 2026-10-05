package mihomo

// mihomo external-controller REST 客户端（127.0.0.1:9090）。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func ctrlURL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", CtrlPort, path)
}

// Online 内核 API 是否可达
func Online() bool {
	resp, err := httpClient.Get(ctrlURL("/version"))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// GetMode 获取当前出站模式
func GetMode() string {
	resp, err := httpClient.Get(ctrlURL("/configs"))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var v struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return ""
	}
	return v.Mode
}

// SetMode 切换出站模式 rule/global/direct
func SetMode(mode string) error {
	body, _ := json.Marshal(map[string]string{"mode": mode})
	req, _ := http.NewRequest(http.MethodPatch, ctrlURL("/configs"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("切换模式失败: %s", resp.Status)
	}
	return nil
}

// ProxyNode 节点信息
type ProxyNode struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Delay  int    `json:"delay"`  // -1 表示未知
	Source string `json:"source"` // manual=手动添加 subscription=订阅
}

// ListNodes 列出主选择组的全部节点（含当前选中）
func ListNodes() (nodes []ProxyNode, current string, err error) {
	resp, err := httpClient.Get(ctrlURL("/proxies"))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	var v struct {
		Proxies map[string]struct {
			Name string   `json:"name"`
			Type string   `json:"type"`
			Now  string   `json:"now"`
			All  []string `json:"all"`
		} `json:"proxies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, "", err
	}
	sel, ok := v.Proxies[SelectGroup]
	if !ok {
		// 零订阅时配置里没有代理组：返回空列表，Web 端提示添加订阅
		return []ProxyNode{}, "", nil
	}
	current = sel.Now
	for _, name := range sel.All {
		p, ok := v.Proxies[name]
		typ := ""
		if ok {
			typ = p.Type
		}
		nodes = append(nodes, ProxyNode{Name: name, Type: typ, Delay: -1})
	}
	return nodes, current, nil
}

// SelectNode 切换当前节点
func SelectNode(name string) error {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, _ := http.NewRequest(http.MethodPut, ctrlURL("/proxies/"+url.PathEscape(SelectGroup)), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("切换节点失败: %s %s", resp.Status, string(b))
	}
	return nil
}

// Delay 测试单个节点延迟（毫秒），失败返回 -1
func Delay(name string, timeoutMs int) int {
	u := fmt.Sprintf("%s/proxies/%s/delay?timeout=%d&url=%s",
		ctrlURL(""), url.PathEscape(name), timeoutMs,
		url.QueryEscape("http://www.gstatic.com/generate_204"))
	c := &http.Client{Timeout: time.Duration(timeoutMs+2000) * time.Millisecond}
	resp, err := c.Get(u)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	var v struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil || v.Delay <= 0 {
		return -1
	}
	return v.Delay
}

// ConnectionCount 返回当前活跃连接数
func ConnectionCount() (int, error) {
	resp, err := httpClient.Get(ctrlURL("/connections"))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var v struct {
		Connections []json.RawMessage `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return 0, err
	}
	return len(v.Connections), nil
}
