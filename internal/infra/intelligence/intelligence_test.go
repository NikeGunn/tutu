package intelligence

import (
	"testing"
	"time"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func fixedClock(start time.Time, step time.Duration) func() time.Time {
	t := start
	return func() time.Time {
		now := t
		t = t.Add(step)
		return now
	}
}

func testConfig(start time.Time) Config {
	return Config{
		RetirementDays:          30,
		PlacementInterval:       7 * 24 * time.Hour,
		MinRequestsForPlacement: 5,
		MaxRecommendations:      50,
		MaxRetirementCandidates: 100,
		HealthHistorySize:       1000,
		Now:                     fixedClock(start, time.Second),
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestNewOptimizer_DefaultConfig(t *testing.T) {
	o := NewOptimizer(DefaultConfig())
	if o == nil {
		t.Fatal("NewOptimizer returned nil")
	}
	st := o.Stats()
	if st.TrackedModels != 0 || st.TrackedNodes != 0 {
		t.Error("new optimizer should track 0 models and 0 nodes")
	}
}

func TestNewOptimizer_InvalidConfig(t *testing.T) {
	cfg := Config{
		RetirementDays:          -1,
		MinRequestsForPlacement: -1,
		MaxRecommendations:      0,
		MaxRetirementCandidates: 0,
		HealthHistorySize:       -1,
	}
	o := NewOptimizer(cfg)
	if o.cfg.RetirementDays != 30 {
		t.Errorf("expected RetirementDays=30, got %d", o.cfg.RetirementDays)
	}
}

func TestRecordRequest_TracksPopularity(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))

	o.RecordRequest("llama-3", "node-A", 50, true)
	o.RecordRequest("llama-3", "node-A", 60, false)
	o.RecordRequest("llama-3", "node-B", 70, true)

	top := o.TopModels(5)
	if len(top) != 1 {
		t.Fatalf("expected 1 model, got %d", len(top))
	}
	if top[0].ModelName != "llama-3" {
		t.Errorf("model = %s, want llama-3", top[0].ModelName)
	}
	if top[0].TotalReqs != 3 {
		t.Errorf("total reqs = %d, want 3", top[0].TotalReqs)
	}
	if top[0].AvgLatencyMs != 60 {
		t.Errorf("avg latency = %f, want 60", top[0].AvgLatencyMs)
	}
}

func TestRecordRequest_MultipleModels(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))

	for i := 0; i < 10; i++ {
		o.RecordRequest("popular", "n1", 30, true)
	}
	for i := 0; i < 3; i++ {
		o.RecordRequest("niche", "n2", 50, false)
	}

	top := o.TopModels(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 models, got %d", len(top))
	}
	// "popular" should be first.
	if top[0].ModelName != "popular" {
		t.Errorf("top model = %s, want popular", top[0].ModelName)
	}
}

func TestNodeAffinities(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))

	// Node A: fast, lots of cache hits.
	for i := 0; i < 20; i++ {
		o.RecordRequest("llama-3", "node-A", 30, true)
	}
	// Node B: slow, no cache hits.
	for i := 0; i < 5; i++ {
		o.RecordRequest("llama-3", "node-B", 200, false)
	}

	affs := o.NodeAffinities("llama-3")
	if len(affs) != 2 {
		t.Fatalf("expected 2 affinities, got %d", len(affs))
	}

	// Node A should have higher affinity.
	if affs[0].NodeID != "node-A" {
		t.Errorf("highest affinity should be node-A, got %s", affs[0].NodeID)
	}
	if affs[0].CacheHitRate != 1.0 {
		t.Errorf("node-A cache hit rate = %f, want 1.0", affs[0].CacheHitRate)
	}
	if affs[1].CacheHitRate != 0.0 {
		t.Errorf("node-B cache hit rate = %f, want 0.0", affs[1].CacheHitRate)
	}
}

func TestSetVRAMFit(t *testing.T) {
	o := NewOptimizer(DefaultConfig())
	o.SetVRAMFit("node-A", "llama-3", 0.2)

	// Verify we can still compute affinity.
	affs := o.NodeAffinities("llama-3")
	if len(affs) != 1 {
		t.Fatalf("expected 1 affinity, got %d", len(affs))
	}
	if affs[0].VRAMFitScore != 0.2 {
		t.Errorf("VRAM fit = %f, want 0.2", affs[0].VRAMFitScore)
	}
}

func TestOptimize_ProducesRecommendations(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := testConfig(base)
	cfg.MinRequestsForPlacement = 5
	o := NewOptimizer(cfg)

	// Node A: high affinity (fast, cache hits).
	for i := 0; i < 20; i++ {
		o.RecordRequest("llama-3", "node-A", 20, true)
	}
	// Node B: low affinity (slow, cache misses).
	for i := 0; i < 10; i++ {
		o.RecordRequest("llama-3", "node-B", 300, false)
	}

	recs := o.Optimize()
	// Should recommend moving llama-3 from node-B to node-A.
	if len(recs) == 0 {
		t.Fatal("expected at least 1 recommendation")
	}

	found := false
	for _, r := range recs {
		if r.ModelName == "llama-3" && r.Type == RecommendMove {
			found = true
			if r.FromNode != "node-B" {
				t.Errorf("from = %s, want node-B", r.FromNode)
			}
			if r.ToNode != "node-A" {
				t.Errorf("to = %s, want node-A", r.ToNode)
			}
		}
	}
	if !found {
		t.Error("expected a MOVE recommendation for llama-3")
	}
}

func TestOptimize_NotEnoughData(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := testConfig(base)
	cfg.MinRequestsForPlacement = 100 // very high threshold
	o := NewOptimizer(cfg)

	o.RecordRequest("tiny-model", "n1", 50, true)

	recs := o.Optimize()
	if len(recs) != 0 {
		t.Errorf("expected 0 recs with insufficient data, got %d", len(recs))
	}
}

func TestScanRetirements(t *testing.T) {
	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := testConfig(base)
	cfg.RetirementDays = 30
	// Use a stable clock that always returns base.
	cfg.Now = func() time.Time { return base }
	o := NewOptimizer(cfg)

	// Model last requested 60 days ago — should be retirement candidate.
	o.mu.Lock()
	o.popularity["old-model"] = &modelStats{
		totalReqs: 5,
		lastReq:   base.AddDate(0, 0, -60),
	}
	// Model last requested 10 days ago — should NOT be candidate.
	o.popularity["recent-model"] = &modelStats{
		totalReqs: 50,
		lastReq:   base.AddDate(0, 0, -10),
	}
	o.mu.Unlock()

	candidates := o.ScanRetirements()
	if len(candidates) != 1 {
		t.Fatalf("expected 1 retirement candidate, got %d", len(candidates))
	}
	if candidates[0].ModelName != "old-model" {
		t.Errorf("candidate = %s, want old-model", candidates[0].ModelName)
	}
	if candidates[0].DaysSinceUse != 60 {
		t.Errorf("days since use = %d, want 60", candidates[0].DaysSinceUse)
	}
}

func TestRetirementCandidates_Persists(t *testing.T) {
	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := testConfig(base)
	cfg.Now = func() time.Time { return base }
	o := NewOptimizer(cfg)

	o.mu.Lock()
	o.popularity["old"] = &modelStats{lastReq: base.AddDate(0, 0, -90)}
	o.mu.Unlock()

	o.ScanRetirements()

	persisted := o.RetirementCandidates()
	if len(persisted) != 1 {
		t.Errorf("expected 1 persisted candidate, got %d", len(persisted))
	}
}

func TestReportHealthPattern(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))

	o.ReportHealthPattern(HealthPattern{
		OrgID:          "org-a",
		AvgFailureRate: 0.05,
		AvgMTTR:        120.0, // seconds
		TopFailureType: "HIGH_ERROR_RATE",
		NodeCount:      10,
		TaskVolume:     1000,
		ReportedAt:     base,
	})
	o.ReportHealthPattern(HealthPattern{
		OrgID:          "org-b",
		AvgFailureRate: 0.15,
		AvgMTTR:        300.0,
		TopFailureType: "DISK_FULL",
		NodeCount:      5,
		TaskVolume:     500,
		ReportedAt:     base,
	})

	insight := o.AggregateHealthInsights()
	if insight.OrgCount != 2 {
		t.Errorf("org count = %d, want 2", insight.OrgCount)
	}
	if insight.AvgFailureRate != 0.1 { // (0.05 + 0.15) / 2
		t.Errorf("avg failure rate = %f, want 0.1", insight.AvgFailureRate)
	}
	if insight.TotalNodes != 15 {
		t.Errorf("total nodes = %d, want 15", insight.TotalNodes)
	}
	if insight.TotalTasks != 1500 {
		t.Errorf("total tasks = %d, want 1500", insight.TotalTasks)
	}
}

func TestAggregateHealthInsights_Empty(t *testing.T) {
	o := NewOptimizer(DefaultConfig())
	insight := o.AggregateHealthInsights()
	if insight.OrgCount != 0 {
		t.Errorf("empty org count = %d, want 0", insight.OrgCount)
	}
}

func TestGatePassed(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))

	if o.GatePassed() {
		t.Error("gate should fail before any optimization")
	}

	o.Optimize()

	if !o.GatePassed() {
		t.Error("gate should pass after optimization")
	}
}

func TestRecentRecommendations(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := testConfig(base)
	cfg.MinRequestsForPlacement = 5
	o := NewOptimizer(cfg)

	for i := 0; i < 20; i++ {
		o.RecordRequest("model-x", "fast-node", 20, true)
	}
	for i := 0; i < 10; i++ {
		o.RecordRequest("model-x", "slow-node", 300, false)
	}

	o.Optimize()

	recs := o.RecentRecommendations(10)
	if len(recs) == 0 {
		t.Error("expected recent recommendations")
	}
}

func TestRecommendationType_String(t *testing.T) {
	tests := []struct {
		r    RecommendationType
		want string
	}{
		{RecommendPlace, "PLACE"},
		{RecommendEvict, "EVICT"},
		{RecommendMove, "MOVE"},
		{RecommendationType(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStats(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))

	o.RecordRequest("m1", "n1", 50, true)
	o.RecordRequest("m2", "n2", 60, false)

	st := o.Stats()
	if st.TrackedModels != 2 {
		t.Errorf("tracked models = %d, want 2", st.TrackedModels)
	}
	if st.TrackedNodes != 2 {
		t.Errorf("tracked nodes = %d, want 2", st.TrackedNodes)
	}
}

func TestReset(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	o := NewOptimizer(testConfig(base))
	o.RecordRequest("m1", "n1", 50, true)
	o.Optimize()

	o.Reset()

	st := o.Stats()
	if st.TrackedModels != 0 {
		t.Errorf("models after reset = %d, want 0", st.TrackedModels)
	}
	if st.TotalOptimizations != 0 {
		t.Errorf("optimizations after reset = %d, want 0", st.TotalOptimizations)
	}
}
