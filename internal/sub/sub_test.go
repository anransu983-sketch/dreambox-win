package sub

import "testing"

const clashYAML = `
proxies:
  - {name: "节点A", type: vless, server: 1.1.1.1, port: 443, uuid: u1}
  - {name: "节点B", type: trojan, server: 2.2.2.2, port: 443, password: p2}
`

const singboxJSON = `{
  "outbounds": [
    {"type": "vless", "tag": "SB节点", "server": "3.3.3.3", "server_port": 443, "uuid": "u3",
     "tls": {"enabled": true, "server_name": "s.example.com",
             "reality": {"enabled": true, "public_key": "K", "short_id": "s1"}}}
  ]
}`

func TestParseProxiesClash(t *testing.T) {
	list, err := ParseProxies([]byte(clashYAML))
	if err != nil {
		t.Fatalf("Clash YAML 解析失败: %v", err)
	}
	if len(list) != 2 || list[0]["name"] != "节点A" {
		t.Fatalf("结果错误: %v", list)
	}
}

func TestParseProxiesSingBox(t *testing.T) {
	list, err := ParseProxies([]byte(singboxJSON))
	if err != nil {
		t.Fatalf("sing-box JSON 解析失败: %v", err)
	}
	if len(list) != 1 || list[0]["name"] != "SB节点" || list[0]["type"] != "vless" {
		t.Fatalf("结果错误: %v", list)
	}
	ro, _ := list[0]["reality-opts"].(map[string]any)
	if ro["public-key"] != "K" {
		t.Fatalf("reality 转换错误: %v", list[0])
	}
}

func TestDetectFormat(t *testing.T) {
	if detectFormat([]byte("  \n"+singboxJSON)) != "sing-box JSON" {
		t.Fatal("sing-box 识别失败")
	}
	if detectFormat([]byte(clashYAML)) != "Clash YAML" {
		t.Fatal("Clash YAML 识别失败")
	}
	if detectFormat([]byte("{not json")) != "Clash YAML" {
		t.Fatal("非 JSON 应回退到 Clash YAML")
	}
	if detectFormat([]byte("")) != "Clash YAML" {
		t.Fatal("空内容应回退到 Clash YAML")
	}
}

func TestParseUserInfo(t *testing.T) {
	ui := parseUserInfo("upload=123; download=456; total=1073741824; expire=1712345678")
	if ui == nil {
		t.Fatal("期望解析出 userinfo，得到 nil")
	}
	if ui.Upload != 123 || ui.Download != 456 || ui.Total != 1073741824 || ui.Expire != 1712345678 {
		t.Errorf("解析结果不对: %+v", ui)
	}
	if parseUserInfo("") != nil {
		t.Error("空 header 应返回 nil")
	}
	if parseUserInfo("garbage-without-equals") != nil {
		t.Error("无 = 的内容应返回 nil")
	}
	// 大小写与空格容忍
	ui2 := parseUserInfo("Upload=1 ;Download=2;TOTAL=100")
	if ui2 == nil || ui2.Upload != 1 || ui2.Download != 2 || ui2.Total != 100 {
		t.Errorf("大小写/空格容忍失败: %+v", ui2)
	}
}
