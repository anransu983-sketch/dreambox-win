package clipboard

import "testing"

func TestExtractLink(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"vless://abc@h:443#n", "vless://abc@h:443#n"},
		{"  ss://abc@h:1  ", "ss://abc@h:1"},
		{"VMESS://abc", "VMESS://abc"},
		{"hy2://pw@h:443?sni=x", "hy2://pw@h:443?sni=x"},
		{"hysteria2://pw@h:443", "hysteria2://pw@h:443"},
		{"trojan://pw@h:443", "trojan://pw@h:443"},
		{"https://example.com/sub", ""},
		{"hello world", ""},
		{"", ""},
		{"prefix vless://abc@h:443", ""}, // 不在行首不算
	}
	for _, c := range cases {
		if got := ExtractLink(c.in); got != c.want {
			t.Fatalf("ExtractLink(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
