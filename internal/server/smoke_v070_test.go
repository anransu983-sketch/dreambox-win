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

func TestSmokeV070(t *testing.T) {
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
		if v["ok"] != true {
			t.Fatalf("%s %s: ok != true: %v", method, path, v)
		}
		return v
	}
	v := do("GET", "/api/traffic/total", "")
	if _, ok := v["up"]; !ok {
		t.Fatal("traffic/total 缺 up")
	}
	v = do("POST", "/api/settings/closetotray", `{"enable":false}`)
	if v["enable"] != false {
		t.Fatalf("closetotray: %v", v)
	}
	v = do("GET", "/api/settings/closetotray", "")
	if v["enable"] != false {
		t.Fatalf("closetotray GET: %v", v)
	}
	v = do("POST", "/api/settings/silentstart", `{"enable":true}`)
	if v["enable"] != true {
		t.Fatalf("silentstart: %v", v)
	}
	v = do("POST", "/api/settings/update", `{"checkUpdate":true,"updateURL":"https://example.com/v.json"}`)
	if v["checkUpdate"] != true || v["updateURL"] != "https://example.com/v.json" {
		t.Fatalf("update settings: %v", v)
	}
	v = do("POST", "/api/settings/subua", `{"ua":"MyUA/2.0"}`)
	if v["ua"] != "MyUA/2.0" {
		t.Fatalf("subua: %v", v)
	}
	v = do("GET", "/api/update", "")
	if v["available"] != false {
		t.Fatalf("update info: %v", v)
	}
	app.SetUpdateAvailable(&UpdateInfo{Version: "v9.9.9", DownloadURL: "https://example.com/d.zip", Notes: "test"})
	v = do("GET", "/api/update", "")
	if v["available"] != true || v["version"] != "v9.9.9" {
		t.Fatalf("update info 2: %v", v)
	}
	v = do("GET", "/api/status", "")
	for _, k := range []string{"closeToTray", "silentStart", "checkUpdate", "updateURL", "subUserAgent"} {
		if _, ok := v[k]; !ok {
			t.Fatalf("status 缺字段 %s", k)
		}
	}
	// kernel restart 在 Linux 下应返回 ok:false（仅 Windows）
	r := httptest.NewRequest("POST", "/api/kernel/restart", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var rv map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rv)
	if rv["ok"] != false {
		t.Fatalf("linux 下 restart 应失败: %v", rv)
	}
	t.Log("v0.7.0 冒烟全过")
}
