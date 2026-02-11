// Package observability implements Phase 3 full observability.
// Architecture Part XVIII: OpenTelemetry tracing, structured logging, extended metrics.
//
// This provides:
//   - Trace spans for the full task lifecycle (submit → schedule → assign → execute → verify → pay)
//   - W3C TraceContext propagation
//   - Extended Prometheus metrics for Phase 3 features
//   - Structured log correlation with trace IDs
package observability

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ═══════════════════════════════════════════════════════════════════════════
// Trace Spans — Lightweight span tracking without external OTel SDK dependency
// ═══════════════════════════════════════════════════════════════════════════

// SpanKind classifies a span.
type SpanKind int

const (
	SpanInternal SpanKind = iota
	SpanServer
	SpanClient
)

// Span represents a unit of work within a distributed trace.
type Span struct {
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Operation string            `json:"operation"`
	Kind      SpanKind          `json:"kind"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	Duration  time.Duration     `json:"duration,omitempty"`
	Status    SpanStatus        `json:"status"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

// SpanStatus indicates success/failure.
type SpanStatus int

const (
	SpanOK SpanStatus = iota
	SpanError
)

// ─── Tracer ─────────────────────────────────────────────────────────────────

// Tracer provides lightweight distributed tracing.
// In production, this would wrap OpenTelemetry SDK.
// Phase 3 implementation stores spans in-memory for inspection and export.
type Tracer struct {
	mu       sync.Mutex
	spans    []Span
	maxSpans int
	enabled  bool
}

// TracerConfig configures the tracer.
type TracerConfig struct {
	Enabled  bool
	MaxSpans int // ring buffer size (default 10_000)
}

// DefaultTracerConfig returns production defaults.
func DefaultTracerConfig() TracerConfig {
	return TracerConfig{
		Enabled:  true,
		MaxSpans: 10_000,
	}
}

// NewTracer creates a new tracer.
func NewTracer(cfg TracerConfig) *Tracer {
	return &Tracer{
		spans:    make([]Span, 0, cfg.MaxSpans),
		maxSpans: cfg.MaxSpans,
		enabled:  cfg.Enabled,
	}
}

// StartSpan begins a new span with the given operation name.
// Returns the span (caller must call EndSpan when done).
func (t *Tracer) StartSpan(ctx context.Context, operation string, attrs map[string]string) *Span {
	if !t.enabled {
		return &Span{Operation: operation}
	}

	span := &Span{
		TraceID:   traceIDFromContext(ctx),
		SpanID:    generateID(),
		ParentID:  spanIDFromContext(ctx),
		Operation: operation,
		Kind:      SpanInternal,
		StartTime: time.Now(),
		Status:    SpanOK,
		Attrs:     attrs,
	}

	return span
}

// EndSpan completes a span and records it.
func (t *Tracer) EndSpan(span *Span, err error) {
	if !t.enabled || span == nil {
		return
	}

	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
	if err != nil {
		span.Status = SpanError
		if span.Attrs == nil {
			span.Attrs = make(map[string]string)
		}
		span.Attrs["error"] = err.Error()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Ring buffer: overwrite oldest if at capacity
	if len(t.spans) >= t.maxSpans {
		t.spans = t.spans[1:]
	}
	t.spans = append(t.spans, *span)
}

// Spans returns a copy of the recent spans.
func (t *Tracer) Spans(limit int) []Span {
	t.mu.Lock()
	defer t.mu.Unlock()

	if limit <= 0 || limit > len(t.spans) {
		limit = len(t.spans)
	}

	// Return most recent spans
	start := len(t.spans) - limit
	out := make([]Span, limit)
	copy(out, t.spans[start:])
	return out
}

// SpanCount returns the number of recorded spans.
func (t *Tracer) SpanCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.spans)
}

// Reset clears all recorded spans.
func (t *Tracer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = t.spans[:0]
}

// ─── Context Helpers ────────────────────────────────────────────────────────

type contextKey string

const (
	traceIDKey contextKey = "tutu-trace-id"
	spanIDKey  contextKey = "tutu-span-id"
)

// WithTraceID returns a context with the given trace ID.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// WithSpanID returns a context with the given span ID.
func WithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDKey, spanID)
}

func traceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}
	return generateID()
}

func spanIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(spanIDKey).(string); ok {
		return v
	}
	return ""
}

// generateID creates a short unique ID (not cryptographically secure — fine for tracing).
var spanCounter atomic.Int64

func generateID() string {
	n := spanCounter.Add(1)
	return fmt.Sprintf("%s-%d", time.Now().Format("20060102150405"), n)
}

// ═══════════════════════════════════════════════════════════════════════════
// Phase 3 Extended Prometheus Metrics
// ═══════════════════════════════════════════════════════════════════════════

// ─── Scheduler Metrics ──────────────────────────────────────────────────────

// SchedulerQueueDepth tracks current scheduler queue depth.
var SchedulerQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "tutu",
	Subsystem: "scheduler",
	Name:      "queue_depth",
	Help:      "Current number of tasks in the scheduler queue.",
})

// SchedulerBackPressure tracks the current back-pressure level.
var SchedulerBackPressure = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "tutu",
	Subsystem: "scheduler",
	Name:      "back_pressure_level",
	Help:      "Current back-pressure level (0=none, 1=soft, 2=medium, 3=hard).",
})

// SchedulerTasksStolen tracks total tasks stolen via work stealing.
var SchedulerTasksStolen = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "scheduler",
	Name:      "tasks_stolen_total",
	Help:      "Total tasks stolen from other nodes via work stealing.",
})

// SchedulerPreemptions tracks total preempted tasks.
var SchedulerPreemptions = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "scheduler",
	Name:      "preemptions_total",
	Help:      "Total tasks preempted by higher-priority tasks.",
})

// SchedulerTasksRejected tracks tasks rejected by back-pressure.
var SchedulerTasksRejected = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "scheduler",
	Name:      "tasks_rejected_total",
	Help:      "Total tasks rejected by back-pressure.",
}, []string{"level"})

// ─── Region Metrics ─────────────────────────────────────────────────────────

// RegionRoutingDecisions tracks routing decisions by reason.
var RegionRoutingDecisions = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "region",
	Name:      "routing_decisions_total",
	Help:      "Total routing decisions by reason.",
}, []string{"reason"})

// RegionLatency tracks cross-region latency.
var RegionLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "tutu",
	Subsystem: "region",
	Name:      "latency_ms",
	Help:      "Cross-region latency in milliseconds.",
	Buckets:   []float64{1, 5, 10, 25, 50, 100, 200, 500},
}, []string{"from", "to"})

// RegionHealth tracks whether each region is healthy.
var RegionHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "tutu",
	Subsystem: "region",
	Name:      "healthy",
	Help:      "Whether a region is healthy (1) or not (0).",
}, []string{"region"})

// ─── Circuit Breaker Metrics ────────────────────────────────────────────────

// CircuitBreakerState tracks circuit breaker states.
var CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "tutu",
	Subsystem: "circuit_breaker",
	Name:      "state",
	Help:      "Current circuit breaker state (0=closed, 1=open, 2=half-open).",
}, []string{"name"})

// CircuitBreakerTrips tracks total circuit breaker trips.
var CircuitBreakerTrips = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "circuit_breaker",
	Name:      "trips_total",
	Help:      "Total circuit breaker trips.",
}, []string{"name"})

// ─── Quarantine Metrics ─────────────────────────────────────────────────────

// QuarantinedNodes tracks currently quarantined nodes.
var QuarantinedNodes = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "tutu",
	Subsystem: "quarantine",
	Name:      "active_nodes",
	Help:      "Number of currently quarantined nodes.",
})

// QuarantineEvents tracks quarantine events by reason.
var QuarantineEvents = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "quarantine",
	Name:      "events_total",
	Help:      "Total quarantine events by reason.",
}, []string{"reason"})

// ─── NAT Traversal Metrics ──────────────────────────────────────────────────

// NATConnections tracks NAT traversal outcomes by strategy.
var NATConnections = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "nat",
	Name:      "connections_total",
	Help:      "Total NAT traversal connection attempts by strategy.",
}, []string{"strategy", "success"})

// NATLatency tracks NAT connection latency by strategy.
var NATLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "tutu",
	Subsystem: "nat",
	Name:      "latency_ms",
	Help:      "NAT traversal connection latency in milliseconds.",
	Buckets:   []float64{1, 3, 5, 10, 20, 50, 100},
}, []string{"strategy"})

// ─── Trace Metrics ──────────────────────────────────────────────────────────

// TracesRecorded tracks total spans recorded.
var TracesRecorded = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "traces",
	Name:      "spans_recorded_total",
	Help:      "Total trace spans recorded.",
})

// TraceErrors tracks error spans.
var TraceErrors = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "tutu",
	Subsystem: "traces",
	Name:      "error_spans_total",
	Help:      "Total trace spans with error status.",
})
