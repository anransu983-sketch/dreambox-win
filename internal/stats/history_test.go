package stats

// History 单元测试：小时桶累计、跨天切换、90 天清理、域名聚合、落盘回读。

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestHistory(t *testing.T) (*History, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "traffic_history.json")
	return NewHistory(path), path
}

func TestHistoryAddHourBucket(t *testing.T) {
	h, _ := newTestHistory(t)
	h.Add(100, 200, 10*time.Second) // 1000 up, 2000 down
	day := time.Now().Format("2006-01-02")
	hour := time.Now().Format("15")
	b := h.hours[day][hour]
	if b == nil || b.Up != 1000 || b.Down != 2000 {
		t.Fatalf("小时桶累计错误: %+v", b)
	}
	// 零间隔不累计
	h.Add(100, 200, 0)
	if b.Up != 1000 {
		t.Fatal("零间隔不应累计")
	}
}

func TestHistoryDayRollover(t *testing.T) {
	h, _ := newTestHistory(t)
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	h.mu.Lock()
	h.bucketFor(h.hours, yesterday, "23").Up = 500
	h.mu.Unlock()
	h.Add(10, 10, time.Second)
	today := time.Now().Format("2006-01-02")
	if _, ok := h.hours[today]; !ok {
		t.Fatal("今日桶未创建")
	}
	if h.hours[yesterday]["23"].Up != 500 {
		t.Fatal("昨日数据被破坏")
	}
}

func TestHistoryPrune90Days(t *testing.T) {
	h, _ := newTestHistory(t)
	oldDay := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	edgeDay := time.Now().AddDate(0, 0, -89).Format("2006-01-02")
	h.mu.Lock()
	h.bucketFor(h.hours, oldDay, "00").Up = 1
	h.bucketFor(h.hours, edgeDay, "00").Up = 2
	h.bucketFor(h.hosts, oldDay, "a.com").Up = 3
	h.pruneLocked()
	h.mu.Unlock()
	if _, ok := h.hours[oldDay]; ok {
		t.Fatal("100 天前的数据应被清理")
	}
	if _, ok := h.hosts[oldDay]; ok {
		t.Fatal("100 天前的域名数据应被清理")
	}
	if _, ok := h.hours[edgeDay]; !ok {
		t.Fatal("89 天前的数据应保留")
	}
}

func TestHistoryTopHosts(t *testing.T) {
	h, _ := newTestHistory(t)
	h.AddHost("b.com", 100, 100)   // total 200
	h.AddHost("a.com", 500, 1000)  // total 1500
	h.AddHost("", 9999, 9999)      // 空域名忽略
	for i := 0; i < 25; i++ {
		h.AddHost("x"+string(rune('a'+i))+".com", 1, 1)
	}
	top := h.TopHosts(1)
	if len(top) != 20 {
		t.Fatalf("应截断为 20，实际 %d", len(top))
	}
	if top[0].Host != "a.com" || top[0].Total != 1500 {
		t.Fatalf("排序错误，首位: %+v", top[0])
	}
	if top[1].Host != "b.com" {
		t.Fatalf("次位应为 b.com，实际 %+v", top[1])
	}
}

func TestHistorySaveLoad(t *testing.T) {
	h, path := newTestHistory(t)
	h.Add(50, 60, 2*time.Second)
	h.AddHost("example.com", 70, 80)
	if err := h.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("落盘文件不存在")
	}
	h2 := NewHistory(path)
	today := time.Now().Format("2006-01-02")
	if h2.hours[today] == nil {
		t.Fatal("回读后小时数据丢失")
	}
	if h2.hosts[today]["example.com"] == nil || h2.hosts[today]["example.com"].Up != 70 {
		t.Fatal("回读后域名数据丢失")
	}
}

func TestHistoryTodayWeek(t *testing.T) {
	h, _ := newTestHistory(t)
	h.Add(100, 200, 10*time.Second)
	today := h.Today()
	if len(today.Hours) != 24 {
		t.Fatalf("今日应有 24 个小时桶，实际 %d", len(today.Hours))
	}
	if today.Up != 1000 || today.Down != 2000 {
		t.Fatalf("今日总量错误: up=%d down=%d", today.Up, today.Down)
	}
	week := h.Week()
	if len(week) != 7 {
		t.Fatalf("本周应有 7 天，实际 %d", len(week))
	}
	if week[6].Date != today.Date || week[6].Up != 1000 {
		t.Fatalf("本周最后一天应为今天: %+v", week[6])
	}
}
