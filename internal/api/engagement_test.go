package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tutu-network/tutu/internal/app/engagement"
	"github.com/tutu-network/tutu/internal/infra/sqlite"
)

// ─── Engagement API Tests ───────────────────────────────────────────────────

func setupEngagementAPI(t *testing.T) (*EngagementAPI, *sqlite.DB) {
	t.Helper()
	db, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return &EngagementAPI{
		Streak:       engagement.NewStreakService(db),
		Level:        engagement.NewLevelService(db),
		Achievement:  engagement.NewAchievementService(db),
		Quest:        engagement.NewQuestService(db),
		Notification: engagement.NewNotificationService(db),
	}, db
}

func TestEngagementAPI_Streak(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/streak", nil)
	w := httptest.NewRecorder()
	api.HandleStreak(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["current_days"] != float64(0) {
		t.Errorf("expected current_days=0, got %v", resp["current_days"])
	}
	if resp["multiplier"] != float64(1.0) {
		t.Errorf("expected multiplier=1.0, got %v", resp["multiplier"])
	}
}

func TestEngagementAPI_Streak_WithData(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	// Record a contribution to create a streak
	today := time.Now().Truncate(24 * time.Hour)
	if err := api.Streak.RecordContribution(today); err != nil {
		t.Fatalf("record: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/streak", nil)
	w := httptest.NewRecorder()
	api.HandleStreak(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["current_days"] != float64(1) {
		t.Errorf("expected current_days=1, got %v", resp["current_days"])
	}
	if resp["bonus_percent"] != float64(5) {
		t.Errorf("expected bonus_percent=5, got %v", resp["bonus_percent"])
	}
}

func TestEngagementAPI_Level(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/level", nil)
	w := httptest.NewRecorder()
	api.HandleLevel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["level"] != float64(1) {
		t.Errorf("expected level=1, got %v", resp["level"])
	}
}

func TestEngagementAPI_Level_WithXP(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	// Add enough XP to reach level 2
	api.Level.AddXP(200, "TASK_COMPLETED")

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/level", nil)
	w := httptest.NewRecorder()
	api.HandleLevel(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	level := int(resp["level"].(float64))
	if level < 2 {
		t.Errorf("expected level >= 2 with 200 XP, got %d", level)
	}
}

func TestEngagementAPI_Achievements(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/achievements", nil)
	w := httptest.NewRecorder()
	api.HandleAchievements(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	totalCount := int(resp["total_count"].(float64))
	if totalCount != 25 {
		t.Errorf("expected 25 achievements, got %d", totalCount)
	}
	if resp["unlocked_count"] != float64(0) {
		t.Errorf("expected 0 unlocked, got %v", resp["unlocked_count"])
	}
}

func TestEngagementAPI_Quests(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/quests", nil)
	w := httptest.NewRecorder()
	api.HandleQuests(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	quests := resp["quests"]
	// May be nil/empty if no quests generated yet — that's fine
	if quests != nil {
		qs := quests.([]interface{})
		for _, q := range qs {
			qm := q.(map[string]interface{})
			if qm["id"] == nil || qm["type"] == nil {
				t.Error("quest missing required fields")
			}
		}
	}
}

func TestEngagementAPI_Notifications(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/notifications", nil)
	w := httptest.NewRecorder()
	api.HandleNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestEngagementAPI_Summary(t *testing.T) {
	api, _ := setupEngagementAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/engagement/summary", nil)
	w := httptest.NewRecorder()
	api.HandleSummary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Should have streak, level, achievements, active_quests, notifications sections
	for _, key := range []string{"streak", "level", "achievements", "active_quests", "notifications"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("summary missing key: %s", key)
		}
	}
}

func TestEngagementAPI_NilServices(t *testing.T) {
	// Verify graceful handling when services are nil
	api := &EngagementAPI{}

	endpoints := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"streak", api.HandleStreak},
		{"level", api.HandleLevel},
		{"achievements", api.HandleAchievements},
		{"quests", api.HandleQuests},
		{"notifications", api.HandleNotifications},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			ep.handler(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("expected 503, got %d", w.Code)
			}
		})
	}
}

// ─── Earnings Hub Tests ─────────────────────────────────────────────────────

func TestEarningsHub_BroadcastAndSubscribe(t *testing.T) {
	hub := NewEarningsHub()

	ch, unsub := hub.Subscribe()
	defer unsub()

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	event := EarningsEvent{
		Type:      "credit_earned",
		Amount:    2.4,
		TaskType:  "inference",
		Model:     "llama-3.2-7b",
		Timestamp: time.Now().Unix(),
	}
	hub.Broadcast(event)

	select {
	case data := <-ch:
		var received EarningsEvent
		if err := json.Unmarshal(data, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if received.Amount != 2.4 {
			t.Errorf("expected amount 2.4, got %f", received.Amount)
		}
		if received.Type != "credit_earned" {
			t.Errorf("expected type credit_earned, got %s", received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestEarningsHub_MultipleClients(t *testing.T) {
	hub := NewEarningsHub()

	ch1, unsub1 := hub.Subscribe()
	ch2, unsub2 := hub.Subscribe()
	defer unsub1()
	defer unsub2()

	if hub.ClientCount() != 2 {
		t.Errorf("expected 2 clients, got %d", hub.ClientCount())
	}

	hub.Broadcast(EarningsEvent{Type: "credit_earned", Amount: 1.0})

	// Both should receive
	select {
	case <-ch1:
	case <-time.After(time.Second):
		t.Error("client 1 timeout")
	}
	select {
	case <-ch2:
	case <-time.After(time.Second):
		t.Error("client 2 timeout")
	}
}

func TestEarningsHub_Unsubscribe(t *testing.T) {
	hub := NewEarningsHub()

	_, unsub := hub.Subscribe()
	if hub.ClientCount() != 1 {
		t.Errorf("expected 1, got %d", hub.ClientCount())
	}

	unsub()
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 after unsub, got %d", hub.ClientCount())
	}
}

func TestEarningsHub_SSE_Endpoint(t *testing.T) {
	hub := NewEarningsHub()

	// Start SSE handler in background
	server := httptest.NewServer(http.HandlerFunc(hub.HandleEarningsSSE))
	defer server.Close()

	// Connect as SSE client
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	// Broadcast an event
	hub.Broadcast(EarningsEvent{Type: "credit_earned", Amount: 5.5})

	// Read the SSE data
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	data := string(buf[:n])
	if len(data) == 0 {
		t.Fatal("expected SSE data")
	}
}

// ─── Path Extraction Tests ──────────────────────────────────────────────────

func TestExtractPathParam(t *testing.T) {
	tests := []struct {
		path   string
		after  string
		expect string
	}{
		{"/api/engagement/notifications/123/shown", "notifications", "123"},
		{"/api/engagement/notifications/456/shown", "notifications", "456"},
		{"/api/engagement/quests", "quests", ""},
		{"/api/engagement/streak", "engagement", "streak"},
	}
	for _, tt := range tests {
		got := extractPathParam(tt.path, tt.after)
		if got != tt.expect {
			t.Errorf("extractPathParam(%q, %q) = %q, want %q", tt.path, tt.after, got, tt.expect)
		}
	}
}
