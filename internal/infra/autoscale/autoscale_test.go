package autoscale

import (
	"math"
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

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestNewScaler_DefaultConfig(t *testing.T) {
	s := NewScaler(DefaultConfig())
	if s == nil {
		t.Fatal("NewScaler returned nil")
	}
	if s.Capacity() != 1 {
		t.Errorf("initial capacity = %d, want 1 (MinCapacity)", s.Capacity())
	}
	if len(s.seasonal) != 24 {
		t.Errorf("seasonal buckets = %d, want 24", len(s.seasonal))
	}
}

func TestNewScaler_InvalidConfig(t *testing.T) {
	cfg := Config{
		Alpha:          -1,
		SeasonalPeriod: -1,
		SeasonalAlpha:  0,
		MinCapacity:    -5,
		MaxCapacity:    -1,
	}
	s := NewScaler(cfg)
	if s.cfg.Alpha != 0.3 {
		t.Errorf("expected Alpha=0.3 after fix, got %f", s.cfg.Alpha)
	}
	if s.cfg.SeasonalPeriod != 24 {
		t.Errorf("expected SeasonalPeriod=24, got %d", s.cfg.SeasonalPeriod)
	}
}

func TestRecordDemand_InitializesSmoothed(t *testing.T) {
	s := NewScaler(DefaultConfig())

	s.RecordDemand(Sample{Demand: 100, Timestamp: time.Now()})
	forecast := s.Forecast(time.Now())

	// First observation should set smoothed to 100, seasonal=1.0 → forecast ≈ 100.
	if math.Abs(forecast-100) > 1 {
		t.Errorf("forecast after first sample = %f, want ~100", forecast)
	}
}

func TestRecordDemand_ExponentialSmoothing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Alpha = 0.5 // 50% weight to newest
	// SeasonalAlpha is clamped to 0.1 minimum, so seasonal indices learn slowly.
	// The forecast includes a small seasonal adjustment → allow wider tolerance.
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC) // hour 12

	s := NewScaler(cfg)

	// First sample
	s.RecordDemand(Sample{Demand: 100, Timestamp: base})
	// Second sample at same hour: smoothed ≈ 0.5*200 + 0.5*100 = 150
	s.RecordDemand(Sample{Demand: 200, Timestamp: base.Add(time.Minute)})

	forecast := s.Forecast(base)
	// Smoothed ≈ 150, seasonal index ≈ 1.03 → forecast ≈ 155.
	if math.Abs(forecast-155) > 10 {
		t.Errorf("forecast = %f, want ~155", forecast)
	}
}

func TestSeasonBucket(t *testing.T) {
	cfg := DefaultConfig() // period=24
	s := NewScaler(cfg)

	tests := []struct {
		name   string
		hour   int
		bucket int
	}{
		{"midnight", 0, 0},
		{"noon", 12, 12},
		{"23h", 23, 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := time.Date(2025, 1, 1, tt.hour, 30, 0, 0, time.UTC)
			got := s.seasonBucket(ts)
			if got != tt.bucket {
				t.Errorf("seasonBucket(%02d:30) = %d, want %d", tt.hour, got, tt.bucket)
			}
		})
	}
}

func TestEvaluate_ScaleUp(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.ScaleUpThreshold = 0.8
	cfg.MinCapacity = 1
	cfg.MaxCapacity = 100
	cfg.CooldownPeriod = 0 // no cooldown for test
	cfg.Now = fixedClock(base, time.Minute)
	s := NewScaler(cfg)
	s.SetCapacity(5)

	// Feed high demand to push forecast up.
	for i := 0; i < 10; i++ {
		s.RecordDemand(Sample{Demand: 50, Timestamp: base.Add(time.Duration(i) * time.Minute)})
	}

	d := s.Evaluate()
	// With capacity=5 and forecast~50, we should scale up since 50 > 5*0.8=4.
	if d.Direction != ScaleUp && d.Direction != PreWarm {
		t.Errorf("expected ScaleUp or PreWarm, got %s", d.Direction)
	}
	if d.TargetCapacity <= 5 {
		t.Errorf("target capacity should exceed 5, got %d", d.TargetCapacity)
	}
}

func TestEvaluate_ScaleDown(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.ScaleDownThreshold = 0.3
	cfg.MinCapacity = 1
	cfg.MaxCapacity = 100
	cfg.CooldownPeriod = 0
	cfg.PreWarmLeadTime = time.Millisecond // minimize pre-warm influence
	cfg.Now = fixedClock(base, time.Minute)
	s := NewScaler(cfg)
	s.SetCapacity(50)

	// Feed low demand.
	for i := 0; i < 10; i++ {
		s.RecordDemand(Sample{Demand: 2, Timestamp: base.Add(time.Duration(i) * time.Minute)})
	}

	d := s.Evaluate()
	// forecast~2, capacity=50, threshold=0.3 → 2 < 50*0.3=15 → scale down.
	if d.Direction != ScaleDown {
		t.Errorf("expected ScaleDown, got %s (forecast=%.1f, cap=%d)", d.Direction, d.ForecastDemand, d.CurrentCapacity)
	}
}

func TestEvaluate_Hold(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.ScaleUpThreshold = 0.8
	cfg.ScaleDownThreshold = 0.3
	cfg.MinCapacity = 1
	cfg.MaxCapacity = 100
	cfg.CooldownPeriod = 0
	cfg.PreWarmLeadTime = time.Millisecond
	cfg.Now = fixedClock(base, time.Minute)
	s := NewScaler(cfg)
	s.SetCapacity(20)

	// Feed moderate demand.
	for i := 0; i < 10; i++ {
		s.RecordDemand(Sample{Demand: 10, Timestamp: base.Add(time.Duration(i) * time.Minute)})
	}

	d := s.Evaluate()
	// forecast~10, capacity=20, up threshold=16, down threshold=6 → hold.
	if d.Direction != Hold {
		t.Errorf("expected Hold, got %s (forecast=%.1f, cap=%d)", d.Direction, d.ForecastDemand, d.CurrentCapacity)
	}
}

func TestEvaluate_Cooldown(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.CooldownPeriod = 10 * time.Minute
	cfg.Now = fixedClock(base, time.Second) // 1s increments
	s := NewScaler(cfg)
	s.SetCapacity(5)

	for i := 0; i < 10; i++ {
		s.RecordDemand(Sample{Demand: 50, Timestamp: base.Add(time.Duration(i) * time.Minute)})
	}

	// First evaluation should trigger scale up.
	d1 := s.Evaluate()
	if d1.Direction == Hold {
		t.Fatal("first evaluation should scale up")
	}

	// Second evaluation within cooldown should hold.
	d2 := s.Evaluate()
	if d2.Direction != Hold {
		t.Errorf("expected Hold during cooldown, got %s", d2.Direction)
	}
}

func TestRecordSpike(t *testing.T) {
	s := NewScaler(DefaultConfig())
	s.RecordSpike(true)
	s.RecordSpike(true)
	s.RecordSpike(false)

	st := s.Stats()
	if st.TotalSpikes != 3 {
		t.Errorf("expected 3 spikes, got %d", st.TotalSpikes)
	}
	if st.ProactiveSpikes != 2 {
		t.Errorf("expected 2 proactive, got %d", st.ProactiveSpikes)
	}
	// 2/3 = 66.7%
	expectedPct := 200.0 / 3.0
	if math.Abs(st.ProactivePct-expectedPct) > 0.1 {
		t.Errorf("proactive pct = %.1f, want %.1f", st.ProactivePct, expectedPct)
	}
}

func TestGatePassed(t *testing.T) {
	s := NewScaler(DefaultConfig())

	// Not enough data.
	if s.GatePassed(90) {
		t.Error("gate should not pass with 0 spikes")
	}

	// 9 proactive out of 10 = 90%.
	for i := 0; i < 9; i++ {
		s.RecordSpike(true)
	}
	s.RecordSpike(false)

	if !s.GatePassed(90) {
		t.Error("gate should pass at 90%")
	}
	if s.GatePassed(95) {
		t.Error("gate should NOT pass at 95%")
	}
}

func TestPeakHours(t *testing.T) {
	cfg := DefaultConfig()
	s := NewScaler(cfg)

	// Manually set seasonal indices: hour 14 = busiest.
	s.mu.Lock()
	s.seasonal[14] = 2.5
	s.seasonal[9] = 1.8
	s.seasonal[0] = 0.3
	s.mu.Unlock()

	peaks := s.PeakHours(3)
	if len(peaks) != 3 {
		t.Fatalf("expected 3 peak hours, got %d", len(peaks))
	}
	if peaks[0] != 14 {
		t.Errorf("top peak should be hour 14, got %d", peaks[0])
	}
}

func TestRecentDecisions(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.CooldownPeriod = 0
	cfg.PreWarmLeadTime = time.Millisecond
	cfg.Now = fixedClock(base, time.Minute)
	s := NewScaler(cfg)
	s.SetCapacity(5)

	for i := 0; i < 10; i++ {
		s.RecordDemand(Sample{Demand: 50, Timestamp: base.Add(time.Duration(i) * time.Minute)})
	}
	s.Evaluate()

	recent := s.RecentDecisions(5)
	if len(recent) == 0 {
		t.Error("expected at least 1 decision")
	}
}

func TestSetCapacity_Clamped(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinCapacity = 5
	cfg.MaxCapacity = 50
	s := NewScaler(cfg)

	s.SetCapacity(3) // below min
	if s.Capacity() != 5 {
		t.Errorf("capacity should clamp to 5, got %d", s.Capacity())
	}

	s.SetCapacity(100) // above max
	if s.Capacity() != 50 {
		t.Errorf("capacity should clamp to 50, got %d", s.Capacity())
	}
}

func TestReset(t *testing.T) {
	s := NewScaler(DefaultConfig())
	s.RecordDemand(Sample{Demand: 100, Timestamp: time.Now()})
	s.RecordSpike(true)

	s.Reset()

	st := s.Stats()
	if st.Observations != 0 {
		t.Errorf("expected 0 observations after reset, got %d", st.Observations)
	}
	if st.TotalSpikes != 0 {
		t.Errorf("expected 0 spikes after reset, got %d", st.TotalSpikes)
	}
}

func TestDirection_String(t *testing.T) {
	tests := []struct {
		d    Direction
		want string
	}{
		{Hold, "HOLD"},
		{ScaleUp, "SCALE_UP"},
		{ScaleDown, "SCALE_DOWN"},
		{PreWarm, "PRE_WARM"},
		{Direction(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Direction(%d).String() = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
