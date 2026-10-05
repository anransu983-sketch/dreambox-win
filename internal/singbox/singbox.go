package singbox

// sing-box JSON 订阅 → mihomo proxy 列表。
// 输入：{"outbounds":[...]} 或完整 sing-box config（log/dns/inbounds 等忽略）。
// 支持 vless / vmess / trojan / shadowsocks / hysteria2；
// selector / urltest 的 tag 引用递归展平，最终只输出真实节点。
// 纯 Go，可在 Linux 下单元测试。

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// outbound sing-box 出站通用骨架
type outbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`

	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	UUID       string `json:"uuid"`
	Password   string `json:"password"`
	Method     string `json:"method"`
	Security   string `json:"security"`
	AlterID    int    `json:"alter_id"`
	Flow       string `json:"flow"`

	TLS struct {
		Enabled    bool   `json:"enabled"`
		ServerName string `json:"server_name"`
		Insecure   bool   `json:"insecure"`
		UTLS       struct {
			Enabled     bool   `json:"enabled"`
			Fingerprint string `json:"fingerprint"`
		} `json:"utls"`
		Reality struct {
			Enabled   bool   `json:"enabled"`
			PublicKey string `json:"public_key"`
			ShortID   string `json:"short_id"`
		} `json:"reality"`
	} `json:"tls"`

	Transport struct {
		Type        string            `json:"type"`
		Path        string            `json:"path"`
		ServiceName string            `json:"service_name"`
		Headers     map[string]string `json:"headers"`
	} `json:"transport"`

	Obfs struct {
		Type     string `json:"type"`
		Password string `json:"password"`
	} `json:"obfs"`

	Outbounds []string `json:"outbounds"` // selector / urltest 的 tag 引用
}

// Convert 解析 sing-box JSON，返回 mihomo proxy 列表（按原顺序）
func Convert(data []byte) ([]map[string]any, error) {
	var cfg struct {
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("sing-box JSON 解析失败: %w", err)
	}
	if cfg.Outbounds == nil {
		return nil, fmt.Errorf("不是 sing-box 订阅：缺少 outbounds 数组")
	}
	var obs []outbound
	for i, raw := range cfg.Outbounds {
		var ob outbound
		if err := json.Unmarshal(raw, &ob); err != nil {
			return nil, fmt.Errorf("第 %d 个 outbound 解析失败: %w", i+1, err)
		}
		obs = append(obs, ob)
	}

	// tag → outbound 索引
	byTag := map[string]*outbound{}
	for i := range obs {
		if obs[i].Tag != "" {
			byTag[obs[i].Tag] = &obs[i]
		}
	}

	var out []map[string]any
	seen := map[string]bool{} // 去重（同名节点只保留第一个）
	add := func(p map[string]any) {
		name, _ := p["name"].(string)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, p)
	}
	// 递归展平 selector/urltest 的 tag 引用
	var resolve func(ob *outbound, visiting map[string]bool)
	resolve = func(ob *outbound, visiting map[string]bool) {
		if visiting[ob.Tag] {
			return // 环引用保护
		}
		visiting[ob.Tag] = true
		defer delete(visiting, ob.Tag)
		for _, tag := range ob.Outbounds {
			target, ok := byTag[tag]
			if !ok {
				continue
			}
			switch target.Type {
			case "selector", "urltest":
				resolve(target, visiting)
			default:
				if p := convertOne(target); p != nil {
					add(p)
				}
			}
		}
	}

	for i := range obs {
		ob := &obs[i]
		switch ob.Type {
		case "selector", "urltest":
			resolve(ob, map[string]bool{})
		default:
			if p := convertOne(ob); p != nil {
				add(p)
			}
			// direct/block/dns 等：convertOne 返回 nil，静默跳过
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ni, _ := out[i]["name"].(string)
		nj, _ := out[j]["name"].(string)
		return ni < nj
	})
	return out, nil
}

// convertOne 单个真实节点转换；不支持的类型返回 nil
func convertOne(ob *outbound) map[string]any {
	name := ob.Tag
	if name == "" {
		name = fmt.Sprintf("%s-%s:%d", ob.Type, ob.Server, ob.ServerPort)
	}
	if ob.Server == "" || ob.ServerPort <= 0 {
		return nil
	}
	base := map[string]any{
		"name": name, "server": ob.Server, "port": ob.ServerPort, "udp": true,
	}
	// TLS 通用处理
	applyTLS := func(p map[string]any) {
		if !ob.TLS.Enabled {
			return
		}
		p["tls"] = true
		if ob.TLS.ServerName != "" {
			p["servername"] = ob.TLS.ServerName
		}
		if ob.TLS.Insecure {
			p["skip-cert-verify"] = true
		}
		if ob.TLS.UTLS.Enabled && ob.TLS.UTLS.Fingerprint != "" {
			p["fingerprint"] = ob.TLS.UTLS.Fingerprint
		}
		if ob.TLS.Reality.Enabled {
			ro := map[string]any{}
			if ob.TLS.Reality.PublicKey != "" {
				ro["public-key"] = ob.TLS.Reality.PublicKey
			}
			if ob.TLS.Reality.ShortID != "" {
				ro["short-id"] = ob.TLS.Reality.ShortID
			}
			p["reality-opts"] = ro
		}
	}
	// 传输层通用处理
	applyTransport := func(p map[string]any) {
		switch strings.ToLower(ob.Transport.Type) {
		case "ws", "websocket":
			p["network"] = "ws"
			wsOpts := map[string]any{}
			if ob.Transport.Path != "" {
				wsOpts["path"] = ob.Transport.Path
			}
			if h := wsHost(ob.Transport.Headers); h != "" {
				wsOpts["headers"] = map[string]any{"Host": h}
			}
			if len(wsOpts) > 0 {
				p["ws-opts"] = wsOpts
			}
		case "grpc":
			p["network"] = "grpc"
			if ob.Transport.ServiceName != "" {
				p["grpc-opts"] = map[string]any{"grpc-service-name": ob.Transport.ServiceName}
			} else if ob.Transport.Path != "" {
				p["grpc-opts"] = map[string]any{"grpc-service-name": ob.Transport.Path}
			}
		}
	}

	switch ob.Type {
	case "vless":
		if ob.UUID == "" {
			return nil
		}
		p := base
		p["type"] = "vless"
		p["uuid"] = ob.UUID
		p["encryption"] = "none"
		if ob.Flow != "" {
			p["flow"] = ob.Flow
		}
		applyTLS(p)
		applyTransport(p)
		return p
	case "vmess":
		if ob.UUID == "" {
			return nil
		}
		p := base
		p["type"] = "vmess"
		p["uuid"] = ob.UUID
		p["alterId"] = ob.AlterID
		if ob.Security != "" {
			p["cipher"] = ob.Security
		} else {
			p["cipher"] = "auto"
		}
		applyTLS(p)
		applyTransport(p)
		return p
	case "trojan":
		if ob.Password == "" {
			return nil
		}
		p := base
		p["type"] = "trojan"
		p["password"] = ob.Password
		if ob.TLS.Enabled && ob.TLS.ServerName != "" {
			p["sni"] = ob.TLS.ServerName
		} else {
			p["sni"] = ob.Server
		}
		// trojan 默认走 TLS；若 tls.enabled 为 false 则不强制加 tls 标记
		//（mihomo trojan 始终 TLS，这里仅透传 sni 与 skip-cert-verify）
		if ob.TLS.Insecure {
			p["skip-cert-verify"] = true
		}
		applyTransport(p)
		return p
	case "shadowsocks":
		if ob.Method == "" || ob.Password == "" {
			return nil
		}
		p := base
		p["type"] = "shadowsocks"
		p["cipher"] = ob.Method
		p["password"] = ob.Password
		return p
	case "hysteria2":
		if ob.Password == "" {
			return nil
		}
		p := base
		p["type"] = "hysteria2"
		p["password"] = ob.Password
		if ob.TLS.Enabled && ob.TLS.ServerName != "" {
			p["sni"] = ob.TLS.ServerName
		}
		if ob.TLS.Insecure {
			p["skip-cert-verify"] = true
		}
		if ob.Obfs.Type != "" {
			p["obfs"] = ob.Obfs.Type
			if ob.Obfs.Password != "" {
				p["obfs-password"] = ob.Obfs.Password
			}
		}
		return p
	}
	return nil // direct / block / dns / wireguard 等暂不支持
}

func wsHost(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "host") {
			return v
		}
	}
	return ""
}
