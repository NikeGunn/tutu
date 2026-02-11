// Package federation implements federated sub-networks for organizations.
//
// Federation lets organizations deploy private TuTu clusters where tasks
// stay within org boundaries (data sovereignty). Spare capacity is shared
// with the public network, earning the org 80% of task revenue.
//
// Architecture Part IX — Hierarchical gossip → region → zone → cluster → node.
// Phase 5 spec: "Organizations deploy private TuTu clusters."
package federation

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ─── Constants ──────────────────────────────────────────────────────────────

const (
	// DefaultRevenueSharePct is the org's cut of task revenue (80%).
	DefaultRevenueSharePct = 80

	// PlatformFeePct is the platform fee on federated tasks.
	PlatformFeePct = 20

	// MaxNodesPerFederation prevents unbounded growth.
	MaxNodesPerFederation = 10000

	// MinNameLength for federation names to avoid typos.
	MinNameLength = 3
)

// ─── Types ──────────────────────────────────────────────────────────────────

// FederationStatus represents the lifecycle state of a federation.
type FederationStatus int

const (
	FedPending  FederationStatus = iota // Awaiting approval
	FedActive                           // Fully operational
	FedSuspended                        // Temporarily suspended
	FedDissolved                        // Permanently removed
)

// String returns a human-readable status label.
func (s FederationStatus) String() string {
	switch s {
	case FedPending:
		return "PENDING"
	case FedActive:
		return "ACTIVE"
	case FedSuspended:
		return "SUSPENDED"
	case FedDissolved:
		return "DISSOLVED"
	default:
		return "UNKNOWN"
	}
}

// SharingPolicy controls how a federation shares spare capacity.
type SharingPolicy int

const (
	ShareNothing SharingPolicy = iota // No capacity shared
	ShareSpare                        // Only idle capacity
	ShareAll                          // Full capacity shared
)

// String returns the policy name.
func (p SharingPolicy) String() string {
	switch p {
	case ShareNothing:
		return "NONE"
	case ShareSpare:
		return "SPARE"
	case ShareAll:
		return "ALL"
	default:
		return "UNKNOWN"
	}
}

// Federation represents a private sub-network owned by an organization.
type Federation struct {
	ID              string           `json:"id"`               // Unique federation ID (e.g. "fed-acme-corp")
	Name            string           `json:"name"`             // Human-readable name
	AdminNodeID     string           `json:"admin_node_id"`    // Node that created this federation
	Status          FederationStatus `json:"status"`           // Lifecycle state
	SharingPolicy   SharingPolicy    `json:"sharing_policy"`   // How spare capacity is shared
	RevenueSharePct int              `json:"revenue_share_pct"`// Org revenue share (default 80%)
	DataSovereignty bool             `json:"data_sovereignty"` // Tasks must stay within federation
	AllowedRegions  []string         `json:"allowed_regions"`  // Restrict to specific regions
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// FederationMember represents a node's membership in a federation.
type FederationMember struct {
	NodeID     string    `json:"node_id"`
	FedID      string    `json:"fed_id"`
	Role       string    `json:"role"`       // "admin", "member", "observer"
	JoinedAt   time.Time `json:"joined_at"`
	LastActive time.Time `json:"last_active"`
}

// FederationStats aggregates metrics for a federation.
type FederationStats struct {
	FedID             string  `json:"fed_id"`
	MemberCount       int     `json:"member_count"`
	ActiveMembers     int     `json:"active_members"`
	TotalCreditsEarned int64  `json:"total_credits_earned"`
	TasksCompleted    int64   `json:"tasks_completed"`
	SharedCapacityPct float64 `json:"shared_capacity_pct"` // % of capacity shared to public
}

// ─── Configuration ──────────────────────────────────────────────────────────

// RegistryConfig configures the federation registry.
type RegistryConfig struct {
	MaxFederations    int // Maximum number of active federations (0 = unlimited)
	RequireApproval   bool // New federations need admin approval
	DefaultSharePolicy SharingPolicy
}

// DefaultRegistryConfig returns sensible defaults.
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		MaxFederations:    1000,
		RequireApproval:   false,
		DefaultSharePolicy: ShareSpare,
	}
}

// ─── Registry ───────────────────────────────────────────────────────────────

// Registry manages all federations in the network.
// Thread-safe: concurrent reads and writes are serialized by mutex.
type Registry struct {
	mu          sync.RWMutex
	config      RegistryConfig
	federations map[string]*Federation           // fedID → Federation
	members     map[string]map[string]*FederationMember // fedID → nodeID → Member
	nodeIndex   map[string]string                // nodeID → fedID (quick lookup)
}

// NewRegistry creates a federation registry.
func NewRegistry(cfg RegistryConfig) *Registry {
	return &Registry{
		config:      cfg,
		federations: make(map[string]*Federation),
		members:     make(map[string]map[string]*FederationMember),
		nodeIndex:   make(map[string]string),
	}
}

// ─── Federation Lifecycle ───────────────────────────────────────────────────

// CreateFederation registers a new private sub-network.
// The creating node becomes the admin automatically.
func (r *Registry) CreateFederation(name, adminNodeID string) (*Federation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.TrimSpace(name)
	if len(name) < MinNameLength {
		return nil, fmt.Errorf("federation name must be at least %d characters", MinNameLength)
	}

	// Check if admin is already in a federation
	if existingFed, ok := r.nodeIndex[adminNodeID]; ok {
		return nil, fmt.Errorf("node %s already belongs to federation %s", adminNodeID, existingFed)
	}

	// Check max federations
	activeCount := 0
	for _, fed := range r.federations {
		if fed.Status == FedActive || fed.Status == FedPending {
			activeCount++
		}
	}
	if r.config.MaxFederations > 0 && activeCount >= r.config.MaxFederations {
		return nil, errors.New("maximum number of federations reached")
	}

	// Check name uniqueness
	for _, fed := range r.federations {
		if strings.EqualFold(fed.Name, name) && fed.Status != FedDissolved {
			return nil, fmt.Errorf("federation name %q already exists", name)
		}
	}

	now := time.Now()
	fedID := fmt.Sprintf("fed-%s-%d", sanitizeName(name), now.UnixMilli()%100000)

	status := FedActive
	if r.config.RequireApproval {
		status = FedPending
	}

	fed := &Federation{
		ID:              fedID,
		Name:            name,
		AdminNodeID:     adminNodeID,
		Status:          status,
		SharingPolicy:   r.config.DefaultSharePolicy,
		RevenueSharePct: DefaultRevenueSharePct,
		DataSovereignty: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	r.federations[fedID] = fed

	// Auto-add admin as first member
	r.members[fedID] = map[string]*FederationMember{
		adminNodeID: {
			NodeID:     adminNodeID,
			FedID:      fedID,
			Role:       "admin",
			JoinedAt:   now,
			LastActive: now,
		},
	}
	r.nodeIndex[adminNodeID] = fedID

	return fed, nil
}

// GetFederation returns a federation by ID.
func (r *Registry) GetFederation(fedID string) (*Federation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return nil, fmt.Errorf("federation %s not found", fedID)
	}
	return fed, nil
}

// ListFederations returns all non-dissolved federations.
func (r *Registry) ListFederations() []*Federation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Federation, 0, len(r.federations))
	for _, fed := range r.federations {
		if fed.Status != FedDissolved {
			result = append(result, fed)
		}
	}
	return result
}

// ApproveFederation activates a pending federation.
func (r *Registry) ApproveFederation(fedID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return fmt.Errorf("federation %s not found", fedID)
	}
	if fed.Status != FedPending {
		return fmt.Errorf("federation %s is %s, not PENDING", fedID, fed.Status)
	}

	fed.Status = FedActive
	fed.UpdatedAt = time.Now()
	return nil
}

// SuspendFederation temporarily disables a federation.
func (r *Registry) SuspendFederation(fedID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return fmt.Errorf("federation %s not found", fedID)
	}
	if fed.Status == FedDissolved {
		return errors.New("cannot suspend a dissolved federation")
	}

	fed.Status = FedSuspended
	fed.UpdatedAt = time.Now()
	return nil
}

// DissolveFederation permanently removes a federation. All members are released.
func (r *Registry) DissolveFederation(fedID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return fmt.Errorf("federation %s not found", fedID)
	}

	// Release all members from node index
	for nodeID := range r.members[fedID] {
		delete(r.nodeIndex, nodeID)
	}
	delete(r.members, fedID)

	fed.Status = FedDissolved
	fed.UpdatedAt = time.Now()
	return nil
}

// SetSharingPolicy updates how a federation shares capacity.
func (r *Registry) SetSharingPolicy(fedID string, policy SharingPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return fmt.Errorf("federation %s not found", fedID)
	}
	if fed.Status != FedActive {
		return fmt.Errorf("federation %s is not active", fedID)
	}

	fed.SharingPolicy = policy
	fed.UpdatedAt = time.Now()
	return nil
}

// SetAllowedRegions restricts a federation to specific regions.
func (r *Registry) SetAllowedRegions(fedID string, regions []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return fmt.Errorf("federation %s not found", fedID)
	}

	fed.AllowedRegions = regions
	fed.UpdatedAt = time.Now()
	return nil
}

// ─── Membership Management ─────────────────────────────────────────────────

// JoinFederation adds a node to a federation.
func (r *Registry) JoinFederation(fedID, nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fed, ok := r.federations[fedID]
	if !ok {
		return fmt.Errorf("federation %s not found", fedID)
	}
	if fed.Status != FedActive {
		return fmt.Errorf("federation %s is not active (status: %s)", fedID, fed.Status)
	}

	// Check if node is already in any federation
	if existing, exists := r.nodeIndex[nodeID]; exists {
		return fmt.Errorf("node %s already belongs to federation %s", nodeID, existing)
	}

	members := r.members[fedID]
	if len(members) >= MaxNodesPerFederation {
		return fmt.Errorf("federation %s reached max node limit (%d)", fedID, MaxNodesPerFederation)
	}

	now := time.Now()
	members[nodeID] = &FederationMember{
		NodeID:     nodeID,
		FedID:      fedID,
		Role:       "member",
		JoinedAt:   now,
		LastActive: now,
	}
	r.nodeIndex[nodeID] = fedID
	fed.UpdatedAt = now
	return nil
}

// LeaveFederation removes a node from its federation.
// Admin cannot leave — must dissolve the federation instead.
func (r *Registry) LeaveFederation(nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fedID, ok := r.nodeIndex[nodeID]
	if !ok {
		return fmt.Errorf("node %s is not in any federation", nodeID)
	}

	fed := r.federations[fedID]
	if fed.AdminNodeID == nodeID {
		return errors.New("admin cannot leave — dissolve the federation instead")
	}

	delete(r.members[fedID], nodeID)
	delete(r.nodeIndex, nodeID)
	fed.UpdatedAt = time.Now()
	return nil
}

// Members returns all members of a federation.
func (r *Registry) Members(fedID string) ([]*FederationMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	members, ok := r.members[fedID]
	if !ok {
		return nil, fmt.Errorf("federation %s not found", fedID)
	}

	result := make([]*FederationMember, 0, len(members))
	for _, m := range members {
		result = append(result, m)
	}
	return result, nil
}

// MemberCount returns the number of members in a federation.
func (r *Registry) MemberCount(fedID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members[fedID])
}

// NodeFederation returns which federation a node belongs to, if any.
func (r *Registry) NodeFederation(nodeID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fedID, ok := r.nodeIndex[nodeID]
	return fedID, ok
}

// ─── Task Routing ───────────────────────────────────────────────────────────

// ShouldRouteInternal checks if a task from a federated node must stay
// within the federation boundary (data sovereignty).
func (r *Registry) ShouldRouteInternal(nodeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fedID, ok := r.nodeIndex[nodeID]
	if !ok {
		return false // Not in a federation → public routing
	}

	fed := r.federations[fedID]
	return fed.DataSovereignty && fed.Status == FedActive
}

// CanShareCapacity checks if a federated node is allowed to serve
// public network tasks based on the federation's sharing policy.
func (r *Registry) CanShareCapacity(nodeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fedID, ok := r.nodeIndex[nodeID]
	if !ok {
		return true // Not in a federation → always available for public tasks
	}

	fed := r.federations[fedID]
	if fed.Status != FedActive {
		return false
	}
	return fed.SharingPolicy != ShareNothing
}

// RevenueShare calculates the org and platform split for a task.
// Returns (orgCredits, platformCredits).
func (r *Registry) RevenueShare(nodeID string, totalCredits int64) (int64, int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fedID, ok := r.nodeIndex[nodeID]
	if !ok {
		return totalCredits, 0 // Not federated → node keeps everything
	}

	fed := r.federations[fedID]
	orgShare := totalCredits * int64(fed.RevenueSharePct) / 100
	platformShare := totalCredits - orgShare
	return orgShare, platformShare
}

// Stats returns aggregate metrics for a federation.
func (r *Registry) Stats(fedID string) FederationStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := FederationStats{FedID: fedID}
	members := r.members[fedID]
	stats.MemberCount = len(members)

	// Count active members (seen in last 10 minutes)
	cutoff := time.Now().Add(-10 * time.Minute)
	for _, m := range members {
		if m.LastActive.After(cutoff) {
			stats.ActiveMembers++
		}
	}

	fed := r.federations[fedID]
	if fed != nil {
		switch fed.SharingPolicy {
		case ShareNothing:
			stats.SharedCapacityPct = 0
		case ShareSpare:
			stats.SharedCapacityPct = 30 // Estimate: ~30% idle capacity shared
		case ShareAll:
			stats.SharedCapacityPct = 100
		}
	}

	return stats
}

// ActiveCount returns the number of active federations.
func (r *Registry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, fed := range r.federations {
		if fed.Status == FedActive {
			count++
		}
	}
	return count
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// sanitizeName converts a name to a URL-safe slug.
func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")

	// Keep only alphanumeric and hyphens
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	result := b.String()
	if len(result) > 32 {
		result = result[:32]
	}
	return result
}
