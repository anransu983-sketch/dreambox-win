package server

import (
	"os"
	"testing"

	"dreambox-win/internal/mihomo"
	"dreambox-win/internal/paths"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.7.0", "v0.7.1", true},
		{"v0.7.0", "v0.8.0", true},
		{"v0.7.0", "v1.0.0", true},
		{"v0.7.0", "v0.7.0", false},
		{"v0.7.1", "v0.7.0", false},
		{"v0.7.0", "v0.6.9", false},
		{"0.7.0", "0.7.1", true},   // 无 v 前缀
		{"v0.7", "v0.7.1", true},   // 位数不同
		{"v0.7.10", "v0.7.9", false}, // 数字比较而非字符串
		{"v0.7.0", "", false},
	}
	for _, c := range cases {
		if got := IsNewerVersion(c.cur, c.latest); got != c.want {
			t.Errorf("IsNewerVersion(%q,%q)=%v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestSettingsV070Roundtrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", "")
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	st := mihomo.DefaultSettings()
	if !st.CloseToTrayEnabled() {
		t.Error("CloseToTray 默认应为 true")
	}
	if st.SilentStart {
		t.Error("SilentStart 默认应为 false")
	}
	if st.SubUA() == "" {
		t.Error("SubUA 不应为空")
	}
	if st.CheckUpdate {
		t.Error("CheckUpdate 默认应为 false")
	}
	if st.UpdateURL != "" {
		t.Error("UpdateURL 默认应为空")
	}

	// 改值 → 保存 → 重载
	f := false
	st.CloseToTray = &f
	st.SilentStart = true
	st.SubUserAgent = "TestUA/1.0"
	st.CheckUpdate = true
	st.UpdateURL = "https://example.com/version.json"
	if err := mihomo.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	got := mihomo.LoadSettings()
	if got.CloseToTrayEnabled() {
		t.Error("CloseToTray 应为 false")
	}
	if !got.SilentStart {
		t.Error("SilentStart 应为 true")
	}
	if got.SubUA() != "TestUA/1.0" {
		t.Errorf("SubUA=%q", got.SubUA())
	}
	if !got.CheckUpdate || got.UpdateURL != "https://example.com/version.json" {
		t.Errorf("更新设置丢失: %+v", got)
	}

	// 老配置（无新字段）→ 缺省值
	st2 := mihomo.DefaultSettings()
	st2.CloseToTray = nil
	st2.SubUserAgent = ""
	if err := mihomo.SaveSettings(st2); err != nil {
		t.Fatal(err)
	}
	got2 := mihomo.LoadSettings()
	if !got2.CloseToTrayEnabled() {
		t.Error("老配置 CloseToTray 缺省应为 true")
	}
	if got2.SubUA() == "" {
		t.Error("老配置 SubUA 缺省不应为空")
	}
}
