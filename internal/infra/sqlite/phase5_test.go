package sqlite

import (
	"testing"
	"time"
)

// ─── Phase 5 Migration Test ─────────────────────────────────────────────────

func TestPhase5Migrations_TablesExist(t *testing.T) {
	db := newTestDB(t)

	// Phase 5 tables should have been created automatically by Open → migrate()
	tables := []string{
		"federations",
		"federation_members",
		"governance_proposals",
		"governance_votes",
		"node_reputation",
		"anomaly_profiles",
		"anomaly_events",
		"threat_feed",
	}

	for _, table := range tables {
		var count int
		err := db.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("checking table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s not found in database", table)
		}
	}
}

// ─── Federation CRUD Tests ──────────────────────────────────────────────────

func TestInsertAndListFederations(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	err := db.InsertFederation("fed-acme", "Acme Corp", "node-admin", "ACTIVE", "SPARE", 80, true, "", now, now)
	if err != nil {
		t.Fatalf("InsertFederation failed: %v", err)
	}

	feds, err := db.ListActiveFederations()
	if err != nil {
		t.Fatalf("ListActiveFederations failed: %v", err)
	}
	if len(feds) != 1 {
		t.Fatalf("federation count = %d, want 1", len(feds))
	}
	if feds[0]["name"] != "Acme Corp" {
		t.Errorf("name = %v, want %q", feds[0]["name"], "Acme Corp")
	}
}

func TestUpdateFederationStatus(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	db.InsertFederation("fed-test", "TestCorp", "node-1", "ACTIVE", "SPARE", 80, true, "", now, now)
	err := db.UpdateFederationStatus("fed-test", "SUSPENDED", now+100)
	if err != nil {
		t.Fatalf("UpdateFederationStatus failed: %v", err)
	}

	// SUSPENDED federations should still appear (only DISSOLVED filtered)
	feds, _ := db.ListActiveFederations()
	if len(feds) != 1 {
		t.Fatalf("count = %d, want 1 (suspended still listed)", len(feds))
	}
	if feds[0]["status"] != "SUSPENDED" {
		t.Errorf("status = %v, want SUSPENDED", feds[0]["status"])
	}
}

func TestFederationMember_InsertAndCount(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	db.InsertFederation("fed-1", "TestCorp", "node-admin", "ACTIVE", "SPARE", 80, true, "", now, now)
	db.InsertFederationMember("node-admin", "fed-1", "admin", now, now)
	db.InsertFederationMember("node-worker", "fed-1", "member", now, now)

	count, err := db.FederationMemberCount("fed-1")
	if err != nil {
		t.Fatalf("FederationMemberCount failed: %v", err)
	}
	if count != 2 {
		t.Errorf("member count = %d, want 2", count)
	}
}

func TestRemoveFederationMember(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	db.InsertFederation("fed-1", "TestCorp", "node-admin", "ACTIVE", "SPARE", 80, true, "", now, now)
	db.InsertFederationMember("node-worker", "fed-1", "member", now, now)
	db.RemoveFederationMember("node-worker", "fed-1")

	count, _ := db.FederationMemberCount("fed-1")
	if count != 0 {
		t.Errorf("count after remove = %d, want 0", count)
	}
}

// ─── Governance CRUD Tests ──────────────────────────────────────────────────

func TestInsertAndListProposals(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	err := db.InsertProposal("prop-1", "Test Proposal", "A test description", "NETWORK_PARAM", "node-author", "DRAFT", "test.key", "new-value", now)
	if err != nil {
		t.Fatalf("InsertProposal failed: %v", err)
	}

	proposals, err := db.ListProposalsByStatus("DRAFT")
	if err != nil {
		t.Fatalf("ListProposalsByStatus failed: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("proposal count = %d, want 1", len(proposals))
	}
	if proposals[0]["title"] != "Test Proposal" {
		t.Errorf("title = %v, want %q", proposals[0]["title"], "Test Proposal")
	}
}

func TestUpdateProposalStatus(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	db.InsertProposal("prop-1", "Test", "desc", "NETWORK_PARAM", "node-1", "ACTIVE", "", "", now)
	closedAt := now + 1000
	err := db.UpdateProposalStatus("prop-1", "PASSED", &closedAt)
	if err != nil {
		t.Fatalf("UpdateProposalStatus failed: %v", err)
	}

	proposals, _ := db.ListProposalsByStatus("PASSED")
	if len(proposals) != 1 {
		t.Fatalf("passed count = %d, want 1", len(proposals))
	}
}

func TestInsertVoteAndTally(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	db.InsertProposal("prop-1", "Test", "desc", "NETWORK_PARAM", "node-1", "ACTIVE", "", "", now)
	db.InsertVote("prop-1", "voter-1", "FOR", 500, now)
	db.InsertVote("prop-1", "voter-2", "AGAINST", 300, now)
	db.InsertVote("prop-1", "voter-3", "ABSTAIN", 100, now)

	forW, againstW, abstainW, voterCount, err := db.VoteTally("prop-1")
	if err != nil {
		t.Fatalf("VoteTally failed: %v", err)
	}
	if forW != 500 {
		t.Errorf("for = %d, want 500", forW)
	}
	if againstW != 300 {
		t.Errorf("against = %d, want 300", againstW)
	}
	if abstainW != 100 {
		t.Errorf("abstain = %d, want 100", abstainW)
	}
	if voterCount != 3 {
		t.Errorf("voter count = %d, want 3", voterCount)
	}
}

// ─── Reputation CRUD Tests ──────────────────────────────────────────────────

func TestUpsertAndGetReputation(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	err := db.UpsertReputation("node-1", 0.9, 0.8, 0.7, 0.6, 0.5, 0.1, 100, 30, now, now, now-86400*30)
	if err != nil {
		t.Fatalf("UpsertReputation failed: %v", err)
	}

	rel, acc, avail, spd, lng, pen, tc, da, err := db.GetReputation("node-1")
	if err != nil {
		t.Fatalf("GetReputation failed: %v", err)
	}
	if rel != 0.9 {
		t.Errorf("reliability = %f, want 0.9", rel)
	}
	if acc != 0.8 {
		t.Errorf("accuracy = %f, want 0.8", acc)
	}
	if avail != 0.7 {
		t.Errorf("availability = %f, want 0.7", avail)
	}
	if spd != 0.6 {
		t.Errorf("speed = %f, want 0.6", spd)
	}
	if lng != 0.5 {
		t.Errorf("longevity = %f, want 0.5", lng)
	}
	if pen != 0.1 {
		t.Errorf("penalties = %f, want 0.1", pen)
	}
	if tc != 100 {
		t.Errorf("task count = %d, want 100", tc)
	}
	if da != 30 {
		t.Errorf("days active = %d, want 30", da)
	}
}

func TestUpsertReputation_Update(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	// Insert
	db.UpsertReputation("node-1", 0.5, 0.5, 0.5, 0.5, 0.0, 0.0, 0, 0, now, now, now)

	// Update
	db.UpsertReputation("node-1", 0.9, 0.8, 0.7, 0.6, 0.5, 0.0, 50, 15, now+100, now+100, now)

	rel, _, _, _, _, _, tc, _, err := db.GetReputation("node-1")
	if err != nil {
		t.Fatalf("GetReputation failed: %v", err)
	}
	if rel != 0.9 {
		t.Errorf("updated reliability = %f, want 0.9", rel)
	}
	if tc != 50 {
		t.Errorf("updated task count = %d, want 50", tc)
	}
}

// ─── Anomaly CRUD Tests ────────────────────────────────────────────────────

func TestInsertAndCountAnomalyEvents(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	err := db.InsertAnomalyEvent("node-1", "DURATION_OUTLIER", "WARNING", "task too slow", now)
	if err != nil {
		t.Fatalf("InsertAnomalyEvent failed: %v", err)
	}
	db.InsertAnomalyEvent("node-1", "LOW_CPU", "CRITICAL", "fake inference", now+1)

	count, err := db.AnomalyEventCount("node-1")
	if err != nil {
		t.Fatalf("AnomalyEventCount failed: %v", err)
	}
	if count != 2 {
		t.Errorf("anomaly count = %d, want 2", count)
	}
}

func TestInsertThreatAndCheck(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().Unix()

	err := db.InsertThreatEntry("node-evil", "sybil attack", "node-reporter", now, false)
	if err != nil {
		t.Fatalf("InsertThreatEntry failed: %v", err)
	}

	isThreat, err := db.IsNodeThreat("node-evil")
	if err != nil {
		t.Fatalf("IsNodeThreat failed: %v", err)
	}
	if !isThreat {
		t.Error("expected node-evil to be a threat")
	}

	notThreat, _ := db.IsNodeThreat("node-good")
	if notThreat {
		t.Error("expected node-good to NOT be a threat")
	}
}
