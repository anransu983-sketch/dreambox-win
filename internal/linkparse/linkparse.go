package linkparse

// 单个代理链接解析 → mihomo proxy map。
// 支持 vless / vmess / trojan / ss / hysteria2(hy2) 分享链接。
// 纯 Go，可在 Linux 下单元测试。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Result 解析结果
type Result struct {
	Name   string         `json:"name"`   // 节点备注
	Type   string         `json:"type"`   // mihomo 类型：vless/vmess/trojan/shadowsocks/hysteria2
	Server string         `json:"server"` // 地址
	Port   int            `json:"port"`   // 端口
	Proxy  map[string]any `json:"-"`      // mihomo proxy 段
}

// SchemeOf 返回链接的 scheme（小写），不识别返回 ""
func SchemeOf(link string) string {
	link = strings.TrimSpace(link)
	i := strings.Index(link, "://")
	if i <= 0 {
		return ""
	}
	s := strings.ToLower(link[:i])
	switch s {
	case "vless", "vmess", "trojan", "ss", "shadowsocks", "hysteria2", "hy2":
		return s
	}
	return ""
}

// Parse 解析单个代理链接
func Parse(link string) (*Result, error) {
	link = strings.TrimSpace(link)
	switch SchemeOf(link) {
	case "vless":
		return parseVLESS(link)
	case "vmess":
		return parseVMess(link)
	case "trojan":
		return parseTrojan(link)
	case "ss", "shadowsocks":
		return parseSS(link)
	case "hysteria2", "hy2":
		return parseHysteria2(link)
	}
	return nil, fmt.Errorf("不支持的链接格式（支持 vless/vmess/trojan/ss/hysteria2）")
}

// splitFrag 切掉 #备注，返回（主体，解码后的备注）
func splitFrag(s string) (string, string) {
	if i := strings.Index(s, "#"); i >= 0 {
		name := s[i+1:]
		if dec, err := url.QueryUnescape(name); err == nil {
			name = dec
		} else if dec, err := url.PathUnescape(name); err == nil {
			name = dec
		}
		return s[:i], name
	}
	return s, ""
}

func parsePort(portStr string) (int, error) {
	p, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("端口无效: %s", portStr)
	}
	return p, nil
}

// decodeB64 兼容多种 base64 变体（标准/URL 安全/缺 padding/经 URL 编码）。
// 注意：只有含 % 时才做 URL 解码，避免 QueryUnescape 把标准 base64 的 + 转成空格。
func decodeB64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	candidates := []string{s}
	if strings.Contains(s, "%") {
		if u, err := url.QueryUnescape(s); err == nil && u != s {
			candidates = append(candidates, u)
		}
	}
	encs := []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.StdEncoding,
	}
	var lastErr error
	for _, c := range candidates {
		tries := []string{c}
		if m := len(c) % 4; m != 0 {
			tries = append(tries, c+strings.Repeat("=", 4-m))
		}
		for _, t := range tries {
			for _, e := range encs {
				if b, err := e.DecodeString(t); err == nil {
					return b, nil
				} else {
					lastErr = err
				}
			}
		}
	}
	return nil, fmt.Errorf("base64 解码失败: %v", lastErr)
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		p, _ := strconv.Atoi(strings.TrimSpace(n))
		return p
	}
	return 0
}

func strOf(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// ---------- vless ----------
//
// vless://uuid@host:port?encryption=none&security=reality&sni=..&fp=..&pbk=..&sid=..&type=tcp|ws|grpc&path=..&host=..&flow=..#名称

func parseVLESS(link string) (*Result, error) {
	rest, name := splitFrag(strings.TrimPrefix(link, "vless://"))
	u, err := url.Parse("vless://" + rest)
	if err != nil {
		return nil, fmt.Errorf("vless 链接解析失败: %w", err)
	}
	uuid := u.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("vless 链接缺少 uuid")
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("vless 链接缺少地址")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, fmt.Errorf("vless %w", err)
	}
	q := u.Query()
	security := strings.ToLower(q.Get("security"))
	if security == "" {
		security = "none"
	}
	proxy := map[string]any{
		"name": name, "type": "vless",
		"server": host, "port": port,
		"uuid": uuid,
		"udp":  true,
	}
	if enc := q.Get("encryption"); enc != "" {
		proxy["encryption"] = enc
	} else {
		proxy["encryption"] = "none"
	}
	if flow := q.Get("flow"); flow != "" {
		proxy["flow"] = flow
	}
	// TLS / Reality
	if security == "tls" || security == "reality" {
		proxy["tls"] = true
		if sni := q.Get("sni"); sni != "" {
			proxy["servername"] = sni
		} else {
			proxy["servername"] = host
		}
		if fp := q.Get("fp"); fp != "" {
			proxy["fingerprint"] = fp
		}
		if security == "reality" {
			ro := map[string]any{}
			if pbk := q.Get("pbk"); pbk != "" {
				ro["public-key"] = pbk
			} else {
				return nil, fmt.Errorf("reality 缺少 pbk（public-key）参数")
			}
			if sid := q.Get("sid"); sid != "" {
				ro["short-id"] = sid
			}
			proxy["reality-opts"] = ro
		}
	}
	// 传输层
	switch strings.ToLower(q.Get("type")) {
	case "ws", "websocket":
		proxy["network"] = "ws"
		wsOpts := map[string]any{}
		if p := q.Get("path"); p != "" {
			wsOpts["path"] = p
		}
		if h := q.Get("host"); h != "" {
			wsOpts["headers"] = map[string]any{"Host": h}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	case "grpc":
		proxy["network"] = "grpc"
		svc := q.Get("serviceName")
		if svc == "" {
			svc = q.Get("service_name")
		}
		if svc != "" {
			proxy["grpc-opts"] = map[string]any{"grpc-service-name": svc}
		}
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	proxy["name"] = name
	return &Result{Name: name, Type: "vless", Server: host, Port: port, Proxy: proxy}, nil
}

// ---------- vmess ----------
//
// vmess://base64({"ps":"备注","add":"host","port":"443","id":"uuid","aid":"0","scy":"auto","net":"ws","host":"","path":"/","tls":"tls","sni":""})

func parseVMess(link string) (*Result, error) {
	rest, fragName := splitFrag(strings.TrimPrefix(link, "vmess://"))
	// 去掉可能残留的查询串
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}
	raw, err := decodeB64(rest)
	if err != nil {
		return nil, fmt.Errorf("vmess %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("vmess JSON 解析失败: %w", err)
	}
	host := strOf(m, "add")
	if host == "" {
		return nil, fmt.Errorf("vmess 缺少地址 (add)")
	}
	port := toInt(m["port"])
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("vmess 端口无效: %v", m["port"])
	}
	uuid := strOf(m, "id")
	if uuid == "" {
		return nil, fmt.Errorf("vmess 缺少 id")
	}
	name := strOf(m, "ps")
	if name == "" {
		name = fragName
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	proxy := map[string]any{
		"name": name, "type": "vmess",
		"server": host, "port": port,
		"uuid": uuid, "udp": true,
	}
	if aid := toInt(m["aid"]); aid > 0 {
		proxy["alterId"] = aid
	} else {
		proxy["alterId"] = 0
	}
	if scy := strOf(m, "scy"); scy != "" && !strings.EqualFold(scy, "auto") {
		proxy["cipher"] = scy
	} else {
		proxy["cipher"] = "auto"
	}
	if tls := strOf(m, "tls"); strings.EqualFold(tls, "tls") {
		proxy["tls"] = true
		if sni := strOf(m, "sni"); sni != "" {
			proxy["servername"] = sni
		}
		if fp := strOf(m, "fp"); fp != "" {
			proxy["fingerprint"] = fp
		}
	}
	switch strings.ToLower(strOf(m, "net")) {
	case "ws":
		proxy["network"] = "ws"
		wsOpts := map[string]any{}
		if p := strOf(m, "path"); p != "" {
			wsOpts["path"] = p
		}
		if h := strOf(m, "host"); h != "" {
			wsOpts["headers"] = map[string]any{"Host": h}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	case "grpc":
		proxy["network"] = "grpc"
		if p := strOf(m, "path"); p != "" {
			proxy["grpc-opts"] = map[string]any{"grpc-service-name": p}
		}
	}
	return &Result{Name: name, Type: "vmess", Server: host, Port: port, Proxy: proxy}, nil
}

// ---------- trojan ----------
//
// trojan://password@host:port?sni=..&type=ws&path=..&host=..#名称

func parseTrojan(link string) (*Result, error) {
	rest, name := splitFrag(strings.TrimPrefix(link, "trojan://"))
	u, err := url.Parse("trojan://" + rest)
	if err != nil {
		return nil, fmt.Errorf("trojan 链接解析失败: %w", err)
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("trojan 链接缺少密码")
	}
	if dec, err := url.QueryUnescape(password); err == nil {
		password = dec
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("trojan 链接缺少地址")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, fmt.Errorf("trojan %w", err)
	}
	q := u.Query()
	proxy := map[string]any{
		"name": name, "type": "trojan",
		"server": host, "port": port,
		"password": password,
		"udp":      true,
	}
	if sni := q.Get("sni"); sni != "" {
		proxy["sni"] = sni
	} else {
		proxy["sni"] = host
	}
	switch strings.ToLower(q.Get("type")) {
	case "ws", "websocket":
		proxy["network"] = "ws"
		wsOpts := map[string]any{}
		if p := q.Get("path"); p != "" {
			wsOpts["path"] = p
		}
		if h := q.Get("host"); h != "" {
			wsOpts["headers"] = map[string]any{"Host": h}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	case "grpc":
		proxy["network"] = "grpc"
		if svc := q.Get("serviceName"); svc != "" {
			proxy["grpc-opts"] = map[string]any{"grpc-service-name": svc}
		}
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	proxy["name"] = name
	return &Result{Name: name, Type: "trojan", Server: host, Port: port, Proxy: proxy}, nil
}

// ---------- ss ----------
//
// 形式1: ss://base64(method:password)@host:port#名称
// 形式2: ss://base64(method:password@host:port)#名称

func parseSS(link string) (*Result, error) {
	rest, name := splitFrag(strings.TrimPrefix(link, "ss://"))
	// 去掉查询串（插件参数暂不支持）
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}
	var method, password, host, portStr string
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		// 形式1：@ 左边是 base64(method:password)，右边是 host:port
		cred, err := decodeB64(rest[:i])
		if err != nil {
			return nil, fmt.Errorf("ss 用户信息 %w", err)
		}
		parts := strings.SplitN(string(cred), ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("ss 用户信息格式错误，应为 method:password")
		}
		method, password = parts[0], parts[1]
		hp := rest[i+1:]
		if j := strings.LastIndex(hp, ":"); j >= 0 {
			host, portStr = hp[:j], hp[j+1:]
		} else {
			return nil, fmt.Errorf("ss 缺少端口")
		}
	} else {
		// 形式2：整体是 base64(method:password@host:port)
		cred, err := decodeB64(rest)
		if err != nil {
			return nil, fmt.Errorf("ss %w", err)
		}
		s := string(cred)
		// 先按最后一个 @ 切出 host:port
		at := strings.LastIndex(s, "@")
		if at < 0 {
			return nil, fmt.Errorf("ss 格式错误，缺少 @ 分隔")
		}
		mp := s[:at]
		hp := s[at+1:]
		parts := strings.SplitN(mp, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("ss 用户信息格式错误，应为 method:password")
		}
		method, password = parts[0], parts[1]
		if j := strings.LastIndex(hp, ":"); j >= 0 {
			host, portStr = hp[:j], hp[j+1:]
		} else {
			return nil, fmt.Errorf("ss 缺少端口")
		}
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return nil, fmt.Errorf("ss 缺少地址")
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, fmt.Errorf("ss %w", err)
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	proxy := map[string]any{
		"name": name, "type": "shadowsocks",
		"server": host, "port": port,
		"cipher": method, "password": password,
		"udp": true,
	}
	return &Result{Name: name, Type: "shadowsocks", Server: host, Port: port, Proxy: proxy}, nil
}

// ---------- hysteria2 / hy2 ----------
//
// hysteria2://password@host:port?sni=..&insecure=0#名称

func parseHysteria2(link string) (*Result, error) {
	scheme := SchemeOf(link)
	rest, name := splitFrag(link[strings.Index(link, "://")+3:])
	u, err := url.Parse(scheme + "://" + rest)
	if err != nil {
		return nil, fmt.Errorf("hysteria2 链接解析失败: %w", err)
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("hysteria2 链接缺少密码")
	}
	if dec, err := url.QueryUnescape(password); err == nil {
		password = dec
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("hysteria2 链接缺少地址")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return nil, fmt.Errorf("hysteria2 %w", err)
	}
	q := u.Query()
	proxy := map[string]any{
		"name": name, "type": "hysteria2",
		"server": host, "port": port,
		"password": password,
	}
	if sni := q.Get("sni"); sni != "" {
		proxy["sni"] = sni
	} else if peer := q.Get("peer"); peer != "" {
		proxy["sni"] = peer
	}
	if insecure := q.Get("insecure"); insecure == "1" || strings.EqualFold(insecure, "true") {
		proxy["skip-cert-verify"] = true
	}
	if obfs := q.Get("obfs"); obfs != "" {
		proxy["obfs"] = obfs
		if pw := q.Get("obfs-password"); pw != "" {
			proxy["obfs-password"] = pw
		}
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}
	proxy["name"] = name
	return &Result{Name: name, Type: "hysteria2", Server: host, Port: port, Proxy: proxy}, nil
}
