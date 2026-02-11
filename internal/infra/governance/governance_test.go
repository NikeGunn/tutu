package governance

import (
	"testing"
	"time"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine(DefaultEngineConfig())
	e.SetTotalCredits(10000)
	return e
}

// fixedTime returns a clock function pinned to a specific time.
func fixedTime(year int, month time.Month, day int) func() time.Time {
	t := time.Date(year, month, day, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// tickingClock returns a clock that advances 1ms on each call (avoids ID collision).
func tickingClock() func() time.Time {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	call := 0
	return func() time.Time {
		call++
		return base.Add(time.Duration(call) * time.Millisecond)
	}
}

// createAndOpenProposal is a helper that creates + opens a proposal.
func createAndOpenProposal(t *testing.T, e *Engine, title string) *Proposal {
	t.Helper()
	prop, err := e.CreateProposal(title, "test description", CatNetworkParam, "node-author", 500, "test.key", "new-value")
	if err != nil {
		t.Fatalf("CreateProposal(%q) failed: %v", title, err)
	}
	if err := e.OpenProposal(prop.ID); err != nil {
		t.Fatalf("OpenProposal(%s) failed: %v", prop.ID, err)
	}
	return prop
}

// ─── Proposal Lifecycle ─────────────────────────────────────────────────────

func TestCreateProposal(t *testing.T) {
	e := newTestEngine(t)

	prop, err := e.CreateProposal(
		"Increase earning rate",
		"Raise base rate to 1.5x",
		CatEarningRate,
		"node-proposer",
		500, // authorCredits
		"earnings.base_rate",
		"1.5",
	)
	if err != nil {
		t.Fatalf("CreateProposal failed: %v", err)
	}

	if prop.Status != PropDraft {
		t.Errorf("status = %v, want PropDraft", prop.Status)
	}
	if prop.Author != "node-proposer" {
		t.Errorf("author = %q, want %q", prop.Author, "node-proposer")
	}
	if prop.Category != CatEarningRate {
		t.Errorf("category = %v, want CatEarningRate", prop.Category)
	}
	if prop.ParamKey != "earnings.base_rate" {
		t.Errorf("param_key = %q, want %q", prop.ParamKey, "earnings.base_rate")
	}
}

func TestCreateProposal_InsufficientCredits(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.CreateProposal("Test", "desc", CatNetworkParam, "node-poor", 50, "", "")
	if err == nil {
		t.Fatal("expected error for insufficient credits")
	}
}

func TestCreateProposal_EmptyTitle(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.CreateProposal("", "desc", CatNetworkParam, "node-1", 500, "", "")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestCreateProposal_EmptyAuthor(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.CreateProposal("Test", "desc", CatNetworkParam, "", 500, "", "")
	if err == nil {
		t.Fatal("expected error for empty author")
	}
}

func TestOpenProposal(t *testing.T) {
	e := newTestEngine(t)
	prop, _ := e.CreateProposal("Test", "desc", CatNetworkParam, "node-1", 500, "", "")

	if err := e.OpenProposal(prop.ID); err != nil {
		t.Fatalf("OpenProposal failed: %v", err)
	}

	got, _ := e.GetProposal(prop.ID)
	if got.Status != PropActive {
		t.Errorf("status = %v, want PropActive", got.Status)
	}
	if got.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set after opening")
	}
}

func TestOpenProposal_NotDraft(t *testing.T) {
	e := newTestEngine(t)
	prop := createAndOpenProposal(t, e, "Test")

	err := e.OpenProposal(prop.ID)
	if err == nil {
		t.Fatal("expected error opening already-active proposal")
	}
}

func TestCancelProposal(t *testing.T) {
	e := newTestEngine(t)
	prop, _ := e.CreateProposal("Test", "desc", CatNetworkParam, "node-author", 500, "", "")

	if err := e.CancelProposal(prop.ID, "node-author"); err != nil {
		t.Fatalf("CancelProposal failed: %v", err)
	}

	got, _ := e.GetProposal(prop.ID)
	if got.Status != PropCancelled {
		t.Errorf("status = %v, want PropCancelled", got.Status)
	}
}

func TestCancelProposal_WrongAuthor(t *testing.T) {
	e := newTestEngine(t)
	prop, _ := e.CreateProposal("Test", "desc", CatNetworkParam, "node-author", 500, "", "")

	err := e.CancelProposal(prop.ID, "node-impostor")
	if err == nil {
		t.Fatal("expected error for non-author cancel")
	}
}

func TestCancelProposal_NotFound(t *testing.T) {
	e := newTestEngine(t)
	err := e.CancelProposal("prop-nonexistent", "node-1")
	if err == nil {
		t.Fatal("expected error for non-existent proposal")
	}
}

// ─── Voting Tests ───────────────────────────────────────────────────────────

func TestCastVote(t *testing.T) {
	e := newTestEngine(t)
	prop := createAndOpenProposal(t, e, "Vote Test")

	if err := e.CastVote(prop.ID, "node-voter-1", VoteFor, 500); err != nil {
		t.Fatalf("CastVote failed: %v", err)
	}

	tally, err := e.Tally(prop.ID)
	if err != nil {
		t.Fatalf("Tally failed: %v", err)
	}
	if tally.ForWeight != 500 {
		t.Errorf("for weight = %d, want 500", tally.ForWeight)
	}
	if tally.VoterCount != 1 {
		t.Errorf("voter count = %d, want 1", tally.VoterCount)
	}
}

func TestCastVote_MultipleVoters(t *testing.T) {
	e := newTestEngine(t)
	prop := createAndOpenProposal(t, e, "Multi Vote")

	e.CastVote(prop.ID, "node-1", VoteFor, 3000)
	e.CastVote(prop.ID, "node-2", VoteAgainst, 1000)
	e.CastVote(prop.ID, "node-3", VoteAbstain, 200)

	tally, _ := e.Tally(prop.ID)
	if tally.ForWeight != 3000 {
		t.Errorf("for = %d, want 3000", tally.ForWeight)
	}
	if tally.AgainstWeight != 1000 {
		t.Errorf("against = %d, want 1000", tally.AgainstWeight)
	}
	if tally.AbstainWeight != 200 {
		t.Errorf("abstain = %d, want 200", tally.AbstainWeight)
	}
	if tally.TotalWeight != 4200 {
		t.Errorf("total = %d, want 4200", tally.TotalWeight)
	}
	if tally.VoterCount != 3 {
		t.Errorf("voters = %d, want 3", tally.VoterCount)
	}
}

func TestCastVote_ChangeVote(t *testing.T) {
	e := newTestEngine(t)
	prop := createAndOpenProposal(t, e, "Change Vote")

	e.CastVote(prop.ID, "node-1", VoteFor, 500)
	e.CastVote(prop.ID, "node-1", VoteAgainst, 500) // Change to against

	tally, _ := e.Tally(prop.ID)
	if tally.ForWeight != 0 {
		t.Errorf("for = %d, want 0 after change", tally.ForWeight)
	}
	if tally.AgainstWeight != 500 {
		t.Errorf("against = %d, want 500 after change", tally.AgainstWeight)
	}
}

func TestCastVote_NotActive(t *testing.T) {
	e := newTestEngine(t)
	prop, _ := e.CreateProposal("Draft", "desc", CatNetworkParam, "node-1", 500, "", "")

	// Still DRAFT — not opened
	err := e.CastVote(prop.ID, "node-voter", VoteFor, 100)
	if err == nil {
		t.Fatal("expected error voting on draft proposal")
	}
}

func TestCastVote_ZeroWeight(t *testing.T) {
	e := newTestEngine(t)
	prop := createAndOpenProposal(t, e, "Zero Weight")

	err := e.CastVote(prop.ID, "node-1", VoteFor, 0)
	if err == nil {
		t.Fatal("expected error for zero weight vote")
	}
}

func TestCastVote_Expired(t *testing.T) {
	e := newTestEngine(t)
	e.now = fixedTime(2025, 1, 1)
	prop := createAndOpenProposal(t, e, "Expired Test")

	// Advance time past expiry
	e.now = fixedTime(2025, 1, 10) // 9 days later > 7 day voting period

	err := e.CastVote(prop.ID, "node-late", VoteFor, 100)
	if err == nil {
		t.Fatal("expected error for expired voting")
	}
}

// ─── Quorum + Approval Tests ───────────────────────────────────────────────

func TestTally_QuorumReached(t *testing.T) {
	e := newTestEngine(t)
	e.SetTotalCredits(10000)
	prop := createAndOpenProposal(t, e, "Quorum Test")

	// 30% of 10000 = 3000 needed for quorum
	e.CastVote(prop.ID, "node-1", VoteFor, 2000)
	e.CastVote(prop.ID, "node-2", VoteFor, 1500)

	tally, _ := e.Tally(prop.ID)
	if !tally.QuorumReached {
		t.Error("expected quorum reached (3500 >= 3000)")
	}
	if tally.ApprovalPct != 100 {
		t.Errorf("approval = %.1f%%, want 100%%", tally.ApprovalPct)
	}
}

func TestTally_QuorumNotReached(t *testing.T) {
	e := newTestEngine(t)
	e.SetTotalCredits(10000)
	prop := createAndOpenProposal(t, e, "No Quorum")

	e.CastVote(prop.ID, "node-1", VoteFor, 500) // Only 5% — below 30%

	tally, _ := e.Tally(prop.ID)
	if tally.QuorumReached {
		t.Error("expected quorum NOT reached")
	}
}

// ─── Resolution Tests ───────────────────────────────────────────────────────

func TestResolveExpired_Passed(t *testing.T) {
	e := newTestEngine(t)
	e.SetTotalCredits(10000)
	e.now = fixedTime(2025, 1, 1)

	prop := createAndOpenProposal(t, e, "Pass Test")
	e.CastVote(prop.ID, "node-1", VoteFor, 4000) // Quorum + majority

	// Advance past expiry
	e.now = fixedTime(2025, 1, 10)
	changed := e.ResolveExpired()

	if len(changed) != 1 {
		t.Fatalf("changed count = %d, want 1", len(changed))
	}
	if changed[0].Status != PropPassed {
		t.Errorf("status = %v, want PropPassed", changed[0].Status)
	}
}

func TestResolveExpired_Rejected(t *testing.T) {
	e := newTestEngine(t)
	e.SetTotalCredits(10000)
	e.now = fixedTime(2025, 1, 1)

	prop := createAndOpenProposal(t, e, "Reject Test")
	e.CastVote(prop.ID, "node-1", VoteAgainst, 4000)

	e.now = fixedTime(2025, 1, 10)
	changed := e.ResolveExpired()

	if len(changed) != 1 {
		t.Fatalf("changed count = %d, want 1", len(changed))
	}
	if changed[0].Status != PropRejected {
		t.Errorf("status = %v, want PropRejected", changed[0].Status)
	}
}

func TestResolveExpired_NoQuorum(t *testing.T) {
	e := newTestEngine(t)
	e.SetTotalCredits(10000)
	e.now = fixedTime(2025, 1, 1)

	prop := createAndOpenProposal(t, e, "Expire Test")
	e.CastVote(prop.ID, "node-1", VoteFor, 100) // Way below quorum

	e.now = fixedTime(2025, 1, 10)
	changed := e.ResolveExpired()

	if len(changed) != 1 {
		t.Fatalf("changed = %d, want 1", len(changed))
	}
	if changed[0].Status != PropExpired {
		t.Errorf("status = %v, want PropExpired", changed[0].Status)
	}
}

func TestMarkExecuted(t *testing.T) {
	e := newTestEngine(t)
	e.SetTotalCredits(10000)
	e.now = fixedTime(2025, 1, 1)

	prop := createAndOpenProposal(t, e, "Execute Test")
	e.CastVote(prop.ID, "node-1", VoteFor, 5000)

	e.now = fixedTime(2025, 1, 10)
	e.ResolveExpired()

	if err := e.MarkExecuted(prop.ID); err != nil {
		t.Fatalf("MarkExecuted failed: %v", err)
	}
	got, _ := e.GetProposal(prop.ID)
	if got.Status != PropExecuted {
		t.Errorf("status = %v, want PropExecuted", got.Status)
	}
}

func TestMarkExecuted_NotPassed(t *testing.T) {
	e := newTestEngine(t)
	prop, _ := e.CreateProposal("Test", "desc", CatNetworkParam, "node-1", 500, "", "")

	err := e.MarkExecuted(prop.ID) // Still DRAFT
	if err == nil {
		t.Fatal("expected error for non-PASSED proposal")
	}
}

// ─── List + Stats Tests ────────────────────────────────────────────────────

func TestListProposals(t *testing.T) {
	e := newTestEngine(t)
	e.now = tickingClock() // avoid ID collision
	e.CreateProposal("First", "desc", CatNetworkParam, "node-1", 500, "", "")
	e.CreateProposal("Second", "desc", CatModelPolicy, "node-2", 500, "", "")

	all := e.ListProposals(nil)
	if len(all) != 2 {
		t.Errorf("total proposals = %d, want 2", len(all))
	}

	draft := PropDraft
	drafts := e.ListProposals(&draft)
	if len(drafts) != 2 {
		t.Errorf("drafts = %d, want 2", len(drafts))
	}
}

func TestStats(t *testing.T) {
	e := newTestEngine(t)
	e.now = tickingClock()
	e.CreateProposal("Test", "desc", CatNetworkParam, "node-1", 500, "", "")
	prop := createAndOpenProposal(t, e, "Active Test")
	e.CastVote(prop.ID, "node-voter", VoteFor, 100)

	stats := e.Stats()
	if stats.TotalProposals != 2 {
		t.Errorf("total = %d, want 2", stats.TotalProposals)
	}
	if stats.TotalVotesCast != 1 {
		t.Errorf("votes = %d, want 1", stats.TotalVotesCast)
	}
}

func TestProposalCount(t *testing.T) {
	e := newTestEngine(t)
	if e.ProposalCount() != 0 {
		t.Errorf("initial count = %d, want 0", e.ProposalCount())
	}
	e.CreateProposal("Test", "desc", CatNetworkParam, "node-1", 500, "", "")
	if e.ProposalCount() != 1 {
		t.Errorf("count = %d, want 1", e.ProposalCount())
	}
}

// ─── String Methods ─────────────────────────────────────────────────────────

func TestProposalStatusString(t *testing.T) {
	tests := []struct {
		status ProposalStatus
		want   string
	}{
		{PropDraft, "DRAFT"},
		{PropActive, "ACTIVE"},
		{PropPassed, "PASSED"},
		{PropRejected, "REJECTED"},
		{PropExpired, "EXPIRED"},
		{PropExecuted, "EXECUTED"},
		{PropCancelled, "CANCELLED"},
		{ProposalStatus(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("ProposalStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestProposalCategoryString(t *testing.T) {
	tests := []struct {
		cat  ProposalCategory
		want string
	}{
		{CatEarningRate, "EARNING_RATE"},
		{CatModelPolicy, "MODEL_POLICY"},
		{CatSLAPricing, "SLA_PRICING"},
		{CatNetworkParam, "NETWORK_PARAM"},
		{CatFederation, "FEDERATION"},
		{CatSecurity, "SECURITY"},
		{ProposalCategory(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.cat.String(); got != tt.want {
			t.Errorf("ProposalCategory(%d).String() = %q, want %q", tt.cat, got, tt.want)
		}
	}
}

// ─── Max Active Proposals ──────────────────────────────────────────────────

func TestCreateProposal_MaxActive(t *testing.T) {
	cfg := DefaultEngineConfig()
	e := NewEngine(cfg)
	e.SetTotalCredits(10000)
	e.now = tickingClock() // unique IDs per proposal

	// Fill up to max active proposals
	for i := 0; i < MaxActiveProposals; i++ {
		_, err := e.CreateProposal("Test", "desc", CatNetworkParam, "node-1", 500, "", "")
		if err != nil {
			t.Fatalf("proposal %d failed: %v", i, err)
		}
	}

	_, err := e.CreateProposal("One too many", "desc", CatNetworkParam, "node-1", 500, "", "")
	if err == nil {
		t.Fatal("expected error for exceeding max active proposals")
	}
}
