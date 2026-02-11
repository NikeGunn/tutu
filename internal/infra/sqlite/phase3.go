// Phase 3 SQLite schema and operations.
// Persistence for regions, scheduler stats, circuit breakers, quarantines,
// and earnings reports.
package sqlite

import (
	"database/sql"
	"time"
)

// ─── Phase 3 Schema ─────────────────────────────────────────────────────────

// Phase3Migrations returns the Phase 3 schema migration statements.
// Each string is a single SQL statement (SQLite executes one at a time).
func Phase3Migrations() []string {
	return []string{
		// Region status tracking
		`CREATE TABLE IF NOT EXISTS region_status (
			region       TEXT PRIMARY KEY,
			healthy      INTEGER NOT NULL DEFAULT 1,
			node_count   INTEGER NOT NULL DEFAULT 0,
			active_tasks INTEGER NOT NULL DEFAULT 0,
			queue_depth  INTEGER NOT NULL DEFAULT 0,
			avg_latency_ms REAL NOT NULL DEFAULT 0,
			updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Circuit breaker state persistence
		`CREATE TABLE IF NOT EXISTS circuit_breakers (
			name         TEXT PRIMARY KEY,
			state        INTEGER NOT NULL DEFAULT 0,
			failures     INTEGER NOT NULL DEFAULT 0,
			total_trips  INTEGER NOT NULL DEFAULT 0,
			tripped_at   TEXT,
			updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Quarantine records
		`CREATE TABLE IF NOT EXISTS quarantine_records (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id    TEXT NOT NULL,
			reason     TEXT NOT NULL,
			started_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			released   INTEGER NOT NULL DEFAULT 0,
			UNIQUE(node_id, started_at)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_quarantine_node ON quarantine_records(node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_quarantine_active ON quarantine_records(released, expires_at)`,

		// Earnings reports (morning summary)
		`CREATE TABLE IF NOT EXISTS earnings_reports (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			period_start    TEXT NOT NULL,
			period_end      TEXT NOT NULL,
			credits_earned  INTEGER NOT NULL DEFAULT 0,
			tasks_completed INTEGER NOT NULL DEFAULT 0,
			uptime_hours    REAL NOT NULL DEFAULT 0,
			hardware_tier   INTEGER NOT NULL DEFAULT 0,
			top_model       TEXT,
			created_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Scheduler statistics snapshots
		`CREATE TABLE IF NOT EXISTS scheduler_snapshots (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			queue_depth     INTEGER NOT NULL DEFAULT 0,
			back_pressure   INTEGER NOT NULL DEFAULT 0,
			total_enqueued  INTEGER NOT NULL DEFAULT 0,
			total_completed INTEGER NOT NULL DEFAULT 0,
			total_rejected  INTEGER NOT NULL DEFAULT 0,
			total_stolen    INTEGER NOT NULL DEFAULT 0,
			total_preempted INTEGER NOT NULL DEFAULT 0,
			snapshot_at     TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Model popularity tracking
		`CREATE TABLE IF NOT EXISTS model_popularity (
			model_name     TEXT PRIMARY KEY,
			request_count  INTEGER NOT NULL DEFAULT 0,
			last_requested TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}
}

// ─── Region Status Operations ───────────────────────────────────────────────

// UpsertRegionStatus inserts or updates a region's status.
func (db *DB) UpsertRegionStatus(region string, healthy bool, nodeCount, activeTasks, queueDepth int, avgLatencyMs float64) error {
	healthyInt := 0
	if healthy {
		healthyInt = 1
	}
	_, err := db.db.Exec(`
		INSERT INTO region_status (region, healthy, node_count, active_tasks, queue_depth, avg_latency_ms, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(region) DO UPDATE SET
			healthy      = excluded.healthy,
			node_count   = excluded.node_count,
			active_tasks = excluded.active_tasks,
			queue_depth  = excluded.queue_depth,
			avg_latency_ms = excluded.avg_latency_ms,
			updated_at   = datetime('now')
	`, region, healthyInt, nodeCount, activeTasks, queueDepth, avgLatencyMs)
	return err
}

// GetRegionStatus retrieves a region's status.
func (db *DB) GetRegionStatus(region string) (healthy bool, nodeCount, activeTasks, queueDepth int, avgLatencyMs float64, err error) {
	var healthyInt int
	err = db.db.QueryRow(`
		SELECT healthy, node_count, active_tasks, queue_depth, avg_latency_ms
		FROM region_status WHERE region = ?
	`, region).Scan(&healthyInt, &nodeCount, &activeTasks, &queueDepth, &avgLatencyMs)
	if err == sql.ErrNoRows {
		return true, 0, 0, 0, 0, nil
	}
	healthy = healthyInt == 1
	return
}

// ListRegionStatuses returns all region statuses.
func (db *DB) ListRegionStatuses() ([]struct {
	Region       string
	Healthy      bool
	NodeCount    int
	ActiveTasks  int
	QueueDepth   int
	AvgLatencyMs float64
	UpdatedAt    time.Time
}, error) {
	rows, err := db.db.Query(`
		SELECT region, healthy, node_count, active_tasks, queue_depth, avg_latency_ms, updated_at
		FROM region_status ORDER BY region
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type RegionRow struct {
		Region       string
		Healthy      bool
		NodeCount    int
		ActiveTasks  int
		QueueDepth   int
		AvgLatencyMs float64
		UpdatedAt    time.Time
	}

	var result []struct {
		Region       string
		Healthy      bool
		NodeCount    int
		ActiveTasks  int
		QueueDepth   int
		AvgLatencyMs float64
		UpdatedAt    time.Time
	}

	for rows.Next() {
		var r struct {
			Region       string
			Healthy      bool
			NodeCount    int
			ActiveTasks  int
			QueueDepth   int
			AvgLatencyMs float64
			UpdatedAt    time.Time
		}
		var healthyInt int
		var updatedStr string
		if err := rows.Scan(&r.Region, &healthyInt, &r.NodeCount, &r.ActiveTasks, &r.QueueDepth, &r.AvgLatencyMs, &updatedStr); err != nil {
			return nil, err
		}
		r.Healthy = healthyInt == 1
		r.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedStr)
		result = append(result, r)
	}
	return result, rows.Err()
}

// ─── Circuit Breaker Operations ─────────────────────────────────────────────

// UpsertCircuitBreaker saves circuit breaker state.
func (db *DB) UpsertCircuitBreaker(name string, state, failures, totalTrips int, trippedAt *time.Time) error {
	var trippedStr *string
	if trippedAt != nil {
		s := trippedAt.Format(time.RFC3339)
		trippedStr = &s
	}
	_, err := db.db.Exec(`
		INSERT INTO circuit_breakers (name, state, failures, total_trips, tripped_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(name) DO UPDATE SET
			state      = excluded.state,
			failures   = excluded.failures,
			total_trips = excluded.total_trips,
			tripped_at = excluded.tripped_at,
			updated_at = datetime('now')
	`, name, state, failures, totalTrips, trippedStr)
	return err
}

// GetCircuitBreaker retrieves a circuit breaker's persisted state.
func (db *DB) GetCircuitBreaker(name string) (state, failures, totalTrips int, trippedAt *time.Time, err error) {
	var trippedStr sql.NullString
	err = db.db.QueryRow(`
		SELECT state, failures, total_trips, tripped_at
		FROM circuit_breakers WHERE name = ?
	`, name).Scan(&state, &failures, &totalTrips, &trippedStr)
	if err == sql.ErrNoRows {
		return 0, 0, 0, nil, nil
	}
	if err != nil {
		return
	}
	if trippedStr.Valid {
		t, _ := time.Parse(time.RFC3339, trippedStr.String)
		trippedAt = &t
	}
	return
}

// ─── Quarantine Operations ──────────────────────────────────────────────────

// InsertQuarantineRecord adds a quarantine record.
func (db *DB) InsertQuarantineRecord(nodeID, reason string, startedAt, expiresAt time.Time) (int64, error) {
	res, err := db.db.Exec(`
		INSERT OR IGNORE INTO quarantine_records (node_id, reason, started_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, nodeID, reason, startedAt.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// IsNodeQuarantined checks if a node has an active quarantine.
func (db *DB) IsNodeQuarantined(nodeID string) (bool, error) {
	var count int
	err := db.db.QueryRow(`
		SELECT COUNT(*) FROM quarantine_records
		WHERE node_id = ? AND released = 0 AND expires_at > datetime('now')
	`, nodeID).Scan(&count)
	return count > 0, err
}

// ReleaseQuarantine marks all quarantines for a node as released.
func (db *DB) ReleaseQuarantine(nodeID string) error {
	_, err := db.db.Exec(`
		UPDATE quarantine_records SET released = 1 WHERE node_id = ? AND released = 0
	`, nodeID)
	return err
}

// QuarantineCountSince counts quarantines for a node since a given time.
func (db *DB) QuarantineCountSince(nodeID string, since time.Time) (int, error) {
	var count int
	err := db.db.QueryRow(`
		SELECT COUNT(*) FROM quarantine_records
		WHERE node_id = ? AND started_at > ?
	`, nodeID, since.Format(time.RFC3339)).Scan(&count)
	return count, err
}

// ─── Earnings Report Operations ─────────────────────────────────────────────

// InsertEarningsReport saves an earnings report.
func (db *DB) InsertEarningsReport(periodStart, periodEnd time.Time, credits int64, tasks int, uptimeHrs float64, tier int, topModel string) (int64, error) {
	res, err := db.db.Exec(`
		INSERT INTO earnings_reports (period_start, period_end, credits_earned, tasks_completed, uptime_hours, hardware_tier, top_model)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339), credits, tasks, uptimeHrs, tier, topModel)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LatestEarningsReport returns the most recent earnings report.
func (db *DB) LatestEarningsReport() (periodStart, periodEnd time.Time, credits int64, tasks int, uptimeHrs float64, tier int, topModel string, err error) {
	var startStr, endStr string
	err = db.db.QueryRow(`
		SELECT period_start, period_end, credits_earned, tasks_completed, uptime_hours, hardware_tier, top_model
		FROM earnings_reports ORDER BY created_at DESC LIMIT 1
	`).Scan(&startStr, &endStr, &credits, &tasks, &uptimeHrs, &tier, &topModel)
	if err == sql.ErrNoRows {
		return time.Time{}, time.Time{}, 0, 0, 0, 0, "", nil
	}
	if err != nil {
		return
	}
	periodStart, _ = time.Parse(time.RFC3339, startStr)
	periodEnd, _ = time.Parse(time.RFC3339, endStr)
	return
}

// ─── Model Popularity Operations ────────────────────────────────────────────

// RecordModelRequest increments the request count for a model.
func (db *DB) RecordModelRequest(modelName string) error {
	_, err := db.db.Exec(`
		INSERT INTO model_popularity (model_name, request_count, last_requested)
		VALUES (?, 1, datetime('now'))
		ON CONFLICT(model_name) DO UPDATE SET
			request_count  = request_count + 1,
			last_requested = datetime('now')
	`, modelName)
	return err
}

// TopRequestedModels returns the most requested models.
func (db *DB) TopRequestedModels(limit int) ([]struct {
	ModelName    string
	RequestCount int64
}, error) {
	rows, err := db.db.Query(`
		SELECT model_name, request_count FROM model_popularity
		ORDER BY request_count DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []struct {
		ModelName    string
		RequestCount int64
	}
	for rows.Next() {
		var r struct {
			ModelName    string
			RequestCount int64
		}
		if err := rows.Scan(&r.ModelName, &r.RequestCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ─── Scheduler Snapshot Operations ──────────────────────────────────────────

// InsertSchedulerSnapshot saves a scheduler statistics snapshot.
func (db *DB) InsertSchedulerSnapshot(queueDepth, backPressure int, totalEnqueued, totalCompleted, totalRejected, totalStolen, totalPreempted int64) error {
	_, err := db.db.Exec(`
		INSERT INTO scheduler_snapshots (queue_depth, back_pressure, total_enqueued, total_completed, total_rejected, total_stolen, total_preempted)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, queueDepth, backPressure, totalEnqueued, totalCompleted, totalRejected, totalStolen, totalPreempted)
	return err
}
