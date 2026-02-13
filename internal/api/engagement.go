package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/tutu-network/tutu/internal/app/engagement"
	"github.com/tutu-network/tutu/internal/domain"
)

// ─── Engagement API ─────────────────────────────────────────────────────────
// Architecture Part XIII: REST endpoints for the desktop UI and CLI to
// display streaks, levels, achievements, quests, and notifications.
//
// GET /api/engagement/streak       — current streak + multiplier
// GET /api/engagement/level        — level, XP, progress, unlocks
// GET /api/engagement/achievements — all achievements (locked + unlocked)
// GET /api/engagement/quests       — active weekly quests
// GET /api/engagement/notifications — pending notifications
// POST /api/engagement/notifications/{id}/shown — mark notification shown
// GET /api/engagement/summary      — full engagement dashboard snapshot

// EngagementAPI holds references to all engagement services.
type EngagementAPI struct {
	Streak       *engagement.StreakService
	Level        *engagement.LevelService
	Achievement  *engagement.AchievementService
	Quest        *engagement.QuestService
	Notification *engagement.NotificationService
}

// HandleStreak returns the current streak data.
// GET /api/engagement/streak
func (e *EngagementAPI) HandleStreak(w http.ResponseWriter, r *http.Request) {
	if e.Streak == nil {
		writeError(w, http.StatusServiceUnavailable, "engagement not initialized")
		return
	}

	streak, err := e.Streak.CurrentStreak()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	mult := e.Streak.CreditMultiplier()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_days":  streak.CurrentDays,
		"longest_days":  streak.LongestDays,
		"last_date":     streak.LastDate.Format(time.DateOnly),
		"freeze_used":   streak.FreezeUsed,
		"multiplier":    mult,
		"bonus_percent": int((mult - 1.0) * 100),
	})
}

// HandleLevel returns current level and XP progress.
// GET /api/engagement/level
func (e *EngagementAPI) HandleLevel(w http.ResponseWriter, r *http.Request) {
	if e.Level == nil {
		writeError(w, http.StatusServiceUnavailable, "engagement not initialized")
		return
	}

	lvl, err := e.Level.CurrentLevel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pct, err := e.Level.ProgressPct()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	toNext, err := e.Level.XPToNextLevel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	unlocks := engagement.UnlocksForLevel(lvl.Level)
	nextUnlocks := engagement.UnlocksForLevel(lvl.Level + 1)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"level":        lvl.Level,
		"current_xp":   lvl.CurrentXP,
		"xp_to_next":   toNext,
		"progress_pct": pct,
		"unlocks":      unlocks,
		"next_unlocks": nextUnlocks,
	})
}

// HandleAchievements returns all achievements with unlock status.
// GET /api/engagement/achievements
func (e *EngagementAPI) HandleAchievements(w http.ResponseWriter, r *http.Request) {
	if e.Achievement == nil {
		writeError(w, http.StatusServiceUnavailable, "engagement not initialized")
		return
	}

	defs := e.Achievement.Definitions()
	unlocked, err := e.Achievement.ListUnlocked()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build a map for quick lookup
	unlockedMap := make(map[string]domain.UnlockedAchievement)
	for _, u := range unlocked {
		unlockedMap[u.ID] = u
	}

	type achievementResponse struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Category   string `json:"category"`
		Icon       string `json:"icon"`
		RewardXP   int64  `json:"reward_xp"`
		RewardCr   int64  `json:"reward_cr"`
		Unlocked   bool   `json:"unlocked"`
		UnlockedAt string `json:"unlocked_at,omitempty"`
	}

	var all []achievementResponse
	for _, def := range defs {
		a := achievementResponse{
			ID:       def.ID,
			Name:     def.Name,
			Category: string(def.Category),
			Icon:     def.Icon,
			RewardXP: def.RewardXP,
			RewardCr: def.RewardCr,
		}
		if u, ok := unlockedMap[def.ID]; ok {
			a.Unlocked = true
			a.UnlockedAt = u.UnlockedAt.Format(time.RFC3339)
		}
		all = append(all, a)
	}

	count, _ := e.Achievement.UnlockedCount()
	total := e.Achievement.TotalCount()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"achievements":   all,
		"unlocked_count": count,
		"total_count":    total,
		"completion_pct": float64(count) / float64(total) * 100,
	})
}

// HandleQuests returns active weekly quests.
// GET /api/engagement/quests
func (e *EngagementAPI) HandleQuests(w http.ResponseWriter, r *http.Request) {
	if e.Quest == nil {
		writeError(w, http.StatusServiceUnavailable, "engagement not initialized")
		return
	}

	quests, err := e.Quest.ActiveQuests()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type questResponse struct {
		ID            string  `json:"id"`
		Type          string  `json:"type"`
		Description   string  `json:"description"`
		Target        int     `json:"target"`
		Progress      int     `json:"progress"`
		ProgressPct   float64 `json:"progress_pct"`
		RewardXP      int64   `json:"reward_xp"`
		RewardCredits int64   `json:"reward_credits"`
		ExpiresAt     string  `json:"expires_at"`
		Completed     bool    `json:"completed"`
	}

	var out []questResponse
	for _, q := range quests {
		out = append(out, questResponse{
			ID:            q.ID,
			Type:          string(q.Type),
			Description:   q.Description,
			Target:        q.Target,
			Progress:      q.Progress,
			ProgressPct:   q.ProgressPct(),
			RewardXP:      q.RewardXP,
			RewardCredits: q.RewardCredits,
			ExpiresAt:     q.ExpiresAt.Format(time.RFC3339),
			Completed:     q.Completed,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"quests": out,
	})
}

// HandleNotifications returns pending notifications.
// GET /api/engagement/notifications
func (e *EngagementAPI) HandleNotifications(w http.ResponseWriter, r *http.Request) {
	if e.Notification == nil {
		writeError(w, http.StatusServiceUnavailable, "engagement not initialized")
		return
	}

	pending, err := e.Notification.Pending(20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": pending,
	})
}

// HandleNotificationShown marks a notification as shown.
// POST /api/engagement/notifications/{id}/shown
func (e *EngagementAPI) HandleNotificationShown(w http.ResponseWriter, r *http.Request) {
	if e.Notification == nil {
		writeError(w, http.StatusServiceUnavailable, "engagement not initialized")
		return
	}

	// Extract ID from URL path — last segment before /shown
	// Path: /api/engagement/notifications/123/shown
	idStr := extractPathParam(r.URL.Path, "notifications")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	if err := e.Notification.MarkShown(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
	})
}

// HandleSummary returns a full engagement dashboard snapshot.
// GET /api/engagement/summary
func (e *EngagementAPI) HandleSummary(w http.ResponseWriter, r *http.Request) {
	summary := make(map[string]interface{})

	// Streak
	if e.Streak != nil {
		streak, err := e.Streak.CurrentStreak()
		if err == nil {
			summary["streak"] = map[string]interface{}{
				"current_days": streak.CurrentDays,
				"longest_days": streak.LongestDays,
				"multiplier":   e.Streak.CreditMultiplier(),
			}
		}
	}

	// Level
	if e.Level != nil {
		lvl, err := e.Level.CurrentLevel()
		if err == nil {
			pct, _ := e.Level.ProgressPct()
			summary["level"] = map[string]interface{}{
				"level":        lvl.Level,
				"current_xp":   lvl.CurrentXP,
				"progress_pct": pct,
			}
		}
	}

	// Achievements
	if e.Achievement != nil {
		count, _ := e.Achievement.UnlockedCount()
		total := e.Achievement.TotalCount()
		summary["achievements"] = map[string]interface{}{
			"unlocked": count,
			"total":    total,
		}
	}

	// Quests
	if e.Quest != nil {
		active, _ := e.Quest.ActiveQuests()
		summary["active_quests"] = len(active)
	}

	// Notifications
	if e.Notification != nil {
		todayCount, _ := e.Notification.TodayCount()
		policy := e.Notification.Policy()
		summary["notifications"] = map[string]interface{}{
			"today_count": todayCount,
			"max_per_day": policy.MaxPerDay,
		}
	}

	writeJSON(w, http.StatusOK, summary)
}

// extractPathParam extracts a parameter value from a URL path after a given segment.
// For /api/engagement/notifications/123/shown, extractPathParam(path, "notifications") = "123".
func extractPathParam(path, after string) string {
	parts := splitPath(path)
	for i, p := range parts {
		if p == after && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// ─── Live Earnings WebSocket ────────────────────────────────────────────────
// Architecture Part XIII #5: Live earnings feed ("The Mining Screen").
// Delivered via WebSocket: {type: "credit_earned", amount: 2.4, task_type: "inference"}

// EarningsHub manages WebSocket connections for live earnings feed.
type EarningsHub struct {
	clients map[chan []byte]struct{}
}

// NewEarningsHub creates a new earnings broadcast hub.
func NewEarningsHub() *EarningsHub {
	return &EarningsHub{
		clients: make(map[chan []byte]struct{}),
	}
}

// Broadcast sends an earnings event to all connected clients.
func (h *EarningsHub) Broadcast(event EarningsEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
			// Client too slow — drop message
		}
	}
}

// Subscribe registers a new client. Returns the channel and an unsubscribe func.
func (h *EarningsHub) Subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 32)
	h.clients[ch] = struct{}{}
	return ch, func() {
		delete(h.clients, ch)
		close(ch)
	}
}

// ClientCount returns the number of connected clients.
func (h *EarningsHub) ClientCount() int {
	return len(h.clients)
}

// EarningsEvent represents a single credit earning event.
type EarningsEvent struct {
	Type      string  `json:"type"`      // "credit_earned"
	Amount    float64 `json:"amount"`    // Credits earned
	TaskType  string  `json:"task_type"` // "inference", "embedding", "training"
	Model     string  `json:"model"`     // Model used
	Timestamp int64   `json:"timestamp"` // Unix epoch
}

// HandleEarningsSSE serves the live earnings feed via Server-Sent Events.
// GET /api/earnings/live
// Uses SSE instead of WebSocket for simplicity and HTTP/2 compatibility.
func (h *EarningsHub) HandleEarningsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ch, unsub := h.Subscribe()
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
