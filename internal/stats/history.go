package stats

// 流量历史：按小时桶累计上/下行字节落盘（保留 90 天），
// 按目标域名按日聚合（Top 排行用）。纯 Go，可单元测试。
//
// 落盘格式（%APPDATA%\DreamBox\traffic_history.json）：
// {"hours":{"2026-10-06":{"00":{"up":1,"down":2}}},"hosts":{"2026-10-06":{"example.com":{"up":1,"down":2}}}}

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// retentionDays 历史保留天数
const retentionDays = 90

type bucket struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type historyFile struct {
	Hours map[string]map[string]*bucket `json:"hours"`
	Hosts map[string]map[string]*bucket `json:"hosts"`
}

// HourStat 单小时统计（API 输出）
type HourStat struct {
	H string `json:"h"`
	Up int64 `json:"up"`
	Down int64 `json:"down"`
}

// DayStat 单日统计（API 输出）
type DayStat struct {
	Date  string     `json:"date"`
	Up    int64      `json:"up"`
	Down  int64      `json:"down"`
	Hours []HourStat `json:"hours,omitempty"`
}

// HostStat 域名统计（API 输出）
type HostStat struct {
	Host  string `json:"host"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Total int64  `json:"total"`
}

// History 流量历史记录器（线程安全）
type History struct {
	mu    sync.Mutex
	path  string
	hours map[string]map[string]*bucket
	hosts map[string]map[string]*bucket
}

// NewHistory 新建记录器并从 path 加载（文件不存在则从空开始）
func NewHistory(path string) *History {
	h := &History{
		path:  path,
		hours: map[string]map[string]*bucket{},
		hosts: map[string]map[string]*bucket{},
	}
	h.load()
	return h
}

// bucketFor 取（或创建）某天的某小时桶
func (h *History) bucketFor(m map[string]map[string]*bucket, day, key string) *bucket {
	dm, ok := m[day]
	if !ok {
		dm = map[string]*bucket{}
		m[day] = dm
	}
	b, ok := dm[key]
	if !ok {
		b = &bucket{}
		dm[key] = b
	}
	return b
}

// Add 按瞬时速率（字节/秒）与时间间隔累加到当前小时桶
func (h *History) Add(upRate, downRate int64, dt time.Duration) {
	if dt <= 0 {
		return
	}
	secs := dt.Seconds()
	up := int64(float64(upRate) * secs)
	down := int64(float64(downRate) * secs)
	if up == 0 && down == 0 {
		return
	}
	now := time.Now()
	h.mu.Lock()
	b := h.bucketFor(h.hours, now.Format("2006-01-02"), now.Format("15"))
	b.Up += up
	b.Down += down
	h.mu.Unlock()
}

// AddHost 按目标域名累计（按日聚合）
func (h *History) AddHost(host string, up, down int64) {
	if host == "" || (up == 0 && down == 0) {
		return
	}
	day := time.Now().Format("2006-01-02")
	h.mu.Lock()
	b := h.bucketFor(h.hosts, day, host)
	b.Up += up
	b.Down += down
	h.mu.Unlock()
}

// Save 落盘（原子写：临时文件 + 改名）
func (h *History) Save() error {
	h.mu.Lock()
	h.pruneLocked()
	data := historyFile{Hours: h.hours, Hosts: h.hosts}
	h.mu.Unlock()
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, h.path)
}

func (h *History) load() {
	raw, err := os.ReadFile(h.path)
	if err != nil {
		return
	}
	var data historyFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	if data.Hours != nil {
		h.hours = data.Hours
	}
	if data.Hosts != nil {
		h.hosts = data.Hosts
	}
	h.pruneLocked()
}

// pruneLocked 删除超过保留天数的数据（调用方持锁）
func (h *History) pruneLocked() {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	for day := range h.hours {
		if day < cutoff {
			delete(h.hours, day)
		}
	}
	for day := range h.hosts {
		if day < cutoff {
			delete(h.hosts, day)
		}
	}
}

// Today 返回今日统计（含 24 小时桶）
func (h *History) Today() DayStat {
	day := time.Now().Format("2006-01-02")
	h.mu.Lock()
	defer h.mu.Unlock()
	st := DayStat{Date: day, Hours: make([]HourStat, 24)}
	dm := h.hours[day]
	for i := 0; i < 24; i++ {
		hs := HourStat{H: hourLabel(i)}
		if dm != nil {
			if b, ok := dm[hourKey(i)]; ok && b != nil {
				hs.Up, hs.Down = b.Up, b.Down
			}
		}
		st.Hours[i] = hs
		st.Up += hs.Up
		st.Down += hs.Down
	}
	return st
}

// Week 返回最近 7 天（含今天）的按日统计
func (h *History) Week() []DayStat {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	days := make([]DayStat, 7)
	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -(6 - i))
		day := d.Format("2006-01-02")
		st := DayStat{Date: day}
		for _, b := range h.hours[day] {
			if b != nil {
				st.Up += b.Up
				st.Down += b.Down
			}
		}
		days[i] = st
	}
	return days
}

// TopHosts 返回最近 days 天的目标域名 Top（按总量倒序，最多 20）
func (h *History) TopHosts(days int) []HostStat {
	if days < 1 {
		days = 1
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	agg := map[string]*HostStat{}
	for i := 0; i < days; i++ {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		for host, b := range h.hosts[day] {
			if b == nil {
				continue
			}
			s, ok := agg[host]
			if !ok {
				s = &HostStat{Host: host}
				agg[host] = s
			}
			s.Up += b.Up
			s.Down += b.Down
		}
	}
	out := make([]HostStat, 0, len(agg))
	for _, s := range agg {
		s.Total = s.Up + s.Down
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func hourKey(i int) string { return fmt.Sprintf("%02d", i) }

func hourLabel(i int) string { return fmt.Sprintf("%02d:00", i) }

// ---- 进程级单例：采样循环调用 ----

var (
	recorderMu sync.Mutex
	recorder   *History
	recOnce    sync.Once
)

// StartRecorder 启动历史记录器（只启动一次）：加载落盘数据，每 5 分钟自动保存
func StartRecorder(path string) {
	recOnce.Do(func() {
		recorderMu.Lock()
		recorder = NewHistory(path)
		rec := recorder
		recorderMu.Unlock()
		go func() {
			t := time.NewTicker(5 * time.Minute)
			defer t.Stop()
			for range t.C {
				_ = rec.Save()
			}
		}()
	})
}

// Recorder 返回记录器（未启动时为 nil）
func Recorder() *History {
	recorderMu.Lock()
	defer recorderMu.Unlock()
	return recorder
}

// RecordTraffic 记录一次流量采样（小时桶累计）
func RecordTraffic(upRate, downRate int64, dt time.Duration) {
	if r := Recorder(); r != nil {
		r.Add(upRate, downRate, dt)
	}
}

// RecordHost 记录域名流量增量
func RecordHost(host string, up, down int64) {
	if r := Recorder(); r != nil {
		r.AddHost(host, up, down)
	}
}
