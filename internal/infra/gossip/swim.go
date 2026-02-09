// Package gossip implements the SWIM membership protocol.
// Architecture Part IX: O(log N) convergence via piggybacked state dissemination.
//
// SWIM cycle (every 1s):
//  1. Pick random member → PING
//  2. No ACK within 500ms → PING-REQ to k=3 random members
//  3. No indirect ACK → mark SUSPECT
//  4. After suspectTTL (5s) → mark DEAD
//  5. State changes piggybacked on PING/ACK messages
package gossip

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/tutu-network/tutu/internal/domain"
	"github.com/tutu-network/tutu/internal/security"
)

// Config controls the SWIM protocol parameters.
type Config struct {
	BindAddr    string        // UDP listen address (e.g. ":7946")
	PingTimeout time.Duration // ACK timeout (default: 500ms)
	Interval    time.Duration // Probe cycle (default: 1s)
	SuspectTTL  time.Duration // Time before SUSPECT → DEAD (default: 5s)
	K           int           // Indirect ping targets (default: 3)
	Lambda      int           // Piggyback retransmission factor (default: 3)
}

// DefaultConfig returns conservative SWIM defaults.
func DefaultConfig() Config {
	return Config{
		BindAddr:    ":7946",
		PingTimeout: 500 * time.Millisecond,
		Interval:    1 * time.Second,
		SuspectTTL:  5 * time.Second,
		K:           3,
		Lambda:      3,
	}
}

// MessageType identifies SWIM protocol messages.
type MessageType uint8

const (
	MsgPing    MessageType = 1
	MsgAck     MessageType = 2
	MsgPingReq MessageType = 3
	MsgState   MessageType = 4 // Piggybacked state update
)

// Message is a SWIM protocol message sent over UDP.
type Message struct {
	Type      MessageType    `json:"type"`
	SeqNo     uint64         `json:"seq"`
	From      string         `json:"from"`
	Target    string         `json:"target,omitempty"`
	State     []StateUpdate  `json:"state,omitempty"` // Piggybacked
	Signature []byte         `json:"sig,omitempty"`
}

// StateUpdate is a piggybacked membership state change.
type StateUpdate struct {
	NodeID     string           `json:"node_id"`
	State      domain.PeerState `json:"state"`
	Incarnation uint64          `json:"incarnation"`
}

// member tracks internal membership state.
type member struct {
	nodeID      string
	addr        *net.UDPAddr
	state       domain.PeerState
	incarnation uint64
	suspectAt   time.Time // When node was marked SUSPECT
	lastAck     time.Time
}

// SWIM implements the SWIM membership protocol over UDP.
type SWIM struct {
	mu        sync.RWMutex
	config    Config
	selfID    string
	selfAddr  *net.UDPAddr
	conn      *net.UDPConn
	members   map[string]*member
	seqNo     uint64
	keypair   *security.Keypair
	broadcast []StateUpdate // Pending piggybacked state changes
	bcastLeft map[string]int  // nodeID → remaining retransmissions

	// Callbacks
	onJoin  func(nodeID string)
	onLeave func(nodeID string)

	// Pending acks
	pendingMu sync.Mutex
	pending   map[uint64]chan bool // seqNo → ack channel
}

// New creates a new SWIM protocol instance.
func New(selfID string, cfg Config, kp *security.Keypair) *SWIM {
	return &SWIM{
		config:    cfg,
		selfID:    selfID,
		keypair:   kp,
		members:   make(map[string]*member),
		pending:   make(map[uint64]chan bool),
		bcastLeft: make(map[string]int),
	}
}

// OnJoin sets a callback for when a new member is discovered.
func (s *SWIM) OnJoin(fn func(nodeID string)) { s.onJoin = fn }

// OnLeave sets a callback for when a member is declared dead.
func (s *SWIM) OnLeave(fn func(nodeID string)) { s.onLeave = fn }

// Members returns the current membership list (excludes seed entries).
func (s *SWIM) Members() []domain.Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]domain.Peer, 0, len(s.members))
	for id, m := range s.members {
		// Skip temporary seed entries that haven't been resolved yet
		if len(id) >= 5 && id[:5] == "seed:" {
			continue
		}
		peers = append(peers, domain.Peer{
			NodeID:   m.nodeID,
			Endpoint: m.addr.String(),
			State:    m.state,
			LastSeen: m.lastAck,
		})
	}
	return peers
}

// AliveCount returns the number of alive members (excludes seed entries).
func (s *SWIM) AliveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for id, m := range s.members {
		if m.state == domain.PeerAlive && (len(id) < 5 || id[:5] != "seed:") {
			count++
		}
	}
	return count
}

// Join seeds the membership with known peers.
func (s *SWIM) Join(addrs []string) error {
	for _, a := range addrs {
		addr, err := net.ResolveUDPAddr("udp4", a)
		if err != nil {
			return fmt.Errorf("resolve seed %s: %w", a, err)
		}
		s.mu.Lock()
		// Use addr as temporary ID until they respond
		tempID := "seed:" + a
		s.members[tempID] = &member{
			nodeID: tempID,
			addr:   addr,
			state:  domain.PeerAlive,
		}
		s.mu.Unlock()

		// Send a ping to discover their real ID
		s.sendPing(addr, tempID)
	}
	return nil
}

// Start begins the SWIM protocol. Blocks until ctx is cancelled.
func (s *SWIM) Start(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp4", s.config.BindAddr)
	if err != nil {
		return fmt.Errorf("resolve bind addr: %w", err)
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	s.conn = conn
	s.selfAddr = conn.LocalAddr().(*net.UDPAddr)

	// Receiver goroutine
	go s.receiveLoop(ctx)

	// Probe cycle
	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.conn.Close()
			return nil
		case <-ticker.C:
			s.probeCycle()
			s.reapSuspects()
		}
	}
}

// probeCycle picks a random member and probes it.
func (s *SWIM) probeCycle() {
	target := s.randomMember()
	if target == nil {
		return
	}

	s.mu.Lock()
	s.seqNo++
	seq := s.seqNo
	s.mu.Unlock()

	ackCh := make(chan bool, 1)
	s.pendingMu.Lock()
	s.pending[seq] = ackCh
	s.pendingMu.Unlock()

	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, seq)
		s.pendingMu.Unlock()
	}()

	// Phase 1: Direct PING
	s.sendMessage(target.addr, Message{
		Type:  MsgPing,
		SeqNo: seq,
		From:  s.selfID,
		State: s.drainBroadcast(),
	})

	timer := time.NewTimer(s.config.PingTimeout)
	defer timer.Stop()

	select {
	case <-ackCh:
		// Direct ACK received
		return
	case <-timer.C:
		// No response — Phase 2: Indirect PING-REQ
	}

	// Send PING-REQ to k random members
	indirects := s.randomMembers(s.config.K, target.nodeID)
	for _, m := range indirects {
		s.sendMessage(m.addr, Message{
			Type:   MsgPingReq,
			SeqNo:  seq,
			From:   s.selfID,
			Target: target.nodeID,
		})
	}

	// Wait again for indirect ACK
	timer2 := time.NewTimer(s.config.PingTimeout)
	defer timer2.Stop()

	select {
	case <-ackCh:
		return
	case <-timer2.C:
		// No indirect ACK — mark SUSPECT
		s.markSuspect(target.nodeID)
	}
}

// reapSuspects promotes long-running SUSPECT nodes to DEAD.
func (s *SWIM) reapSuspects() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, m := range s.members {
		if m.state == domain.PeerSuspect && !m.suspectAt.IsZero() {
			if now.Sub(m.suspectAt) > s.config.SuspectTTL {
				m.state = domain.PeerDead
				s.queueBroadcast(StateUpdate{
					NodeID: id,
					State:  domain.PeerDead,
				})
				if s.onLeave != nil {
					go s.onLeave(id)
				}
			}
		}
	}
}

// receiveLoop reads UDP packets and dispatches them.
func (s *SWIM) receiveLoop(ctx context.Context) {
	buf := make([]byte, 65536)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		var msg Message
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}

		s.handleMessage(msg, remoteAddr)
	}
}

// handleMessage processes a received SWIM message.
func (s *SWIM) handleMessage(msg Message, from *net.UDPAddr) {
	// Process piggybacked state updates
	for _, su := range msg.State {
		s.applyStateUpdate(su)
	}

	switch msg.Type {
	case MsgPing:
		s.handlePing(msg, from)
	case MsgAck:
		s.handleAck(msg, from)
	case MsgPingReq:
		s.handlePingReq(msg, from)
	}
}

func (s *SWIM) handlePing(msg Message, from *net.UDPAddr) {
	// Update or add the sender as alive
	s.mu.Lock()
	if m, ok := s.members[msg.From]; ok {
		m.state = domain.PeerAlive
		m.lastAck = time.Now()
		m.addr = from
	} else {
		// Remove any seed entries matching this addr
		for id, m := range s.members {
			if m.addr.String() == from.String() && id != msg.From {
				delete(s.members, id)
			}
		}
		s.members[msg.From] = &member{
			nodeID:  msg.From,
			addr:    from,
			state:   domain.PeerAlive,
			lastAck: time.Now(),
		}
		if s.onJoin != nil {
			go s.onJoin(msg.From)
		}
	}
	s.mu.Unlock()

	// Reply with ACK
	s.sendMessage(from, Message{
		Type:  MsgAck,
		SeqNo: msg.SeqNo,
		From:  s.selfID,
		State: s.drainBroadcast(),
	})
}

func (s *SWIM) handleAck(msg Message, from *net.UDPAddr) {
	// Update sender as alive — may need to upgrade from seed entry
	s.mu.Lock()
	if m, ok := s.members[msg.From]; ok {
		m.state = domain.PeerAlive
		m.lastAck = time.Now()
		m.suspectAt = time.Time{}
		m.addr = from
	} else {
		// Sender not in our membership — check for seed entries matching this address
		// and upgrade them to the real ID
		for id, m := range s.members {
			if m.addr != nil && m.addr.String() == from.String() && id != msg.From {
				delete(s.members, id)
			}
		}
		s.members[msg.From] = &member{
			nodeID:  msg.From,
			addr:    from,
			state:   domain.PeerAlive,
			lastAck: time.Now(),
		}
		if s.onJoin != nil {
			go s.onJoin(msg.From)
		}
	}
	s.mu.Unlock()

	// Signal waiting probe
	s.pendingMu.Lock()
	if ch, ok := s.pending[msg.SeqNo]; ok {
		select {
		case ch <- true:
		default:
		}
	}
	s.pendingMu.Unlock()
}

func (s *SWIM) handlePingReq(msg Message, from *net.UDPAddr) {
	// Ping the target on behalf of the requester
	s.mu.RLock()
	target, ok := s.members[msg.Target]
	s.mu.RUnlock()

	if !ok {
		return
	}

	// Forward a PING to the target with the original sequence number
	s.sendMessage(target.addr, Message{
		Type:  MsgPing,
		SeqNo: msg.SeqNo,
		From:  s.selfID,
	})
}

// markSuspect transitions a member to SUSPECT state.
func (s *SWIM) markSuspect(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.members[nodeID]
	if !ok {
		return
	}
	if m.state == domain.PeerAlive {
		m.state = domain.PeerSuspect
		m.suspectAt = time.Now()
		s.queueBroadcast(StateUpdate{
			NodeID: nodeID,
			State:  domain.PeerSuspect,
		})
	}
}

// applyStateUpdate processes a piggybacked state change.
func (s *SWIM) applyStateUpdate(su StateUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.members[su.NodeID]
	if !ok {
		return
	}

	// Only apply if incarnation is newer (or same incarnation + worse state)
	if su.Incarnation < m.incarnation {
		return
	}

	switch su.State {
	case domain.PeerSuspect:
		if m.state == domain.PeerAlive {
			m.state = domain.PeerSuspect
			m.suspectAt = time.Now()
			m.incarnation = su.Incarnation
		}
	case domain.PeerDead:
		m.state = domain.PeerDead
		m.incarnation = su.Incarnation
		if s.onLeave != nil {
			go s.onLeave(su.NodeID)
		}
	case domain.PeerAlive:
		if su.Incarnation > m.incarnation {
			m.state = domain.PeerAlive
			m.incarnation = su.Incarnation
			m.suspectAt = time.Time{}
		}
	}
}

// queueBroadcast adds a state update to the piggyback queue.
// Must be called with s.mu held.
func (s *SWIM) queueBroadcast(su StateUpdate) {
	s.broadcast = append(s.broadcast, su)
	s.bcastLeft[su.NodeID] = s.config.Lambda * s.logN()
}

// drainBroadcast returns pending state updates for piggybacking.
func (s *SWIM) drainBroadcast() []StateUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.broadcast) == 0 {
		return nil
	}

	result := make([]StateUpdate, 0, len(s.broadcast))
	remaining := make([]StateUpdate, 0)

	for _, su := range s.broadcast {
		result = append(result, su)
		s.bcastLeft[su.NodeID]--
		if s.bcastLeft[su.NodeID] > 0 {
			remaining = append(remaining, su)
		} else {
			delete(s.bcastLeft, su.NodeID)
		}
	}

	s.broadcast = remaining
	return result
}

// logN returns ceil(log2(N+1)) for dissemination factor.
func (s *SWIM) logN() int {
	n := len(s.members) + 1
	l := 1
	for 1<<l < n {
		l++
	}
	return l
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func (s *SWIM) sendPing(addr *net.UDPAddr, target string) {
	s.mu.Lock()
	s.seqNo++
	seq := s.seqNo
	s.mu.Unlock()

	s.sendMessage(addr, Message{
		Type:  MsgPing,
		SeqNo: seq,
		From:  s.selfID,
	})
}

func (s *SWIM) sendMessage(addr *net.UDPAddr, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	// Sign the message if we have a keypair
	if s.keypair != nil {
		msg.Signature = s.keypair.Sign(data)
	}

	data, _ = json.Marshal(msg) // Re-marshal with signature
	s.conn.WriteToUDP(data, addr)
}

func (s *SWIM) randomMember() *member {
	s.mu.RLock()
	defer s.mu.RUnlock()

	alive := make([]*member, 0)
	for _, m := range s.members {
		if m.state != domain.PeerDead {
			alive = append(alive, m)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	return alive[rand.Intn(len(alive))]
}

func (s *SWIM) randomMembers(k int, exclude string) []*member {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]*member, 0)
	for _, m := range s.members {
		if m.nodeID != exclude && m.state != domain.PeerDead {
			candidates = append(candidates, m)
		}
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	if k > len(candidates) {
		k = len(candidates)
	}
	return candidates[:k]
}
