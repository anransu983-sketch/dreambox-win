package manual

import (
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", "") // 确保走 HOME 分支
}

func TestAddDelete(t *testing.T) {
	withTempHome(t)
	n, err := Add("trojan://pw123@192.0.2.9:443?sni=t.example.com#手动1")
	if err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	if n.Name != "手动1" {
		t.Fatalf("名称错误: %s", n.Name)
	}
	nodes, _ := Load()
	if len(nodes) != 1 || nodes[0].Link == "" || nodes[0].Proxy == nil {
		t.Fatalf("读取错误: %+v", nodes)
	}
	// 重名自动加后缀
	n2, err := Add("trojan://pw123@192.0.2.9:443#手动1")
	if err != nil {
		t.Fatalf("重名添加失败: %v", err)
	}
	if n2.Name == "手动1" {
		t.Fatal("重名未加后缀")
	}
	if err := Delete(n.Name); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := Delete("不存在的"); err == nil {
		t.Fatal("删除不存在的节点应报错")
	}
	nodes, _ = Load()
	if len(nodes) != 1 || nodes[0].Name != n2.Name {
		t.Fatalf("删除后剩余错误: %+v", nodes)
	}
}

func TestAddInvalid(t *testing.T) {
	withTempHome(t)
	if _, err := Add("https://example.com"); err == nil {
		t.Fatal("非法链接应报错")
	}
	if _, err := Add(""); err == nil {
		t.Fatal("空链接应报错")
	}
}

func TestMerge(t *testing.T) {
	withTempHome(t)
	if _, err := Add("ss://YWVzLTI1Ni1nY206cHdk@192.0.2.1:8388#重名节点"); err != nil {
		t.Fatal(err)
	}
	subs := []map[string]any{
		{"name": "重名节点", "type": "vless", "server": "9.9.9.9", "port": 443},
		{"name": "订阅节点", "type": "vmess", "server": "8.8.8.8", "port": 443},
	}
	merged := Merge(subs)
	if len(merged) != 2 {
		t.Fatalf("合并后应为 2 个，实际 %d", len(merged))
	}
	for _, p := range merged {
		if p["name"] == "重名节点" && p["server"] != "192.0.2.1" {
			t.Fatal("重名时手动节点应优先")
		}
	}
	if !Names()["重名节点"] {
		t.Fatal("Names() 缺少手动节点")
	}
	if len(Proxies()) != 1 {
		t.Fatal("Proxies() 数量错误")
	}
}
