package selfheal

import (
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

func testConfig(start time.Time) Config {
	return Config{
		MaxRemediationAttempts: 3,
		IsolationTimeout:      2 * time.Minute,
		VerificationTimeout:   1 * time.Minute,
		IncidentTTL:           24 * time.Hour,
		MaxActiveIncidents:    100,
		Now:                   fixedClock(start, 30*time.Second),
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestNewMesh_DefaultRunbooks(t *testing.T) {
	m := NewMesh(DefaultConfig())
	rbs := m.Runbooks()

	expectedTypes := []FailureType{
		FailHighErrorRate, FailCPUOverload, FailMemoryExhausted,
		FailDiskFull, FailNetworkPartial, FailGPUError,
		FailModelCorrupt, FailHeartbeatLost,
	}
	for _, ft := range expectedTypes {
		if _, ok := rbs[ft]; !ok {
			t.Errorf("missing default runbook for %s", ft)
		}
	}
}

func TestDetect_CreatesIncident(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewMesh(testConfig(base))

	inc, isNew := m.Detect("node-1", FailHighErrorRate)
	if !isNew {
		t.Error("first detection should be new")
	}
	if inc == nil {
		t.Fatal("incident should not be nil")
	}
	if inc.State != StateDetected {
		t.Errorf("state = %s, want DETECTED", inc.State)
	}
	if inc.NodeID != "node-1" {
		t.Errorf("nodeID = %s, want node-1", inc.NodeID)
	}
}

func TestDetect_DuplicateReturnsExisting(t *testing.T) {
	m := NewMesh(DefaultConfig())

	inc1, new1 := m.Detect("node-1", FailHighErrorRate)
	inc2, new2 := m.Detect("node-1", FailCPUOverload)

	if !new1 {
		t.Error("first should be new")
	}
	if new2 {
		t.Error("second should NOT be new")
	}
	if inc1.ID != inc2.ID {
		t.Error("should return same incident")
	}
}

func TestDetect_MaxActiveIncidents(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxActiveIncidents = 2
	m := NewMesh(cfg)

	m.Detect("n1", FailHighErrorRate)
	m.Detect("n2", FailCPUOverload)
	inc, _ := m.Detect("n3", FailDiskFull)

	if inc != nil {
		t.Error("should return nil when max active incidents reached")
	}
}

func TestIsolate_TransitionsState(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)

	err := m.Isolate(inc.ID, 5)
	if err != nil {
		t.Fatalf("Isolate failed: %v", err)
	}
	if inc.State != StateIsolating {
		t.Errorf("state = %s, want ISOLATING", inc.State)
	}
	if inc.DrainedTasks != 5 {
		t.Errorf("drained = %d, want 5", inc.DrainedTasks)
	}
}

func TestIsolate_WrongState(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)
	m.Isolate(inc.ID, 0)

	// Try to isolate again — should fail.
	err := m.Isolate(inc.ID, 0)
	if err == nil {
		t.Error("should fail when NOT in DETECTED state")
	}
}

func TestRemediate_ReturnsRunbookActions(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)
	m.Isolate(inc.ID, 3)

	actions, err := m.Remediate(inc.ID)
	if err != nil {
		t.Fatalf("Remediate failed: %v", err)
	}
	if len(actions) == 0 {
		t.Error("expected runbook actions for HIGH_ERROR_RATE")
	}
	if inc.State != StateRemediating {
		t.Errorf("state = %s, want REMEDIATING", inc.State)
	}
	if inc.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", inc.Attempts)
	}
}

func TestRemediate_WrongState(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)

	// Not isolated yet.
	_, err := m.Remediate(inc.ID)
	if err == nil {
		t.Error("should fail when NOT in ISOLATING state")
	}
}

func TestVerify_Resolved(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewMesh(testConfig(base))

	inc, _ := m.Detect("node-1", FailHighErrorRate)
	m.Isolate(inc.ID, 2)
	m.Remediate(inc.ID)

	err := m.Verify(inc.ID, true)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if inc.State != StateResolved {
		t.Errorf("state = %s, want RESOLVED", inc.State)
	}
	if inc.MTTR <= 0 {
		t.Error("MTTR should be positive after resolution")
	}

	// Should no longer be in active.
	if m.ActiveIncidentCount() != 0 {
		t.Errorf("active count = %d, want 0", m.ActiveIncidentCount())
	}
}

func TestVerify_RetryThenEscalate(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := testConfig(base)
	cfg.MaxRemediationAttempts = 2
	m := NewMesh(cfg)

	inc, _ := m.Detect("node-1", FailHighErrorRate)

	// Attempt 1: isolate → remediate → verify (fail)
	m.Isolate(inc.ID, 0)
	m.Remediate(inc.ID)
	m.Verify(inc.ID, false)

	// Should go back to ISOLATING for retry.
	if inc.State != StateIsolating {
		t.Errorf("state after failed verify = %s, want ISOLATING", inc.State)
	}

	// Attempt 2: remediate → verify (fail again) → should escalate.
	m.Remediate(inc.ID)
	m.Verify(inc.ID, false)

	if inc.State != StateEscalated {
		t.Errorf("state = %s, want ESCALATED after max attempts", inc.State)
	}
}

func TestRecordActionComplete(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)
	m.Isolate(inc.ID, 0)
	m.Remediate(inc.ID)

	err := m.RecordActionComplete(inc.ID, "drain_tasks")
	if err != nil {
		t.Fatalf("RecordActionComplete failed: %v", err)
	}
	if len(inc.ActionsComplete) != 1 {
		t.Errorf("actions complete = %d, want 1", len(inc.ActionsComplete))
	}
	if inc.CurrentAction != "drain_tasks" {
		t.Errorf("current action = %s, want drain_tasks", inc.CurrentAction)
	}
}

func TestEscalate_Manual(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)

	err := m.Escalate(inc.ID, "manual override")
	if err != nil {
		t.Fatalf("Escalate failed: %v", err)
	}
	if inc.State != StateEscalated {
		t.Errorf("state = %s, want ESCALATED", inc.State)
	}
	if inc.Error != "manual override" {
		t.Errorf("error = %q, want 'manual override'", inc.Error)
	}
}

func TestEscalate_NotFound(t *testing.T) {
	m := NewMesh(DefaultConfig())
	err := m.Escalate("INC-999999", "test")
	if err == nil {
		t.Error("should fail for unknown incident")
	}
}

func TestRegisterRunbook(t *testing.T) {
	m := NewMesh(DefaultConfig())
	custom := Runbook{
		FailureType: "CUSTOM_FAIL",
		DrainFirst:  true,
		Actions: []RunbookAction{
			{Name: "custom_step", Description: "do something custom"},
		},
	}
	m.RegisterRunbook(custom)

	rbs := m.Runbooks()
	if _, ok := rbs["CUSTOM_FAIL"]; !ok {
		t.Error("custom runbook should be registered")
	}
}

func TestFullLifecycle_MTTR(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewMesh(testConfig(base))

	// Process several incidents.
	for i := 0; i < 5; i++ {
		inc, _ := m.Detect("node-"+string(rune('A'+i)), FailDiskFull)
		m.Isolate(inc.ID, 0)
		m.Remediate(inc.ID)
		m.Verify(inc.ID, true) // all resolve successfully
	}

	stats := m.Stats()
	if stats.TotalResolved != 5 {
		t.Errorf("resolved = %d, want 5", stats.TotalResolved)
	}
	if stats.TotalEscalated != 0 {
		t.Errorf("escalated = %d, want 0", stats.TotalEscalated)
	}
	if stats.AvgMTTR <= 0 {
		t.Error("avg MTTR should be positive")
	}
	if stats.ResolutionRate < 99.9 {
		t.Errorf("resolution rate = %.1f%%, want ~100%%", stats.ResolutionRate)
	}
}

func TestGatePassed(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewMesh(testConfig(base))

	// No incidents → gate fails.
	if m.GatePassed(5*time.Minute, 95) {
		t.Error("gate should fail with 0 resolved")
	}

	// Process 10 resolved, 0 escalated.
	for i := 0; i < 10; i++ {
		inc, _ := m.Detect("node-"+string(rune('A'+i)), FailDiskFull)
		m.Isolate(inc.ID, 0)
		m.Remediate(inc.ID)
		m.Verify(inc.ID, true)
	}

	if !m.GatePassed(5*time.Minute, 95) {
		st := m.Stats()
		t.Errorf("gate should pass; mttr=%v, rate=%.1f%%", st.AvgMTTR, st.ResolutionRate)
	}
}

func TestNodeHasActiveIncident(t *testing.T) {
	m := NewMesh(DefaultConfig())

	if m.NodeHasActiveIncident("node-1") {
		t.Error("should not have active incident before detection")
	}

	m.Detect("node-1", FailHighErrorRate)

	if !m.NodeHasActiveIncident("node-1") {
		t.Error("should have active incident after detection")
	}
}

func TestGetIncident(t *testing.T) {
	m := NewMesh(DefaultConfig())
	inc, _ := m.Detect("node-1", FailHighErrorRate)

	got, ok := m.GetIncident(inc.ID)
	if !ok {
		t.Fatal("GetIncident should find the incident")
	}
	if got.NodeID != "node-1" {
		t.Errorf("nodeID = %s, want node-1", got.NodeID)
	}

	// Unknown ID.
	_, ok = m.GetIncident("INC-999999")
	if ok {
		t.Error("GetIncident should not find unknown ID")
	}
}

func TestResolvedIncidents_RingBuffer(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewMesh(testConfig(base))

	for i := 0; i < 5; i++ {
		inc, _ := m.Detect("node-"+string(rune('A'+i)), FailDiskFull)
		m.Isolate(inc.ID, 0)
		m.Remediate(inc.ID)
		m.Verify(inc.ID, true)
	}

	resolved := m.ResolvedIncidents(3)
	if len(resolved) != 3 {
		t.Fatalf("expected 3 resolved, got %d", len(resolved))
	}
	// Most recent should be node-E (last resolved).
	if resolved[0].NodeID != "node-E" {
		t.Errorf("most recent = %s, want node-E", resolved[0].NodeID)
	}
}

func TestIncidentState_String(t *testing.T) {
	tests := []struct {
		s    IncidentState
		want string
	}{
		{StateDetected, "DETECTED"},
		{StateIsolating, "ISOLATING"},
		{StateRemediating, "REMEDIATING"},
		{StateVerifying, "VERIFYING"},
		{StateResolved, "RESOLVED"},
		{StateEscalated, "ESCALATED"},
		{IncidentState(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIncidentState_IsTerminal(t *testing.T) {
	if !StateResolved.IsTerminal() {
		t.Error("RESOLVED should be terminal")
	}
	if !StateEscalated.IsTerminal() {
		t.Error("ESCALATED should be terminal")
	}
	if StateDetected.IsTerminal() {
		t.Error("DETECTED should NOT be terminal")
	}
}

func TestReset(t *testing.T) {
	m := NewMesh(DefaultConfig())
	m.Detect("n1", FailHighErrorRate)

	m.Reset()

	if m.ActiveIncidentCount() != 0 {
		t.Error("expected 0 active after reset")
	}
	st := m.Stats()
	if st.TotalResolved != 0 || st.TotalEscalated != 0 {
		t.Error("expected 0 resolved and escalated after reset")
	}
}
