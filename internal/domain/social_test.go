package domain

import "testing"

// ─── Social Domain Tests ────────────────────────────────────────────────────

func TestDefaultReferralReward(t *testing.T) {
	r := DefaultReferralReward()
	if r.ReferrerCredits != 500 {
		t.Errorf("expected 500 referrer credits, got %d", r.ReferrerCredits)
	}
	if r.ReferrerXP != 200 {
		t.Errorf("expected 200 referrer XP, got %d", r.ReferrerXP)
	}
	if r.RefereeCredits != 200 {
		t.Errorf("expected 200 referee credits, got %d", r.RefereeCredits)
	}
	if r.ChainBonus != 100 {
		t.Errorf("expected 100 chain bonus, got %d", r.ChainBonus)
	}
	if r.MaxPerMonth != 50 {
		t.Errorf("expected max 50/month, got %d", r.MaxPerMonth)
	}
}

func TestLeaderboardScore(t *testing.T) {
	// 0.4×credits + 0.3×tasks + 0.2×uptime + 0.1×streak
	score := LeaderboardScore(1000, 500, 100.0, 30)
	expected := 0.4*1000 + 0.3*500 + 0.2*100 + 0.1*30 // 400+150+20+3 = 573
	if score != expected {
		t.Errorf("expected score %f, got %f", expected, score)
	}
}

func TestLeaderboardScore_AllZero(t *testing.T) {
	score := LeaderboardScore(0, 0, 0, 0)
	if score != 0 {
		t.Errorf("expected 0, got %f", score)
	}
}

func TestDefaultLeaderboardConfig(t *testing.T) {
	cfg := DefaultLeaderboardConfig()
	if !cfg.OptIn {
		t.Error("expected opt-in by default")
	}
	if cfg.TopN != 100 {
		t.Errorf("expected top 100, got %d", cfg.TopN)
	}
	if !cfg.Anonymized {
		t.Error("expected anonymized by default")
	}
}

func TestOnboardingProgress(t *testing.T) {
	p := NewOnboardingProgress()
	if p.IsComplete() {
		t.Error("fresh onboarding should not be complete")
	}

	p.Steps[OnboardInstall] = true
	p.Steps[OnboardFirstChat] = true
	if p.IsComplete() {
		t.Error("partial onboarding should not be complete")
	}

	p.Steps[OnboardComplete] = true
	if !p.IsComplete() {
		t.Error("should be complete when OnboardComplete is set")
	}
}

func TestEstimateOvernightEarnings(t *testing.T) {
	tests := []struct {
		name   string
		vramGB int
		cpu    int
		demand float64
		tier   string
	}{
		{"high_gpu", 24, 16, 1.0, "high"},
		{"mid_gpu", 8, 8, 1.0, "mid"},
		{"low_gpu", 4, 4, 1.0, "low"},
		{"high_demand", 12, 8, 2.0, "mid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := EstimateOvernightEarnings(tt.vramGB, tt.cpu, tt.demand)
			if est.EstimatedCredits <= 0 {
				t.Errorf("expected positive credits, got %f", est.EstimatedCredits)
			}
			if est.HardwareTier != tt.tier {
				t.Errorf("expected tier %s, got %s", tt.tier, est.HardwareTier)
			}
		})
	}
}

func TestEstimateOvernightEarnings_DemandLevels(t *testing.T) {
	high := EstimateOvernightEarnings(16, 8, 2.0)
	if high.NetworkDemand != "high" {
		t.Errorf("expected high demand, got %s", high.NetworkDemand)
	}

	low := EstimateOvernightEarnings(16, 8, 0.3)
	if low.NetworkDemand != "low" {
		t.Errorf("expected low demand, got %s", low.NetworkDemand)
	}

	normal := EstimateOvernightEarnings(16, 8, 1.0)
	if normal.NetworkDemand != "normal" {
		t.Errorf("expected normal demand, got %s", normal.NetworkDemand)
	}
}
