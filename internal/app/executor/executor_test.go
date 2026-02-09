package executor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tutu-network/tutu/internal/domain"
	"github.com/tutu-network/tutu/internal/infra/resource"
	"github.com/tutu-network/tutu/internal/infra/sqlite"
)

// mockBackend implements Backend for testing.
type mockBackend struct {
	result []byte
	err    error
	delay  time.Duration
}

func (m *mockBackend) Execute(ctx context.Context, task domain.Task) ([]byte, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}
	return m.result, m.err
}

func newTestDB(t *testing.T) *sqlite.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	db := newTestDB(t)
	gov := resource.NewGovernor(resource.DefaultGovernorConfig())
	cfg := DefaultConfig()
	cfg.MaxConcurrent = 2
	cfg.DefaultTimeout = 2 * time.Second
	return New(cfg, gov, db)
}

// ─── Config Tests ───────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxConcurrent != 4 {
		t.Errorf("MaxConcurrent = %d, want 4", cfg.MaxConcurrent)
	}
	if cfg.DefaultTimeout != 5*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 5m", cfg.DefaultTimeout)
	}
}

// ─── Executor Tests ─────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	e := newTestExecutor(t)
	if e == nil {
		t.Fatal("New() returned nil")
	}
	if e.ActiveCount() != 0 {
		t.Errorf("initial ActiveCount = %d, want 0", e.ActiveCount())
	}
}

func TestRegisterBackend(t *testing.T) {
	e := newTestExecutor(t)
	backend := &mockBackend{result: []byte("ok")}

	e.RegisterBackend(domain.TaskInference, backend)

	e.mu.RLock()
	_, ok := e.backends[domain.TaskInference]
	e.mu.RUnlock()

	if !ok {
		t.Error("backend should be registered for INFERENCE")
	}
}

func TestSubmit_Success(t *testing.T) {
	e := newTestExecutor(t)
	e.RegisterBackend(domain.TaskInference, &mockBackend{
		result: []byte("test result"),
		delay:  50 * time.Millisecond,
	})

	task := domain.Task{
		ID:   "task-1",
		Type: domain.TaskInference,
	}

	err := e.Submit(context.Background(), task)
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}

	// Wait for async execution
	time.Sleep(300 * time.Millisecond)

	stats := e.Stats()
	if stats.Completed != 1 {
		t.Errorf("Completed = %d, want 1", stats.Completed)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0", stats.Failed)
	}
}

func TestSubmit_BackendError(t *testing.T) {
	e := newTestExecutor(t)
	e.RegisterBackend(domain.TaskInference, &mockBackend{
		err: fmt.Errorf("model not loaded"),
	})

	task := domain.Task{
		ID:   "task-2",
		Type: domain.TaskInference,
	}

	err := e.Submit(context.Background(), task)
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	stats := e.Stats()
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
}

func TestSubmit_NoBackend(t *testing.T) {
	e := newTestExecutor(t)

	task := domain.Task{
		ID:   "task-3",
		Type: domain.TaskFineTune, // No backend registered for this
	}

	err := e.Submit(context.Background(), task)
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	stats := e.Stats()
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1 (no backend)", stats.Failed)
	}
}

func TestSubmit_ConcurrencyLimit(t *testing.T) {
	e := newTestExecutor(t) // MaxConcurrent = 2
	e.RegisterBackend(domain.TaskInference, &mockBackend{
		result: []byte("ok"),
		delay:  500 * time.Millisecond,
	})

	// Submit 2 tasks (fills the semaphore)
	for i := 0; i < 2; i++ {
		err := e.Submit(context.Background(), domain.Task{
			ID:   fmt.Sprintf("task-%d", i),
			Type: domain.TaskInference,
		})
		if err != nil {
			t.Fatalf("Submit(%d) error: %v", i, err)
		}
	}

	// Give tasks a moment to start
	time.Sleep(50 * time.Millisecond)

	// 3rd should fail — at capacity
	err := e.Submit(context.Background(), domain.Task{
		ID:   "task-overflow",
		Type: domain.TaskInference,
	})
	if err == nil {
		t.Error("Submit should fail when at capacity")
	}
}

func TestStats(t *testing.T) {
	e := newTestExecutor(t)
	stats := e.Stats()

	if stats.MaxSlots != 2 {
		t.Errorf("MaxSlots = %d, want 2", stats.MaxSlots)
	}
	if stats.FreeSlots != 2 {
		t.Errorf("FreeSlots = %d, want 2", stats.FreeSlots)
	}
	if stats.Active != 0 {
		t.Errorf("Active = %d, want 0", stats.Active)
	}
}

func TestMultipleTaskTypes(t *testing.T) {
	e := newTestExecutor(t)
	e.RegisterBackend(domain.TaskInference, &mockBackend{result: []byte("inference")})
	e.RegisterBackend(domain.TaskEmbedding, &mockBackend{result: []byte("embedding")})

	e.Submit(context.Background(), domain.Task{ID: "t1", Type: domain.TaskInference})
	e.Submit(context.Background(), domain.Task{ID: "t2", Type: domain.TaskEmbedding})

	time.Sleep(300 * time.Millisecond)

	stats := e.Stats()
	if stats.Completed != 2 {
		t.Errorf("Completed = %d, want 2", stats.Completed)
	}
}
