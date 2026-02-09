// Package domain contains pure business types with ZERO infrastructure imports.
// This is the innermost ring of clean architecture — it depends on nothing.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ─── Model Types ────────────────────────────────────────────────────────────

// ModelInfo represents a locally installed model.
type ModelInfo struct {
	Name         string    `json:"name"`
	Digest       string    `json:"digest"`
	SizeBytes    int64     `json:"size_bytes"`
	Format       string    `json:"format"`
	Family       string    `json:"family,omitempty"`
	Parameters   string    `json:"parameters,omitempty"`
	Quantization string    `json:"quantization,omitempty"`
	PulledAt     time.Time `json:"pulled_at"`
	LastUsed     time.Time `json:"last_used"`
	Pinned       bool      `json:"pinned"`
}

// Manifest describes a model's layers in OCI-like content-addressed format.
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        Layer  `json:"config"`
	Layers        []Layer `json:"layers"`
}

// Layer is a content-addressed blob in the model store.
type Layer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// TotalSize returns sum of all layer sizes in bytes.
func (m *Manifest) TotalSize() int64 {
	var total int64
	for _, l := range m.Layers {
		total += l.Size
	}
	return total
}

// ModelRef is a parsed model reference (registry/namespace/name:tag).
type ModelRef struct {
	Registry  string
	Namespace string
	Name      string
	Tag       string
}

// String formats the model reference.
func (r ModelRef) String() string {
	s := r.Name
	if r.Namespace != "" && r.Namespace != "library" {
		s = r.Namespace + "/" + s
	}
	if r.Tag != "" && r.Tag != "latest" {
		s += ":" + r.Tag
	}
	return s
}

// FullPath returns the full registry path.
func (r ModelRef) FullPath() string {
	return fmt.Sprintf("%s/%s/%s", r.Registry, r.Namespace, r.Name)
}

// ─── Message Types ──────────────────────────────────────────────────────────

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ─── Inference Types ────────────────────────────────────────────────────────

// InferenceRequest holds parameters for a generation request.
type InferenceRequest struct {
	Model       string    `json:"model"`
	Prompt      string    `json:"prompt,omitempty"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages,omitempty"`
	Temperature float32   `json:"temperature,omitempty"`
	TopP        float32   `json:"top_p,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	NumCtx      int       `json:"num_ctx,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	Format      string    `json:"format,omitempty"`
}

// Token is a single generated token from the inference engine.
type Token struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// EmbeddingRequest holds parameters for an embedding request.
type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ─── TuTufile Types ─────────────────────────────────────────────────────────

// TuTufile represents a parsed TuTufile definition.
type TuTufile struct {
	From       string
	Parameters map[string][]string // key → values (some keys like "stop" can repeat)
	System     string
	Template   string
	Adapter    string
	Messages   []Message
	License    string
}

// ─── Loaded Model Info ──────────────────────────────────────────────────────

// LoadedModel describes a model currently loaded in memory.
type LoadedModel struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size"`
	Processor string    `json:"processor"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ExpiresIn returns human-readable time until model is unloaded.
func (m LoadedModel) ExpiresIn() string {
	d := time.Until(m.ExpiresAt)
	if d < 0 {
		return "expired"
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// ─── Utilities ──────────────────────────────────────────────────────────────

// SHA256Hex computes SHA-256 hash and returns hex string.
func SHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// HumanSize formats bytes into human-readable string.
func HumanSize(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
