package domain

import "context"

// ─── Service Interfaces ─────────────────────────────────────────────────────
// These interfaces define boundaries between layers.
// Infrastructure implements them; application layer depends on them.

// InferenceEngine abstracts the AI inference backend (llama.cpp via CGO).
type InferenceEngine interface {
	// Generate streams tokens for the given request.
	Generate(ctx context.Context, req InferenceRequest) (<-chan Token, error)

	// Embed generates embeddings for the given inputs.
	Embed(ctx context.Context, model string, input []string) ([][]float32, error)

	// LoadedModels returns models currently held in memory.
	LoadedModels() []LoadedModel

	// UnloadAll releases all models from memory.
	UnloadAll() error
}

// ModelStore abstracts persistent model metadata storage.
type ModelStore interface {
	UpsertModel(info ModelInfo) error
	GetModel(name string) (*ModelInfo, error)
	ListModels() ([]ModelInfo, error)
	DeleteModel(name string) error
	TouchModel(name string) error // Update last_used
}

// ModelManager abstracts pull/push/resolve operations.
type ModelManager interface {
	Pull(ctx context.Context, name string, progress func(float64)) error
	Resolve(name string) (string, error) // name → local file path
	HasLocal(name string) bool
	List() ([]ModelInfo, error)
	Remove(name string) error
	Show(name string) (*Manifest, error)
}
