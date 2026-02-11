package dsa

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sync"
)

// ─── Bloom Filter ───────────────────────────────────────────────────────────
// Probabilistic set membership for peer model inventory.
// Answers "does node X have model Y?" with:
//   - Yes → probably (false positive rate ≤ configured FPR)
//   - No  → definitely not (zero false negatives)
//
// Architecture Part XX §5: O(1) lookup, 0.1% FP rate.
// Space: ~14.4 bits per element for 0.1% FP → 1.8 KB for 1000 models.

// BloomConfig configures a Bloom filter.
type BloomConfig struct {
	ExpectedItems int     // Expected number of elements
	FPRate        float64 // Desired false positive rate (e.g. 0.001 = 0.1%)
}

// DefaultBloomConfig returns defaults for 1000 models at 0.1% FP rate.
func DefaultBloomConfig() BloomConfig {
	return BloomConfig{
		ExpectedItems: 1000,
		FPRate:        0.001,
	}
}

// BloomFilter is a space-efficient probabilistic set.
type BloomFilter struct {
	mu       sync.RWMutex
	bits     []uint64 // bit array stored as uint64 words
	numBits  uint     // total bits
	numHash  uint     // number of hash functions
	count    int      // elements added
}

// NewBloomFilter creates a Bloom filter sized to achieve the target FP rate.
// Optimal sizing formulas:
//
//	m = -(n * ln(p)) / (ln(2)^2)   — total bits
//	k = (m/n) * ln(2)              — hash functions
func NewBloomFilter(cfg BloomConfig) *BloomFilter {
	if cfg.ExpectedItems <= 0 {
		cfg.ExpectedItems = 1000
	}
	if cfg.FPRate <= 0 || cfg.FPRate >= 1 {
		cfg.FPRate = 0.001
	}

	n := float64(cfg.ExpectedItems)
	p := cfg.FPRate

	// Optimal number of bits
	m := uint(math.Ceil(-(n * math.Log(p)) / (math.Log(2) * math.Log(2))))
	// Optimal number of hash functions
	k := uint(math.Ceil(float64(m) / n * math.Log(2)))

	if m == 0 {
		m = 64
	}
	if k == 0 {
		k = 1
	}

	// Round up to next uint64 boundary
	words := (m + 63) / 64

	return &BloomFilter{
		bits:    make([]uint64, words),
		numBits: m,
		numHash: k,
	}
}

// Add inserts an item into the filter.
func (bf *BloomFilter) Add(item string) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	h1, h2 := bf.baseHashes(item)
	for i := uint(0); i < bf.numHash; i++ {
		pos := bf.nthHash(h1, h2, i)
		bf.bits[pos/64] |= 1 << (pos % 64)
	}
	bf.count++
}

// Contains tests whether an item might be in the filter.
// False means definitely not present. True means probably present.
func (bf *BloomFilter) Contains(item string) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	h1, h2 := bf.baseHashes(item)
	for i := uint(0); i < bf.numHash; i++ {
		pos := bf.nthHash(h1, h2, i)
		if bf.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false // Definitely not present
		}
	}
	return true // Probably present
}

// Count returns the number of items added.
func (bf *BloomFilter) Count() int {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	return bf.count
}

// EstimatedFPRate returns the estimated current false positive rate
// based on the number of items added.
func (bf *BloomFilter) EstimatedFPRate() float64 {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	m := float64(bf.numBits)
	k := float64(bf.numHash)
	n := float64(bf.count)

	// FP rate ≈ (1 - e^(-kn/m))^k
	return math.Pow(1-math.Exp(-k*n/m), k)
}

// Config returns the filter's configuration parameters.
func (bf *BloomFilter) Config() (numBits, numHash uint) {
	return bf.numBits, bf.numHash
}

// Reset clears the filter.
func (bf *BloomFilter) Reset() {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	for i := range bf.bits {
		bf.bits[i] = 0
	}
	bf.count = 0
}

// baseHashes computes two independent 32-bit hashes using SHA-256.
// We use double-hashing (Kirsch-Mitzenmacker technique) to derive k hashes
// from just 2 base hashes: h_i(x) = h1(x) + i*h2(x).
func (bf *BloomFilter) baseHashes(item string) (uint32, uint32) {
	sum := sha256.Sum256([]byte(item))
	h1 := binary.BigEndian.Uint32(sum[0:4])
	h2 := binary.BigEndian.Uint32(sum[4:8])
	return h1, h2
}

// nthHash derives the i-th hash position using double hashing.
func (bf *BloomFilter) nthHash(h1, h2 uint32, i uint) uint {
	return uint((uint64(h1) + uint64(i)*uint64(h2)) % uint64(bf.numBits))
}
