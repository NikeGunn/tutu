package anomaly

import (
	"testing"
	"time"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newTestDetector(t *testing.T) *Detector {
	t.Helper()
	d := NewDetector(DefaultDetectorConfig())
	d.now = func() time.Time {
		return time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	}
	return d
}

func normalEvent(nodeID string, dur time.Duration, cpu float64, ok bool) TaskEvent {
	return TaskEvent{
		NodeID:     nodeID,
		TaskID:     "task-1",
		TaskType:   "INFERENCE",
		Duration:   dur,
		CPUUsage:   cpu,
		Successful: ok,
		Timestamp:  time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}
}

// buildProfile feeds several normal events with slight variance to establish
// baseline stats with non-zero standard deviation.
func buildProfile(t *testing.T, d *Detector, nodeID string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		// Vary duration ±5ms to produce non-zero variance
		dur := 100*time.Millisecond + time.Duration(i%5-2)*time.Millisecond
		d.Analyze(normalEvent(nodeID, dur, 0.5, true))
	}
}

// ─── Basic Analysis Tests ──────────────────────────────────────────────────

func TestAnalyze_NormalEvent(t *testing.T) {
	d := newTestDetector(t)

	result := d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))

	if result.IsAnomaly {
		t.Error("expected no anomaly for first normal event")
	}
	if d.ProfileCount() != 1 {
		t.Errorf("profile count = %d, want 1", d.ProfileCount())
	}
}

func TestAnalyze_ProfileBuilding(t *testing.T) {
	d := newTestDetector(t)

	// Feed 10 normal events
	for i := 0; i < 10; i++ {
		d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))
	}

	profile := d.GetProfile("node-1")
	if profile == nil {
		t.Fatal("profile is nil after 10 events")
	}
	if profile.DurationCount != 10 {
		t.Errorf("duration count = %d, want 10", profile.DurationCount)
	}
	if profile.SuccessCount != 10 {
		t.Errorf("success count = %d, want 10", profile.SuccessCount)
	}
}

// ─── Duration Outlier Detection ────────────────────────────────────────────

func TestAnalyze_DurationOutlier(t *testing.T) {
	d := newTestDetector(t)

	// Build a baseline with consistent 100ms durations
	buildProfile(t, d, "node-1", 20)

	// Now send an extreme outlier (10 seconds instead of 100ms)
	result := d.Analyze(normalEvent("node-1", 10*time.Second, 0.5, true))

	if !result.IsAnomaly {
		t.Fatal("expected anomaly for duration outlier")
	}
	if result.Type != AnomalyDurationOutlier {
		t.Errorf("type = %v, want AnomalyDurationOutlier", result.Type)
	}
	if result.Severity != SevWarning {
		t.Errorf("severity = %v, want SevWarning", result.Severity)
	}
}

// ─── Low CPU Detection ─────────────────────────────────────────────────────

func TestAnalyze_LowCPU(t *testing.T) {
	d := newTestDetector(t)

	// INFERENCE task with near-zero CPU → suspicious
	event := TaskEvent{
		NodeID:     "node-cheater",
		TaskID:     "task-fake",
		TaskType:   "INFERENCE",
		Duration:   100 * time.Millisecond,
		CPUUsage:   0.001, // Below MinCPUForInference (0.01)
		Successful: true,
		Timestamp:  time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}

	result := d.Analyze(event)

	if !result.IsAnomaly {
		t.Fatal("expected anomaly for low CPU inference")
	}
	if result.Type != AnomalyLowCPU {
		t.Errorf("type = %v, want AnomalyLowCPU", result.Type)
	}
	if result.Severity != SevCritical {
		t.Errorf("severity = %v, want SevCritical", result.Severity)
	}
}

func TestAnalyze_LowCPU_NonInference(t *testing.T) {
	d := newTestDetector(t)

	// Non-INFERENCE task type → low CPU is fine
	event := TaskEvent{
		NodeID:     "node-1",
		TaskType:   "EMBEDDING",
		Duration:   100 * time.Millisecond,
		CPUUsage:   0.001,
		Successful: true,
		Timestamp:  time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}

	result := d.Analyze(event)
	if result.IsAnomaly {
		t.Error("expected no anomaly for low CPU on non-inference task")
	}
}

// ─── High Failure Rate Detection ───────────────────────────────────────────

func TestAnalyze_HighFailRate(t *testing.T) {
	d := newTestDetector(t)

	// Build a good baseline: 10 successes
	buildProfile(t, d, "node-1", 10)

	// Now introduce a burst of failures
	for i := 0; i < 10; i++ {
		d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, false))
	}

	// The high-failure-rate check fires if rate > 0.5 and historical > 0.8
	// After 10 successes + 10 failures: rate = 50%, historical was 100%
	// The check compares current FailureCount/Total > 0.5 and SuccessRate > 0.8
	profile := d.GetProfile("node-1")
	if profile.FailureCount != 10 {
		t.Errorf("failure count = %d, want 10", profile.FailureCount)
	}
}

// ─── Consecutive Anomaly Escalation ────────────────────────────────────────

func TestAnalyze_ConsecutiveEscalation(t *testing.T) {
	d := newTestDetector(t)

	var lastResult AnomalyResult
	// Send multiple low-CPU inference events → each is an anomaly
	for i := 0; i < 5; i++ {
		lastResult = d.Analyze(TaskEvent{
			NodeID:     "node-bad",
			TaskID:     "task-bad",
			TaskType:   "INFERENCE",
			Duration:   100 * time.Millisecond,
			CPUUsage:   0.001,
			Successful: true,
			Timestamp:  time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		})
	}

	// After ≥3 consecutive anomalies → CRITICAL
	if lastResult.Severity != SevCritical {
		t.Errorf("severity after 5 anomalies = %v, want SevCritical", lastResult.Severity)
	}

	profile := d.GetProfile("node-bad")
	if profile.ConsecutiveAnomalies < MaxConsecutiveAnomalies {
		t.Errorf("consecutive = %d, want >= %d", profile.ConsecutiveAnomalies, MaxConsecutiveAnomalies)
	}
}

func TestAnalyze_ConsecutiveReset(t *testing.T) {
	d := newTestDetector(t)

	// 2 anomalies
	d.Analyze(TaskEvent{
		NodeID: "node-1", TaskType: "INFERENCE", Duration: 100 * time.Millisecond,
		CPUUsage: 0.001, Successful: true,
		Timestamp: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	})
	d.Analyze(TaskEvent{
		NodeID: "node-1", TaskType: "INFERENCE", Duration: 100 * time.Millisecond,
		CPUUsage: 0.001, Successful: true,
		Timestamp: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	})

	// A clean event should reset consecutive count
	d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))

	profile := d.GetProfile("node-1")
	if profile.ConsecutiveAnomalies != 0 {
		t.Errorf("consecutive after reset = %d, want 0", profile.ConsecutiveAnomalies)
	}
}

// ─── Threat Intelligence Tests ─────────────────────────────────────────────

func TestReportThreat(t *testing.T) {
	d := newTestDetector(t)

	d.ReportThreat("node-evil", "sybil attack", "node-reporter")

	if !d.IsKnownThreat("node-evil") {
		t.Error("expected node-evil to be a known threat")
	}
	if d.IsKnownThreat("node-innocent") {
		t.Error("expected node-innocent to NOT be a known threat")
	}
}

func TestReportThreat_Duplicate(t *testing.T) {
	d := newTestDetector(t)

	d.ReportThreat("node-evil", "sybil attack", "reporter-1")
	d.ReportThreat("node-evil", "sybil attack", "reporter-2") // Same reason → deduplicated

	feed := d.ThreatFeed()
	if len(feed) != 1 {
		t.Errorf("threat feed size = %d, want 1 (deduplicated)", len(feed))
	}
}

func TestReportThreat_DifferentReasons(t *testing.T) {
	d := newTestDetector(t)

	d.ReportThreat("node-evil", "sybil attack", "reporter-1")
	d.ReportThreat("node-evil", "resource abuse", "reporter-2")

	feed := d.ThreatFeed()
	if len(feed) != 2 {
		t.Errorf("threat feed size = %d, want 2 (different reasons)", len(feed))
	}
}

func TestThreatFeed(t *testing.T) {
	d := newTestDetector(t)

	d.ReportThreat("node-1", "reason-1", "reporter-1")
	d.ReportThreat("node-2", "reason-2", "reporter-2")

	feed := d.ThreatFeed()
	if len(feed) != 2 {
		t.Errorf("feed size = %d, want 2", len(feed))
	}
}

// ─── Profile Queries ────────────────────────────────────────────────────────

func TestGetProfile(t *testing.T) {
	d := newTestDetector(t)

	// No profile yet
	if d.GetProfile("node-1") != nil {
		t.Error("expected nil profile before any events")
	}

	d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))

	profile := d.GetProfile("node-1")
	if profile == nil {
		t.Fatal("expected profile after event")
	}
	if profile.NodeID != "node-1" {
		t.Errorf("nodeID = %q, want %q", profile.NodeID, "node-1")
	}
}

func TestProfileCount(t *testing.T) {
	d := newTestDetector(t)
	if d.ProfileCount() != 0 {
		t.Errorf("initial count = %d, want 0", d.ProfileCount())
	}

	d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))
	d.Analyze(normalEvent("node-2", 200*time.Millisecond, 0.6, true))

	if d.ProfileCount() != 2 {
		t.Errorf("count = %d, want 2", d.ProfileCount())
	}
}

// ─── Stats ──────────────────────────────────────────────────────────────────

func TestStats(t *testing.T) {
	d := newTestDetector(t)

	// Generate some anomalies
	d.Analyze(TaskEvent{
		NodeID: "node-1", TaskType: "INFERENCE", Duration: 100 * time.Millisecond,
		CPUUsage: 0.001, Successful: true,
		Timestamp: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	})
	d.ReportThreat("node-bad", "reason", "reporter")

	stats := d.Stats()
	if stats.ProfileCount != 1 {
		t.Errorf("profile count = %d, want 1", stats.ProfileCount)
	}
	if stats.TotalAnomalies != 1 {
		t.Errorf("total anomalies = %d, want 1", stats.TotalAnomalies)
	}
	if stats.ThreatFeedSize != 1 {
		t.Errorf("threat feed = %d, want 1", stats.ThreatFeedSize)
	}
}

// ─── Cleanup ────────────────────────────────────────────────────────────────

func TestCleanupStaleProfiles(t *testing.T) {
	d := newTestDetector(t)
	startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return startTime }

	d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))
	d.Analyze(normalEvent("node-2", 100*time.Millisecond, 0.5, true))

	// Advance past expiry (91 days)
	d.now = func() time.Time {
		return startTime.Add(time.Duration(ProfileExpiryDays+1) * 24 * time.Hour)
	}

	removed := d.CleanupStaleProfiles()
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	if d.ProfileCount() != 0 {
		t.Errorf("profile count after cleanup = %d, want 0", d.ProfileCount())
	}
}

func TestCleanupStaleProfiles_KeepsRecent(t *testing.T) {
	d := newTestDetector(t)
	startTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return startTime }

	d.Analyze(normalEvent("node-1", 100*time.Millisecond, 0.5, true))

	// Only advance 30 days — not expired
	d.now = func() time.Time { return startTime.Add(30 * 24 * time.Hour) }

	removed := d.CleanupStaleProfiles()
	if removed != 0 {
		t.Errorf("removed = %d, want 0 (not expired)", removed)
	}
}

// ─── Welford's Algorithm Tests ─────────────────────────────────────────────

func TestNodeProfile_DurationStddev(t *testing.T) {
	p := &NodeProfile{}
	if p.DurationStddev() != 0 {
		t.Errorf("stddev with 0 samples = %f, want 0", p.DurationStddev())
	}

	p.DurationCount = 1
	if p.DurationStddev() != 0 {
		t.Errorf("stddev with 1 sample = %f, want 0", p.DurationStddev())
	}
}

func TestNodeProfile_SuccessRate(t *testing.T) {
	p := &NodeProfile{}
	if p.SuccessRate() != 1.0 {
		t.Errorf("success rate with no events = %f, want 1.0", p.SuccessRate())
	}

	p.SuccessCount = 8
	p.FailureCount = 2
	got := p.SuccessRate()
	if got != 0.8 {
		t.Errorf("success rate = %f, want 0.8", got)
	}
}

// ─── String Methods ─────────────────────────────────────────────────────────

func TestAnomalyTypeString(t *testing.T) {
	tests := []struct {
		at   AnomalyType
		want string
	}{
		{AnomalyNone, "NONE"},
		{AnomalyDurationOutlier, "DURATION_OUTLIER"},
		{AnomalyLowCPU, "LOW_CPU"},
		{AnomalyHighFailRate, "HIGH_FAIL_RATE"},
		{AnomalyEarningSpike, "EARNING_SPIKE"},
		{AnomalyPatternMismatch, "PATTERN_MISMATCH"},
		{AnomalyType(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.at.String(); got != tt.want {
			t.Errorf("AnomalyType(%d).String() = %q, want %q", tt.at, got, tt.want)
		}
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		sev  Severity
		want string
	}{
		{SevInfo, "INFO"},
		{SevWarning, "WARNING"},
		{SevCritical, "CRITICAL"},
		{Severity(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.sev.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.sev, got, tt.want)
		}
	}
}
