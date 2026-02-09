package domain

import "errors"

// ─── Sentinel Errors ────────────────────────────────────────────────────────
// Domain errors are pure — no infrastructure dependency.

var (
	// Model errors
	ErrModelNotFound    = errors.New("model not found")
	ErrModelExists      = errors.New("model already exists")
	ErrModelCorrupted   = errors.New("model integrity check failed")
	ErrModelTooLarge    = errors.New("insufficient storage for model")

	// Inference errors
	ErrInferenceTimeout = errors.New("inference request timed out")
	ErrModelNotLoaded   = errors.New("model not loaded in memory")
	ErrContextExceeded  = errors.New("context length exceeded")

	// TuTufile errors
	ErrNoFromDirective  = errors.New("TuTufile must include FROM directive")
	ErrInvalidDirective = errors.New("invalid TuTufile directive")
	ErrBaseModelMissing = errors.New("base model specified in FROM not found")

	// Network errors (prepared for Phase 1)
	ErrOffline          = errors.New("no internet connection available")
	ErrRegistryDown     = errors.New("model registry is unreachable")

	// Pool errors
	ErrPoolExhausted    = errors.New("model pool memory exhausted — all models in use")
)
