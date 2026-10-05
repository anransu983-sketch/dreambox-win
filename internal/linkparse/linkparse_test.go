package linkparse

import (
	"encoding/base64"
	"strings"
	"testing"
)

func mustParse(t *testing.T, link string) *Result {
	t.Helper()
	r, err := Parse(link)
	if err != nil {
		t.Fatalf("解析失败: %v\n链接: %s", err, link)
	}
	return r
}

func TestVLESSReality(t *testing.T) {
	r := mustParse(t, "vless://11111111-2222-3333-4444-555555555555@203.0.113.10:443?encryption=none&security=reality&sni=gateway.icloud.com&fp=chrome&pbk=AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ABCD&sid=a1b2c3&type=tcp#美国%20Reality")
	if r.Type != "vless" || r.Server != "203.0.113.10" || r.Port != 443 {
		t.Fatalf("基本字段错误: %+v", r)
	}
	if r.Name != "美国 Reality" {
		t.Fatalf("备注解码错误: %q", r.Name)
	}
	p := r.Proxy
	if p["tls"] != true || p["servername"] != "gateway.icloud.com" || p["fingerprint"] != "chrome" {
		t.Fatalf("tls 字段错误: %v", p)
	}
	ro, ok := p["reality-opts"].(map[string]any)
	if !ok || ro["public-key"] != "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ABCD" || ro["short-id"] != "a1b2c3" {
		t.Fatalf("reality-opts 错误: %v", p["reality-opts"])
	}
	if p["uuid"] != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("uuid 错误: %v", p["uuid"])
	}
}

func TestVLESSWsTLS(t *testing.T) {
	r := mustParse(t, "vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@198.51.100.7:8443?security=tls&sni=cdn.example.com&type=ws&path=%2Fray&host=cdn.example.com#WS节点")
	p := r.Proxy
	if p["network"] != "ws" {
		t.Fatalf("network 应为 ws: %v", p)
	}
	ws, ok := p["ws-opts"].(map[string]any)
	if !ok || ws["path"] != "/ray" {
		t.Fatalf("ws-opts 错误: %v", p["ws-opts"])
	}
	h, _ := ws["headers"].(map[string]any)
	if h["Host"] != "cdn.example.com" {
		t.Fatalf("ws host 错误: %v", ws)
	}
	if p["tls"] != true || p["servername"] != "cdn.example.com" {
		t.Fatalf("tls 错误: %v", p)
	}
	if _, hasReality := p["reality-opts"]; hasReality {
		t.Fatalf("不应有 reality-opts")
	}
}

func TestVLESSGrpc(t *testing.T) {
	r := mustParse(t, "vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@198.51.100.8:443?security=reality&sni=grpc.example.com&pbk=KEY123&sid=ff&type=grpc&serviceName=gun#GRPC")
	p := r.Proxy
	if p["network"] != "grpc" {
		t.Fatalf("network 应为 grpc: %v", p)
	}
	g, _ := p["grpc-opts"].(map[string]any)
	if g["grpc-service-name"] != "gun" {
		t.Fatalf("grpc-opts 错误: %v", p["grpc-opts"])
	}
}

func TestVMess(t *testing.T) {
	j := `{"ps":"测试VMess","add":"192.0.2.5","port":"10086","id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","aid":"0","scy":"auto","net":"ws","host":"ws.example.com","path":"/vm","tls":"tls","sni":"ws.example.com"}`
	r := mustParse(t, "vmess://"+base64.StdEncoding.EncodeToString([]byte(j)))
	if r.Type != "vmess" || r.Name != "测试VMess" || r.Server != "192.0.2.5" || r.Port != 10086 {
		t.Fatalf("基本字段错误: %+v", r)
	}
	p := r.Proxy
	if p["tls"] != true || p["servername"] != "ws.example.com" || p["network"] != "ws" {
		t.Fatalf("vmess 字段错误: %v", p)
	}
	ws, _ := p["ws-opts"].(map[string]any)
	if ws["path"] != "/vm" {
		t.Fatalf("ws path 错误: %v", ws)
	}
}

func TestTrojan(t *testing.T) {
	r := mustParse(t, "trojan://mypassword@192.0.2.9:443?sni=tj.example.com#Trojan节点")
	if r.Type != "trojan" || r.Name != "Trojan节点" {
		t.Fatalf("基本字段错误: %+v", r)
	}
	p := r.Proxy
	if p["password"] != "mypassword" || p["sni"] != "tj.example.com" {
		t.Fatalf("trojan 字段错误: %v", p)
	}
}

func TestSSForm1(t *testing.T) {
	cred := base64.URLEncoding.EncodeToString([]byte("aes-256-gcm:sspass123"))
	r := mustParse(t, "ss://"+cred+"@192.0.2.20:8388#SS节点")
	if r.Type != "shadowsocks" || r.Server != "192.0.2.20" || r.Port != 8388 {
		t.Fatalf("基本字段错误: %+v", r)
	}
	p := r.Proxy
	if p["cipher"] != "aes-256-gcm" || p["password"] != "sspass123" {
		t.Fatalf("ss 字段错误: %v", p)
	}
}

func TestSSForm2(t *testing.T) {
	cred := base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pwd456@192.0.2.21:8389"))
	r := mustParse(t, "ss://"+cred+"#SS2")
	if r.Type != "shadowsocks" || r.Server != "192.0.2.21" || r.Port != 8389 {
		t.Fatalf("基本字段错误: %+v", r)
	}
	if r.Proxy["cipher"] != "chacha20-ietf-poly1305" || r.Proxy["password"] != "pwd456" {
		t.Fatalf("ss 字段错误: %v", r.Proxy)
	}
}

func TestHysteria2(t *testing.T) {
	r := mustParse(t, "hysteria2://hy2pass@192.0.2.30:8443?sni=hy.example.com#Hy2节点")
	if r.Type != "hysteria2" || r.Server != "192.0.2.30" || r.Port != 8443 {
		t.Fatalf("基本字段错误: %+v", r)
	}
	if r.Proxy["password"] != "hy2pass" || r.Proxy["sni"] != "hy.example.com" {
		t.Fatalf("hy2 字段错误: %v", r.Proxy)
	}
	r2 := mustParse(t, "hy2://hy2pass@192.0.2.30:8443?insecure=1#Hy2b")
	if r2.Type != "hysteria2" || r2.Proxy["skip-cert-verify"] != true {
		t.Fatalf("hy2 别名/insecure 错误: %v", r2.Proxy)
	}
}

func TestFailures(t *testing.T) {
	bad := []string{
		"",
		"http://example.com/sub",
		"vless://",
		"vless://uuid@host",            // 无端口
		"vless://uuid@host:99999",      // 端口越界
		"ss://not-base64!!!@h:1",       // base64 失败
		"vmess://aGVsbG8=",             // base64 是 "hello"，不是 JSON
		"trojan://@h:443",              // 无密码
		"socks://user@h:1080",          // 不支持的 scheme
		"hysteria2://nopass@:443",      // 无 host
	}
	for _, b := range bad {
		if _, err := Parse(b); err == nil {
			t.Fatalf("应解析失败却成功: %q", b)
		}
	}
}

func TestSchemeOf(t *testing.T) {
	if SchemeOf("  VLESS://x ") != "vless" {
		t.Fatal("大小写/空格处理错误")
	}
	if SchemeOf("ss://x") != "ss" || SchemeOf("hy2://x") != "hy2" {
		t.Fatal("scheme 识别错误")
	}
	if SchemeOf("https://x") != "" {
		t.Fatal("https 不应被识别")
	}
}

func TestFragmentDecode(t *testing.T) {
	r := mustParse(t, "trojan://p@192.0.2.9:443#"+strings.ReplaceAll(url_PathEscape("香港 01"), " ", "%20"))
	_ = r
}

// url_PathEscape 本地小工具，避免引入 net/url 到测试顶层
func url_PathEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' {
			b.WriteString("%20")
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
