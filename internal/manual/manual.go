package manual

// 手动添加的节点：存 %APPDATA%/DreamBox/manual.json。
// 纯 Go，可在 Linux 下单元测试。

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"dreambox-win/internal/linkparse"
	"dreambox-win/internal/paths"
)

// ManualNode 一条手动节点
type ManualNode struct {
	Name  string         `json:"name"`
	Link  string         `json:"link"`
	Proxy map[string]any `json:"proxy"`
}

var mu sync.Mutex

// Load 读取全部手动节点
func Load() ([]ManualNode, error) {
	data, err := os.ReadFile(paths.ManualFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var nodes []ManualNode
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func save(nodes []ManualNode) error {
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ManualFile(), data, 0o644)
}

// Add 解析链接并添加为手动节点（重名自动加序号后缀），返回节点信息
func Add(link string) (*ManualNode, error) {
	mu.Lock()
	defer mu.Unlock()
	r, err := linkparse.Parse(link)
	if err != nil {
		return nil, err
	}
	nodes, _ := Load()
	name := dedupeName(nodes, r.Name)
	n := ManualNode{Name: name, Link: strings.TrimSpace(link), Proxy: r.Proxy}
	n.Proxy["name"] = name
	nodes = append(nodes, n)
	if err := save(nodes); err != nil {
		return nil, err
	}
	return &n, nil
}

// Delete 按名称删除手动节点
func Delete(name string) error {
	mu.Lock()
	defer mu.Unlock()
	nodes, _ := Load()
	kept := nodes[:0]
	found := false
	for _, n := range nodes {
		if n.Name == name {
			found = true
			continue
		}
		kept = append(kept, n)
	}
	if !found {
		return fmt.Errorf("手动节点不存在: %s", name)
	}
	return save(kept)
}

// Names 返回手动节点名称集合（供来源标注用）
func Names() map[string]bool {
	nodes, _ := Load()
	m := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		m[n.Name] = true
	}
	return m
}

// Proxies 返回手动节点的 proxy 列表（供生成配置用）
func Proxies() []map[string]any {
	nodes, _ := Load()
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		if n.Proxy != nil {
			out = append(out, n.Proxy)
		}
	}
	return out
}

// Merge 合并订阅节点与手动节点：按名称去重，手动节点优先。
func Merge(subs []map[string]any) []map[string]any {
	manualNodes, _ := Load()
	manualNames := map[string]bool{}
	var out []map[string]any
	for _, n := range manualNodes {
		if n.Proxy == nil {
			continue
		}
		manualNames[n.Name] = true
		out = append(out, n.Proxy)
	}
	for _, p := range subs {
		name, _ := p["name"].(string)
		if manualNames[name] {
			continue // 重名：手动节点优先
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni, _ := out[i]["name"].(string)
		nj, _ := out[j]["name"].(string)
		return ni < nj
	})
	return out
}

func dedupeName(nodes []ManualNode, name string) string {
	taken := map[string]bool{}
	for _, n := range nodes {
		taken[n.Name] = true
	}
	if !taken[name] {
		return name
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s (%d)", name, i)
		if !taken[cand] {
			return cand
		}
	}
}
