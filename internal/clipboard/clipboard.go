package clipboard

// 剪贴板代理链接嗅探：轮询剪贴板，发现新的代理链接时回调。
// Windows 实现见 clipboard_windows.go；其他平台为空实现。

import (
	"regexp"
	"strings"
)

// linkRe 代理链接匹配：行首的 scheme://
var linkRe = regexp.MustCompile(`(?i)^\s*(vless|vmess|trojan|ss|hysteria2|hy2)://\S+`)

// ExtractLink 从文本中提取代理链接，找不到返回 ""
func ExtractLink(text string) string {
	return strings.TrimSpace(linkRe.FindString(text))
}
