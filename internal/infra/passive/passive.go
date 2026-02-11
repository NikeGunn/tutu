// Package passive implements Phase 3 passive income optimization.
// Architecture Part XIII §10 — earns credits while the user sleeps.
//
// Key features:
//   - Idle-aware capacity advertising (advertises more capacity when idle)
//   - Popular model prefetching (pre-loads models likely to be requested)
//   - Morning earnings report (summarizes overnight earnings)
//   - Hardware tier classification for earnings estimation
package passive

import (
	"sort"
	"sync"
	"time"

	"github.com/tutu-network/tutu/internal/domain"
)

// ─── Hardware Tier ──────────────────────────────────────────────────────────

// HardwareTier classifies a node's hardware for earnings estimation.
type HardwareTier int

const (
	TierBasic    HardwareTier = iota // CPU only, ≤8 cores
	TierMid                          // CPU + basic GPU (4–8GB VRAM)
	TierHigh                         // High-end GPU (12–24GB VRAM)
	TierUltra                        // Multi-GPU / server-grade (48GB+ VRAM)
)

// String returns a human-readable hardware tier.
func (t HardwareTier) String() string {
	switch t {
	case TierBasic:
		return "basic"
	case TierMid:
		return "mid"
	case TierHigh:
		return "high"
	case TierUltra:
		return "ultra"
	default:
		return "unknown"
	}
}

// ClassifyHardware determines the hardware tier from specs.
func ClassifyHardware(cpuCores int, vramGB float64) HardwareTier {
	switch {
	case vramGB >= 48:
		return TierUltra
	case vramGB >= 12:
		return TierHigh
	case vramGB >= 4:
		return TierMid
	default:
		return TierBasic
	}
}

// EstimatedHourlyCredits returns the estimated credits per hour for a tier.
func EstimatedHourlyCredits(tier HardwareTier, demandMultiplier float64) int64 {
	if demandMultiplier <= 0 {
		demandMultiplier = 1.0
	}

	base := int64(0)
	switch tier {
	case TierBasic:
		base = 5
	case TierMid:
		base = 15
	case TierHigh:
		base = 40
	case TierUltra:
		base = 100
	}

	return int64(float64(base) * demandMultiplier)
}

// ─── Capacity Advertiser ────────────────────────────────────────────────────

// CapacityAdvertiser adjusts advertised capacity based on idle level.
// When the user is deeply idle (sleeping), we advertise more capacity to
// attract more tasks and earn more credits.
type CapacityAdvertiser struct {
	mu          sync.Mutex
	tier        HardwareTier
	idleLevel   domain.IdleLevel
	baseCapacity int // percentage (0–100)
}

// NewCapacityAdvertiser creates a new capacity advertiser.
func NewCapacityAdvertiser(tier HardwareTier) *CapacityAdvertiser {
	return &CapacityAdvertiser{
		tier:         tier,
		baseCapacity: 100,
	}
}

// UpdateIdleLevel updates the current idle level.
func (ca *CapacityAdvertiser) UpdateIdleLevel(level domain.IdleLevel) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.idleLevel = level
}

// AdvertisedCapacity returns the capacity percentage to advertise to the network.
// Higher idle levels = more advertised capacity.
func (ca *CapacityAdvertiser) AdvertisedCapacity() int {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	switch ca.idleLevel {
	case domain.IdleActive:
		return 10 // barely advertise — user is active
	case domain.IdleLight:
		return 30
	case domain.IdleDeep:
		return 80
	case domain.IdleLocked:
		return 90
	case domain.IdleServer:
		return 95
	default:
		return 10
	}
}

// ─── Model Prefetcher ───────────────────────────────────────────────────────

// ModelPopularity tracks how often a model is requested across the network.
type ModelPopularity struct {
	ModelName    string  `json:"model_name"`
	RequestCount int64   `json:"request_count"`
	LastRequested time.Time `json:"last_requested"`
}

// Prefetcher suggests models to pre-load based on network demand.
type Prefetcher struct {
	mu         sync.Mutex
	popularity map[string]*ModelPopularity
	maxSlots   int // max models to keep pre-loaded
}

// NewPrefetcher creates a model prefetcher.
func NewPrefetcher(maxSlots int) *Prefetcher {
	if maxSlots <= 0 {
		maxSlots = 3
	}
	return &Prefetcher{
		popularity: make(map[string]*ModelPopularity),
		maxSlots:   maxSlots,
	}
}

// RecordRequest records a model request from the network.
func (p *Prefetcher) RecordRequest(modelName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mp, ok := p.popularity[modelName]
	if !ok {
		mp = &ModelPopularity{ModelName: modelName}
		p.popularity[modelName] = mp
	}
	mp.RequestCount++
	mp.LastRequested = time.Now()
}

// TopModels returns the most requested models (up to maxSlots).
func (p *Prefetcher) TopModels() []ModelPopularity {
	p.mu.Lock()
	defer p.mu.Unlock()

	all := make([]ModelPopularity, 0, len(p.popularity))
	for _, mp := range p.popularity {
		all = append(all, *mp)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].RequestCount > all[j].RequestCount
	})

	if len(all) > p.maxSlots {
		all = all[:p.maxSlots]
	}
	return all
}

// ShouldPrefetch returns model names that should be pre-loaded.
// Filters to models requested in the last 24 hours with at least minRequests.
func (p *Prefetcher) ShouldPrefetch(minRequests int64) []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	var candidates []ModelPopularity
	for _, mp := range p.popularity {
		if mp.RequestCount >= minRequests && mp.LastRequested.After(cutoff) {
			candidates = append(candidates, *mp)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].RequestCount > candidates[j].RequestCount
	})

	if len(candidates) > p.maxSlots {
		candidates = candidates[:p.maxSlots]
	}

	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.ModelName
	}
	return names
}

// ─── Earnings Report ────────────────────────────────────────────────────────

// EarningsReport summarizes earnings over a period (e.g., overnight).
type EarningsReport struct {
	PeriodStart    time.Time    `json:"period_start"`
	PeriodEnd      time.Time    `json:"period_end"`
	CreditsEarned  int64        `json:"credits_earned"`
	TasksCompleted int          `json:"tasks_completed"`
	UptimeHours    float64      `json:"uptime_hours"`
	HardwareTier   HardwareTier `json:"hardware_tier"`
	TopModel       string       `json:"top_model,omitempty"` // most-requested model during period
}

// GenerateReport creates an earnings report for the given period.
func GenerateReport(start, end time.Time, credits int64, tasks int, uptimeHours float64, tier HardwareTier, topModel string) EarningsReport {
	return EarningsReport{
		PeriodStart:    start,
		PeriodEnd:      end,
		CreditsEarned:  credits,
		TasksCompleted: tasks,
		UptimeHours:    uptimeHours,
		HardwareTier:   tier,
		TopModel:       topModel,
	}
}

// HoursInPeriod returns the duration of the report period in hours.
func (r EarningsReport) HoursInPeriod() float64 {
	return r.PeriodEnd.Sub(r.PeriodStart).Hours()
}

// CreditsPerHour returns the average earnings rate.
func (r EarningsReport) CreditsPerHour() float64 {
	hours := r.HoursInPeriod()
	if hours <= 0 {
		return 0
	}
	return float64(r.CreditsEarned) / hours
}
