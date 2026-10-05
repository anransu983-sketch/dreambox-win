package mihomo

// 实时流量采样：长连内核 /traffic websocket，缓存最新上下行速率，
// Web UI 通过 /api/traffic 轮询读取。

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"dreambox-win/internal/stats"
)

var (
	trafficMu     sync.RWMutex
	trafficUp     int64
	trafficDown   int64
	trafficAt     time.Time
	samplerOnce   sync.Once
	trafficTotal  stats.Total
)

// StartTrafficSampler 启动后台采样（只启动一次）
func StartTrafficSampler() {
	samplerOnce.Do(func() { go samplerLoop() })
}

func samplerLoop() {
	for {
		if !Online() {
			time.Sleep(3 * time.Second)
			continue
		}
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:9090/traffic", nil)
			if err != nil {
				return
			}
			defer c.Close(websocket.StatusNormalClosure, "")
			lastMsgAt := time.Now()
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					return
				}
				var v struct {
					Up   int64 `json:"up"`
					Down int64 `json:"down"`
				}
				if err := json.Unmarshal(data, &v); err != nil {
					continue
				}
				now := time.Now()
				dt := now.Sub(lastMsgAt)
				lastMsgAt = now
				if dt > 10*time.Second {
					dt = 10 * time.Second // 重连/卡顿保护，避免异常大间隔
				}
				trafficTotal.Add(v.Up, v.Down, dt)
				stats.RecordTraffic(v.Up, v.Down, dt)
				trafficMu.Lock()
				trafficUp, trafficDown, trafficAt = v.Up, v.Down, now
				trafficMu.Unlock()
			}
		}()
		time.Sleep(2 * time.Second)
	}
}

// TrafficSnapshot 返回最近一次采样的上下行速率（字节/秒）与采样时间
func TrafficSnapshot() (up, down int64, at time.Time) {
	trafficMu.RLock()
	defer trafficMu.RUnlock()
	return trafficUp, trafficDown, trafficAt
}

// TrafficTotal 返回进程启动以来累计的总上传/总下载字节数
func TrafficTotal() (up, down int64) {
	return trafficTotal.Get()
}
