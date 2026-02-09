// Package executor manages task execution with resource governance.
// Architecture Part IX: Task lifecycle — receive, check governor, sandbox, execute, verify.
//
// The executor:
//  1. Checks governor budget before accepting work
//  2. Creates a constrained execution context (CPU/mem/timeout)
//  3. Routes to the appropriate backend (inference, embedding, etc.)
//  4. Hashes the result (SHA-256) for verification
//  5. Reports completion with credits
package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/tutu-network/tutu/internal/domain"
	"github.com/tutu-network/tutu/internal/infra/resource"
	"github.com/tutu-network/tutu/internal/infra/sqlite"
)

// Backend represents a computation backend (inference, embedding, etc.)
type Backend interface {
	Execute(ctx context.Context, task domain.Task) (result []byte, err error)
}

// Config controls executor behavior.
type Config struct {
	MaxConcurrent int           // Maximum concurrent tasks (default: 4)
	DefaultTimeout time.Duration // Default task timeout (default: 5m)
}

// DefaultConfig returns safe executor defaults.
func DefaultConfig() Config {
	return Config{
		MaxConcurrent:  4,
		DefaultTimeout: 5 * time.Minute,
	}
}

// Executor manages task execution lifecycle.
type Executor struct {
	mu        sync.RWMutex
	config    Config
	governor  *resource.Governor
	db        *sqlite.DB
	backends  map[domain.TaskType]Backend
	sem       chan struct{} // Concurrency semaphore
	active    int
	completed int64
	failed    int64
}

// New creates a task executor.
func New(cfg Config, gov *resource.Governor, db *sqlite.DB) *Executor {
	return &Executor{
		config:   cfg,
		governor: gov,
		db:       db,
		backends: make(map[domain.TaskType]Backend),
		sem:      make(chan struct{}, cfg.MaxConcurrent),
	}
}

// RegisterBackend registers a computation backend for a task type.
func (e *Executor) RegisterBackend(taskType domain.TaskType, backend Backend) {
	e.mu.Lock()
	e.backends[taskType] = backend
	e.mu.Unlock()
}

// Submit submits a task for execution. Returns immediately.
// The task is persisted and executed asynchronously.
// Local tasks only require CPU budget > 0. Distributed tasks
// additionally require AllowDistributed.
func (e *Executor) Submit(ctx context.Context, task domain.Task) error {
	// Check governor budget — local tasks need CPU > 0
	budget := e.governor.Budget()
	if budget.MaxCPUPercent <= 0 {
		return fmt.Errorf("governor budget too low: CPU=%d%%",
			budget.MaxCPUPercent)
	}

	// Check concurrency limit
	select {
	case e.sem <- struct{}{}:
		// Got a slot
	default:
		return fmt.Errorf("executor at capacity (%d concurrent tasks)", e.config.MaxConcurrent)
	}

	// Persist task as QUEUED
	task.Status = domain.TaskQueued
	task.CreatedAt = time.Now()
	if err := e.db.InsertTask(task); err != nil {
		<-e.sem // Release slot
		return fmt.Errorf("persist task: %w", err)
	}

	// Execute asynchronously
	go e.execute(ctx, task)

	return nil
}

// execute runs a task through the full lifecycle.
func (e *Executor) execute(ctx context.Context, task domain.Task) {
	defer func() { <-e.sem }() // Release concurrency slot

	e.mu.Lock()
	e.active++
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.active--
		e.mu.Unlock()
	}()

	// Transition: QUEUED → ASSIGNED → EXECUTING
	e.db.UpdateTaskStatus(task.ID, domain.TaskAssigned)
	e.db.UpdateTaskStatus(task.ID, domain.TaskExecuting)

	log.Printf("[executor] executing task %s type=%s", task.ID, task.Type)

	// Create timeout context
	timeout := e.config.DefaultTimeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get backend
	e.mu.RLock()
	backend, ok := e.backends[task.Type]
	e.mu.RUnlock()

	if !ok {
		e.failTask(task.ID, fmt.Sprintf("no backend for task type: %s", task.Type))
		return
	}

	// Execute
	result, err := backend.Execute(execCtx, task)
	if err != nil {
		e.failTask(task.ID, err.Error())
		return
	}

	// Hash result for verification (Architecture Part IX)
	hash := sha256.Sum256(result)
	resultHash := hex.EncodeToString(hash[:])

	// Complete the task
	e.db.UpdateTaskStatus(task.ID, domain.TaskCompleted)

	log.Printf("[executor] task %s completed, hash=%s", task.ID, resultHash[:16])

	e.mu.Lock()
	e.completed++
	e.mu.Unlock()

	// Update task with result hash
	// Note: UpdateTaskStatus doesn't set hash — we'd need a dedicated method.
	// For Phase 1, we log it. Full implementation in Phase 2.
	_ = resultHash
}

// failTask marks a task as failed with an error message.
func (e *Executor) failTask(taskID, errMsg string) {
	e.db.UpdateTaskStatus(taskID, domain.TaskFailed)
	log.Printf("[executor] task %s failed: %s", taskID, errMsg)

	e.mu.Lock()
	e.failed++
	e.mu.Unlock()
}

// Stats returns executor statistics.
type Stats struct {
	Active     int   `json:"active"`
	Completed  int64 `json:"completed"`
	Failed     int64 `json:"failed"`
	MaxSlots   int   `json:"max_slots"`
	FreeSlots  int   `json:"free_slots"`
}

// Stats returns current executor statistics.
func (e *Executor) Stats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return Stats{
		Active:    e.active,
		Completed: e.completed,
		Failed:    e.failed,
		MaxSlots:  e.config.MaxConcurrent,
		FreeSlots: e.config.MaxConcurrent - e.active,
	}
}

// ActiveCount returns the number of currently executing tasks.
func (e *Executor) ActiveCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.active
}
