package domain

// ─── Social & Referral Types (Phase 2 — Architecture Part XIII §7, §10) ────
// Referral system, teams, and leaderboards drive viral growth.
// All leaderboards are opt-in only (v3.0 refinement).

// ReferralInfo tracks a user's referral status.
type ReferralInfo struct {
	Code       string `json:"code"`        // Unique referral code (e.g., "tutu-abc123")
	ReferredBy string `json:"referred_by"` // Code of referrer (empty if organic)
	Count      int    `json:"count"`       // Number of successful referrals
}

// ReferralReward defines the rewards for referral events.
type ReferralReward struct {
	ReferrerCredits int64 `json:"referrer_credits"` // 500 credits
	ReferrerXP      int64 `json:"referrer_xp"`      // 200 XP
	RefereeCredits  int64 `json:"referee_credits"`   // 200 bonus credits on install
	ChainBonus      int64 `json:"chain_bonus"`       // 100 credits if referee refers someone
	MaxPerMonth     int   `json:"max_per_month"`     // 50 referral rewards/month cap
}

// DefaultReferralReward returns the architecture-defined referral rewards.
func DefaultReferralReward() ReferralReward {
	return ReferralReward{
		ReferrerCredits: 500,
		ReferrerXP:      200,
		RefereeCredits:  200,
		ChainBonus:      100,
		MaxPerMonth:     50,
	}
}

// ─── Team Types ─────────────────────────────────────────────────────────────

// Team represents a group of users who contribute together.
// Teams: 2–50 members, team leaderboard, team challenges.
type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id"`
	MemberCount int    `json:"member_count"`
	CreatedAt   int64  `json:"created_at"`
}

// TeamMember represents a user's membership in a team.
type TeamMember struct {
	TeamID   string `json:"team_id"`
	UserID   string `json:"user_id"`
	JoinedAt int64  `json:"joined_at"`
	Role     string `json:"role"` // "owner", "member"
}

// ─── Leaderboard Types ──────────────────────────────────────────────────────

// LeaderboardType defines the scope of a leaderboard.
type LeaderboardType string

const (
	LeaderboardRegional LeaderboardType = "regional"
	LeaderboardGlobal   LeaderboardType = "global"
	LeaderboardWeekly   LeaderboardType = "weekly"
)

// LeaderboardEntry represents a user's position on a leaderboard.
type LeaderboardEntry struct {
	Rank            int     `json:"rank"`
	AnonymizedName  string  `json:"name"`      // Privacy: anonymized username
	Score           float64 `json:"score"`
	CreditsEarned   int64   `json:"credits_earned"`
	TasksCompleted  int64   `json:"tasks_completed"`
	UptimeHours     float64 `json:"uptime_hours"`
	StreakLength     int     `json:"streak_length"`
}

// LeaderboardScore calculates the weighted score for ranking.
// Architecture Part XIII §7: 0.4×credits + 0.3×tasks + 0.2×uptime + 0.1×streak
func LeaderboardScore(credits int64, tasks int64, uptimeHrs float64, streak int) float64 {
	return 0.4*float64(credits) + 0.3*float64(tasks) + 0.2*uptimeHrs + 0.1*float64(streak)
}

// LeaderboardConfig controls leaderboard behavior.
type LeaderboardConfig struct {
	OptIn       bool `json:"opt_in"`       // v3.0: opt-in only
	TopN        int  `json:"top_n"`        // Display top N (default 100)
	Anonymized  bool `json:"anonymized"`   // No real names shown
}

// DefaultLeaderboardConfig returns the v3.0 leaderboard policy.
func DefaultLeaderboardConfig() LeaderboardConfig {
	return LeaderboardConfig{
		OptIn:      true,
		TopN:       100,
		Anonymized: true,
	}
}

// ─── Onboarding Types ───────────────────────────────────────────────────────

// OnboardingStep tracks the user's progress through the first-5-minutes flow.
type OnboardingStep string

const (
	OnboardInstall       OnboardingStep = "install"        // Minute 0
	OnboardFirstChat     OnboardingStep = "first_chat"     // Minute 1
	OnboardFirstTask     OnboardingStep = "first_task"     // Minute 2
	OnboardCreditsShown  OnboardingStep = "credits_shown"  // Minute 3
	OnboardIdleDetected  OnboardingStep = "idle_detected"  // Minute 4
	OnboardFirstAchieve  OnboardingStep = "first_achieve"  // Minute 5
	OnboardComplete      OnboardingStep = "complete"
)

// OnboardingProgress tracks which onboarding steps are complete.
type OnboardingProgress struct {
	Steps       map[OnboardingStep]bool `json:"steps"`
	StartedAt   int64                   `json:"started_at"`
	CompletedAt int64                   `json:"completed_at"` // 0 if not complete
}

// NewOnboardingProgress creates a fresh onboarding tracker.
func NewOnboardingProgress() OnboardingProgress {
	return OnboardingProgress{
		Steps: make(map[OnboardingStep]bool),
	}
}

// IsComplete returns true if all onboarding steps are done.
func (o OnboardingProgress) IsComplete() bool {
	return o.Steps[OnboardComplete]
}

// ─── Sleep Earner Types ─────────────────────────────────────────────────────

// SleepEarnerEstimate provides the "estimated overnight earnings" shown before bed.
// Architecture Part XIII §9: The most powerful retention mechanic.
type SleepEarnerEstimate struct {
	EstimatedCredits float64 `json:"estimated_credits"` // Based on hardware + network demand
	HardwareTier     string  `json:"hardware_tier"`     // "high", "mid", "low"
	NetworkDemand    string  `json:"network_demand"`    // "high", "normal", "low"
}

// EstimateOvernightEarnings calculates expected overnight credits.
// Based on the user's hardware profile and current network demand.
func EstimateOvernightEarnings(vramGB int, cpuCores int, demandMultiplier float64) SleepEarnerEstimate {
	// Base estimate: VRAM contributes more (GPU tasks pay more)
	baseCredits := float64(vramGB)*3.0 + float64(cpuCores)*0.5
	estimated := baseCredits * demandMultiplier * 8.0 // 8 hours overnight

	tier := "low"
	if vramGB >= 16 {
		tier = "high"
	} else if vramGB >= 8 {
		tier = "mid"
	}

	demand := "normal"
	if demandMultiplier > 1.5 {
		demand = "high"
	} else if demandMultiplier < 0.5 {
		demand = "low"
	}

	return SleepEarnerEstimate{
		EstimatedCredits: estimated,
		HardwareTier:     tier,
		NetworkDemand:    demand,
	}
}
