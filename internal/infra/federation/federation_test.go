package federation

import (
	"strings"
	"testing"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry(DefaultRegistryConfig())
}

// ─── Creation Tests ─────────────────────────────────────────────────────────

func TestCreateFederation(t *testing.T) {
	r := newTestRegistry(t)

	fed, err := r.CreateFederation("Acme Corp", "node-admin-1")
	if err != nil {
		t.Fatalf("CreateFederation failed: %v", err)
	}
	if fed.Name != "Acme Corp" {
		t.Errorf("name = %q, want %q", fed.Name, "Acme Corp")
	}
	if fed.AdminNodeID != "node-admin-1" {
		t.Errorf("admin = %q, want %q", fed.AdminNodeID, "node-admin-1")
	}
	if fed.Status != FedActive {
		t.Errorf("status = %v, want FedActive", fed.Status)
	}
	if fed.RevenueSharePct != DefaultRevenueSharePct {
		t.Errorf("revenue share = %d, want %d", fed.RevenueSharePct, DefaultRevenueSharePct)
	}
	if !strings.HasPrefix(fed.ID, "fed-") {
		t.Errorf("ID %q does not start with 'fed-'", fed.ID)
	}

	// Admin should be auto-added as a member
	members, err := r.Members(fed.ID)
	if err != nil {
		t.Fatalf("Members failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("member count = %d, want 1", len(members))
	}
	if members[0].Role != "admin" {
		t.Errorf("admin role = %q, want %q", members[0].Role, "admin")
	}
}

func TestCreateFederation_RequireApproval(t *testing.T) {
	cfg := DefaultRegistryConfig()
	cfg.RequireApproval = true
	r := NewRegistry(cfg)

	fed, err := r.CreateFederation("Pending Corp", "node-admin-2")
	if err != nil {
		t.Fatalf("CreateFederation failed: %v", err)
	}
	if fed.Status != FedPending {
		t.Errorf("status = %v, want FedPending", fed.Status)
	}
}

func TestCreateFederation_ShortName(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.CreateFederation("AB", "node-1")
	if err == nil {
		t.Fatal("expected error for short name")
	}
}

func TestCreateFederation_DuplicateAdmin(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.CreateFederation("First Corp", "node-1")
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}
	_, err = r.CreateFederation("Second Corp", "node-1")
	if err == nil {
		t.Fatal("expected error when admin already in a federation")
	}
}

func TestCreateFederation_DuplicateName(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.CreateFederation("Acme Corp", "node-1")
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}
	_, err = r.CreateFederation("Acme Corp", "node-2")
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestCreateFederation_MaxFederations(t *testing.T) {
	cfg := DefaultRegistryConfig()
	cfg.MaxFederations = 1
	r := NewRegistry(cfg)

	_, err := r.CreateFederation("First", "node-1")
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}
	_, err = r.CreateFederation("Second", "node-2")
	if err == nil {
		t.Fatal("expected error for exceeding max federations")
	}
}

// ─── Lifecycle Tests ────────────────────────────────────────────────────────

func TestApproveFederation(t *testing.T) {
	cfg := DefaultRegistryConfig()
	cfg.RequireApproval = true
	r := NewRegistry(cfg)

	fed, _ := r.CreateFederation("PendingCorp", "node-1")
	if fed.Status != FedPending {
		t.Fatalf("initial status = %v, want FedPending", fed.Status)
	}

	if err := r.ApproveFederation(fed.ID); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	got, _ := r.GetFederation(fed.ID)
	if got.Status != FedActive {
		t.Errorf("status after approve = %v, want FedActive", got.Status)
	}
}

func TestSuspendFederation(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-1")

	if err := r.SuspendFederation(fed.ID); err != nil {
		t.Fatalf("suspend failed: %v", err)
	}
	got, _ := r.GetFederation(fed.ID)
	if got.Status != FedSuspended {
		t.Errorf("status = %v, want FedSuspended", got.Status)
	}
}

func TestDissolveFederation(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("DoomCorp", "node-admin")
	_ = r.JoinFederation(fed.ID, "node-member")

	if err := r.DissolveFederation(fed.ID); err != nil {
		t.Fatalf("dissolve failed: %v", err)
	}

	got, _ := r.GetFederation(fed.ID)
	if got.Status != FedDissolved {
		t.Errorf("status = %v, want FedDissolved", got.Status)
	}

	// Members should be released
	_, found := r.NodeFederation("node-admin")
	if found {
		t.Error("admin still indexed after dissolve")
	}
	_, found = r.NodeFederation("node-member")
	if found {
		t.Error("member still indexed after dissolve")
	}
}

func TestGetFederation_NotFound(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.GetFederation("fed-nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent federation")
	}
}

func TestListFederations(t *testing.T) {
	r := newTestRegistry(t)
	r.CreateFederation("Alpha Corp", "node-1")
	r.CreateFederation("Beta Corp", "node-2")

	list := r.ListFederations()
	if len(list) != 2 {
		t.Errorf("list count = %d, want 2", len(list))
	}
}

// ─── Membership Tests ───────────────────────────────────────────────────────

func TestJoinFederation(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")

	if err := r.JoinFederation(fed.ID, "node-worker-1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	count := r.MemberCount(fed.ID)
	if count != 2 { // admin + worker
		t.Errorf("member count = %d, want 2", count)
	}

	fedID, found := r.NodeFederation("node-worker-1")
	if !found || fedID != fed.ID {
		t.Errorf("NodeFederation = (%q, %v), want (%q, true)", fedID, found, fed.ID)
	}
}

func TestJoinFederation_AlreadyInFederation(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")

	_ = r.JoinFederation(fed.ID, "node-worker")
	err := r.JoinFederation(fed.ID, "node-worker")
	if err == nil {
		t.Fatal("expected error for double join")
	}
}

func TestJoinFederation_NotActive(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")
	_ = r.SuspendFederation(fed.ID)

	err := r.JoinFederation(fed.ID, "node-new")
	if err == nil {
		t.Fatal("expected error joining suspended federation")
	}
}

func TestLeaveFederation(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")
	_ = r.JoinFederation(fed.ID, "node-worker")

	if err := r.LeaveFederation("node-worker"); err != nil {
		t.Fatalf("leave failed: %v", err)
	}

	count := r.MemberCount(fed.ID)
	if count != 1 { // just admin
		t.Errorf("member count = %d, want 1", count)
	}
}

func TestLeaveFederation_AdminCannot(t *testing.T) {
	r := newTestRegistry(t)
	r.CreateFederation("TestCorp", "node-admin")

	err := r.LeaveFederation("node-admin")
	if err == nil {
		t.Fatal("expected error when admin tries to leave")
	}
}

func TestLeaveFederation_NotInAny(t *testing.T) {
	r := newTestRegistry(t)
	err := r.LeaveFederation("node-stranger")
	if err == nil {
		t.Fatal("expected error when node not in any federation")
	}
}

// ─── Task Routing Tests ────────────────────────────────────────────────────

func TestShouldRouteInternal(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("SovCorp", "node-admin")

	// Admin is in a data-sovereign federation → internal routing
	if !r.ShouldRouteInternal("node-admin") {
		t.Error("expected internal routing for sovereign member")
	}

	// Unknown node → no internal routing
	if r.ShouldRouteInternal("node-unknown") {
		t.Error("expected no internal routing for unknown node")
	}

	// Non-sovereign federation
	r.federations[fed.ID].DataSovereignty = false
	if r.ShouldRouteInternal("node-admin") {
		t.Error("expected no internal routing when sovereignty is off")
	}
}

func TestCanShareCapacity(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")

	// Default policy is ShareSpare → can share
	if !r.CanShareCapacity("node-admin") {
		t.Error("expected capacity sharing with ShareSpare policy")
	}

	// ShareNothing → cannot share
	_ = r.SetSharingPolicy(fed.ID, ShareNothing)
	if r.CanShareCapacity("node-admin") {
		t.Error("expected no capacity sharing with ShareNothing policy")
	}

	// Unknown node → always can share (public)
	if !r.CanShareCapacity("node-unknown") {
		t.Error("expected capacity sharing for non-federated node")
	}
}

// ─── Revenue Share Tests ────────────────────────────────────────────────────

func TestRevenueShare(t *testing.T) {
	r := newTestRegistry(t)
	r.CreateFederation("TestCorp", "node-admin")

	org, platform := r.RevenueShare("node-admin", 1000)
	if org != 800 {
		t.Errorf("org share = %d, want 800", org)
	}
	if platform != 200 {
		t.Errorf("platform share = %d, want 200", platform)
	}

	// Non-federated node keeps everything
	org, platform = r.RevenueShare("node-solo", 500)
	if org != 500 || platform != 0 {
		t.Errorf("solo share = (%d, %d), want (500, 0)", org, platform)
	}
}

// ─── Policy Tests ───────────────────────────────────────────────────────────

func TestSetSharingPolicy(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")

	if err := r.SetSharingPolicy(fed.ID, ShareAll); err != nil {
		t.Fatalf("set policy failed: %v", err)
	}
	got, _ := r.GetFederation(fed.ID)
	if got.SharingPolicy != ShareAll {
		t.Errorf("policy = %v, want ShareAll", got.SharingPolicy)
	}
}

func TestSetAllowedRegions(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")

	regions := []string{"us-east-1", "eu-west-1"}
	if err := r.SetAllowedRegions(fed.ID, regions); err != nil {
		t.Fatalf("set regions failed: %v", err)
	}
	got, _ := r.GetFederation(fed.ID)
	if len(got.AllowedRegions) != 2 {
		t.Errorf("regions count = %d, want 2", len(got.AllowedRegions))
	}
}

// ─── Stats + ActiveCount Tests ─────────────────────────────────────────────

func TestStats(t *testing.T) {
	r := newTestRegistry(t)
	fed, _ := r.CreateFederation("TestCorp", "node-admin")
	_ = r.JoinFederation(fed.ID, "node-worker")

	stats := r.Stats(fed.ID)
	if stats.MemberCount != 2 {
		t.Errorf("member count = %d, want 2", stats.MemberCount)
	}
}

func TestActiveCount(t *testing.T) {
	r := newTestRegistry(t)
	r.CreateFederation("Alpha", "node-1")
	r.CreateFederation("Beta", "node-2")

	if got := r.ActiveCount(); got != 2 {
		t.Errorf("active count = %d, want 2", got)
	}
}

// ─── String Methods ─────────────────────────────────────────────────────────

func TestFederationStatusString(t *testing.T) {
	tests := []struct {
		status FederationStatus
		want   string
	}{
		{FedPending, "PENDING"},
		{FedActive, "ACTIVE"},
		{FedSuspended, "SUSPENDED"},
		{FedDissolved, "DISSOLVED"},
		{FederationStatus(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("FederationStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestSharingPolicyString(t *testing.T) {
	tests := []struct {
		policy SharingPolicy
		want   string
	}{
		{ShareNothing, "NONE"},
		{ShareSpare, "SPARE"},
		{ShareAll, "ALL"},
		{SharingPolicy(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.policy.String(); got != tt.want {
			t.Errorf("SharingPolicy(%d).String() = %q, want %q", tt.policy, got, tt.want)
		}
	}
}
