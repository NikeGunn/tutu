package dsa

import (
	"sync"
	"time"
)

// ─── Priority Queue (Min-Heap) ──────────────────────────────────────────────
// Architecture Part XX §1: Binary min-heap for task scheduling.
//
// Operations:
//   Push:    O(log n) — sift up
//   Pop:     O(log n) — sift down (extract-min)
//   Peek:    O(1)
//   Len:     O(1)
//
// Starvation prevention (Architecture Part XX):
//   Every item has a base priority and a submission time.
//   effective_priority = base_priority - age_boost
//   After BoostInterval, low-priority tasks get boosted by 1 level.
//   This prevents P4 tasks from starving indefinitely.

// HeapItem is an element in the priority queue.
type HeapItem struct {
	Key         string    // Unique identifier (e.g. task ID)
	Priority    int       // Base priority (lower = higher priority, 0 = realtime)
	SubmittedAt time.Time // Used for starvation prevention
	Value       any       // Payload (caller stores whatever they need)
}

// PriorityQueueConfig configures starvation prevention.
type PriorityQueueConfig struct {
	BoostInterval time.Duration // Time before priority is boosted by 1 level
	MaxBoost      int           // Maximum levels a task can be boosted
}

// DefaultPriorityQueueConfig returns defaults from Architecture Part XX.
// After 5 minutes, P4 tasks start getting boosted toward P3.
func DefaultPriorityQueueConfig() PriorityQueueConfig {
	return PriorityQueueConfig{
		BoostInterval: 5 * time.Minute,
		MaxBoost:      2, // P4 can boost to P2 at most
	}
}

// PriorityQueue is a thread-safe min-heap with starvation prevention.
type PriorityQueue struct {
	mu     sync.Mutex
	heap   []HeapItem
	config PriorityQueueConfig
	now    func() time.Time // injectable clock for testing
}

// NewPriorityQueue creates an empty priority queue.
func NewPriorityQueue(cfg PriorityQueueConfig) *PriorityQueue {
	return &PriorityQueue{
		config: cfg,
		now:    time.Now,
	}
}

// Push adds an item to the queue. O(log n).
func (pq *PriorityQueue) Push(item HeapItem) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if item.SubmittedAt.IsZero() {
		item.SubmittedAt = pq.now()
	}
	pq.heap = append(pq.heap, item)
	pq.siftUp(len(pq.heap) - 1)
}

// Pop removes and returns the highest-priority item. O(log n).
// Returns the item and true, or zero-value and false if empty.
func (pq *PriorityQueue) Pop() (HeapItem, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.heap) == 0 {
		return HeapItem{}, false
	}

	top := pq.heap[0]
	last := len(pq.heap) - 1
	pq.heap[0] = pq.heap[last]
	pq.heap = pq.heap[:last]
	if len(pq.heap) > 0 {
		pq.siftDown(0)
	}
	return top, true
}

// Peek returns the highest-priority item without removing it. O(1).
func (pq *PriorityQueue) Peek() (HeapItem, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.heap) == 0 {
		return HeapItem{}, false
	}
	return pq.heap[0], true
}

// Len returns the number of items in the queue.
func (pq *PriorityQueue) Len() int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.heap)
}

// effectivePriority calculates priority with age-based starvation prevention.
// Lower value = higher priority.
func (pq *PriorityQueue) effectivePriority(item *HeapItem) int {
	if pq.config.BoostInterval <= 0 {
		return item.Priority
	}

	age := pq.now().Sub(item.SubmittedAt)
	boost := int(age / pq.config.BoostInterval)
	if boost > pq.config.MaxBoost {
		boost = pq.config.MaxBoost
	}
	eff := item.Priority - boost
	if eff < 0 {
		eff = 0
	}
	return eff
}

// less returns true if item i should be dequeued before item j.
func (pq *PriorityQueue) less(i, j int) bool {
	pi := pq.effectivePriority(&pq.heap[i])
	pj := pq.effectivePriority(&pq.heap[j])
	if pi != pj {
		return pi < pj // lower number = higher priority
	}
	// Tie-break: older items first (FIFO within same priority)
	return pq.heap[i].SubmittedAt.Before(pq.heap[j].SubmittedAt)
}

// siftUp restores heap property after insertion.
func (pq *PriorityQueue) siftUp(idx int) {
	for idx > 0 {
		parent := (idx - 1) / 2
		if pq.less(idx, parent) {
			pq.heap[idx], pq.heap[parent] = pq.heap[parent], pq.heap[idx]
			idx = parent
		} else {
			break
		}
	}
}

// siftDown restores heap property after extraction.
func (pq *PriorityQueue) siftDown(idx int) {
	n := len(pq.heap)
	for {
		smallest := idx
		left := 2*idx + 1
		right := 2*idx + 2

		if left < n && pq.less(left, smallest) {
			smallest = left
		}
		if right < n && pq.less(right, smallest) {
			smallest = right
		}
		if smallest == idx {
			break
		}
		pq.heap[idx], pq.heap[smallest] = pq.heap[smallest], pq.heap[idx]
		idx = smallest
	}
}
