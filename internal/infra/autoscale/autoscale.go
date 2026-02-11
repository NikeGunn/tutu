// Package autoscale implements Phase 6 predictive auto-scaling.
//
// Traditional auto-scaling is REACTIVE: observe high load → add capacity.
// This is always too late — the spike already hit before we respond.
//
// Predictive scaling is PROACTIVE: forecast demand → add capacity BEFORE
// the spike arrives. We use exponential smoothing with seasonal decomposition
// (a simplified Holt-Winters model) to learn hour-of-day and day-of-week
// patterns in task arrival rates.
//
// Key concepts for beginners:
//
//   - Exponential Smoothing: a forecasting technique where recent observations
//     have more influence than older ones. The smoothing parameter α (alpha)
//     controls how quickly the forecast adapts. α=0.3 means "30% weight to
//     the newest observation, 70% to the previous forecast."
//
//   - Seasonal Decomposition: demand often follows patterns — busy during
//     work hours, quiet at night. We maintain a "seasonal index" for each
//     hour of the day (24 buckets). If the 10:00 hour is typically 1.5×
//     average demand, its seasonal index is 1.5.
//
//   - Pre-warming: waking idle nodes BEFORE the predicted spike so they're
//     ready to serve when traffic arrives. Avoids cold-start latency.
//
//   - Scale decisions: UP, DOWN, or HOLD based on forecast vs current capacity.
//
// Architecture ref: Phase 6 spec — "Predictive Scaling" deliverable.
// Gate check: 90% of demand spikes handled proactively.
package autoscale

import (
	"sync"
	"time"
)

// ─── Configuration ──────────────────────────────────────────────────────────

// Config configures the predictive auto-scaler.
type Config struct {
	// Alpha is the smoothing factor for exponential smoothing (0 < α ≤ 1).
	// Higher = adapts faster to new data, lower = smoother, slower adaptation.
	Alpha float64

	// SeasonalPeriod is the number of buckets in one seasonal cycle.
	// Default 24 = one bucket per hour of the day.
	SeasonalPeriod int

	// SeasonalAlpha is the learning rate for seasonal indices.
	// Lower = seasonal patterns change slowly (good for stable workloads).
	SeasonalAlpha float64

	// ScaleUpThreshold: if forecast exceeds capacity × this factor, scale up.
	// E.g., 0.8 means "scale up when forecast is 80% of current capacity."
	ScaleUpThreshold float64

	// ScaleDownThreshold: if forecast is below capacity × this factor, scale down.
	// E.g., 0.3 means "scale down when forecast is below 30% of capacity."
	ScaleDownThreshold float64

	// MinCapacity is the floor — never scale below this.
	MinCapacity int

	// MaxCapacity is the ceiling — never scale above this.
	MaxCapacity int

	// PreWarmLeadTime is how far ahead to pre-warm nodes before a predicted spike.
	PreWarmLeadTime time.Duration

	// CooldownPeriod prevents rapid oscillation between scale-up and scale-down.
	CooldownPeriod time.Duration

	// Now is an injectable clock for testing.
	Now func() time.Time
}

// DefaultConfig returns production defaults.
func DefaultConfig() Config {
	return Config{
		Alpha:              0.3,
		SeasonalPeriod:     24, // 24 hourly buckets
		SeasonalAlpha:      0.1,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.3,
		MinCapacity:        1,
		MaxCapacity:        1000,
		PreWarmLeadTime:    10 * time.Minute,
		CooldownPeriod:     5 * time.Minute,
		Now:                time.Now,
	}
}

// ─── Scale Decision ─────────────────────────────────────────────────────────

// Direction indicates the scaling recommendation.
type Direction int

const (
	Hold     Direction = iota // No change needed
	ScaleUp                   // Increase capacity
	ScaleDown                 // Decrease capacity
	PreWarm                   // Wake idle nodes proactively
)

// String returns a human-readable direction label.
func (d Direction) String() string {
	switch d {
	case Hold:
		return "HOLD"
	case ScaleUp:
		return "SCALE_UP"
	case ScaleDown:
		return "SCALE_DOWN"
	case PreWarm:
		return "PRE_WARM"
	default:
		return "UNKNOWN"
	}
}

// Decision is a scaling recommendation produced by the forecaster.
type Decision struct {
	Direction       Direction // what to do
	CurrentCapacity int       // current node count
	TargetCapacity  int       // recommended node count
	ForecastDemand  float64   // predicted demand (tasks per interval)
	Confidence      float64   // 0..1, based on data maturity
	Reason          string    // human-readable explanation
	DecidedAt       time.Time // when the decision was made
	Proactive       bool      // true if decided BEFORE the spike (vs reactive)
}

// ─── Demand Sample ──────────────────────────────────────────────────────────

// Sample records the observed demand at a point in time.
type Sample struct {
	Demand    float64   // observed task arrival rate (tasks/interval)
	Timestamp time.Time // when observed
}

// ─── Scaler ─────────────────────────────────────────────────────────────────

// Scaler is the Phase 6 predictive auto-scaler.
type Scaler struct {
	mu  sync.RWMutex
	cfg Config

	// Exponential smoothing state.
	smoothed float64 // current smoothed level estimate
	inited   bool    // whether smoothed has been initialized

	// Seasonal indices: one per hour-of-day (default 24 buckets).
	// A value of 1.0 = average demand, 1.5 = 50% above average, etc.
	seasonal []float64

	// Current capacity.
	capacity int

	// Decision tracking.
	lastDecision  time.Time // for cooldown enforcement
	decisions     []Decision
	maxDecisions  int // ring buffer cap
	dIdx          int
	dFull         bool

	// Proactiveness tracking (gate check: 90% proactive).
	totalSpikes     int64 // total demand spikes observed
	proactiveSpikes int64 // spikes where we scaled BEFORE they hit

	// Observation count for confidence calculation.
	observationCount int
}

// NewScaler creates a new predictive auto-scaler.
func NewScaler(cfg Config) *Scaler {
	if cfg.Alpha <= 0 || cfg.Alpha > 1 {
		cfg.Alpha = 0.3
	}
	if cfg.SeasonalPeriod <= 0 {
		cfg.SeasonalPeriod = 24
	}
	if cfg.SeasonalAlpha <= 0 || cfg.SeasonalAlpha > 1 {
		cfg.SeasonalAlpha = 0.1
	}
	if cfg.MinCapacity <= 0 {
		cfg.MinCapacity = 1
	}
	if cfg.MaxCapacity <= 0 {
		cfg.MaxCapacity = 1000
	}
	if cfg.MaxCapacity < cfg.MinCapacity {
		cfg.MaxCapacity = cfg.MinCapacity
	}
	if cfg.PreWarmLeadTime <= 0 {
		cfg.PreWarmLeadTime = 10 * time.Minute
	}
	if cfg.CooldownPeriod <= 0 {
		cfg.CooldownPeriod = 5 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	seasonal := make([]float64, cfg.SeasonalPeriod)
	for i := range seasonal {
		seasonal[i] = 1.0 // start flat — no seasonal pattern learned yet
	}

	return &Scaler{
		cfg:          cfg,
		seasonal:     seasonal,
		capacity:     cfg.MinCapacity,
		maxDecisions: 10_000,
		decisions:    make([]Decision, 10_000),
	}
}

// ─── Seasonal Bucket ────────────────────────────────────────────────────────

// seasonBucket returns which seasonal bucket a timestamp falls into.
// For the default period of 24, this is just the hour of the day.
func (s *Scaler) seasonBucket(t time.Time) int {
	if s.cfg.SeasonalPeriod == 24 {
		return t.Hour()
	}
	// Generic: divide the day into N equal buckets.
	minuteOfDay := t.Hour()*60 + t.Minute()
	bucketSize := (24 * 60) / s.cfg.SeasonalPeriod
	if bucketSize <= 0 {
		bucketSize = 1
	}
	bucket := minuteOfDay / bucketSize
	if bucket >= s.cfg.SeasonalPeriod {
		bucket = s.cfg.SeasonalPeriod - 1
	}
	return bucket
}

// ─── Core: Record Observation ───────────────────────────────────────────────

// RecordDemand records an observed demand sample and updates the forecasting model.
//
// Exponential smoothing update:
//
//	deseasonalized = demand / seasonal[bucket]
//	smoothed = α * deseasonalized + (1 - α) * smoothed
//	seasonal[bucket] = β * (demand / smoothed) + (1 - β) * seasonal[bucket]
//
// This is a simplified multiplicative Holt-Winters without the trend component
// (we omit trend because P2P network demand is more cyclical than trending).
func (s *Scaler) RecordDemand(sample Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.seasonBucket(sample.Timestamp)

	if !s.inited {
		// First observation — initialize smoothed level directly.
		s.smoothed = sample.Demand
		s.inited = true
		s.observationCount++
		return
	}

	// Deseasonalize: remove seasonal effect to get the "true" level.
	seasonalFactor := s.seasonal[bucket]
	if seasonalFactor <= 0 {
		seasonalFactor = 1.0
	}
	deseasonalized := sample.Demand / seasonalFactor

	// Update smoothed level.
	s.smoothed = s.cfg.Alpha*deseasonalized + (1-s.cfg.Alpha)*s.smoothed

	// Update seasonal index — learn how this hour differs from average.
	if s.smoothed > 0 {
		observed := sample.Demand / s.smoothed
		s.seasonal[bucket] = s.cfg.SeasonalAlpha*observed + (1-s.cfg.SeasonalAlpha)*s.seasonal[bucket]
	}

	s.observationCount++
}

// ─── Core: Forecast ─────────────────────────────────────────────────────────

// Forecast predicts demand at a given future time.
//
// forecast(t) = smoothed_level × seasonal[bucket(t)]
//
// The smoothed level is our best estimate of "base demand" right now,
// and the seasonal index adjusts it for the time of day.
func (s *Scaler) Forecast(at time.Time) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.inited {
		return 0
	}

	bucket := s.seasonBucket(at)
	return s.smoothed * s.seasonal[bucket]
}

// ─── Core: Evaluate & Decide ────────────────────────────────────────────────

// Evaluate examines the current state and produces a scaling decision.
// Call this periodically (e.g., every minute) to get recommendations.
func (s *Scaler) Evaluate() Decision {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.cfg.Now()
	forecast := s.forecastLocked(now)
	forecastAhead := s.forecastLocked(now.Add(s.cfg.PreWarmLeadTime))

	// Confidence based on data maturity (ramp up over first 48 observations).
	confidence := float64(s.observationCount) / 48.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	decision := Decision{
		Direction:       Hold,
		CurrentCapacity: s.capacity,
		TargetCapacity:  s.capacity,
		ForecastDemand:  forecast,
		Confidence:      confidence,
		DecidedAt:       now,
	}

	// Check cooldown.
	if !s.lastDecision.IsZero() && now.Sub(s.lastDecision) < s.cfg.CooldownPeriod {
		decision.Reason = "cooldown active — holding"
		s.recordDecisionLocked(decision)
		return decision
	}

	capFloat := float64(s.capacity)

	// Check if pre-warm is needed: forecast shows upcoming spike.
	if forecastAhead > capFloat*s.cfg.ScaleUpThreshold && forecast <= capFloat*s.cfg.ScaleUpThreshold {
		target := s.clampCapacity(int(forecastAhead/s.cfg.ScaleUpThreshold) + 1)
		decision.Direction = PreWarm
		decision.TargetCapacity = target
		decision.Proactive = true
		decision.Reason = "forecast shows upcoming spike — pre-warming nodes"
		s.capacity = target
		s.lastDecision = now
		s.proactiveSpikes++
		s.totalSpikes++
		s.recordDecisionLocked(decision)
		return decision
	}

	// Scale up: current demand exceeds threshold.
	if forecast > capFloat*s.cfg.ScaleUpThreshold {
		target := s.clampCapacity(int(forecast/s.cfg.ScaleUpThreshold) + 1)
		decision.Direction = ScaleUp
		decision.TargetCapacity = target
		decision.Proactive = false // reactive — spike already here
		decision.Reason = "demand exceeds capacity threshold — scaling up"
		s.capacity = target
		s.lastDecision = now
		s.totalSpikes++
		s.recordDecisionLocked(decision)
		return decision
	}

	// Scale down: demand well below capacity.
	if forecast < capFloat*s.cfg.ScaleDownThreshold && s.capacity > s.cfg.MinCapacity {
		target := s.clampCapacity(int(forecast/s.cfg.ScaleDownThreshold) + 1)
		if target < s.capacity {
			decision.Direction = ScaleDown
			decision.TargetCapacity = target
			decision.Reason = "demand below threshold — scaling down"
			s.capacity = target
			s.lastDecision = now
			s.recordDecisionLocked(decision)
			return decision
		}
	}

	decision.Reason = "demand within acceptable range — holding"
	s.recordDecisionLocked(decision)
	return decision
}

// forecastLocked predicts demand at a time. Must hold at least mu.RLock.
func (s *Scaler) forecastLocked(at time.Time) float64 {
	if !s.inited {
		return 0
	}
	bucket := s.seasonBucket(at)
	return s.smoothed * s.seasonal[bucket]
}

// clampCapacity keeps capacity within configured bounds.
func (s *Scaler) clampCapacity(target int) int {
	if target < s.cfg.MinCapacity {
		return s.cfg.MinCapacity
	}
	if target > s.cfg.MaxCapacity {
		return s.cfg.MaxCapacity
	}
	return target
}

// recordDecisionLocked appends a decision to the ring buffer.
func (s *Scaler) recordDecisionLocked(d Decision) {
	s.decisions[s.dIdx] = d
	s.dIdx++
	if s.dIdx >= s.maxDecisions {
		s.dIdx = 0
		s.dFull = true
	}
}

// ─── Spike Recording ────────────────────────────────────────────────────────

// RecordSpike records that a demand spike was observed.
// If the scaler had already scaled/pre-warmed before this spike,
// proactive is true. Otherwise it's a reactive response.
func (s *Scaler) RecordSpike(proactive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalSpikes++
	if proactive {
		s.proactiveSpikes++
	}
}

// ─── Capacity Management ────────────────────────────────────────────────────

// SetCapacity updates the current capacity (e.g., after external scaling).
func (s *Scaler) SetCapacity(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capacity = s.clampCapacity(n)
}

// Capacity returns the current capacity.
func (s *Scaler) Capacity() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capacity
}

// ─── Statistics & Gate Check ────────────────────────────────────────────────

// ScalerStats exposes auto-scaler metrics.
type ScalerStats struct {
	SmoothedLevel    float64         // current smoothed demand level
	SeasonalIndices  []float64       // one per season bucket
	CurrentCapacity  int             // current capacity
	Observations     int             // total observations recorded
	TotalDecisions   int             // total decisions made
	TotalSpikes      int64           // total spikes observed
	ProactiveSpikes  int64           // spikes handled proactively
	ProactivePct     float64         // proactive / total × 100
	Confidence       float64         // data maturity confidence 0..1
}

// Stats returns current scaler statistics.
func (s *Scaler) Stats() ScalerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	indices := make([]float64, len(s.seasonal))
	copy(indices, s.seasonal)

	var proactivePct float64
	if s.totalSpikes > 0 {
		proactivePct = float64(s.proactiveSpikes) / float64(s.totalSpikes) * 100.0
	}

	var totalDecisions int
	if s.dFull {
		totalDecisions = s.maxDecisions
	} else {
		totalDecisions = s.dIdx
	}

	confidence := float64(s.observationCount) / 48.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return ScalerStats{
		SmoothedLevel:   s.smoothed,
		SeasonalIndices: indices,
		CurrentCapacity: s.capacity,
		Observations:    s.observationCount,
		TotalDecisions:  totalDecisions,
		TotalSpikes:     s.totalSpikes,
		ProactiveSpikes: s.proactiveSpikes,
		ProactivePct:    proactivePct,
		Confidence:      confidence,
	}
}

// GatePassed returns true if at least the given percentage of demand spikes
// were handled proactively.
//
// Phase 6 gate check: "90% of demand spikes handled proactively".
func (s *Scaler) GatePassed(minProactivePct float64) bool {
	st := s.Stats()
	return st.TotalSpikes > 0 && st.ProactivePct >= minProactivePct
}

// ─── Recent Decisions ───────────────────────────────────────────────────────

// RecentDecisions returns the most recent N scaling decisions.
func (s *Scaler) RecentDecisions(limit int) []Decision {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	if s.dFull {
		count = s.maxDecisions
	} else {
		count = s.dIdx
	}
	if limit > count {
		limit = count
	}
	if limit <= 0 {
		return nil
	}

	result := make([]Decision, limit)
	idx := s.dIdx
	for i := 0; i < limit; i++ {
		idx--
		if idx < 0 {
			idx = s.maxDecisions - 1
		}
		result[i] = s.decisions[idx]
	}
	return result
}

// ─── Season Inspection ──────────────────────────────────────────────────────

// PeakHours returns the top N seasonal buckets by demand index.
// Useful for understanding when the network is busiest.
func (s *Scaler) PeakHours(topN int) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if topN <= 0 || topN > len(s.seasonal) {
		topN = len(s.seasonal)
	}

	// Build index list sorted by seasonal value (descending).
	type hourVal struct {
		hour int
		val  float64
	}
	hvs := make([]hourVal, len(s.seasonal))
	for i, v := range s.seasonal {
		hvs[i] = hourVal{i, v}
	}
	// Simple insertion sort — only 24 elements.
	for i := 1; i < len(hvs); i++ {
		key := hvs[i]
		j := i - 1
		for j >= 0 && hvs[j].val < key.val {
			hvs[j+1] = hvs[j]
			j--
		}
		hvs[j+1] = key
	}

	result := make([]int, topN)
	for i := 0; i < topN; i++ {
		result[i] = hvs[i].hour
	}
	return result
}

// Reset clears all learned state.
func (s *Scaler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.smoothed = 0
	s.inited = false
	s.observationCount = 0
	for i := range s.seasonal {
		s.seasonal[i] = 1.0
	}
	s.capacity = s.cfg.MinCapacity
	s.lastDecision = time.Time{}
	s.decisions = make([]Decision, s.maxDecisions)
	s.dIdx = 0
	s.dFull = false
	s.totalSpikes = 0
	s.proactiveSpikes = 0
}
