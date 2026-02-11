// Package mlscheduler implements the Phase 6 ML-driven scheduler.
//
// The core idea: instead of using hand-tuned heuristic weights to score
// node candidates (Phase 3), we use a multi-armed bandit (UCB1) that
// LEARNS which assignment strategies produce the best outcomes over time.
//
// Key concepts for beginners:
//
//   - Multi-Armed Bandit (MAB): imagine a row of slot machines ("arms").
//     Each pull costs you nothing, but each arm has a different unknown payoff.
//     UCB1 is a famous algorithm that balances EXPLOITATION (pulling the arm
//     that has paid out best so far) with EXPLORATION (trying arms we haven't
//     pulled much, in case they're secretly better).
//
//   - Feature Extraction: before the bandit chooses an arm, we extract
//     numerical features from the {task, node} pair — latency, load, GPU
//     availability, model cache state. These features form the "context".
//
//   - Reward Signal: after a task completes, we score the outcome on a 0–1
//     scale combining latency, cost (credits), and fairness. The bandit uses
//     this to update its belief about each arm.
//
//   - Cost Optimizer: balances three objectives — minimize latency, minimize
//     credit cost, and maximize fairness (spread work across nodes).
//
// Architecture ref: Phase 6 spec — "ML-Driven Scheduling" deliverable.
// Gate check: ML scheduler outperforms heuristic by 30%+ on latency.
package mlscheduler

import (
	"math"
	"sync"
	"time"
)

// ─── Configuration ──────────────────────────────────────────────────────────

// Config configures the ML-driven scheduler.
type Config struct {
	// ExplorationFactor controls exploration vs exploitation in UCB1.
	// Higher = more exploration. Classic UCB1 uses sqrt(2) ≈ 1.41.
	// We default to 1.5 to explore slightly more in a P2P network
	// where node behavior can shift quickly.
	ExplorationFactor float64

	// MinObservations is how many observations an arm needs before
	// we trust its statistics. Below this threshold we always explore.
	MinObservations int

	// DecayFactor controls how strongly older observations are discounted.
	// 1.0 = no decay (all observations weighted equally).
	// 0.95 = each observation's weight decays by 5% per subsequent observation.
	// Helps adapt to changing node behavior over time.
	DecayFactor float64

	// RewardWeights controls the cost optimizer's multi-objective balance.
	// All three should sum to 1.0 for clean interpretation, but we normalize.
	LatencyWeight  float64 // how much we value low latency (default 0.5)
	CostWeight     float64 // how much we value low credit cost (default 0.3)
	FairnessWeight float64 // how much we value spreading work (default 0.2)

	// HistoryCapacity is the maximum number of observations to retain.
	// Oldest observations are evicted when this limit is reached.
	HistoryCapacity int

	// Now is an injectable clock for testing.
	Now func() time.Time
}

// DefaultConfig returns production defaults for the ML scheduler.
func DefaultConfig() Config {
	return Config{
		ExplorationFactor: 1.5,
		MinObservations:   3,
		DecayFactor:       0.95,
		LatencyWeight:     0.5,
		CostWeight:        0.3,
		FairnessWeight:    0.2,
		HistoryCapacity:   100_000,
		Now:               time.Now,
	}
}

// ─── Feature Vector ─────────────────────────────────────────────────────────

// Features describes a {task, node} pair at the moment of scheduling.
// The ML scheduler uses these to "contextualize" each decision.
type Features struct {
	NodeID       string  // which node we're considering
	TaskType     string  // "INFERENCE", "EMBEDDING", "FINE_TUNE", "AGENT"
	Priority     int     // task priority class (0=realtime .. 4=spot)
	NodeLoad     float64 // current node CPU utilization 0..1
	LatencyMs    float64 // estimated network latency to node
	HasModelHot  bool    // is the required model already loaded in memory?
	GPUAvailable bool    // does the node have a free GPU?
	VRAMGB       float64 // GPU VRAM available (0 if no GPU)
	Reputation   float64 // node trust score from reputation system
	CreditRate   float64 // credits per task on this node
	QueueDepth   int     // tasks already queued on this node
}

// armKey returns a coarsened key that groups similar {task, node} scenarios
// into the same "arm" for the bandit. We bucket continuous features so
// the bandit has a tractable number of arms to learn about.
func (f Features) armKey() string {
	// Bucket load into 4 tiers: idle, light, medium, heavy
	loadBucket := "idle"
	switch {
	case f.NodeLoad > 0.75:
		loadBucket = "heavy"
	case f.NodeLoad > 0.50:
		loadBucket = "medium"
	case f.NodeLoad > 0.25:
		loadBucket = "light"
	}

	gpu := "nogpu"
	if f.GPUAvailable {
		gpu = "gpu"
	}
	hot := "cold"
	if f.HasModelHot {
		hot = "hot"
	}

	// Example key: "INFERENCE:light:gpu:hot" — a manageable ~64 arms
	return f.TaskType + ":" + loadBucket + ":" + gpu + ":" + hot
}

// ─── Observation ────────────────────────────────────────────────────────────

// Observation records the outcome of a single scheduling decision.
type Observation struct {
	ArmKey     string    // which arm was pulled
	NodeID     string    // which node executed the task
	Reward     float64   // 0..1 composite score
	LatencyMs  float64   // actual end-to-end latency
	CreditCost float64   // credits consumed
	RecordedAt time.Time // when the observation was recorded
}

// ─── Arm State ──────────────────────────────────────────────────────────────

// armStats tracks the running statistics for one arm of the bandit.
// Uses Welford's online algorithm for numerically stable mean + variance.
type armStats struct {
	pulls    int     // how many times this arm has been pulled
	totalQ   float64 // sum of rewards (for simple mean fallback)
	mean     float64 // running mean (Welford)
	m2       float64 // sum of squared differences (Welford)
	lastPull time.Time
}

// update incorporates a new reward observation using Welford's method.
func (a *armStats) update(reward float64, now time.Time) {
	a.pulls++
	a.totalQ += reward
	delta := reward - a.mean
	a.mean += delta / float64(a.pulls)
	delta2 := reward - a.mean
	a.m2 += delta * delta2
	a.lastPull = now
}

// variance returns the sample variance (0 if fewer than 2 pulls).
func (a *armStats) variance() float64 {
	if a.pulls < 2 {
		return 0
	}
	return a.m2 / float64(a.pulls-1)
}

// ─── Heuristic Baseline ────────────────────────────────────────────────────

// HeuristicScore computes the Phase 3 heuristic score for a {task, node}
// pair. This is our BASELINE that the ML scheduler must outperform by 30%.
//
// The heuristic uses fixed weights:
//
//	score = 0.3*latency + 0.25*load + 0.2*cache + 0.15*reputation + 0.1*gpu
//
// All components are normalized to [0, 1] where higher = better candidate.
func HeuristicScore(f Features) float64 {
	// Latency component: lower latency → higher score.
	// Assume max expected latency ~500ms. Clamp and invert.
	latScore := 1.0 - math.Min(f.LatencyMs/500.0, 1.0)

	// Load component: lower load → higher score.
	loadScore := 1.0 - f.NodeLoad

	// Cache component: hot model = 1.0, cold = 0.0.
	cacheScore := 0.0
	if f.HasModelHot {
		cacheScore = 1.0
	}

	// Reputation: already 0..1.
	repScore := math.Min(f.Reputation, 1.0)

	// GPU availability: binary.
	gpuScore := 0.0
	if f.GPUAvailable {
		gpuScore = 1.0
	}

	return 0.30*latScore + 0.25*loadScore + 0.20*cacheScore + 0.15*repScore + 0.10*gpuScore
}

// ─── ML Scheduler ───────────────────────────────────────────────────────────

// Scheduler is the Phase 6 ML-driven scheduler using UCB1 multi-armed bandit.
type Scheduler struct {
	mu    sync.RWMutex
	cfg   Config
	arms  map[string]*armStats // key → arm statistics
	total int                  // total pulls across all arms
	hist  []Observation        // observation history (ring buffer)
	hIdx  int                  // next write index in ring buffer
	hFull bool                 // whether the ring buffer has wrapped

	// Performance tracking: ML vs heuristic.
	mlLatencySum        float64
	mlCount             int64
	heuristicLatencySum float64
	heuristicCount      int64

	// Fairness tracking: tasks per node.
	nodeTaskCounts map[string]int64
}

// NewScheduler creates a new ML-driven scheduler.
func NewScheduler(cfg Config) *Scheduler {
	if cfg.ExplorationFactor <= 0 {
		cfg.ExplorationFactor = 1.5
	}
	if cfg.MinObservations <= 0 {
		cfg.MinObservations = 3
	}
	if cfg.DecayFactor <= 0 || cfg.DecayFactor > 1 {
		cfg.DecayFactor = 0.95
	}
	if cfg.HistoryCapacity <= 0 {
		cfg.HistoryCapacity = 100_000
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	// Normalize reward weights.
	total := cfg.LatencyWeight + cfg.CostWeight + cfg.FairnessWeight
	if total <= 0 {
		cfg.LatencyWeight = 0.5
		cfg.CostWeight = 0.3
		cfg.FairnessWeight = 0.2
	} else {
		cfg.LatencyWeight /= total
		cfg.CostWeight /= total
		cfg.FairnessWeight /= total
	}
	return &Scheduler{
		cfg:            cfg,
		arms:           make(map[string]*armStats),
		hist:           make([]Observation, cfg.HistoryCapacity),
		nodeTaskCounts: make(map[string]int64),
	}
}

// ─── UCB1 Selection ─────────────────────────────────────────────────────────

// ucb1Score computes the Upper Confidence Bound for an arm.
//
// UCB1 formula:
//
//	UCB(arm) = mean(arm) + C * sqrt( ln(N) / n(arm) )
//
// where:
//   - mean(arm) = average reward observed for this arm
//   - C = exploration factor (our cfg.ExplorationFactor)
//   - N = total pulls across ALL arms
//   - n(arm) = pulls for THIS arm
//
// The first term favors arms that have performed well (exploitation).
// The second term gives a bonus to under-explored arms (exploration).
// As n(arm) grows, the bonus shrinks — we become more confident.
func (s *Scheduler) ucb1Score(arm *armStats) float64 {
	if arm.pulls == 0 || s.total == 0 {
		return math.Inf(1) // never pulled → infinite optimism → always try
	}
	exploitation := arm.mean
	exploration := s.cfg.ExplorationFactor * math.Sqrt(math.Log(float64(s.total))/float64(arm.pulls))
	return exploitation + exploration
}

// SelectNode picks the best node from a set of candidates using UCB1.
// For each candidate, it:
//  1. Extracts the arm key from the features.
//  2. Computes the UCB1 score for that arm.
//  3. Returns the candidate with the highest score.
//
// Returns the selected Features and the arm key (for later reward attribution).
func (s *Scheduler) SelectNode(candidates []Features) (Features, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(candidates) == 0 {
		return Features{}, ""
	}

	bestIdx := 0
	bestScore := math.Inf(-1)

	for i, c := range candidates {
		key := c.armKey()
		arm, exists := s.arms[key]
		var score float64
		if !exists || arm.pulls < s.cfg.MinObservations {
			// Not enough data — give maximum exploration bonus.
			score = math.Inf(1)
		} else {
			score = s.ucb1Score(arm)
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return candidates[bestIdx], candidates[bestIdx].armKey()
}

// ─── Reward Computation ─────────────────────────────────────────────────────

// ComputeReward calculates a [0, 1] reward for a completed task.
//
// Three objectives combined via configured weights:
//   - Latency:  1 - clamp(actual_ms / 1000, 0, 1) → lower latency = higher reward
//   - Cost:     1 - clamp(credits / 100, 0, 1)    → cheaper = higher reward
//   - Fairness: 1 - clamp(gini(node_loads), 0, 1)  → more equal = higher reward
//
// The Gini coefficient measures inequality: 0 = perfect equality, 1 = max inequality.
func (s *Scheduler) ComputeReward(latencyMs, creditCost float64) float64 {
	s.mu.RLock()
	gini := s.giniCoefficient()
	s.mu.RUnlock()

	latReward := 1.0 - math.Min(latencyMs/1000.0, 1.0)
	costReward := 1.0 - math.Min(creditCost/100.0, 1.0)
	fairReward := 1.0 - gini

	return s.cfg.LatencyWeight*latReward + s.cfg.CostWeight*costReward + s.cfg.FairnessWeight*fairReward
}

// giniCoefficient computes the Gini coefficient of node task counts.
// Must be called with at least s.mu.RLock held.
//
// Gini = (2 * Σ i*x_i) / (n * Σ x_i) - (n+1)/n
// where x_i are sorted task counts. Returns 0 if fewer than 2 nodes.
func (s *Scheduler) giniCoefficient() float64 {
	n := len(s.nodeTaskCounts)
	if n < 2 {
		return 0
	}

	// Collect and sort counts.
	counts := make([]float64, 0, n)
	for _, c := range s.nodeTaskCounts {
		counts = append(counts, float64(c))
	}
	// Simple insertion sort — typically <100 nodes.
	for i := 1; i < len(counts); i++ {
		key := counts[i]
		j := i - 1
		for j >= 0 && counts[j] > key {
			counts[j+1] = counts[j]
			j--
		}
		counts[j+1] = key
	}

	var sumWeighted, sumTotal float64
	for i, v := range counts {
		sumWeighted += float64(i+1) * v
		sumTotal += v
	}
	if sumTotal == 0 {
		return 0
	}
	nf := float64(n)
	return (2.0*sumWeighted)/(nf*sumTotal) - (nf+1.0)/nf
}

// ─── Observation Recording ──────────────────────────────────────────────────

// RecordOutcome records the result of a scheduling decision.
// This updates the bandit's arm statistics and the performance trackers.
func (s *Scheduler) RecordOutcome(armKey, nodeID string, latencyMs, creditCost float64) {
	reward := s.ComputeReward(latencyMs, creditCost)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update arm statistics.
	arm, exists := s.arms[armKey]
	if !exists {
		arm = &armStats{}
		s.arms[armKey] = arm
	}
	now := s.cfg.Now()
	arm.update(reward, now)
	s.total++

	// Update per-node fairness tracker.
	s.nodeTaskCounts[nodeID]++

	// Record observation in ring buffer.
	obs := Observation{
		ArmKey:     armKey,
		NodeID:     nodeID,
		Reward:     reward,
		LatencyMs:  latencyMs,
		CreditCost: creditCost,
		RecordedAt: now,
	}
	s.hist[s.hIdx] = obs
	s.hIdx++
	if s.hIdx >= s.cfg.HistoryCapacity {
		s.hIdx = 0
		s.hFull = true
	}

	// Track ML scheduler latency.
	s.mlLatencySum += latencyMs
	s.mlCount++
}

// RecordHeuristicBaseline records a heuristic-scheduled task's latency
// so we can compute the improvement ratio.
func (s *Scheduler) RecordHeuristicBaseline(latencyMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heuristicLatencySum += latencyMs
	s.heuristicCount++
}

// ─── Statistics & Gate Check ────────────────────────────────────────────────

// Stats returns current ML scheduler statistics.
type Stats struct {
	TotalObservations int     // total observations recorded
	UniqueArms        int     // number of distinct arms
	UniqueNodes       int     // number of distinct nodes seen
	MLAvgLatencyMs    float64 // average latency for ML-scheduled tasks
	HeurAvgLatencyMs  float64 // average latency for heuristic-scheduled tasks
	ImprovementPct    float64 // (heur - ml) / heur * 100 — positive = ML is better
	GiniCoefficient   float64 // current fairness measure
}

// Stats returns current performance statistics.
func (s *Scheduler) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var mlAvg, heurAvg, improvement float64
	if s.mlCount > 0 {
		mlAvg = s.mlLatencySum / float64(s.mlCount)
	}
	if s.heuristicCount > 0 {
		heurAvg = s.heuristicLatencySum / float64(s.heuristicCount)
	}
	if heurAvg > 0 {
		improvement = (heurAvg - mlAvg) / heurAvg * 100.0
	}

	return Stats{
		TotalObservations: s.total,
		UniqueArms:        len(s.arms),
		UniqueNodes:       len(s.nodeTaskCounts),
		MLAvgLatencyMs:    mlAvg,
		HeurAvgLatencyMs:  heurAvg,
		ImprovementPct:    improvement,
		GiniCoefficient:   s.giniCoefficient(),
	}
}

// GatePassed returns true if the ML scheduler outperforms the heuristic
// baseline by at least the given percentage (e.g., 30.0 for 30%).
//
// Phase 6 gate check: "ML scheduler outperforms heuristic by 30%+ on latency".
func (s *Scheduler) GatePassed(minImprovementPct float64) bool {
	st := s.Stats()
	return st.HeurAvgLatencyMs > 0 && st.ImprovementPct >= minImprovementPct
}

// ─── Observation History ────────────────────────────────────────────────────

// Observations returns the most recent N observations.
func (s *Scheduler) Observations(limit int) []Observation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if s.hFull {
		count = len(s.hist)
	} else {
		count = s.hIdx
	}
	if limit > count {
		limit = count
	}
	if limit <= 0 {
		return nil
	}

	result := make([]Observation, limit)
	// Read backwards from most recent.
	writeIdx := s.hIdx
	for i := 0; i < limit; i++ {
		writeIdx--
		if writeIdx < 0 {
			writeIdx = len(s.hist) - 1
		}
		result[i] = s.hist[writeIdx]
	}
	return result
}

// ─── Arm Inspection ─────────────────────────────────────────────────────────

// ArmInfo exposes the statistics of a single bandit arm.
type ArmInfo struct {
	Key      string  // arm identifier
	Pulls    int     // times this arm was selected
	MeanQ    float64 // average reward
	Variance float64 // reward variance
	UCBScore float64 // current UCB1 score
}

// Arms returns statistics for all known arms.
func (s *Scheduler) Arms() []ArmInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ArmInfo, 0, len(s.arms))
	for key, arm := range s.arms {
		result = append(result, ArmInfo{
			Key:      key,
			Pulls:    arm.pulls,
			MeanQ:    arm.mean,
			Variance: arm.variance(),
			UCBScore: s.ucb1Score(arm),
		})
	}
	return result
}

// Reset clears all learned state. Useful for testing or when the
// network topology changes dramatically.
func (s *Scheduler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.arms = make(map[string]*armStats)
	s.total = 0
	s.hist = make([]Observation, s.cfg.HistoryCapacity)
	s.hIdx = 0
	s.hFull = false
	s.mlLatencySum = 0
	s.mlCount = 0
	s.heuristicLatencySum = 0
	s.heuristicCount = 0
	s.nodeTaskCounts = make(map[string]int64)
}
