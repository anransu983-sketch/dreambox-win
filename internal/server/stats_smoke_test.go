package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"dreambox-win/internal/stats"
)

func TestStatsAPIEndpoints(t *testing.T) {
	stats.StartRecorder(t.TempDir() + "/h.json")
	stats.RecordTraffic(100, 200, 10*time.Second)
	stats.RecordHost("example.com", 500, 1500)
	a := &App{}
	for _, tc := range []struct{ path, key string }{
		{"/api/stats/today", "hours"}, {"/api/stats/week", "days"}, {"/api/stats/hosts?days=7", "hosts"},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		w := httptest.NewRecorder()
		switch tc.path {
		case "/api/stats/today":
			a.handleStatsToday(w, r)
		case "/api/stats/week":
			a.handleStatsWeek(w, r)
		default:
			a.handleStatsHosts(w, r)
		}
		var v map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatalf("%s: bad json: %v", tc.path, err)
		}
		if v["ok"] != true {
			t.Fatalf("%s: ok != true", tc.path)
		}
		if _, ok := v[tc.key]; !ok {
			t.Fatalf("%s: missing key %s", tc.path, tc.key)
		}
	}
	// today totals
	r := httptest.NewRequest("GET", "/api/stats/today", nil)
	w := httptest.NewRecorder()
	a.handleStatsToday(w, r)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	tot := v["total"].(map[string]any)
	if tot["up"].(float64) != 1000 || tot["down"].(float64) != 2000 {
		t.Fatalf("today total wrong: %v", tot)
	}
	hosts := v["hours"].([]any)
	if len(hosts) != 24 {
		t.Fatalf("hours len = %d", len(hosts))
	}
	r2 := httptest.NewRequest("GET", "/api/stats/hosts?days=7", nil)
	w2 := httptest.NewRecorder()
	a.handleStatsHosts(w2, r2)
	var v2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &v2)
	hs := v2["hosts"].([]any)
	if len(hs) != 1 || hs[0].(map[string]any)["host"] != "example.com" {
		t.Fatalf("hosts wrong: %v", hs)
	}
}
