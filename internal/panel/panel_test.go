package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPanelRegistry(t *testing.T) {
	ps := Panels()
	if len(ps) != 3 {
		t.Fatalf("面板数量 = %d, 期望 3", len(ps))
	}
	ids := map[string]bool{}
	for _, p := range ps {
		if p.ID == "" || p.Name == "" || p.Version == "" {
			t.Errorf("面板元信息不完整: %+v", p)
		}
		ids[p.ID] = true
	}
	for _, want := range []string{"metacubexd", "yacd", "zashboard"} {
		if !ids[want] {
			t.Errorf("缺少面板 %s", want)
		}
	}
	// 三面板 embed 非空
	for i := range registry {
		if len(registry[i].data) == 0 {
			t.Errorf("面板 %s 资源为空", registry[i].ID)
		}
	}
}

func TestPanelByIDInvalid(t *testing.T) {
	if _, err := PanelByID("bogus"); err == nil {
		t.Error("非法 id 应报错")
	}
	if ValidID("bogus") {
		t.Error("ValidID(bogus) 应为 false")
	}
	if !ValidID(DefaultID) {
		t.Error("ValidID(默认面板) 应为 true")
	}
}

func TestEnsurePanelReleases(t *testing.T) {
	for _, id := range []string{"metacubexd", "yacd", "zashboard"} {
		p, err := PanelByID(id)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := ensurePanelTo(p, dir); err != nil {
			t.Fatalf("ensurePanelTo(%s): %v", id, err)
		}
		// index.html 必须存在
		idx, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Errorf("%s 缺少 index.html: %v", id, err)
			continue
		}
		if len(idx) < 100 {
			t.Errorf("%s 的 index.html 过小", id)
		}
		// 署名文件存在
		if _, err := os.Stat(filepath.Join(dir, "ATTRIBUTION.txt")); err != nil {
			t.Errorf("%s 缺少 ATTRIBUTION.txt", id)
		}
		// version 标记正确
		mark, _ := os.ReadFile(filepath.Join(dir, ".version"))
		if string(mark) != versionMark(p) {
			t.Errorf("%s version 标记 = %q, 期望 %q", id, mark, versionMark(p))
		}
		// 幂等
		if err := ensurePanelTo(p, dir); err != nil {
			t.Fatalf("%s 第二次 ensurePanelTo: %v", id, err)
		}
	}
	// 切换面板：旧文件被清理
	dir := t.TempDir()
	pm, _ := PanelByID("metacubexd")
	if err := ensurePanelTo(pm, dir); err != nil {
		t.Fatal(err)
	}
	py, _ := PanelByID("yacd")
	if err := ensurePanelTo(py, dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if strings.Contains(string(data), "MetaCubeXD") {
		t.Error("切换到 yacd 后仍残留 metacubexd 的 index.html")
	}
	mark, _ := os.ReadFile(filepath.Join(dir, ".version"))
	if string(mark) != versionMark(py) {
		t.Errorf("切换后 version 标记 = %q, 期望 %q", mark, versionMark(py))
	}
}

func TestEnsurePanelInvalidID(t *testing.T) {
	if err := EnsurePanel("bogus"); err == nil {
		t.Error("EnsurePanel(非法 id) 应报错")
	}
}
