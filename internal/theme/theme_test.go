package theme

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"dreambox-win/internal/paths"
)

func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	if err := EnsureBuiltin(); err != nil {
		t.Fatalf("EnsureBuiltin: %v", err)
	}
}

func TestEnsureBuiltin(t *testing.T) {
	setup(t)
	for _, id := range builtinIDs {
		if !Exists(id) {
			t.Fatalf("内置主题 %s 未释放", id)
		}
		if _, err := os.Stat(filepath.Join(paths.ThemeDir(id), "theme.css")); err != nil {
			t.Fatalf("内置主题 %s 缺少 theme.css", id)
		}
	}
}

func TestListBuiltinFirst(t *testing.T) {
	setup(t)
	list := List("bocchi-starlight")
	if len(list) != 2 {
		t.Fatalf("期望 2 个内置主题，得到 %d", len(list))
	}
	for _, th := range list {
		if !th.Builtin {
			t.Fatalf("期望全是内置主题")
		}
		if th.ID == "bocchi-starlight" && !th.Active {
			t.Fatalf("bocchi-starlight 应标记 active")
		}
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(content))
	}
	_ = w.Close()
	return buf.Bytes()
}

func TestUploadAndDelete(t *testing.T) {
	setup(t)
	data := makeZip(t, map[string]string{
		"theme.json": `{"id":"my-pink","name":"我的粉","version":"1.0","author":"test","description":"desc"}`,
		"theme.css":  `:root{--acc:#ff0000;}`,
		"bg.png":     "fakepng",
	})
	th, err := Upload(data)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if th.ID != "my-pink" || th.Builtin {
		t.Fatalf("上传返回异常: %+v", th)
	}
	if !Exists("my-pink") {
		t.Fatalf("上传后主题不存在")
	}
	if _, err := os.Stat(filepath.Join(paths.ThemeDir("my-pink"), "bg.png")); err != nil {
		t.Fatalf("bg.png 未解压")
	}
	list := List("")
	if len(list) != 3 || !list[2].Builtin && list[2].ID != "my-pink" {
		t.Fatalf("列表异常: %+v", list)
	}
	// 重复 id 拒绝
	if _, err := Upload(data); err == nil {
		t.Fatalf("重复 id 应被拒绝")
	}
	// 删除
	if err := Delete("my-pink"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if Exists("my-pink") {
		t.Fatalf("删除后仍存在")
	}
}

func TestUploadRejects(t *testing.T) {
	setup(t)
	cases := map[string]map[string]string{
		"缺少 theme.json": {"theme.css": "x"},
		"缺少 theme.css":  {"theme.json": `{"id":"a","name":"b"}`},
		"非法 id":         {"theme.json": `{"id":"BAD ID!","name":"b"}`, "theme.css": "x"},
		"覆盖内置":        {"theme.json": `{"id":"default","name":"b"}`, "theme.css": "x"},
		"嵌套目录":        {"sub/theme.json": `{"id":"x","name":"b"}`, "sub/theme.css": "x"},
	}
	for name, files := range cases {
		if _, err := Upload(makeZip(t, files)); err == nil {
			t.Fatalf("%s：应被拒绝", name)
		}
	}
	// zip slip
	data := makeZip(t, map[string]string{
		"theme.json":  `{"id":"evil","name":"b"}`,
		"theme.css":   "x",
		"../evil.txt": "evil",
	})
	if _, err := Upload(data); err != nil {
		t.Fatalf("含穿越条目的 zip 应跳过条目而非整体失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.DataDir(), "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("zip slip 防护失败")
	}
	_ = Delete("evil")
}

func TestDeleteBuiltinRejected(t *testing.T) {
	setup(t)
	if err := Delete("default"); err == nil {
		t.Fatalf("内置主题删除应被拒绝")
	}
	if err := Delete("no-such"); err == nil {
		t.Fatalf("不存在的主题删除应报错")
	}
}

func TestGet(t *testing.T) {
	setup(t)
	th, err := Get("default", true)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !th.Builtin || !th.Active || th.Name == "" {
		t.Fatalf("Get 返回异常: %+v", th)
	}
	if _, err := Get("../x", false); err == nil {
		t.Fatalf("非法 id 应报错")
	}
}
