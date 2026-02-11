// Phase 4 SQLite schema and operations.
// Persistence for fine-tuning jobs, marketplace listings, reviews,
// quality checks, P2P chunk transfers, and model popularity stats.
package sqlite

import (
	"database/sql"
	"time"
)

// ─── Phase 4 Schema ─────────────────────────────────────────────────────────

// Phase4Migrations returns the Phase 4 schema migration statements.
func Phase4Migrations() []string {
	return []string{
		// Fine-tuning jobs
		`CREATE TABLE IF NOT EXISTS finetune_jobs (
			id           TEXT PRIMARY KEY,
			base_model   TEXT NOT NULL,
			dataset_uri  TEXT NOT NULL,
			method       TEXT NOT NULL DEFAULT 'lora',
			epochs       INTEGER NOT NULL DEFAULT 3,
			min_nodes    INTEGER NOT NULL DEFAULT 2,
			max_nodes    INTEGER NOT NULL DEFAULT 10,
			status       TEXT NOT NULL DEFAULT 'PENDING',
			config_json  TEXT NOT NULL DEFAULT '{}',
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			started_at   TEXT,
			completed_at TEXT,
			error        TEXT,
			credit_cost  INTEGER DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_finetune_status ON finetune_jobs(status)`,
		`CREATE INDEX IF NOT EXISTS idx_finetune_created ON finetune_jobs(created_at)`,

		// Fine-tuning shards (one per node per job)
		`CREATE TABLE IF NOT EXISTS finetune_shards (
			job_id       TEXT NOT NULL,
			shard_index  INTEGER NOT NULL,
			node_id      TEXT NOT NULL,
			sample_count INTEGER NOT NULL,
			size_bytes   INTEGER NOT NULL,
			digest       TEXT NOT NULL,
			PRIMARY KEY (job_id, shard_index)
		)`,

		// Gradient updates (one per node per epoch)
		`CREATE TABLE IF NOT EXISTS gradient_updates (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id      TEXT NOT NULL,
			node_id     TEXT NOT NULL,
			shard_index INTEGER NOT NULL,
			epoch       INTEGER NOT NULL,
			loss        REAL NOT NULL,
			samples     INTEGER NOT NULL,
			timestamp   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gradient_job ON gradient_updates(job_id, epoch)`,

		// Checkpoints (one per epoch aggregation)
		`CREATE TABLE IF NOT EXISTS finetune_checkpoints (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id     TEXT NOT NULL,
			epoch      INTEGER NOT NULL,
			loss       REAL NOT NULL,
			node_count INTEGER NOT NULL,
			digest     TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(job_id, epoch)
		)`,

		// Marketplace listings
		`CREATE TABLE IF NOT EXISTS marketplace_listings (
			id            TEXT PRIMARY KEY,
			model_name    TEXT NOT NULL,
			base_model    TEXT NOT NULL,
			creator       TEXT NOT NULL,
			description   TEXT DEFAULT '',
			category      TEXT DEFAULT 'general',
			tags_json     TEXT DEFAULT '[]',
			version       TEXT DEFAULT '1.0',
			size_bytes    INTEGER NOT NULL DEFAULT 0,
			digest        TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'PENDING',
			price         INTEGER NOT NULL DEFAULT 1,
			downloads     INTEGER DEFAULT 0,
			total_revenue INTEGER DEFAULT 0,
			created_at    TEXT NOT NULL DEFAULT (datetime('now')),
			published_at  TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mkt_status ON marketplace_listings(status)`,
		`CREATE INDEX IF NOT EXISTS idx_mkt_creator ON marketplace_listings(creator)`,
		`CREATE INDEX IF NOT EXISTS idx_mkt_category ON marketplace_listings(category)`,
		`CREATE INDEX IF NOT EXISTS idx_mkt_downloads ON marketplace_listings(downloads DESC)`,

		// Marketplace benchmarks
		`CREATE TABLE IF NOT EXISTS marketplace_benchmarks (
			listing_id    TEXT PRIMARY KEY,
			perplexity    REAL,
			bleu          REAL,
			human_eval    REAL,
			tok_per_sec   REAL,
			memory_mb     INTEGER,
			context_length INTEGER,
			verified      INTEGER DEFAULT 0
		)`,

		// Marketplace reviews
		`CREATE TABLE IF NOT EXISTS marketplace_reviews (
			id         TEXT PRIMARY KEY,
			listing_id TEXT NOT NULL,
			author     TEXT NOT NULL,
			rating     INTEGER NOT NULL CHECK(rating BETWEEN 1 AND 5),
			comment    TEXT DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(listing_id, author)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_review_listing ON marketplace_reviews(listing_id)`,

		// Quality check results
		`CREATE TABLE IF NOT EXISTS quality_checks (
			listing_id  TEXT PRIMARY KEY,
			passed      INTEGER NOT NULL,
			issues_json TEXT DEFAULT '[]',
			signatures  INTEGER DEFAULT 0,
			no_malware  INTEGER DEFAULT 0,
			benchmarked INTEGER DEFAULT 0,
			checked_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// P2P chunk transfer tracking
		`CREATE TABLE IF NOT EXISTS chunk_transfers (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			manifest_id  TEXT NOT NULL,
			chunk_index  INTEGER NOT NULL,
			from_peer    TEXT NOT NULL,
			to_peer      TEXT NOT NULL,
			size_bytes   INTEGER NOT NULL,
			status       TEXT NOT NULL DEFAULT 'PENDING',
			started_at   TEXT,
			completed_at TEXT,
			UNIQUE(manifest_id, chunk_index, to_peer)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transfer_manifest ON chunk_transfers(manifest_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transfer_status ON chunk_transfers(status)`,
	}
}

// ─── Fine-Tuning Job Operations ─────────────────────────────────────────────

// InsertFineTuneJob creates a new fine-tuning job record.
func (db *DB) InsertFineTuneJob(id, baseModel, datasetURI, method string, epochs, minNodes, maxNodes int, configJSON string) error {
	_, err := db.db.Exec(`
		INSERT INTO finetune_jobs (id, base_model, dataset_uri, method, epochs, min_nodes, max_nodes, config_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, baseModel, datasetURI, method, epochs, minNodes, maxNodes, configJSON)
	return err
}

// UpdateFineTuneJobStatus updates a job's status.
func (db *DB) UpdateFineTuneJobStatus(id, status string, errorMsg *string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	switch status {
	case "TRAINING":
		_, err := db.db.Exec(`
			UPDATE finetune_jobs SET status = ?, started_at = ? WHERE id = ?
		`, status, now, id)
		return err
	case "COMPLETED", "FAILED", "CANCELLED":
		_, err := db.db.Exec(`
			UPDATE finetune_jobs SET status = ?, completed_at = ?, error = ? WHERE id = ?
		`, status, now, errorMsg, id)
		return err
	default:
		_, err := db.db.Exec(`
			UPDATE finetune_jobs SET status = ? WHERE id = ?
		`, status, id)
		return err
	}
}

// GetFineTuneJob retrieves a fine-tuning job.
func (db *DB) GetFineTuneJob(id string) (baseModel, datasetURI, method, status, configJSON string, epochs int, creditCost int64, err error) {
	err = db.db.QueryRow(`
		SELECT base_model, dataset_uri, method, status, config_json, epochs, credit_cost
		FROM finetune_jobs WHERE id = ?
	`, id).Scan(&baseModel, &datasetURI, &method, &status, &configJSON, &epochs, &creditCost)
	if err == sql.ErrNoRows {
		return "", "", "", "", "", 0, 0, nil
	}
	return
}

// ListFineTuneJobs returns recent jobs ordered by creation time.
func (db *DB) ListFineTuneJobs(limit int) ([]struct {
	ID        string
	BaseModel string
	Status    string
	Epochs    int
	CreatedAt string
}, error) {
	rows, err := db.db.Query(`
		SELECT id, base_model, status, epochs, created_at
		FROM finetune_jobs ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []struct {
		ID        string
		BaseModel string
		Status    string
		Epochs    int
		CreatedAt string
	}
	for rows.Next() {
		var r struct {
			ID        string
			BaseModel string
			Status    string
			Epochs    int
			CreatedAt string
		}
		if err := rows.Scan(&r.ID, &r.BaseModel, &r.Status, &r.Epochs, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// AddFineTuneCredits adds credit cost to a fine-tuning job.
func (db *DB) AddFineTuneCredits(jobID string, credits int64) error {
	_, err := db.db.Exec(`
		UPDATE finetune_jobs SET credit_cost = credit_cost + ? WHERE id = ?
	`, credits, jobID)
	return err
}

// ─── Gradient Update Operations ─────────────────────────────────────────────

// InsertGradientUpdate records a gradient update from a node.
func (db *DB) InsertGradientUpdate(jobID, nodeID string, shardIndex, epoch int, loss float64, samples int) error {
	_, err := db.db.Exec(`
		INSERT INTO gradient_updates (job_id, node_id, shard_index, epoch, loss, samples)
		VALUES (?, ?, ?, ?, ?, ?)
	`, jobID, nodeID, shardIndex, epoch, loss, samples)
	return err
}

// CountEpochGradients returns how many nodes reported gradients for an epoch.
func (db *DB) CountEpochGradients(jobID string, epoch int) (int, error) {
	var cnt int
	err := db.db.QueryRow(`
		SELECT COUNT(*) FROM gradient_updates WHERE job_id = ? AND epoch = ?
	`, jobID, epoch).Scan(&cnt)
	return cnt, err
}

// ─── Marketplace Listing Operations ─────────────────────────────────────────

// InsertMarketplaceListing creates a new marketplace listing.
func (db *DB) InsertMarketplaceListing(id, modelName, baseModel, creator, description, category, tagsJSON, version string, sizeBytes int64, digest string, price int64) error {
	_, err := db.db.Exec(`
		INSERT INTO marketplace_listings (id, model_name, base_model, creator, description, category, tags_json, version, size_bytes, digest, price)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, modelName, baseModel, creator, description, category, tagsJSON, version, sizeBytes, digest, price)
	return err
}

// UpdateListingStatus updates a listing's status.
func (db *DB) UpdateListingStatus(id, status string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	if status == "APPROVED" {
		_, err := db.db.Exec(`
			UPDATE marketplace_listings SET status = ?, published_at = ? WHERE id = ?
		`, status, now, id)
		return err
	}
	_, err := db.db.Exec(`
		UPDATE marketplace_listings SET status = ? WHERE id = ?
	`, status, id)
	return err
}

// RecordListingDownload increments download count and revenue.
func (db *DB) RecordListingDownload(id string, creatorShare int64) error {
	_, err := db.db.Exec(`
		UPDATE marketplace_listings
		SET downloads = downloads + 1, total_revenue = total_revenue + ?
		WHERE id = ?
	`, creatorShare, id)
	return err
}

// GetListingDownloads returns total downloads for a listing.
func (db *DB) GetListingDownloads(id string) (int64, error) {
	var downloads int64
	err := db.db.QueryRow(`SELECT downloads FROM marketplace_listings WHERE id = ?`, id).Scan(&downloads)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return downloads, err
}

// SearchListings searches approved listings by category/query.
func (db *DB) SearchListings(category, query string, limit int) ([]struct {
	ID        string
	ModelName string
	Creator   string
	Category  string
	Price     int64
	Downloads int64
}, error) {
	q := `SELECT id, model_name, creator, category, price, downloads
		  FROM marketplace_listings WHERE status = 'APPROVED'`
	var args []any

	if category != "" {
		q += ` AND category = ?`
		args = append(args, category)
	}
	if query != "" {
		q += ` AND (model_name LIKE ? OR description LIKE ?)`
		pattern := "%" + query + "%"
		args = append(args, pattern, pattern)
	}
	q += ` ORDER BY downloads DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []struct {
		ID        string
		ModelName string
		Creator   string
		Category  string
		Price     int64
		Downloads int64
	}
	for rows.Next() {
		var r struct {
			ID        string
			ModelName string
			Creator   string
			Category  string
			Price     int64
			Downloads int64
		}
		if err := rows.Scan(&r.ID, &r.ModelName, &r.Creator, &r.Category, &r.Price, &r.Downloads); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ─── Review Operations ──────────────────────────────────────────────────────

// InsertReview adds a review.
func (db *DB) InsertReview(id, listingID, author string, rating int, comment string) error {
	_, err := db.db.Exec(`
		INSERT INTO marketplace_reviews (id, listing_id, author, rating, comment)
		VALUES (?, ?, ?, ?, ?)
	`, id, listingID, author, rating, comment)
	return err
}

// AverageListingRating returns the average rating for a listing.
func (db *DB) AverageListingRating(listingID string) (float64, int, error) {
	var avg sql.NullFloat64
	var cnt int
	err := db.db.QueryRow(`
		SELECT AVG(rating), COUNT(*) FROM marketplace_reviews WHERE listing_id = ?
	`, listingID).Scan(&avg, &cnt)
	if !avg.Valid {
		return 0, 0, err
	}
	return avg.Float64, cnt, err
}

// ─── Quality Check Operations ───────────────────────────────────────────────

// UpsertQualityCheck saves a quality check result.
func (db *DB) UpsertQualityCheck(listingID string, passed, signatures, noMalware, benchmarked bool, issuesJSON string) error {
	boolToInt := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	_, err := db.db.Exec(`
		INSERT INTO quality_checks (listing_id, passed, issues_json, signatures, no_malware, benchmarked)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(listing_id) DO UPDATE SET
			passed      = excluded.passed,
			issues_json = excluded.issues_json,
			signatures  = excluded.signatures,
			no_malware  = excluded.no_malware,
			benchmarked = excluded.benchmarked,
			checked_at  = datetime('now')
	`, listingID, boolToInt(passed), issuesJSON, boolToInt(signatures), boolToInt(noMalware), boolToInt(benchmarked))
	return err
}

// ─── Chunk Transfer Operations ──────────────────────────────────────────────

// InsertChunkTransfer records a chunk transfer.
func (db *DB) InsertChunkTransfer(manifestID string, chunkIndex int, fromPeer, toPeer string, sizeBytes int64) error {
	_, err := db.db.Exec(`
		INSERT OR IGNORE INTO chunk_transfers (manifest_id, chunk_index, from_peer, to_peer, size_bytes)
		VALUES (?, ?, ?, ?, ?)
	`, manifestID, chunkIndex, fromPeer, toPeer, sizeBytes)
	return err
}

// CompleteChunkTransfer marks a chunk transfer as completed.
func (db *DB) CompleteChunkTransfer(manifestID string, chunkIndex int, toPeer string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.db.Exec(`
		UPDATE chunk_transfers SET status = 'COMPLETED', completed_at = ?
		WHERE manifest_id = ? AND chunk_index = ? AND to_peer = ?
	`, now, manifestID, chunkIndex, toPeer)
	return err
}

// TransferProgress returns completion stats for a manifest download.
func (db *DB) TransferProgress(manifestID, toPeer string) (total, completed int, err error) {
	err = db.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN status = 'COMPLETED' THEN 1 ELSE 0 END)
		FROM chunk_transfers WHERE manifest_id = ? AND to_peer = ?
	`, manifestID, toPeer).Scan(&total, &completed)
	return
}
