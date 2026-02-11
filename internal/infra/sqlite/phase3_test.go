package sqlite

import (
	"testing"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// Phase 3 SQLite Persistence Tests
// ═══════════════════════════════════════════════════════════════════════════

// ─── Region Status ──────────────────────────────────────────────────────────

func TestPhase3_UpsertRegionStatus(t *testing.T) {
	db := newTestDB(t)

	err := db.UpsertRegionStatus("us-east", true, 50, 20, 100, 12.5)
	if err != nil {
		t.Fatalf("UpsertRegionStatus() error: %v", err)
	}

	healthy, nodeCount, activeTasks, queueDepth, avgLatency, err := db.GetRegionStatus("us-east")
	if err != nil {
		t.Fatalf("GetRegionStatus() error: %v", err)
	}
	if !healthy {
		t.Error("healthy = false, want true")
	}
	if nodeCount != 50 {
		t.Errorf("nodeCount = %d, want 50", nodeCount)
	}
	if activeTasks != 20 {
		t.Errorf("activeTasks = %d, want 20", activeTasks)
	}
	if queueDepth != 100 {
		t.Errorf("queueDepth = %d, want 100", queueDepth)
	}
	if avgLatency != 12.5 {
		t.Errorf("avgLatency = %f, want 12.5", avgLatency)
	}
}

func TestPhase3_UpsertRegionStatus_Update(t *testing.T) {
	db := newTestDB(t)
	db.UpsertRegionStatus("eu-west", true, 10, 5, 50, 10.0)
	db.UpsertRegionStatus("eu-west", false, 20, 10, 100, 25.0)

	healthy, nodeCount, _, _, _, err := db.GetRegionStatus("eu-west")
	if err != nil {
		t.Fatal(err)
	}
	if healthy {
		t.Error("should be unhealthy after update")
	}
	if nodeCount != 20 {
		t.Errorf("nodeCount = %d, want 20", nodeCount)
	}
}

func TestPhase3_GetRegionStatus_NotFound(t *testing.T) {
	db := newTestDB(t)
	healthy, nodeCount, _, _, _, err := db.GetRegionStatus("nonexistent")
	if err != nil {
		t.Fatalf("GetRegionStatus(nonexistent) error: %v", err)
	}
	// Defaults for missing region
	if !healthy {
		t.Error("default healthy should be true")
	}
	if nodeCount != 0 {
		t.Errorf("default nodeCount = %d, want 0", nodeCount)
	}
}

func TestPhase3_ListRegionStatuses(t *testing.T) {
	db := newTestDB(t)
	db.UpsertRegionStatus("us-east", true, 50, 20, 100, 12.5)
	db.UpsertRegionStatus("eu-west", true, 30, 10, 50, 8.0)

	statuses, err := db.ListRegionStatuses()
	if err != nil {
		t.Fatalf("ListRegionStatuses() error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("ListRegionStatuses() returned %d, want 2", len(statuses))
	}
}

// ─── Circuit Breaker Persistence ────────────────────────────────────────────

func TestPhase3_UpsertCircuitBreaker(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Truncate(time.Second)

	err := db.UpsertCircuitBreaker("api-cb", 1, 3, 2, &now)
	if err != nil {
		t.Fatalf("UpsertCircuitBreaker() error: %v", err)
	}

	state, failures, totalTrips, trippedAt, err := db.GetCircuitBreaker("api-cb")
	if err != nil {
		t.Fatalf("GetCircuitBreaker() error: %v", err)
	}
	if state != 1 {
		t.Errorf("state = %d, want 1 (OPEN)", state)
	}
	if failures != 3 {
		t.Errorf("failures = %d, want 3", failures)
	}
	if totalTrips != 2 {
		t.Errorf("totalTrips = %d, want 2", totalTrips)
	}
	if trippedAt == nil {
		t.Fatal("trippedAt should not be nil")
	}
}

func TestPhase3_GetCircuitBreaker_NotFound(t *testing.T) {
	db := newTestDB(t)
	state, failures, totalTrips, trippedAt, err := db.GetCircuitBreaker("nonexistent")
	if err != nil {
		t.Fatalf("GetCircuitBreaker(nonexistent) error: %v", err)
	}
	if state != 0 || failures != 0 || totalTrips != 0 || trippedAt != nil {
		t.Error("defaults should be zero for missing circuit breaker")
	}
}

func TestPhase3_UpsertCircuitBreaker_NilTrippedAt(t *testing.T) {
	db := newTestDB(t)
	err := db.UpsertCircuitBreaker("clean-cb", 0, 0, 0, nil)
	if err != nil {
		t.Fatalf("UpsertCircuitBreaker(nil trippedAt) error: %v", err)
	}
	_, _, _, trippedAt, err := db.GetCircuitBreaker("clean-cb")
	if err != nil {
		t.Fatal(err)
	}
	if trippedAt != nil {
		t.Error("trippedAt should be nil when not set")
	}
}

// ─── Quarantine Records ─────────────────────────────────────────────────────

func TestPhase3_InsertQuarantineRecord(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Truncate(time.Second)
	expires := now.Add(1 * time.Hour)

	id, err := db.InsertQuarantineRecord("node-1", "task_failures", now, expires)
	if err != nil {
		t.Fatalf("InsertQuarantineRecord() error: %v", err)
	}
	if id <= 0 {
		t.Errorf("InsertQuarantineRecord() id = %d, want > 0", id)
	}

	quarantined, err := db.IsNodeQuarantined("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if !quarantined {
		t.Error("node-1 should be quarantined")
	}
}

func TestPhase3_IsNodeQuarantined_NotFound(t *testing.T) {
	db := newTestDB(t)
	quarantined, err := db.IsNodeQuarantined("ghost")
	if err != nil {
		t.Fatal(err)
	}
	if quarantined {
		t.Error("nonexistent node should not be quarantined")
	}
}

func TestPhase3_ReleaseQuarantine(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Truncate(time.Second)
	expires := now.Add(1 * time.Hour)

	db.InsertQuarantineRecord("node-1", "task_failures", now, expires)

	err := db.ReleaseQuarantine("node-1")
	if err != nil {
		t.Fatalf("ReleaseQuarantine() error: %v", err)
	}

	quarantined, _ := db.IsNodeQuarantined("node-1")
	if quarantined {
		t.Error("node-1 should not be quarantined after release")
	}
}

func TestPhase3_QuarantineCountSince(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Truncate(time.Second)
	expires := now.Add(1 * time.Hour)

	db.InsertQuarantineRecord("node-1", "task_failures", now, expires)
	db.InsertQuarantineRecord("node-1", "verification_fail", now.Add(1*time.Minute), expires)

	count, err := db.QuarantineCountSince("node-1", now.Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("QuarantineCountSince = %d, want 2", count)
	}
}

// ─── Earnings Reports ───────────────────────────────────────────────────────

func TestPhase3_InsertEarningsReport(t *testing.T) {
	db := newTestDB(t)
	start := time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 2, 6, 0, 0, 0, time.UTC)

	id, err := db.InsertEarningsReport(start, end, 500, 120, 8.0, 2, "llama-3")
	if err != nil {
		t.Fatalf("InsertEarningsReport() error: %v", err)
	}
	if id <= 0 {
		t.Error("expected positive ID")
	}

	pStart, pEnd, credits, tasks, uptime, tier, topModel, err := db.LatestEarningsReport()
	if err != nil {
		t.Fatalf("LatestEarningsReport() error: %v", err)
	}
	if credits != 500 {
		t.Errorf("credits = %d, want 500", credits)
	}
	if tasks != 120 {
		t.Errorf("tasks = %d, want 120", tasks)
	}
	if uptime != 8.0 {
		t.Errorf("uptime = %f, want 8.0", uptime)
	}
	if tier != 2 {
		t.Errorf("tier = %d, want 2", tier)
	}
	if topModel != "llama-3" {
		t.Errorf("topModel = %q, want %q", topModel, "llama-3")
	}
	if pStart.IsZero() || pEnd.IsZero() {
		t.Error("period times should not be zero")
	}
}

func TestPhase3_LatestEarningsReport_Empty(t *testing.T) {
	db := newTestDB(t)
	_, _, credits, tasks, _, _, _, err := db.LatestEarningsReport()
	if err != nil {
		t.Fatalf("LatestEarningsReport() on empty DB error: %v", err)
	}
	if credits != 0 || tasks != 0 {
		t.Error("empty DB should return zero values")
	}
}

// ─── Model Popularity ───────────────────────────────────────────────────────

func TestPhase3_RecordModelRequest(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 5; i++ {
		if err := db.RecordModelRequest("llama-3"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := db.RecordModelRequest("phi-2"); err != nil {
			t.Fatal(err)
		}
	}

	top, err := db.TopRequestedModels(2)
	if err != nil {
		t.Fatalf("TopRequestedModels() error: %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("TopRequestedModels(2) = %d, want 2", len(top))
	}
	if top[0].ModelName != "llama-3" {
		t.Errorf("top model = %q, want %q", top[0].ModelName, "llama-3")
	}
	if top[0].RequestCount != 5 {
		t.Errorf("request count = %d, want 5", top[0].RequestCount)
	}
}

// ─── Scheduler Snapshots ────────────────────────────────────────────────────

func TestPhase3_InsertSchedulerSnapshot(t *testing.T) {
	db := newTestDB(t)
	err := db.InsertSchedulerSnapshot(100, 1, 500, 400, 10, 50, 5)
	if err != nil {
		t.Fatalf("InsertSchedulerSnapshot() error: %v", err)
	}
	// Insert another to ensure no unique constraint issues
	err = db.InsertSchedulerSnapshot(200, 0, 600, 500, 15, 60, 8)
	if err != nil {
		t.Fatalf("InsertSchedulerSnapshot() second error: %v", err)
	}
}
