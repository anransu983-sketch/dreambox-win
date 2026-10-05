package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"dreambox-win/internal/paths"
)

func TestSmokeV050(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", "")
	app := New()
	h := app.Handler()

	do := func(method, path, body string) map[string]any {
		t.Helper()
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, path, strings.NewReader(body))
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatalf("%s %s: 非 JSON 响应: %s", method, path, w.Body.String())
		}
		return v
	}

	// 1. 解析预览
	v := do("POST", "/api/nodes/manual", `{"link":"vless://11111111-2222-3333-4444-555555555555@203.0.113.10:443?security=reality&sni=gw.example.com&pbk=K&sid=s1#测试"}`)
	if v["ok"] != true || v["type"] != "vless" || v["server"] != "203.0.113.10" {
		t.Fatalf("预览失败: %v", v)
	}

	// 2. 确认添加（Linux 下跳过内核重启，只写配置）
	v = do("POST", "/api/nodes/manual/add", `{"link":"vless://11111111-2222-3333-4444-555555555555@203.0.113.10:443?security=reality&sni=gw.example.com&pbk=K&sid=s1#测试"}`)
	if v["ok"] != true {
		t.Fatalf("添加失败: %v", v)
	}

	// 3. 手动节点列表
	v = do("GET", "/api/nodes/manual", "")
	nodes, _ := v["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("手动列表应为 1: %v", v)
	}

	// 4. 非法链接预览应报错
	v = do("POST", "/api/nodes/manual", `{"link":"https://x.com"}`)
	if v["ok"] != false {
		t.Fatalf("非法链接应报错: %v", v)
	}

	// 5. 剪贴板 pending（先无）
	v = do("GET", "/api/clipboard/pending", "")
	if v["pending"] != nil {
		t.Fatalf("初始 pending 应为 null: %v", v)
	}
	// 模拟剪贴板发现
	app.OnClipboardLink("trojan://pw@192.0.2.9:443#剪贴板节点")
	v = do("GET", "/api/clipboard/pending", "")
	p, _ := v["pending"].(map[string]any)
	if p["name"] != "剪贴板节点" || p["type"] != "trojan" {
		t.Fatalf("pending 错误: %v", v)
	}
	// 非法链接不产生 pending（先 dismiss 再试）
	do("POST", "/api/clipboard/dismiss", "")
	app.OnClipboardLink("not a link")
	v = do("GET", "/api/clipboard/pending", "")
	if v["pending"] != nil {
		t.Fatalf("非法链接不应产生 pending: %v", v)
	}
	// 导入
	app.OnClipboardLink("ss://YWVzLTI1Ni1nY206cHdk@192.0.2.1:8388#SS导入")
	v = do("POST", "/api/clipboard/import", "")
	if v["ok"] != true {
		t.Fatalf("导入失败: %v", v)
	}
	v = do("GET", "/api/clipboard/pending", "")
	if v["pending"] != nil {
		t.Fatalf("导入后 pending 应清空: %v", v)
	}
	v = do("GET", "/api/nodes/manual", "")
	if len(v["nodes"].([]any)) != 2 {
		t.Fatalf("导入后手动节点应为 2: %v", v)
	}

	// 5.5 生成的 config.yaml 应包含手动节点
	cfgData, err := os.ReadFile(paths.ConfigFile())
	if err != nil {
		t.Fatalf("读取 config.yaml 失败: %v", err)
	}
	cfg := string(cfgData)
	for _, want := range []string{"测试", "SS导入", "type: vless", "type: shadowsocks"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config.yaml 缺少 %q", want)
		}
	}

	// 6. 删除
	v = do("DELETE", "/api/nodes/manual/del?name="+ "SS%E5%AF%BC%E5%85%A5", "")
	if v["ok"] != true {
		t.Fatalf("删除失败: %v", v)
	}

	// 7. /api/nodes 正常响应（内核未运行 → ok:false 报错，可接受；只要不 panic）
	v = do("GET", "/api/nodes", "")
	t.Logf("/api/nodes => ok=%v", v["ok"])
}
