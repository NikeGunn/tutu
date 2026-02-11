package reputation

import (
	"math"
	"testing"
	"time"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newTestTracker(t *testing.T) *Tracker {
	t.Helper()
	tr := NewTracker(DefaultTrackerConfig())
	tr.now = func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	}
	return tr
}

func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// ─── Registration Tests ────────────────────────────────────────────────────

func TestRegister(t *testing.T) {
	tr := newTestTracker(t)

	rep := tr.Register("node-1")
	if rep.NodeID != "node-1" {
		t.Errorf("nodeID = %q, want %q", rep.NodeID, "node-1")
	}
	if rep.Components.Reliability != DefaultReputation {
		t.Errorf("reliability = %f, want %f", rep.Components.Reliability, DefaultReputation)
	}
	if rep.Components.Longevity != 0 {
		t.Errorf("longevity = %f, want 0", rep.Components.Longevity)
	}
	if rep.TaskCount != 0 {
		t.Errorf("task count = %d, want 0", rep.TaskCount)
	}
}

func TestRegister_Idempotent(t *testing.T) {
	tr := newTestTracker(t)

	first := tr.Register("node-1")
	second := tr.Register("node-1")
	if first != second {
		t.Error("Register should return existing node, not create duplicate")
	}
}

func TestGetOrRegister(t *testing.T) {
	tr := newTestTracker(t)

	// Not registered yet — should auto-register
	rep := tr.GetOrRegister("node-new")
	if rep == nil {
		t.Fatal("GetOrRegister returned nil")
	}
	if tr.NodeCount() != 1 {
		t.Errorf("node count = %d, want 1", tr.NodeCount())
	}

	// Already registered — should return existing
	rep2 := tr.GetOrRegister("node-new")
	if rep != rep2 {
		t.Error("GetOrRegister returned different pointer for existing node")
	}
}

func TestGet_NotRegistered(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Get("node-nonexistent")
	if rep != nil {
		t.Error("Get should return nil for unregistered node")
	}
}

// ─── Overall Score Tests ───────────────────────────────────────────────────

func TestOverall_DefaultScore(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Register("node-1")

	// Default: 0.30×0.5 + 0.25×0.5 + 0.20×0.5 + 0.15×0.5 + 0.10×0.0 - 0.05×0.0
	// = 0.15 + 0.125 + 0.10 + 0.075 + 0.0 = 0.45
	expected := 0.45
	if !almostEqual(rep.Overall(), expected, 0.001) {
		t.Errorf("overall = %f, want %f", rep.Overall(), expected)
	}
}

func TestOverall_Clamped(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Register("node-1")

	// Set everything to max
	rep.Components = Components{
		Reliability:  1.0,
		Accuracy:     1.0,
		Availability: 1.0,
		Speed:        1.0,
		Longevity:    1.0,
	}
	if rep.Overall() > CeilingReputation {
		t.Errorf("overall %f exceeded ceiling %f", rep.Overall(), CeilingReputation)
	}

	// Massive penalties
	rep.Penalties = 100.0
	if rep.Overall() < FloorReputation {
		t.Errorf("overall %f below floor %f", rep.Overall(), FloorReputation)
	}
}

// ─── Trust Tier Tests ──────────────────────────────────────────────────────

func TestTrustTier(t *testing.T) {
	tests := []struct {
		components Components
		penalties  float64
		wantTier   string
	}{
		{Components{1.0, 1.0, 1.0, 1.0, 1.0}, 0, "EXCELLENT"},
		{Components{0.8, 0.8, 0.8, 0.8, 0.5}, 0, "GOOD"},
		{Components{0.5, 0.5, 0.5, 0.5, 0.5}, 0, "NEUTRAL"},
		{Components{0.35, 0.35, 0.35, 0.35, 0.35}, 0, "LOW"},
		{Components{0.1, 0.1, 0.1, 0.1, 0.1}, 0, "POOR"},
	}

	for _, tt := range tests {
		rep := &NodeReputation{Components: tt.components, Penalties: tt.penalties}
		got := rep.TrustTier()
		if got != tt.wantTier {
			t.Errorf("TrustTier() with overall=%.2f: got %q, want %q",
				rep.Overall(), got, tt.wantTier)
		}
	}
}

// ─── EMA Alpha Tests ───────────────────────────────────────────────────────

func TestAlpha_ColdStart(t *testing.T) {
	rep := &NodeReputation{TaskCount: 0}
	if rep.alpha() != AlphaColdStart {
		t.Errorf("alpha for cold start = %f, want %f", rep.alpha(), AlphaColdStart)
	}

	rep.TaskCount = ColdStartTasks - 1
	if rep.alpha() != AlphaColdStart {
		t.Errorf("alpha at %d tasks = %f, want %f", rep.TaskCount, rep.alpha(), AlphaColdStart)
	}

	rep.TaskCount = ColdStartTasks
	if rep.alpha() != AlphaNormal {
		t.Errorf("alpha at %d tasks = %f, want %f", rep.TaskCount, rep.alpha(), AlphaNormal)
	}
}

// ─── RecordTask Tests ──────────────────────────────────────────────────────

func TestRecordTask_SuccessfulFast(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Register("node-1")
	initialRel := rep.Components.Reliability

	err := tr.RecordTask("node-1", TaskOutcome{
		Successful:     true,
		ResultVerified: true,
		ExpectedTime:   100 * time.Millisecond,
		ActualTime:     50 * time.Millisecond, // Faster than expected
	})
	if err != nil {
		t.Fatalf("RecordTask failed: %v", err)
	}

	// Reliability should increase with EMA(old, 1.0, α=0.3)
	if rep.Components.Reliability <= initialRel {
		t.Errorf("reliability should increase after success: was %f, now %f",
			initialRel, rep.Components.Reliability)
	}
	if rep.TaskCount != 1 {
		t.Errorf("task count = %d, want 1", rep.TaskCount)
	}
}

func TestRecordTask_Failed(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Register("node-1")
	initialRel := rep.Components.Reliability

	err := tr.RecordTask("node-1", TaskOutcome{
		Successful:   false,
		ExpectedTime: 100 * time.Millisecond,
		ActualTime:   200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("RecordTask failed: %v", err)
	}

	if rep.Components.Reliability >= initialRel {
		t.Errorf("reliability should decrease after failure: was %f, now %f",
			initialRel, rep.Components.Reliability)
	}
}

func TestRecordTask_NotRegistered(t *testing.T) {
	tr := newTestTracker(t)
	err := tr.RecordTask("node-ghost", TaskOutcome{})
	if err == nil {
		t.Fatal("expected error for unregistered node")
	}
}

// ─── Availability Tests ────────────────────────────────────────────────────

func TestRecordAvailability(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Register("node-1")

	// Record several "online" checks → availability should increase
	for i := 0; i < 5; i++ {
		tr.RecordAvailability("node-1", AvailabilityCheck{WasOnline: true})
	}

	if rep.Components.Availability <= DefaultReputation {
		t.Errorf("availability should increase, got %f", rep.Components.Availability)
	}
}

func TestRecordAvailability_NotRegistered(t *testing.T) {
	tr := newTestTracker(t)
	err := tr.RecordAvailability("node-ghost", AvailabilityCheck{WasOnline: true})
	if err == nil {
		t.Fatal("expected error for unregistered node")
	}
}

// ─── Penalty Tests ─────────────────────────────────────────────────────────

func TestRecordPenalty(t *testing.T) {
	tr := newTestTracker(t)
	rep := tr.Register("node-bad")

	err := tr.RecordPenalty("node-bad", PenaltyEvent{Severity: 0.5, Reason: "cheating"})
	if err != nil {
		t.Fatalf("RecordPenalty failed: %v", err)
	}
	if rep.Penalties != 0.5 {
		t.Errorf("penalties = %f, want 0.5", rep.Penalties)
	}

	// Add another penalty → cumulative
	tr.RecordPenalty("node-bad", PenaltyEvent{Severity: 1.0, Reason: "repeat offender"})
	if rep.Penalties != 1.5 {
		t.Errorf("penalties = %f, want 1.5", rep.Penalties)
	}
}

func TestRecordPenalty_NotRegistered(t *testing.T) {
	tr := newTestTracker(t)
	err := tr.RecordPenalty("node-ghost", PenaltyEvent{Severity: 1.0})
	if err == nil {
		t.Fatal("expected error for unregistered node")
	}
}

// ─── Decay Tests ────────────────────────────────────────────────────────────

func TestApplyDecay(t *testing.T) {
	tr := newTestTracker(t)
	startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return startTime }

	rep := tr.Register("node-1")
	// Set high reputation
	rep.Components.Reliability = 0.9

	// Advance 2 weeks without activity
	tr.now = func() time.Time { return startTime.Add(14 * 24 * time.Hour) }

	decayed := tr.ApplyDecay()
	if decayed != 1 {
		t.Errorf("decayed count = %d, want 1", decayed)
	}
	if rep.Components.Reliability >= 0.9 {
		t.Errorf("reliability should decay, still %f", rep.Components.Reliability)
	}
}

func TestApplyDecay_RecentActivity(t *testing.T) {
	tr := newTestTracker(t)
	startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	tr.now = func() time.Time { return startTime }

	rep := tr.Register("node-1")
	rep.Components.Reliability = 0.9

	// Only 3 days later — no decay yet (< 1 week)
	tr.now = func() time.Time { return startTime.Add(3 * 24 * time.Hour) }

	decayed := tr.ApplyDecay()
	if decayed != 0 {
		t.Errorf("decayed count = %d, want 0 (recent activity)", decayed)
	}
}

// ─── Query Tests ────────────────────────────────────────────────────────────

func TestTopNodes(t *testing.T) {
	tr := newTestTracker(t)

	// Register 3 nodes with different reputation levels
	rep1 := tr.Register("node-low")
	rep1.Components.Reliability = 0.2

	rep2 := tr.Register("node-mid")
	rep2.Components.Reliability = 0.6

	rep3 := tr.Register("node-high")
	rep3.Components.Reliability = 0.95

	top := tr.TopNodes(2)
	if len(top) != 2 {
		t.Fatalf("top count = %d, want 2", len(top))
	}
	// Highest first
	if top[0].NodeID != "node-high" {
		t.Errorf("top[0] = %s, want node-high", top[0].NodeID)
	}
}

func TestTrustedNodes(t *testing.T) {
	tr := newTestTracker(t)

	rep1 := tr.Register("node-good")
	rep1.Components = Components{0.9, 0.9, 0.9, 0.9, 0.9}

	rep2 := tr.Register("node-bad")
	rep2.Components = Components{0.1, 0.1, 0.1, 0.1, 0.1}

	trusted := tr.TrustedNodes(0.7)
	if len(trusted) != 1 {
		t.Fatalf("trusted count = %d, want 1", len(trusted))
	}
	if trusted[0].NodeID != "node-good" {
		t.Errorf("trusted node = %s, want node-good", trusted[0].NodeID)
	}
}

func TestNodeCount(t *testing.T) {
	tr := newTestTracker(t)
	if tr.NodeCount() != 0 {
		t.Errorf("initial count = %d, want 0", tr.NodeCount())
	}
	tr.Register("node-1")
	tr.Register("node-2")
	if tr.NodeCount() != 2 {
		t.Errorf("count = %d, want 2", tr.NodeCount())
	}
}

func TestRemove(t *testing.T) {
	tr := newTestTracker(t)
	tr.Register("node-1")
	tr.Remove("node-1")
	if tr.NodeCount() != 0 {
		t.Errorf("count after remove = %d, want 0", tr.NodeCount())
	}
	if tr.Get("node-1") != nil {
		t.Error("node still accessible after remove")
	}
}

// ─── EMA Helper Test ────────────────────────────────────────────────────────

func TestEMA(t *testing.T) {
	// ema(0.5, 1.0, 0.3) = 0.3×1.0 + 0.7×0.5 = 0.3 + 0.35 = 0.65
	got := ema(0.5, 1.0, 0.3)
	if !almostEqual(got, 0.65, 0.001) {
		t.Errorf("ema(0.5, 1.0, 0.3) = %f, want 0.65", got)
	}

	// ema(0.5, 0.0, 0.1) = 0.1×0.0 + 0.9×0.5 = 0.45
	got = ema(0.5, 0.0, 0.1)
	if !almostEqual(got, 0.45, 0.001) {
		t.Errorf("ema(0.5, 0.0, 0.1) = %f, want 0.45", got)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, min, max, want float64
	}{
		{0.5, 0.1, 1.0, 0.5},
		{0.0, 0.1, 1.0, 0.1},
		{1.5, 0.1, 1.0, 1.0},
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}
