package mlscheduler

import (
	"math"
	"testing"
	"time"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

// fixedClock returns a clock function that advances by step on each call.
func fixedClock(start time.Time, step time.Duration) func() time.Time {
	t := start
	return func() time.Time {
		now := t
		t = t.Add(step)
		return now
	}
}

func mkFeatures(nodeID, taskType string, load float64, gpu, hot bool) Features {
	return Features{
		NodeID:       nodeID,
		TaskType:     taskType,
		NodeLoad:     load,
		LatencyMs:    50,
		HasModelHot:  hot,
		GPUAvailable: gpu,
		Reputation:   0.8,
		CreditRate:   10,
		QueueDepth:   5,
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestNewScheduler_DefaultConfig(t *testing.T) {
	s := NewScheduler(DefaultConfig())
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if len(s.arms) != 0 {
		t.Errorf("new scheduler should have 0 arms, got %d", len(s.arms))
	}
	if s.total != 0 {
		t.Errorf("new scheduler should have 0 total, got %d", s.total)
	}
}

func TestNewScheduler_InvalidConfig(t *testing.T) {
	// All invalid values should be fixed to defaults.
	cfg := Config{
		ExplorationFactor: -1,
		MinObservations:   -1,
		DecayFactor:       0,
		HistoryCapacity:   -1,
		LatencyWeight:     0,
		CostWeight:        0,
		FairnessWeight:    0,
	}
	s := NewScheduler(cfg)
	if s.cfg.ExplorationFactor != 1.5 {
		t.Errorf("expected ExplorationFactor=1.5, got %f", s.cfg.ExplorationFactor)
	}
	if s.cfg.MinObservations != 3 {
		t.Errorf("expected MinObservations=3, got %d", s.cfg.MinObservations)
	}
}

func TestFeatures_ArmKey(t *testing.T) {
	tests := []struct {
		name string
		f    Features
		want string
	}{
		{
			name: "inference_idle_gpu_hot",
			f:    mkFeatures("n1", "INFERENCE", 0.1, true, true),
			want: "INFERENCE:idle:gpu:hot",
		},
		{
			name: "embedding_heavy_nogpu_cold",
			f:    mkFeatures("n2", "EMBEDDING", 0.9, false, false),
			want: "EMBEDDING:heavy:nogpu:cold",
		},
		{
			name: "agent_medium_gpu_cold",
			f:    mkFeatures("n3", "AGENT", 0.6, true, false),
			want: "AGENT:medium:gpu:cold",
		},
		{
			name: "fine_tune_light_nogpu_hot",
			f:    mkFeatures("n4", "FINE_TUNE", 0.3, false, true),
			want: "FINE_TUNE:light:nogpu:hot",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.armKey()
			if got != tt.want {
				t.Errorf("armKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeuristicScore(t *testing.T) {
	// Perfect candidate: low latency, low load, hot model, high rep, GPU
	perfect := Features{
		LatencyMs:    10,
		NodeLoad:     0.1,
		HasModelHot:  true,
		Reputation:   1.0,
		GPUAvailable: true,
	}
	score := HeuristicScore(perfect)
	if score < 0.8 {
		t.Errorf("perfect candidate should score >0.8, got %f", score)
	}

	// Terrible candidate: high latency, high load, cold, low rep, no GPU
	terrible := Features{
		LatencyMs:    500,
		NodeLoad:     1.0,
		HasModelHot:  false,
		Reputation:   0.0,
		GPUAvailable: false,
	}
	tScore := HeuristicScore(terrible)
	if tScore > 0.1 {
		t.Errorf("terrible candidate should score <0.1, got %f", tScore)
	}

	if tScore >= score {
		t.Error("perfect should score higher than terrible")
	}
}

func TestSelectNode_ExploresUnknownArms(t *testing.T) {
	s := NewScheduler(DefaultConfig())

	candidates := []Features{
		mkFeatures("n1", "INFERENCE", 0.5, true, true),
		mkFeatures("n2", "INFERENCE", 0.1, true, true),
	}

	// With no prior observations, both arms are unknown.
	// The scheduler should still return a valid candidate.
	selected, key := s.SelectNode(candidates)
	if selected.NodeID == "" {
		t.Error("SelectNode should return a valid candidate")
	}
	if key == "" {
		t.Error("SelectNode should return a valid arm key")
	}
}

func TestSelectNode_Empty(t *testing.T) {
	s := NewScheduler(DefaultConfig())
	selected, key := s.SelectNode(nil)
	if selected.NodeID != "" {
		t.Error("SelectNode with nil should return empty features")
	}
	if key != "" {
		t.Error("SelectNode with nil should return empty key")
	}
}

func TestRecordOutcome_UpdatesStats(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.Now = fixedClock(now, time.Second)
	s := NewScheduler(cfg)

	s.RecordOutcome("INFERENCE:idle:gpu:hot", "node-1", 50.0, 10.0)
	s.RecordOutcome("INFERENCE:idle:gpu:hot", "node-1", 30.0, 8.0)

	stats := s.Stats()
	if stats.TotalObservations != 2 {
		t.Errorf("expected 2 observations, got %d", stats.TotalObservations)
	}
	if stats.UniqueArms != 1 {
		t.Errorf("expected 1 arm, got %d", stats.UniqueArms)
	}
	if stats.UniqueNodes != 1 {
		t.Errorf("expected 1 node, got %d", stats.UniqueNodes)
	}
	if stats.MLAvgLatencyMs != 40.0 {
		t.Errorf("expected avg latency 40.0, got %f", stats.MLAvgLatencyMs)
	}
}

func TestRecordHeuristicBaseline(t *testing.T) {
	s := NewScheduler(DefaultConfig())
	s.RecordHeuristicBaseline(100)
	s.RecordHeuristicBaseline(200)

	stats := s.Stats()
	if stats.HeurAvgLatencyMs != 150 {
		t.Errorf("expected heuristic avg 150, got %f", stats.HeurAvgLatencyMs)
	}
}

func TestGatePassed(t *testing.T) {
	s := NewScheduler(DefaultConfig())

	// Record heuristic baseline: avg 100ms
	for i := 0; i < 10; i++ {
		s.RecordHeuristicBaseline(100)
	}

	// Record ML outcomes: avg 60ms (40% improvement)
	for i := 0; i < 10; i++ {
		s.RecordOutcome("arm", "node", 60.0, 10.0)
	}

	if !s.GatePassed(30.0) {
		st := s.Stats()
		t.Errorf("gate should pass at 30%% threshold; improvement=%.1f%%", st.ImprovementPct)
	}

	if s.GatePassed(50.0) {
		t.Error("gate should NOT pass at 50% threshold with only 40% improvement")
	}
}

func TestGatePassed_NoHeuristic(t *testing.T) {
	s := NewScheduler(DefaultConfig())
	s.RecordOutcome("arm", "node", 50, 10)

	if s.GatePassed(30) {
		t.Error("gate should not pass without heuristic baseline")
	}
}

func TestComputeReward(t *testing.T) {
	s := NewScheduler(DefaultConfig())

	// Perfect: 0 latency, 0 cost, perfect fairness
	reward := s.ComputeReward(0, 0)
	if reward < 0.99 {
		t.Errorf("perfect reward should be ~1.0, got %f", reward)
	}

	// Terrible: 1000ms latency, 100 credits
	badReward := s.ComputeReward(1000, 100)
	if badReward > 0.3 {
		t.Errorf("bad reward should be low, got %f", badReward)
	}

	if badReward >= reward {
		t.Error("perfect reward should exceed bad reward")
	}
}

func TestGiniCoefficient(t *testing.T) {
	s := NewScheduler(DefaultConfig())

	// Perfect equality: all nodes have same count.
	s.nodeTaskCounts = map[string]int64{"a": 10, "b": 10, "c": 10}
	gini := s.giniCoefficient()
	if gini > 0.01 {
		t.Errorf("equal distribution should have gini ~0, got %f", gini)
	}

	// Maximum inequality: one node has all.
	s.nodeTaskCounts = map[string]int64{"a": 100, "b": 0, "c": 0}
	gini = s.giniCoefficient()
	if gini < 0.5 {
		t.Errorf("max inequality should have gini >0.5, got %f", gini)
	}

	// Single node: should be 0.
	s.nodeTaskCounts = map[string]int64{"a": 50}
	gini = s.giniCoefficient()
	if gini != 0 {
		t.Errorf("single node gini should be 0, got %f", gini)
	}
}

func TestObservations_RingBuffer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HistoryCapacity = 5
	cfg.Now = fixedClock(time.Now(), time.Second)
	s := NewScheduler(cfg)

	// Record 8 observations — should wrap around twice.
	for i := 0; i < 8; i++ {
		s.RecordOutcome("arm", "node", float64(i*10), 5)
	}

	obs := s.Observations(3)
	if len(obs) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(obs))
	}

	// Most recent should have latency=70 (last recorded).
	if obs[0].LatencyMs != 70 {
		t.Errorf("most recent should have latency 70, got %f", obs[0].LatencyMs)
	}
}

func TestObservations_LimitExceedsCount(t *testing.T) {
	s := NewScheduler(DefaultConfig())
	s.RecordOutcome("arm", "node", 50, 10)

	obs := s.Observations(100)
	if len(obs) != 1 {
		t.Errorf("expected 1 observation, got %d", len(obs))
	}
}

func TestArms(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.Now = fixedClock(now, time.Second)
	s := NewScheduler(cfg)

	s.RecordOutcome("arm-a", "n1", 50, 10)
	s.RecordOutcome("arm-a", "n1", 60, 12)
	s.RecordOutcome("arm-b", "n2", 30, 5)

	arms := s.Arms()
	if len(arms) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(arms))
	}

	// Find arm-a
	var armA *ArmInfo
	for i := range arms {
		if arms[i].Key == "arm-a" {
			armA = &arms[i]
			break
		}
	}
	if armA == nil {
		t.Fatal("arm-a not found")
	}
	if armA.Pulls != 2 {
		t.Errorf("arm-a should have 2 pulls, got %d", armA.Pulls)
	}
}

func TestUCB1_ExploitsAfterEnoughPulls(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.MinObservations = 3
	cfg.Now = fixedClock(now, time.Millisecond)
	s := NewScheduler(cfg)

	// Train arm "good" with high rewards.
	for i := 0; i < 10; i++ {
		s.RecordOutcome("good", "n1", 20, 5) // low latency = high reward
	}
	// Train arm "bad" with low rewards.
	for i := 0; i < 10; i++ {
		s.RecordOutcome("bad", "n2", 900, 90) // high latency = low reward
	}

	// The good arm should have a higher UCB1 score.
	s.mu.RLock()
	goodArm := s.arms["good"]
	badArm := s.arms["bad"]
	goodUCB := s.ucb1Score(goodArm)
	badUCB := s.ucb1Score(badArm)
	s.mu.RUnlock()

	if goodUCB <= badUCB {
		t.Errorf("good arm UCB (%f) should exceed bad arm UCB (%f)", goodUCB, badUCB)
	}
}

func TestReset(t *testing.T) {
	s := NewScheduler(DefaultConfig())
	s.RecordOutcome("arm", "node", 50, 10)
	s.RecordHeuristicBaseline(100)

	s.Reset()

	stats := s.Stats()
	if stats.TotalObservations != 0 {
		t.Errorf("expected 0 observations after reset, got %d", stats.TotalObservations)
	}
	if stats.UniqueArms != 0 {
		t.Errorf("expected 0 arms after reset, got %d", stats.UniqueArms)
	}
}

func TestArmStats_Welford(t *testing.T) {
	now := time.Now()
	a := &armStats{}

	// Update with known values: 2, 4, 6
	a.update(2, now)
	a.update(4, now)
	a.update(6, now)

	expectedMean := 4.0
	if math.Abs(a.mean-expectedMean) > 0.001 {
		t.Errorf("mean = %f, want %f", a.mean, expectedMean)
	}

	// Sample variance of [2,4,6] = 4.0
	expectedVar := 4.0
	if math.Abs(a.variance()-expectedVar) > 0.001 {
		t.Errorf("variance = %f, want %f", a.variance(), expectedVar)
	}

	// Single sample: variance should be 0
	single := &armStats{}
	single.update(5, now)
	if single.variance() != 0 {
		t.Errorf("single sample variance should be 0, got %f", single.variance())
	}
}
