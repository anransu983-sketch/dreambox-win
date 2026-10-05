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

func TestSmokePanels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", "")
	_ = os.MkdirAll(paths.DataDir(), 0o755)
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
			t.Fatalf("%s %s: 非 JSON: %s", method, path, w.Body.String())
		}
		return v
	}

	// 列表：三面板 + 默认选中 metacubexd
	v := do("GET", "/api/panels", "")
	if v["ok"] != true {
		t.Fatalf("GET /api/panels: %v", v)
	}
	ps, _ := v["panels"].([]any)
	if len(ps) != 3 {
		t.Fatalf("面板数量 = %d, 期望 3", len(ps))
	}
	if v["active"] != "metacubexd" {
		t.Fatalf("默认面板 = %v, 期望 metacubexd", v["active"])
	}

	// 非法 id 必须报错
	v = do("POST", "/api/panels/select", `{"id":"bogus"}`)
	if v["ok"] == true {
		t.Fatal("非法面板 id 应报错")
	}

	// 切换到 yacd（Linux 下跳过内核重启，只写配置+释放面板）
	v = do("POST", "/api/panels/select", `{"id":"yacd"}`)
	if v["ok"] != true {
		t.Fatalf("切换到 yacd 失败: %v", v)
	}
	if v["panelName"] == "" {
		t.Error("返回缺少 panelName")
	}

	// 状态接口带出面板名
	v = do("GET", "/api/status", "")
	if v["ok"] != true {
		t.Fatalf("GET /api/status: %v", v)
	}
	if v["panel"] != "yacd" || v["panelName"] == "" {
		t.Errorf("status 面板字段异常: panel=%v panelName=%v", v["panel"], v["panelName"])
	}

	// 面板文件确实释放到了 panel 目录
	if _, err := os.Stat(paths.PanelDir() + "/index.html"); err != nil {
		t.Errorf("panel 目录缺少 index.html: %v", err)
	}

	// 方法错误
	r := httptest.NewRequest(http.MethodGet, "/api/panels/select", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/panels/select 应返回 405, 实际 %d", w.Code)
	}
}
