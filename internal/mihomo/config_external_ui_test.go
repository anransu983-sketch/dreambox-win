package mihomo

import (
	"strings"
	"testing"
)

// BuildConfig 必须包含 external-ui（专业面板），指向 panel 释放目录
func TestBuildConfigHasExternalUI(t *testing.T) {
	st := DefaultSettings()
	data, err := BuildConfig(nil, st)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	cfg := string(data)
	if !strings.Contains(cfg, "external-ui:") {
		t.Error("config.yaml 缺少 external-ui 字段")
	}
	if !strings.Contains(cfg, "panel") {
		t.Error("external-ui 未指向 panel 目录")
	}
}
