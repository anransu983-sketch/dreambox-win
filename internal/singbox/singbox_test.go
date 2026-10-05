package singbox

import (
	"testing"
)

const sampleConfig = `{
  "log": {"level": "warning"},
  "dns": {"servers": [{"tag": "dns", "address": "223.5.5.5"}]},
  "inbounds": [{"type": "tun", "tag": "tun-in"}],
  "outbounds": [
    {
      "type": "vless", "tag": "美国 Reality",
      "server": "203.0.113.10", "server_port": 443,
      "uuid": "11111111-2222-3333-4444-555555555555",
      "tls": {
        "enabled": true, "server_name": "gateway.icloud.com",
        "utls": {"enabled": true, "fingerprint": "chrome"},
        "reality": {"enabled": true, "public_key": "PUBKEY123", "short_id": "a1b2"}
      }
    },
    {
      "type": "vmess", "tag": "日本 WS",
      "server": "198.51.100.7", "server_port": 443,
      "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
      "security": "auto", "alter_id": 0,
      "tls": {"enabled": true, "server_name": "jp.example.com"},
      "transport": {"type": "ws", "path": "/ray", "headers": {"Host": "jp.example.com"}}
    },
    {
      "type": "trojan", "tag": "香港 Trojan",
      "server": "192.0.2.9", "server_port": 443,
      "password": "trojpass",
      "tls": {"enabled": true, "server_name": "hk.example.com"}
    },
    {
      "type": "shadowsocks", "tag": "新加坡 SS",
      "server": "192.0.2.20", "server_port": 8388,
      "method": "aes-256-gcm", "password": "sspass"
    },
    {
      "type": "hysteria2", "tag": "韩国 Hy2",
      "server": "192.0.2.30", "server_port": 8443,
      "password": "hy2pass",
      "tls": {"enabled": true, "server_name": "kr.example.com"},
      "obfs": {"type": "salamander", "password": "obfspw"}
    },
    {"type": "direct", "tag": "direct"},
    {"type": "block", "tag": "block"},
    {
      "type": "selector", "tag": "手动选择",
      "outbounds": ["美国 Reality", "日本 WS", "不存在的tag"]
    },
    {
      "type": "urltest", "tag": "自动选择",
      "outbounds": ["香港 Trojan", "新加坡 SS", "韩国 Hy2"]
    }
  ]
}`

func byName(t *testing.T, list []map[string]any) map[string]map[string]any {
	t.Helper()
	m := map[string]map[string]any{}
	for _, p := range list {
		name, _ := p["name"].(string)
		if _, dup := m[name]; dup {
			t.Fatalf("节点重名: %s", name)
		}
		m[name] = p
	}
	return m
}

func TestConvertMixed(t *testing.T) {
	list, err := Convert([]byte(sampleConfig))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	// 5 真实节点 + selector 展平 2（去重） + urltest 展平 3（去重）= 5
	if len(list) != 5 {
		t.Fatalf("应输出 5 个节点，实际 %d", len(list))
	}
	m := byName(t, list)

	vless := m["美国 Reality"]
	if vless["type"] != "vless" || vless["tls"] != true || vless["servername"] != "gateway.icloud.com" {
		t.Fatalf("vless 字段错误: %v", vless)
	}
	ro, _ := vless["reality-opts"].(map[string]any)
	if ro["public-key"] != "PUBKEY123" || ro["short-id"] != "a1b2" || vless["fingerprint"] != "chrome" {
		t.Fatalf("reality 字段错误: %v", vless)
	}

	vmess := m["日本 WS"]
	if vmess["type"] != "vmess" || vmess["network"] != "ws" {
		t.Fatalf("vmess 字段错误: %v", vmess)
	}
	ws, _ := vmess["ws-opts"].(map[string]any)
	if ws["path"] != "/ray" {
		t.Fatalf("vmess ws path 错误: %v", ws)
	}

	trojan := m["香港 Trojan"]
	if trojan["type"] != "trojan" || trojan["password"] != "trojpass" || trojan["sni"] != "hk.example.com" {
		t.Fatalf("trojan 字段错误: %v", trojan)
	}

	ss := m["新加坡 SS"]
	if ss["type"] != "shadowsocks" || ss["cipher"] != "aes-256-gcm" || ss["password"] != "sspass" {
		t.Fatalf("ss 字段错误: %v", ss)
	}

	hy2 := m["韩国 Hy2"]
	if hy2["type"] != "hysteria2" || hy2["obfs"] != "salamander" || hy2["obfs-password"] != "obfspw" {
		t.Fatalf("hy2 字段错误: %v", hy2)
	}
}

func TestConvertBareOutbounds(t *testing.T) {
	// 只有 outbounds 数组的精简格式
	j := `{"outbounds": [{"type": "vless", "tag": "t1", "server": "10.0.0.1", "server_port": 443, "uuid": "u"}]}`
	list, err := Convert([]byte(j))
	if err != nil || len(list) != 1 {
		t.Fatalf("精简格式失败: %v %d", err, len(list))
	}
	if list[0]["server"] != "10.0.0.1" || list[0]["port"] != 443 {
		t.Fatalf("字段错误: %v", list[0])
	}
}

func TestConvertGrpc(t *testing.T) {
	j := `{"outbounds": [{
		"type": "vless", "tag": "g1", "server": "10.0.0.2", "server_port": 443, "uuid": "u",
		"tls": {"enabled": true, "server_name": "g.example.com"},
		"transport": {"type": "grpc", "service_name": "gun"}
	}]}`
	list, err := Convert([]byte(j))
	if err != nil || len(list) != 1 {
		t.Fatalf("grpc 失败: %v", err)
	}
	if list[0]["network"] != "grpc" {
		t.Fatalf("network 应为 grpc: %v", list[0])
	}
	g, _ := list[0]["grpc-opts"].(map[string]any)
	if g["grpc-service-name"] != "gun" {
		t.Fatalf("grpc-opts 错误: %v", list[0])
	}
}

func TestConvertNestedSelector(t *testing.T) {
	// selector 引用另一个 selector，递归展平；环引用不死循环
	j := `{"outbounds": [
		{"type": "vless", "tag": "n1", "server": "10.0.0.1", "server_port": 443, "uuid": "u"},
		{"type": "selector", "tag": "s1", "outbounds": ["s2"]},
		{"type": "selector", "tag": "s2", "outbounds": ["s1", "n1"]}
	]}`
	list, err := Convert([]byte(j))
	if err != nil {
		t.Fatalf("嵌套 selector 失败: %v", err)
	}
	if len(list) != 1 || list[0]["name"] != "n1" {
		t.Fatalf("递归展平错误: %v", list)
	}
}

func TestConvertFailures(t *testing.T) {
	for _, j := range []string{
		`not json`,
		`{"log": {}}`,                       // 无 outbounds
		`{"outbounds": "oops"}`,              // outbounds 不是数组
		`{"outbounds": [{"type": 123}]}`,     // 单项解析失败
	} {
		if _, err := Convert([]byte(j)); err == nil {
			t.Fatalf("应失败却成功: %s", j)
		}
	}
	// 空 outbounds → 空列表，不报错
	list, err := Convert([]byte(`{"outbounds": []}`))
	if err != nil || len(list) != 0 {
		t.Fatalf("空 outbounds 应返回空列表: %v", err)
	}
}
