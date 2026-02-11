package sqlite

import (
	"database/sql"
	"testing"
	"time"
)

// ─── Phase 6 Migration Tests ────────────────────────────────────────────────

func TestPhase6Migrations_TablesExist(t *testing.T) {
	db := newTestDB(t)

	tables := []string{
		"ml_scheduler_observations",
		"scaling_decisions",
		"healing_incidents",
		"model_placements",
		"model_retirement_log",
	}
	for _, tbl := range tables {
		t.Run(tbl, func(t *testing.T) {
			var name string
			err := db.db.QueryRow(
				`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
			).Scan(&name)
			if err != nil {
				t.Fatalf("table %s not found: %v", tbl, err)
			}
		})
	}
}

// ─── ml_scheduler_observations ──────────────────────────────────────────────

func TestMLSchedulerObservations_InsertQuery(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	_, err := db.db.Exec(
		`INSERT INTO ml_scheduler_observations (arm_key, node_id, reward, latency_ms, credit_cost, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"INFERENCE:idle:gpu:hot", "node-A", 0.85, 42.5, 1.2, now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var armKey, nodeID string
	var reward, latency, cost float64
	err = db.db.QueryRow(
		`SELECT arm_key, node_id, reward, latency_ms, credit_cost FROM ml_scheduler_observations WHERE arm_key = ?`,
		"INFERENCE:idle:gpu:hot",
	).Scan(&armKey, &nodeID, &reward, &latency, &cost)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if armKey != "INFERENCE:idle:gpu:hot" {
		t.Errorf("arm_key = %s", armKey)
	}
	if nodeID != "node-A" {
		t.Errorf("node_id = %s", nodeID)
	}
	if reward != 0.85 {
		t.Errorf("reward = %f, want 0.85", reward)
	}
}

// ─── scaling_decisions ──────────────────────────────────────────────────────

func TestScalingDecisions_InsertQuery(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	_, err := db.db.Exec(
		`INSERT INTO scaling_decisions (direction, current_capacity, target_capacity, forecast_demand, confidence, proactive, reason, decided_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"SCALE_UP", 10, 15, 14.3, 0.92, true, "forecast predicts peak", now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var dir, reason string
	var cur, target int
	var demand, confidence float64
	var proactive bool
	err = db.db.QueryRow(
		`SELECT direction, current_capacity, target_capacity, forecast_demand, confidence, proactive, reason
		 FROM scaling_decisions ORDER BY id DESC LIMIT 1`,
	).Scan(&dir, &cur, &target, &demand, &confidence, &proactive, &reason)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if dir != "SCALE_UP" {
		t.Errorf("direction = %s", dir)
	}
	if target != 15 {
		t.Errorf("target = %d, want 15", target)
	}
	if !proactive {
		t.Error("expected proactive=true")
	}
}

// ─── healing_incidents ──────────────────────────────────────────────────────

func TestHealingIncidents_InsertQuery(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	_, err := db.db.Exec(
		`INSERT INTO healing_incidents (id, node_id, failure_type, state, attempts, drained_tasks, actions_done, error, detected_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"inc-001", "node-7", "CPU_OVERLOAD", "DETECTED", 0, 0, "", "", now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var id, nodeID, ftype, state string
	err = db.db.QueryRow(
		`SELECT id, node_id, failure_type, state FROM healing_incidents WHERE id = ?`, "inc-001",
	).Scan(&id, &nodeID, &ftype, &state)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if state != "DETECTED" {
		t.Errorf("state = %s, want DETECTED", state)
	}
}

func TestHealingIncidents_UpdateState(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	db.db.Exec(
		`INSERT INTO healing_incidents (id, node_id, failure_type, state, detected_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"inc-002", "node-3", "DISK_FULL", "DETECTED", now,
	)

	_, err := db.db.Exec(
		`UPDATE healing_incidents SET state = ?, isolated_at = ? WHERE id = ?`,
		"ISOLATING", now+1, "inc-002",
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	var state string
	var isolatedAt sql.NullInt64
	db.db.QueryRow(
		`SELECT state, isolated_at FROM healing_incidents WHERE id = ?`, "inc-002",
	).Scan(&state, &isolatedAt)
	if state != "ISOLATING" {
		t.Errorf("state = %s, want ISOLATING", state)
	}
	if !isolatedAt.Valid {
		t.Error("isolated_at should be set")
	}
}

// ─── model_placements ───────────────────────────────────────────────────────

func TestModelPlacements_InsertQuery(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	_, err := db.db.Exec(
		`INSERT INTO model_placements (rec_type, model_name, from_node, to_node, score, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"MOVE", "llama-3", "node-B", "node-A", 0.72, "higher affinity", now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var recType, model, from, to, reason string
	var score float64
	err = db.db.QueryRow(
		`SELECT rec_type, model_name, from_node, to_node, score, reason FROM model_placements WHERE model_name = ?`,
		"llama-3",
	).Scan(&recType, &model, &from, &to, &score, &reason)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if recType != "MOVE" {
		t.Errorf("rec_type = %s, want MOVE", recType)
	}
	if score != 0.72 {
		t.Errorf("score = %f, want 0.72", score)
	}
}

// ─── model_retirement_log ───────────────────────────────────────────────────

func TestModelRetirementLog_InsertQuery(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()
	lastReq := now - 60*86400 // 60 days ago

	_, err := db.db.Exec(
		`INSERT INTO model_retirement_log (model_name, last_requested, days_since_use, size_bytes, reason, retired_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"old-bert", lastReq, 60, 4_000_000_000, "unused for 60 days", now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var model, reason string
	var days int
	var sizeBytes int64
	err = db.db.QueryRow(
		`SELECT model_name, days_since_use, size_bytes, reason FROM model_retirement_log WHERE model_name = ?`,
		"old-bert",
	).Scan(&model, &days, &sizeBytes, &reason)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if days != 60 {
		t.Errorf("days_since_use = %d, want 60", days)
	}
	if sizeBytes != 4_000_000_000 {
		t.Errorf("size_bytes = %d, want 4000000000", sizeBytes)
	}
}

// ─── Index usage checks ─────────────────────────────────────────────────────

func TestPhase6_IndicesExist(t *testing.T) {
	db := newTestDB(t)

	indices := []string{
		"idx_mlobs_arm", "idx_mlobs_node", "idx_mlobs_time",
		"idx_scale_dir", "idx_scale_time",
		"idx_heal_node", "idx_heal_state", "idx_heal_type",
		"idx_place_model", "idx_place_time",
		"idx_retire_model", "idx_retire_time",
	}
	for _, idx := range indices {
		t.Run(idx, func(t *testing.T) {
			var name string
			err := db.db.QueryRow(
				`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
			).Scan(&name)
			if err != nil {
				t.Fatalf("index %s not found: %v", idx, err)
			}
		})
	}
}
