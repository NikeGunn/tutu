package passive

import (
	"testing"
	"time"

	"github.com/tutu-network/tutu/internal/domain"
)

// ═══════════════════════════════════════════════════════════════════════════
// Passive Income Tests — Phase 3
// ═══════════════════════════════════════════════════════════════════════════

// ─── Hardware Tier ──────────────────────────────────────────────────────────

func TestHardwareTier_String(t *testing.T) {
	tests := []struct {
		tier HardwareTier
		want string
	}{
		{TierBasic, "basic"},
		{TierMid, "mid"},
		{TierHigh, "high"},
		{TierUltra, "ultra"},
		{HardwareTier(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("HardwareTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestClassifyHardware(t *testing.T) {
	tests := []struct {
		name     string
		cpuCores int
		vramGB   float64
		want     HardwareTier
	}{
		{"cpu_only", 4, 0, TierBasic},
		{"basic_gpu", 8, 6, TierMid},
		{"high_gpu", 12, 16, TierHigh},
		{"ultra", 32, 80, TierUltra},
		{"4gb_boundary", 8, 4, TierMid},
		{"12gb_boundary", 8, 12, TierHigh},
		{"48gb_boundary", 16, 48, TierUltra},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyHardware(tt.cpuCores, tt.vramGB); got != tt.want {
				t.Errorf("ClassifyHardware(%d, %f) = %s, want %s",
					tt.cpuCores, tt.vramGB, got, tt.want)
			}
		})
	}
}

func TestEstimatedHourlyCredits(t *testing.T) {
	tests := []struct {
		tier       HardwareTier
		demand     float64
		wantMin    int64
	}{
		{TierBasic, 1.0, 5},
		{TierMid, 1.0, 15},
		{TierHigh, 1.0, 40},
		{TierUltra, 1.0, 100},
		{TierHigh, 2.0, 80}, // demand doubles earnings
	}
	for _, tt := range tests {
		t.Run(tt.tier.String(), func(t *testing.T) {
			got := EstimatedHourlyCredits(tt.tier, tt.demand)
			if got < tt.wantMin {
				t.Errorf("EstimatedHourlyCredits(%s, %.1f) = %d, want >= %d",
					tt.tier, tt.demand, got, tt.wantMin)
			}
		})
	}
}

func TestEstimatedHourlyCredits_ZeroDemand(t *testing.T) {
	// Zero demand → defaults to 1.0 multiplier
	got := EstimatedHourlyCredits(TierBasic, 0)
	if got != 5 {
		t.Errorf("EstimatedHourlyCredits(basic, 0) = %d, want 5 (default multiplier)", got)
	}
}

// ─── Capacity Advertiser ────────────────────────────────────────────────────

func TestCapacityAdvertiser_IdleLevels(t *testing.T) {
	ca := NewCapacityAdvertiser(TierHigh)

	tests := []struct {
		level   domain.IdleLevel
		wantMin int
		wantMax int
	}{
		{domain.IdleActive, 5, 15},
		{domain.IdleLight, 25, 35},
		{domain.IdleDeep, 75, 85},
		{domain.IdleLocked, 85, 95},
		{domain.IdleServer, 90, 100},
	}
	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			ca.UpdateIdleLevel(tt.level)
			got := ca.AdvertisedCapacity()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("AdvertisedCapacity() at %s = %d, want [%d, %d]",
					tt.level, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// ─── Model Prefetcher ───────────────────────────────────────────────────────

func TestPrefetcher_RecordAndTop(t *testing.T) {
	p := NewPrefetcher(3)
	for i := 0; i < 10; i++ {
		p.RecordRequest("llama-3")
	}
	for i := 0; i < 5; i++ {
		p.RecordRequest("mixtral")
	}
	for i := 0; i < 3; i++ {
		p.RecordRequest("phi-2")
	}
	p.RecordRequest("tiny-model")

	top := p.TopModels()
	if len(top) != 3 {
		t.Fatalf("TopModels() = %d, want 3 (maxSlots)", len(top))
	}
	if top[0].ModelName != "llama-3" {
		t.Errorf("top model = %q, want %q", top[0].ModelName, "llama-3")
	}
	if top[0].RequestCount != 10 {
		t.Errorf("top request count = %d, want 10", top[0].RequestCount)
	}
}

func TestPrefetcher_ShouldPrefetch(t *testing.T) {
	p := NewPrefetcher(2)
	for i := 0; i < 10; i++ {
		p.RecordRequest("llama-3")
	}
	for i := 0; i < 3; i++ {
		p.RecordRequest("phi-2")
	}
	p.RecordRequest("rare-model") // only 1 request

	names := p.ShouldPrefetch(3) // min 3 requests
	if len(names) != 2 {
		t.Fatalf("ShouldPrefetch(3) = %d, want 2", len(names))
	}
	if names[0] != "llama-3" {
		t.Errorf("first prefetch = %q, want %q", names[0], "llama-3")
	}
}

func TestPrefetcher_DefaultMaxSlots(t *testing.T) {
	p := NewPrefetcher(0) // should default to 3
	for i := 0; i < 10; i++ {
		p.RecordRequest("m1")
		p.RecordRequest("m2")
		p.RecordRequest("m3")
		p.RecordRequest("m4")
	}
	top := p.TopModels()
	if len(top) != 3 {
		t.Errorf("TopModels() = %d, want 3 (default maxSlots)", len(top))
	}
}

// ─── Earnings Report ────────────────────────────────────────────────────────

func TestGenerateReport(t *testing.T) {
	start := time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 2, 6, 0, 0, 0, time.UTC)
	rep := GenerateReport(start, end, 500, 120, 8.0, TierHigh, "llama-3")

	if rep.CreditsEarned != 500 {
		t.Errorf("CreditsEarned = %d, want 500", rep.CreditsEarned)
	}
	if rep.TasksCompleted != 120 {
		t.Errorf("TasksCompleted = %d, want 120", rep.TasksCompleted)
	}
	if rep.TopModel != "llama-3" {
		t.Errorf("TopModel = %q, want %q", rep.TopModel, "llama-3")
	}
}

func TestEarningsReport_HoursInPeriod(t *testing.T) {
	start := time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 2, 6, 0, 0, 0, time.UTC)
	rep := GenerateReport(start, end, 0, 0, 0, TierBasic, "")
	hours := rep.HoursInPeriod()
	if hours < 7.9 || hours > 8.1 {
		t.Errorf("HoursInPeriod() = %f, want ~8.0", hours)
	}
}

func TestEarningsReport_CreditsPerHour(t *testing.T) {
	start := time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 2, 6, 0, 0, 0, time.UTC)
	rep := GenerateReport(start, end, 400, 0, 0, TierBasic, "")

	cph := rep.CreditsPerHour()
	if cph < 49.0 || cph > 51.0 {
		t.Errorf("CreditsPerHour() = %f, want ~50.0", cph)
	}
}

func TestEarningsReport_CreditsPerHour_ZeroDuration(t *testing.T) {
	now := time.Now()
	rep := GenerateReport(now, now, 100, 0, 0, TierBasic, "")
	if rep.CreditsPerHour() != 0 {
		t.Errorf("CreditsPerHour() with zero duration = %f, want 0", rep.CreditsPerHour())
	}
}
