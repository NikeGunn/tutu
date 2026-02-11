// Package intelligence implements Phase 6 network intelligence.
//
// The network intelligence layer optimizes HOW models are distributed
// across nodes in the TuTu network. It answers three questions:
//
//  1. PLACEMENT: Which nodes should host which models for maximum performance?
//  2. RETIREMENT: Which models are no longer used and should be removed?
//  3. FEDERATION: What can we learn from cross-organization health patterns?
//
// Key concepts for beginners:
//
//   - Model Placement: popular models should live on fast, well-connected nodes.
//     Unpopular models can live on slower nodes or be evicted entirely. The
//     Optimizer periodically recomputes an ideal placement and emits
//     recommendations (move model X from node A to node B).
//
//   - Affinity Score: a per-{model, node} score combining cache hit rate,
//     inference latency, GPU VRAM fit, and request frequency. Higher affinity
//     means the model "belongs" on that node.
//
//   - Model Retirement: models that haven't been requested in RetirementDays
//     (default 30) are candidates for removal. Removing unused models frees
//     disk space for hotter models, improving cache hit rates.
//
//   - Federated Health Learning: aggregating health patterns across multiple
//     organizations (federated learning on network telemetry). Each org
//     contributes summary statistics (not raw data) about failure rates,
//     recovery patterns, etc. The aggregate reveals systemic issues.
//
// Architecture ref: Phase 6 spec — "Network Intelligence" deliverable.
// Gate check: "Network self-optimizes model placement weekly."
package intelligence

import (
	"sort"
	"sync"
	"time"
)

// ─── Configuration ──────────────────────────────────────────────────────────

// Config configures the network intelligence engine.
type Config struct {
	// RetirementDays is how many days without requests before a model is
	// considered a retirement candidate.
	RetirementDays int

	// PlacementInterval is how often the optimizer recomputes ideal placement.
	PlacementInterval time.Duration

	// MinRequestsForPlacement is the minimum request count before a model
	// is considered for placement optimization (avoids noise from one-off requests).
	MinRequestsForPlacement int64

	// MaxRecommendations caps how many placement moves are recommended per cycle.
	MaxRecommendations int

	// MaxRetirementCandidates caps how many models are flagged for retirement per cycle.
	MaxRetirementCandidates int

	// HealthHistorySize caps the federated health pattern history.
	HealthHistorySize int

	// Now is an injectable clock for testing.
	Now func() time.Time
}

// DefaultConfig returns production defaults.
func DefaultConfig() Config {
	return Config{
		RetirementDays:          30,
		PlacementInterval:       7 * 24 * time.Hour, // weekly
		MinRequestsForPlacement: 10,
		MaxRecommendations:      50,
		MaxRetirementCandidates: 100,
		HealthHistorySize:       10_000,
		Now:                     time.Now,
	}
}

// ─── Model Popularity ───────────────────────────────────────────────────────

// ModelPopularity tracks how popular a model is across the network.
type ModelPopularity struct {
	ModelName    string    // model identifier
	TotalReqs    int64     // total requests served
	RecentReqs   int64     // requests in the last 24h window
	LastRequested time.Time // most recent request timestamp
	AvgLatencyMs float64   // average inference latency
}

// ─── Node Model Affinity ────────────────────────────────────────────────────

// NodeModelAffinity describes how well a specific model fits on a specific node.
type NodeModelAffinity struct {
	NodeID       string  // node identifier
	ModelName    string  // model identifier
	CacheHitRate float64 // 0..1 — how often the model is found hot in memory
	AvgLatencyMs float64 // average inference latency on this node
	RequestCount int64   // requests served on this node
	VRAMFitScore float64 // 0..1 — how well the model fits in available VRAM
	AffinityScore float64 // computed composite score 0..1
}

// ─── Placement Recommendation ───────────────────────────────────────────────

// RecommendationType describes what kind of placement change is suggested.
type RecommendationType int

const (
	RecommendPlace  RecommendationType = iota // Place model on a new node
	RecommendEvict                             // Remove model from a node
	RecommendMove                              // Move model from one node to another
)

// String returns a human-readable recommendation type.
func (r RecommendationType) String() string {
	switch r {
	case RecommendPlace:
		return "PLACE"
	case RecommendEvict:
		return "EVICT"
	case RecommendMove:
		return "MOVE"
	default:
		return "UNKNOWN"
	}
}

// Recommendation is a single placement optimization suggestion.
type Recommendation struct {
	Type      RecommendationType
	ModelName string  // which model
	FromNode  string  // source node (empty for PLACE)
	ToNode    string  // destination node (empty for EVICT)
	Reason    string  // human-readable justification
	Score     float64 // expected improvement score 0..1
	CreatedAt time.Time
}

// ─── Retirement Candidate ───────────────────────────────────────────────────

// RetirementCandidate is a model flagged for potential removal.
type RetirementCandidate struct {
	ModelName     string
	LastRequested time.Time
	DaysSinceUse  int
	SizeBytes     int64
	Reason        string
}

// ─── Federated Health Pattern ───────────────────────────────────────────────

// HealthPattern is an aggregated health observation from an organization.
// No raw data is shared — only summary statistics (privacy-preserving).
type HealthPattern struct {
	OrgID          string    // anonymous organization identifier
	AvgFailureRate float64   // average task failure rate (0..1)
	AvgMTTR        float64   // average recovery time in seconds
	TopFailureType string    // most common failure type
	NodeCount      int       // number of nodes in the org
	TaskVolume     int64     // total tasks processed in the reporting period
	ReportedAt     time.Time // when the pattern was reported
}

// ─── Optimizer ──────────────────────────────────────────────────────────────

// Optimizer is the Phase 6 network intelligence engine.
type Optimizer struct {
	mu  sync.RWMutex
	cfg Config

	// Model popularity tracking.
	popularity map[string]*modelStats // modelName → stats

	// Per-{node, model} affinity tracking.
	affinities map[string]map[string]*affinityStats // nodeID → modelName → stats

	// Placement recommendation history.
	recommendations []Recommendation
	recIdx          int
	recCap          int
	recFull         bool

	// Retirement candidates from last scan.
	retirementCandidates []RetirementCandidate

	// Federated health patterns.
	healthPatterns []HealthPattern
	hpIdx          int
	hpFull         bool

	// Optimization cycle tracking.
	lastOptimization time.Time
	optimizationCount int64
}

// modelStats tracks request volume and latency for a model.
type modelStats struct {
	totalReqs    int64
	recentReqs   int64
	lastReq      time.Time
	latencySum   float64
	latencyCount int64
}

// affinityStats tracks per-{node, model} performance.
type affinityStats struct {
	requests     int64
	cacheHits    int64
	cacheMisses  int64
	latencySum   float64
	latencyCount int64
	vramFit      float64 // 0..1 — how much of VRAM the model uses (lower = better fit)
}

// NewOptimizer creates a new network intelligence optimizer.
func NewOptimizer(cfg Config) *Optimizer {
	if cfg.RetirementDays <= 0 {
		cfg.RetirementDays = 30
	}
	if cfg.PlacementInterval <= 0 {
		cfg.PlacementInterval = 7 * 24 * time.Hour
	}
	if cfg.MinRequestsForPlacement <= 0 {
		cfg.MinRequestsForPlacement = 10
	}
	if cfg.MaxRecommendations <= 0 {
		cfg.MaxRecommendations = 50
	}
	if cfg.MaxRetirementCandidates <= 0 {
		cfg.MaxRetirementCandidates = 100
	}
	if cfg.HealthHistorySize <= 0 {
		cfg.HealthHistorySize = 10_000
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &Optimizer{
		cfg:             cfg,
		popularity:      make(map[string]*modelStats),
		affinities:      make(map[string]map[string]*affinityStats),
		recommendations: make([]Recommendation, 1000),
		recCap:          1000,
		healthPatterns:  make([]HealthPattern, cfg.HealthHistorySize),
	}
}

// ─── Record Request ─────────────────────────────────────────────────────────

// RecordRequest records that a model was requested on a specific node.
// This updates both the global popularity and the per-node affinity.
func (o *Optimizer) RecordRequest(modelName, nodeID string, latencyMs float64, cacheHit bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := o.cfg.Now()

	// Update global model popularity.
	ms, exists := o.popularity[modelName]
	if !exists {
		ms = &modelStats{}
		o.popularity[modelName] = ms
	}
	ms.totalReqs++
	ms.recentReqs++
	ms.lastReq = now
	ms.latencySum += latencyMs
	ms.latencyCount++

	// Update per-{node, model} affinity.
	nodeMap, exists := o.affinities[nodeID]
	if !exists {
		nodeMap = make(map[string]*affinityStats)
		o.affinities[nodeID] = nodeMap
	}
	as, exists := nodeMap[modelName]
	if !exists {
		as = &affinityStats{}
		nodeMap[modelName] = as
	}
	as.requests++
	if cacheHit {
		as.cacheHits++
	} else {
		as.cacheMisses++
	}
	as.latencySum += latencyMs
	as.latencyCount++
}

// SetVRAMFit updates the VRAM fit score for a model on a node.
// 0.0 = model perfectly fits, 1.0 = model far too large for available VRAM.
func (o *Optimizer) SetVRAMFit(nodeID, modelName string, fitScore float64) {
	o.mu.Lock()
	defer o.mu.Unlock()

	nodeMap, exists := o.affinities[nodeID]
	if !exists {
		nodeMap = make(map[string]*affinityStats)
		o.affinities[nodeID] = nodeMap
	}
	as, exists := nodeMap[modelName]
	if !exists {
		as = &affinityStats{}
		nodeMap[modelName] = as
	}
	as.vramFit = fitScore
}

// ─── Model Popularity ───────────────────────────────────────────────────────

// TopModels returns the top N models by total request count.
func (o *Optimizer) TopModels(n int) []ModelPopularity {
	o.mu.RLock()
	defer o.mu.RUnlock()

	models := make([]ModelPopularity, 0, len(o.popularity))
	for name, ms := range o.popularity {
		var avgLat float64
		if ms.latencyCount > 0 {
			avgLat = ms.latencySum / float64(ms.latencyCount)
		}
		models = append(models, ModelPopularity{
			ModelName:     name,
			TotalReqs:     ms.totalReqs,
			RecentReqs:    ms.recentReqs,
			LastRequested: ms.lastReq,
			AvgLatencyMs:  avgLat,
		})
	}

	// Sort by total requests descending.
	sort.Slice(models, func(i, j int) bool {
		return models[i].TotalReqs > models[j].TotalReqs
	})

	if n > len(models) {
		n = len(models)
	}
	return models[:n]
}

// ─── Affinity Computation ───────────────────────────────────────────────────

// computeAffinity calculates the composite affinity score for a {node, model} pair.
//
// affinity = 0.3 * cacheHitRate + 0.3 * (1 - normalizedLatency) + 0.2 * requestShare + 0.2 * (1 - vramFit)
//
// Higher = model belongs on this node.
func computeAffinity(as *affinityStats, maxLatency float64, maxReqs int64) float64 {
	// Cache hit rate.
	var hitRate float64
	total := as.cacheHits + as.cacheMisses
	if total > 0 {
		hitRate = float64(as.cacheHits) / float64(total)
	}

	// Latency score (lower latency = higher score).
	var latScore float64
	if as.latencyCount > 0 && maxLatency > 0 {
		avgLat := as.latencySum / float64(as.latencyCount)
		latScore = 1.0 - (avgLat / maxLatency)
		if latScore < 0 {
			latScore = 0
		}
	}

	// Request share (how much of this model's traffic goes through this node).
	var reqShare float64
	if maxReqs > 0 {
		reqShare = float64(as.requests) / float64(maxReqs)
		if reqShare > 1 {
			reqShare = 1
		}
	}

	// VRAM fit (lower = better fit for the model).
	vramScore := 1.0 - as.vramFit
	if vramScore < 0 {
		vramScore = 0
	}

	return 0.30*hitRate + 0.30*latScore + 0.20*reqShare + 0.20*vramScore
}

// NodeAffinities returns affinity scores for all {node, model} pairs for a given model.
func (o *Optimizer) NodeAffinities(modelName string) []NodeModelAffinity {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// Find max latency and max request count for normalization.
	var maxLat float64
	var maxReqs int64
	for _, nodeMap := range o.affinities {
		if as, ok := nodeMap[modelName]; ok {
			if as.latencyCount > 0 {
				avg := as.latencySum / float64(as.latencyCount)
				if avg > maxLat {
					maxLat = avg
				}
			}
			if as.requests > maxReqs {
				maxReqs = as.requests
			}
		}
	}

	var result []NodeModelAffinity
	for nodeID, nodeMap := range o.affinities {
		as, ok := nodeMap[modelName]
		if !ok {
			continue
		}
		var hitRate float64
		total := as.cacheHits + as.cacheMisses
		if total > 0 {
			hitRate = float64(as.cacheHits) / float64(total)
		}
		var avgLat float64
		if as.latencyCount > 0 {
			avgLat = as.latencySum / float64(as.latencyCount)
		}

		score := computeAffinity(as, maxLat, maxReqs)
		result = append(result, NodeModelAffinity{
			NodeID:        nodeID,
			ModelName:     modelName,
			CacheHitRate:  hitRate,
			AvgLatencyMs:  avgLat,
			RequestCount:  as.requests,
			VRAMFitScore:  as.vramFit,
			AffinityScore: score,
		})
	}

	// Sort by affinity descending.
	sort.Slice(result, func(i, j int) bool {
		return result[i].AffinityScore > result[j].AffinityScore
	})

	return result
}

// ─── Placement Optimization ─────────────────────────────────────────────────

// Optimize runs the placement optimization cycle.
// Returns a list of recommendations (place, move, or evict models).
func (o *Optimizer) Optimize() []Recommendation {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := o.cfg.Now()
	o.lastOptimization = now
	o.optimizationCount++

	var recs []Recommendation

	// For each popular model, find the best and worst nodes.
	for modelName, ms := range o.popularity {
		if ms.totalReqs < o.cfg.MinRequestsForPlacement {
			continue // not enough data
		}

		// Find max latency / max reqs for this model across nodes.
		var maxLat float64
		var maxReqs int64
		for _, nodeMap := range o.affinities {
			if as, ok := nodeMap[modelName]; ok {
				if as.latencyCount > 0 {
					avg := as.latencySum / float64(as.latencyCount)
					if avg > maxLat {
						maxLat = avg
					}
				}
				if as.requests > maxReqs {
					maxReqs = as.requests
				}
			}
		}

		// Compute affinity for each node that has this model.
		type scored struct {
			nodeID string
			score  float64
		}
		var candidates []scored
		for nodeID, nodeMap := range o.affinities {
			as, ok := nodeMap[modelName]
			if !ok {
				continue
			}
			score := computeAffinity(as, maxLat, maxReqs)
			candidates = append(candidates, scored{nodeID, score})
		}

		if len(candidates) < 2 {
			continue
		}

		// Sort: best node first, worst node last.
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].score > candidates[j].score
		})

		best := candidates[0]
		worst := candidates[len(candidates)-1]

		// Recommend moving model from worst node to best node if there's
		// a significant affinity gap (>0.3).
		gap := best.score - worst.score
		if gap > 0.3 && len(recs) < o.cfg.MaxRecommendations {
			recs = append(recs, Recommendation{
				Type:      RecommendMove,
				ModelName: modelName,
				FromNode:  worst.nodeID,
				ToNode:    best.nodeID,
				Reason:    "significant affinity gap — move to higher-performing node",
				Score:     gap,
				CreatedAt: now,
			})
		}
	}

	// Store recommendations in ring buffer.
	for _, r := range recs {
		o.recommendations[o.recIdx] = r
		o.recIdx++
		if o.recIdx >= o.recCap {
			o.recIdx = 0
			o.recFull = true
		}
	}

	return recs
}

// ─── Retirement Scanning ────────────────────────────────────────────────────

// ScanRetirements identifies models that should be retired (deleted).
// A model is a retirement candidate if it hasn't been requested in RetirementDays.
func (o *Optimizer) ScanRetirements() []RetirementCandidate {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := o.cfg.Now()
	threshold := now.AddDate(0, 0, -o.cfg.RetirementDays)

	var candidates []RetirementCandidate
	for name, ms := range o.popularity {
		if ms.lastReq.Before(threshold) {
			daysSince := int(now.Sub(ms.lastReq).Hours() / 24)
			candidates = append(candidates, RetirementCandidate{
				ModelName:     name,
				LastRequested: ms.lastReq,
				DaysSinceUse:  daysSince,
				Reason:        "inactive for retirement period",
			})
		}
	}

	// Sort by days since use descending (oldest first).
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].DaysSinceUse > candidates[j].DaysSinceUse
	})

	if len(candidates) > o.cfg.MaxRetirementCandidates {
		candidates = candidates[:o.cfg.MaxRetirementCandidates]
	}

	o.retirementCandidates = candidates
	return candidates
}

// RetirementCandidates returns the last computed retirement candidates.
func (o *Optimizer) RetirementCandidates() []RetirementCandidate {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]RetirementCandidate, len(o.retirementCandidates))
	copy(result, o.retirementCandidates)
	return result
}

// ─── Federated Health Learning ──────────────────────────────────────────────

// ReportHealthPattern adds a cross-organization health pattern observation.
// Each org reports summary statistics (no raw data) for federated learning.
func (o *Optimizer) ReportHealthPattern(pattern HealthPattern) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.healthPatterns[o.hpIdx] = pattern
	o.hpIdx++
	if o.hpIdx >= o.cfg.HealthHistorySize {
		o.hpIdx = 0
		o.hpFull = true
	}
}

// AggregateHealthInsights computes network-wide health insights from
// all reported patterns. Returns average failure rate, average MTTR,
// and the most common failure type across organizations.
func (o *Optimizer) AggregateHealthInsights() HealthInsight {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var count int
	if o.hpFull {
		count = len(o.healthPatterns)
	} else {
		count = o.hpIdx
	}
	if count == 0 {
		return HealthInsight{}
	}

	var totalFailRate, totalMTTR float64
	failTypeCounts := make(map[string]int)
	var totalNodes int
	var totalTasks int64
	var orgs int

	for i := 0; i < count; i++ {
		p := o.healthPatterns[i]
		if p.OrgID == "" {
			continue
		}
		totalFailRate += p.AvgFailureRate
		totalMTTR += p.AvgMTTR
		failTypeCounts[p.TopFailureType]++
		totalNodes += p.NodeCount
		totalTasks += p.TaskVolume
		orgs++
	}

	if orgs == 0 {
		return HealthInsight{}
	}

	// Find most common failure type.
	var topType string
	var topCount int
	for ft, c := range failTypeCounts {
		if c > topCount {
			topType = ft
			topCount = c
		}
	}

	return HealthInsight{
		OrgCount:          orgs,
		AvgFailureRate:    totalFailRate / float64(orgs),
		AvgMTTRSeconds:    totalMTTR / float64(orgs),
		TopFailureType:    topType,
		TotalNodes:        totalNodes,
		TotalTasks:        totalTasks,
	}
}

// HealthInsight is an aggregated view of network-wide health.
type HealthInsight struct {
	OrgCount       int     // number of organizations reporting
	AvgFailureRate float64 // average failure rate across orgs
	AvgMTTRSeconds float64 // average MTTR in seconds
	TopFailureType string  // most common failure type
	TotalNodes     int     // total nodes across all orgs
	TotalTasks     int64   // total tasks across all orgs
}

// ─── Statistics & Gate Check ────────────────────────────────────────────────

// OptimizerStats exposes intelligence engine metrics.
type OptimizerStats struct {
	TrackedModels          int   // models being tracked
	TrackedNodes           int   // nodes being tracked
	TotalOptimizations     int64 // optimization cycles completed
	TotalRecommendations   int   // total recommendations produced
	RetirementCandidates   int   // models flagged for retirement
	HealthPatternsReceived int   // federated health observations
}

// Stats returns current optimizer statistics.
func (o *Optimizer) Stats() OptimizerStats {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var totalRecs int
	if o.recFull {
		totalRecs = o.recCap
	} else {
		totalRecs = o.recIdx
	}

	var hpCount int
	if o.hpFull {
		hpCount = len(o.healthPatterns)
	} else {
		hpCount = o.hpIdx
	}

	return OptimizerStats{
		TrackedModels:          len(o.popularity),
		TrackedNodes:           len(o.affinities),
		TotalOptimizations:     o.optimizationCount,
		TotalRecommendations:   totalRecs,
		RetirementCandidates:   len(o.retirementCandidates),
		HealthPatternsReceived: hpCount,
	}
}

// GatePassed returns true if the optimizer has run at least one optimization
// cycle (network self-optimizes model placement weekly).
func (o *Optimizer) GatePassed() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.optimizationCount > 0
}

// ─── Recent Recommendations ─────────────────────────────────────────────────

// RecentRecommendations returns the most recent N recommendations.
func (o *Optimizer) RecentRecommendations(limit int) []Recommendation {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var count int
	if o.recFull {
		count = o.recCap
	} else {
		count = o.recIdx
	}
	if limit > count {
		limit = count
	}
	if limit <= 0 {
		return nil
	}

	result := make([]Recommendation, limit)
	idx := o.recIdx
	for i := 0; i < limit; i++ {
		idx--
		if idx < 0 {
			idx = o.recCap - 1
		}
		result[i] = o.recommendations[idx]
	}
	return result
}

// Reset clears all learned state.
func (o *Optimizer) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.popularity = make(map[string]*modelStats)
	o.affinities = make(map[string]map[string]*affinityStats)
	o.recommendations = make([]Recommendation, o.recCap)
	o.recIdx = 0
	o.recFull = false
	o.retirementCandidates = nil
	o.healthPatterns = make([]HealthPattern, o.cfg.HealthHistorySize)
	o.hpIdx = 0
	o.hpFull = false
	o.lastOptimization = time.Time{}
	o.optimizationCount = 0
}
