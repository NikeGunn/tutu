// Package reputation implements EMA-based trust scoring for network nodes.
//
// Each node has 5 reputation components:
//   - Reliability: did tasks complete successfully?
//   - Accuracy: were results verified correct?
//   - Availability: was the node online when needed?
//   - Speed: how fast relative to expected time?
//   - Longevity: how long has the node been active?
//
// Overall = 0.30×reliability + 0.25×accuracy + 0.20×availability
//         + 0.15×speed + 0.10×longevity − penalties
//
// Architecture: Reputation EMA (Part XX §3).
// Phase 5 spec: "ML-based behavioral analysis, resource abuse patterns."
package reputation

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// ─── Constants ──────────────────────────────────────────────────────────────

const (
	// Component weights (sum to 1.0 before penalty deduction)
	WeightReliability  = 0.30
	WeightAccuracy     = 0.25
	WeightAvailability = 0.20
	WeightSpeed        = 0.15
	WeightLongevity    = 0.10

	// PenaltyWeight is the deduction factor for accumulated penalties.
	PenaltyWeight = 0.05

	// AlphaNormal is the EMA smoothing factor for established nodes.
	// Low α = slow adaptation = resistant to manipulation.
	AlphaNormal = 0.1

	// AlphaColdStart is used for the first ColdStartTasks events.
	// Higher α = faster convergence during onboarding.
	AlphaColdStart = 0.3

	// ColdStartTasks is how many tasks before switching to normal α.
	ColdStartTasks = 10

	// DefaultReputation for brand new nodes (neutral).
	DefaultReputation = 0.5

	// FloorReputation is the minimum score — nodes always get a second chance.
	FloorReputation = 0.1

	// CeilingReputation is the absolute maximum.
	CeilingReputation = 1.0

	// DecayRatePerWeek is the weekly decay for inactive nodes (1%).
	DecayRatePerWeek = 0.01

	// LongevityFullDays is how many active days for maximum longevity score.
	LongevityFullDays = 180
)

// ─── Types ──────────────────────────────────────────────────────────────────

// Components holds the 5 individual reputation components.
type Components struct {
	Reliability  float64 `json:"reliability"`  // EMA of successful task completion
	Accuracy     float64 `json:"accuracy"`     // EMA of verified-correct results
	Availability float64 `json:"availability"` // EMA of online-when-needed checks
	Speed        float64 `json:"speed"`        // EMA of min(1.0, expected/actual time)
	Longevity    float64 `json:"longevity"`    // min(1.0, active_days / 180)
}

// NodeReputation stores a node's complete reputation state.
type NodeReputation struct {
	NodeID       string     `json:"node_id"`
	Components   Components `json:"components"`
	Penalties    float64    `json:"penalties"`     // Accumulated penalty score [0, ∞)
	TaskCount    int        `json:"task_count"`    // Number of tasks evaluated
	DaysActive   int        `json:"days_active"`   // Calendar days node has been active
	LastUpdate   time.Time  `json:"last_update"`
	LastDecay    time.Time  `json:"last_decay"`    // Last weekly decay timestamp
	JoinedAt     time.Time  `json:"joined_at"`
}

// Overall computes the weighted reputation score.
//
//	overall = Σ(weight_i × component_i) − penaltyWeight × penalties
//
// Clamped to [FloorReputation, CeilingReputation].
func (nr *NodeReputation) Overall() float64 {
	c := nr.Components
	score := WeightReliability*c.Reliability +
		WeightAccuracy*c.Accuracy +
		WeightAvailability*c.Availability +
		WeightSpeed*c.Speed +
		WeightLongevity*c.Longevity -
		PenaltyWeight*nr.Penalties

	return clamp(score, FloorReputation, CeilingReputation)
}

// IsTrusted returns whether this node meets a minimum threshold.
func (nr *NodeReputation) IsTrusted(threshold float64) bool {
	return nr.Overall() >= threshold
}

// TrustTier returns a human-label for the trust level.
func (nr *NodeReputation) TrustTier() string {
	o := nr.Overall()
	switch {
	case o >= 0.9:
		return "EXCELLENT"
	case o >= 0.7:
		return "GOOD"
	case o >= 0.5:
		return "NEUTRAL"
	case o >= 0.3:
		return "LOW"
	default:
		return "POOR"
	}
}

// alpha returns the EMA smoothing factor — faster during cold start.
func (nr *NodeReputation) alpha() float64 {
	if nr.TaskCount < ColdStartTasks {
		return AlphaColdStart
	}
	return AlphaNormal
}

// ─── Update Events ──────────────────────────────────────────────────────────

// TaskOutcome describes the result of a task for reputation scoring.
type TaskOutcome struct {
	Successful     bool          // Did the task complete without error?
	ResultVerified bool          // Was the result verified correct?
	ExpectedTime   time.Duration // How long was expected?
	ActualTime     time.Duration // How long did it actually take?
}

// AvailabilityCheck describes whether a node was online when pinged.
type AvailabilityCheck struct {
	WasOnline bool
}

// PenaltyEvent logs a penalty against a node.
type PenaltyEvent struct {
	Severity float64 // How severe (0.1 = minor, 1.0 = major)
	Reason   string
}

// ─── Configuration ──────────────────────────────────────────────────────────

// TrackerConfig configures the reputation tracker.
type TrackerConfig struct {
	DecayInterval time.Duration // How often to check for decay (default: 24h)
	DecayRate     float64       // Weekly decay rate (default: 0.01)
}

// DefaultTrackerConfig returns Phase 5 defaults.
func DefaultTrackerConfig() TrackerConfig {
	return TrackerConfig{
		DecayInterval: 24 * time.Hour,
		DecayRate:     DecayRatePerWeek,
	}
}

// ─── Tracker ────────────────────────────────────────────────────────────────

// Tracker manages reputation for all nodes in the network.
// Thread-safe via RWMutex.
type Tracker struct {
	mu     sync.RWMutex
	config TrackerConfig
	nodes  map[string]*NodeReputation // nodeID → reputation

	// Injectable clock for testing.
	now func() time.Time
}

// NewTracker creates a reputation tracker.
func NewTracker(cfg TrackerConfig) *Tracker {
	return &Tracker{
		config: cfg,
		nodes:  make(map[string]*NodeReputation),
		now:    time.Now,
	}
}

// ─── Node Registration ─────────────────────────────────────────────────────

// Register initializes reputation for a new node at the default neutral level.
func (t *Tracker) Register(nodeID string) *NodeReputation {
	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.nodes[nodeID]; ok {
		return existing
	}

	now := t.now()
	rep := &NodeReputation{
		NodeID: nodeID,
		Components: Components{
			Reliability:  DefaultReputation,
			Accuracy:     DefaultReputation,
			Availability: DefaultReputation,
			Speed:        DefaultReputation,
			Longevity:    0, // No longevity yet
		},
		LastUpdate: now,
		LastDecay:  now,
		JoinedAt:   now,
	}
	t.nodes[nodeID] = rep
	return rep
}

// Get returns a node's current reputation. Returns nil if not registered.
func (t *Tracker) Get(nodeID string) *NodeReputation {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nodes[nodeID]
}

// GetOrRegister returns existing reputation or registers a new node.
func (t *Tracker) GetOrRegister(nodeID string) *NodeReputation {
	t.mu.RLock()
	if rep, ok := t.nodes[nodeID]; ok {
		t.mu.RUnlock()
		return rep
	}
	t.mu.RUnlock()
	return t.Register(nodeID)
}

// ─── Score Updates ──────────────────────────────────────────────────────────

// RecordTask updates reputation based on a task outcome.
// Updates reliability, accuracy, and speed in one call.
func (t *Tracker) RecordTask(nodeID string, outcome TaskOutcome) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	rep, ok := t.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not registered", nodeID)
	}

	α := rep.alpha()

	// Reliability: 1.0 if successful, 0.0 if failed
	reliabilitySignal := 0.0
	if outcome.Successful {
		reliabilitySignal = 1.0
	}
	rep.Components.Reliability = ema(rep.Components.Reliability, reliabilitySignal, α)

	// Accuracy: 1.0 if verified, 0.0 if not (only update if task completed)
	if outcome.Successful {
		accuracySignal := 0.0
		if outcome.ResultVerified {
			accuracySignal = 1.0
		}
		rep.Components.Accuracy = ema(rep.Components.Accuracy, accuracySignal, α)
	}

	// Speed: min(1.0, expected / actual) — 1.0 means on-time or faster
	if outcome.ActualTime > 0 && outcome.ExpectedTime > 0 {
		speedSignal := math.Min(1.0, float64(outcome.ExpectedTime)/float64(outcome.ActualTime))
		rep.Components.Speed = ema(rep.Components.Speed, speedSignal, α)
	}

	rep.TaskCount++
	rep.LastUpdate = t.now()

	// Update longevity based on days since join
	days := int(t.now().Sub(rep.JoinedAt).Hours() / 24)
	if days > rep.DaysActive {
		rep.DaysActive = days
	}
	rep.Components.Longevity = math.Min(1.0, float64(rep.DaysActive)/float64(LongevityFullDays))

	return nil
}

// RecordAvailability updates the availability component.
func (t *Tracker) RecordAvailability(nodeID string, check AvailabilityCheck) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	rep, ok := t.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not registered", nodeID)
	}

	signal := 0.0
	if check.WasOnline {
		signal = 1.0
	}
	rep.Components.Availability = ema(rep.Components.Availability, signal, rep.alpha())
	rep.LastUpdate = t.now()
	return nil
}

// RecordPenalty adds a penalty to a node's reputation.
func (t *Tracker) RecordPenalty(nodeID string, penalty PenaltyEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	rep, ok := t.nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not registered", nodeID)
	}

	rep.Penalties += penalty.Severity
	rep.LastUpdate = t.now()
	return nil
}

// ─── Decay ──────────────────────────────────────────────────────────────────

// ApplyDecay reduces reputation for inactive nodes.
// Decay: 1% per week of inactivity (prevents ghost nodes keeping high rep).
// Should be called periodically (e.g. daily).
func (t *Tracker) ApplyDecay() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	decayed := 0

	for _, rep := range t.nodes {
		weeksSinceUpdate := now.Sub(rep.LastUpdate).Hours() / (24 * 7)
		if weeksSinceUpdate < 1 {
			continue // Active within the last week
		}

		// Only decay if enough time has passed since last decay
		weeksSinceDecay := now.Sub(rep.LastDecay).Hours() / (24 * 7)
		if weeksSinceDecay < 1 {
			continue
		}

		// Apply decay: reduce each component by rate * weeks
		decayFactor := 1.0 - t.config.DecayRate*math.Floor(weeksSinceDecay)
		if decayFactor < 0 {
			decayFactor = 0
		}

		rep.Components.Reliability *= decayFactor
		rep.Components.Accuracy *= decayFactor
		rep.Components.Availability *= decayFactor
		rep.Components.Speed *= decayFactor

		// Enforce floor
		rep.Components.Reliability = math.Max(rep.Components.Reliability, FloorReputation)
		rep.Components.Accuracy = math.Max(rep.Components.Accuracy, FloorReputation)
		rep.Components.Availability = math.Max(rep.Components.Availability, FloorReputation)
		rep.Components.Speed = math.Max(rep.Components.Speed, FloorReputation)

		rep.LastDecay = now
		decayed++
	}

	return decayed
}

// ─── Queries ────────────────────────────────────────────────────────────────

// TopNodes returns nodes sorted by overall reputation, descending.
func (t *Tracker) TopNodes(limit int) []*NodeReputation {
	t.mu.RLock()
	defer t.mu.RUnlock()

	nodes := make([]*NodeReputation, 0, len(t.nodes))
	for _, rep := range t.nodes {
		nodes = append(nodes, rep)
	}

	// Sort by overall descending
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].Overall() > nodes[i].Overall() {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}

	if limit > 0 && limit < len(nodes) {
		nodes = nodes[:limit]
	}
	return nodes
}

// TrustedNodes returns nodes meeting the minimum reputation threshold.
func (t *Tracker) TrustedNodes(threshold float64) []*NodeReputation {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*NodeReputation, 0)
	for _, rep := range t.nodes {
		if rep.Overall() >= threshold {
			result = append(result, rep)
		}
	}
	return result
}

// NodeCount returns total registered nodes.
func (t *Tracker) NodeCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.nodes)
}

// Remove deletes a node's reputation record.
func (t *Tracker) Remove(nodeID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.nodes, nodeID)
}

// ─── Pure Helper Functions ──────────────────────────────────────────────────

// ema computes the Exponential Moving Average:
//
//	new = α × sample + (1 - α) × old
func ema(old, sample, alpha float64) float64 {
	return alpha*sample + (1-alpha)*old
}

// clamp restricts a value to [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
