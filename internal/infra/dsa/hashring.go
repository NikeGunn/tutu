// Package dsa implements critical data structures for planet-scale operations.
// Architecture Part XX: Algorithms & Data Structures — Deep Analysis.
//
// This package provides three structures:
//  1. ConsistentHashRing — O(K/n) rebalancing for model placement
//  2. BloomFilter        — O(1) probabilistic lookup for peer model inventory
//  3. PriorityQueue      — O(log n) min-heap for task scheduling
package dsa

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// ─── Consistent Hash Ring ───────────────────────────────────────────────────
// Maps model names → node IDs with minimal movement on node join/leave.
// Each physical node gets VirtualNodes virtual positions on the ring.
// Lookup: O(log n) via binary search on sorted ring.
// Rebalance on join/leave: only O(K/n) keys move (K=total keys, n=nodes).

// HashRingConfig configures the consistent hash ring.
type HashRingConfig struct {
	VirtualNodes int // Number of virtual nodes per physical node (default 150)
}

// DefaultHashRingConfig returns production defaults.
// 150 virtual nodes gives < 5% standard deviation in load distribution.
func DefaultHashRingConfig() HashRingConfig {
	return HashRingConfig{VirtualNodes: 150}
}

// HashRing implements a consistent hash ring with virtual nodes.
type HashRing struct {
	mu           sync.RWMutex
	config       HashRingConfig
	ring         []ringPoint     // sorted by hash
	nodeMap      map[string]bool // physical node set
	virtualNodes int
}

// ringPoint is a single point on the hash ring.
type ringPoint struct {
	hash uint32
	node string // physical node ID
}

// NewHashRing creates an empty consistent hash ring.
func NewHashRing(cfg HashRingConfig) *HashRing {
	if cfg.VirtualNodes <= 0 {
		cfg.VirtualNodes = 150
	}
	return &HashRing{
		config:       cfg,
		nodeMap:      make(map[string]bool),
		virtualNodes: cfg.VirtualNodes,
	}
}

// AddNode inserts a physical node with its virtual replicas onto the ring.
// After adding, only ~K/n keys need to move (K = total keys, n = total nodes).
func (h *HashRing) AddNode(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.nodeMap[nodeID] {
		return // already present
	}
	h.nodeMap[nodeID] = true

	for i := 0; i < h.virtualNodes; i++ {
		hash := hashKey(fmt.Sprintf("%s#%d", nodeID, i))
		h.ring = append(h.ring, ringPoint{hash: hash, node: nodeID})
	}
	sort.Slice(h.ring, func(i, j int) bool {
		return h.ring[i].hash < h.ring[j].hash
	})
}

// RemoveNode removes a physical node and all its virtual replicas.
func (h *HashRing) RemoveNode(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.nodeMap[nodeID] {
		return
	}
	delete(h.nodeMap, nodeID)

	filtered := h.ring[:0]
	for _, p := range h.ring {
		if p.node != nodeID {
			filtered = append(filtered, p)
		}
	}
	h.ring = filtered
}

// Lookup finds the node responsible for the given key.
// Uses binary search on the sorted ring — O(log n).
func (h *HashRing) Lookup(key string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.ring) == 0 {
		return ""
	}

	hash := hashKey(key)
	idx := sort.Search(len(h.ring), func(i int) bool {
		return h.ring[i].hash >= hash
	})
	// Wrap around if past the end
	if idx >= len(h.ring) {
		idx = 0
	}
	return h.ring[idx].node
}

// LookupN finds the N distinct physical nodes responsible for a key.
// Used for replication: place model on N nodes for redundancy.
func (h *HashRing) LookupN(key string, n int) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.ring) == 0 || n <= 0 {
		return nil
	}

	hash := hashKey(key)
	idx := sort.Search(len(h.ring), func(i int) bool {
		return h.ring[i].hash >= hash
	})
	if idx >= len(h.ring) {
		idx = 0
	}

	seen := make(map[string]bool)
	var result []string
	for i := 0; i < len(h.ring) && len(result) < n; i++ {
		pos := (idx + i) % len(h.ring)
		node := h.ring[pos].node
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
	}
	return result
}

// Nodes returns all physical nodes on the ring.
func (h *HashRing) Nodes() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nodes := make([]string, 0, len(h.nodeMap))
	for id := range h.nodeMap {
		nodes = append(nodes, id)
	}
	sort.Strings(nodes)
	return nodes
}

// Size returns the number of physical nodes.
func (h *HashRing) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodeMap)
}

// hashKey produces a 32-bit hash of a key using SHA-256 truncation.
// SHA-256 gives excellent distribution; we take the first 4 bytes.
func hashKey(key string) uint32 {
	h := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(h[:4])
}
