package mihomo

// 目标域名流量采样：每 30s 轮询内核 /connections，按连接 ID 算增量，
// 聚合到目标域名（metadata.host，无 host 则用目标 IP）。纯 Go。

import (
	"encoding/json"
	"sync"
	"time"

	"dreambox-win/internal/paths"
	"dreambox-win/internal/stats"
)

// Connection 内核活跃连接（精简字段）
type Connection struct {
	ID       string
	Host     string
	Upload   int64
	Download int64
}

// Connections 返回当前全部活跃连接
func Connections() ([]Connection, error) {
	resp, err := httpClient.Get(ctrlURL("/connections"))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		Connections []struct {
			ID       string `json:"id"`
			Upload   int64  `json:"upload"`
			Download int64  `json:"download"`
			Metadata struct {
				Host          string `json:"host"`
				DestinationIP string `json:"destinationIP"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(v.Connections))
	for _, c := range v.Connections {
		host := c.Metadata.Host
		if host == "" {
			host = c.Metadata.DestinationIP
		}
		out = append(out, Connection{ID: c.ID, Host: host, Upload: c.Upload, Download: c.Download})
	}
	return out, nil
}

var hostSamplerOnce sync.Once

// StartHostSampler 启动域名采样（只启动一次）：每 30s 轮询，按连接 ID 算增量聚合
func StartHostSampler() {
	hostSamplerOnce.Do(func() { go hostSamplerLoop() })
}

func hostSamplerLoop() {
	last := map[string][2]int64{} // 连接 ID -> {upload, download}
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		if !Online() {
			continue
		}
		conns, err := Connections()
		if err != nil {
			continue
		}
		seen := make(map[string]bool, len(conns))
		for _, c := range conns {
			seen[c.ID] = true
			prev := last[c.ID]
			du := c.Upload - prev[0]
			dd := c.Download - prev[1]
			if du < 0 {
				du = 0 // 计数器重置保护
			}
			if dd < 0 {
				dd = 0
			}
			if du > 0 || dd > 0 {
				stats.RecordHost(c.Host, du, dd)
			}
			last[c.ID] = [2]int64{c.Upload, c.Download}
		}
		for id := range last {
			if !seen[id] {
				delete(last, id) // 已关闭连接清理
			}
		}
	}
}

// StartHistory 启动流量历史记录：小时桶落盘（90 天）+ 域名聚合采样
func StartHistory() {
	stats.StartRecorder(paths.HistoryFile())
	StartHostSampler()
}
