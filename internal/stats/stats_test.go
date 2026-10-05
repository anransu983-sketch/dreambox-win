package stats

import (
	"testing"
	"time"
)

func TestAddAndGet(t *testing.T) {
	var s Total
	s.Add(1000, 2000, time.Second)
	up, down := s.Get()
	if up != 1000 || down != 2000 {
		t.Fatalf("1s: got up=%d down=%d, want 1000/2000", up, down)
	}
	s.Add(500, 500, 2*time.Second)
	up, down = s.Get()
	if up != 2000 || down != 3000 {
		t.Fatalf("累计: got up=%d down=%d, want 2000/3000", up, down)
	}
}

func TestAddZeroDuration(t *testing.T) {
	var s Total
	s.Add(1000, 1000, 0)
	s.Add(1000, 1000, -time.Second)
	if up, down := s.Get(); up != 0 || down != 0 {
		t.Fatalf("零/负时长不应累加: up=%d down=%d", up, down)
	}
}

func TestReset(t *testing.T) {
	var s Total
	s.Add(1000, 2000, time.Second)
	s.Reset()
	if up, down := s.Get(); up != 0 || down != 0 {
		t.Fatalf("Reset 后应为 0: up=%d down=%d", up, down)
	}
}

func TestFractional(t *testing.T) {
	var s Total
	s.Add(1000, 0, 500*time.Millisecond)
	if up, _ := s.Get(); up != 500 {
		t.Fatalf("0.5s: got up=%d, want 500", up)
	}
}
