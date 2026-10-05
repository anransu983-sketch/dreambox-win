package stats

// 流量总计：按 /traffic 采样到的瞬时速率 × 时间间隔 累加，
// 得到进程启动以来的总上传/总下载字节数。纯 Go，可单元测试。

import (
	"sync"
	"time"
)

// Total 累计器（线程安全）
type Total struct {
	mu   sync.Mutex
	up   int64
	down int64
}

// Add 按速率（字节/秒）与时间间隔累加
func (t *Total) Add(upRate, downRate int64, dt time.Duration) {
	if dt <= 0 {
		return
	}
	secs := dt.Seconds()
	t.mu.Lock()
	t.up += int64(float64(upRate) * secs)
	t.down += int64(float64(downRate) * secs)
	t.mu.Unlock()
}

// Get 返回累计的上/下行总字节数
func (t *Total) Get() (up, down int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.up, t.down
}

// Reset 清零
func (t *Total) Reset() {
	t.mu.Lock()
	t.up, t.down = 0, 0
	t.mu.Unlock()
}
