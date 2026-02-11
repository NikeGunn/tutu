package observability

import (
	"context"
	"errors"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════════
// Observability Tests — Phase 3
// ═══════════════════════════════════════════════════════════════════════════

// ─── Tracer ─────────────────────────────────────────────────────────────────

func TestTracer_StartEnd_RecordsSpan(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()

	span := tr.StartSpan(ctx, "test-op", map[string]string{"key": "val"})
	tr.EndSpan(span, nil)

	if tr.SpanCount() != 1 {
		t.Fatalf("SpanCount() = %d, want 1", tr.SpanCount())
	}

	spans := tr.Spans(1)
	if len(spans) != 1 {
		t.Fatalf("Spans(1) returned %d, want 1", len(spans))
	}
	if spans[0].Operation != "test-op" {
		t.Errorf("Operation = %q, want %q", spans[0].Operation, "test-op")
	}
	if spans[0].Status != SpanOK {
		t.Errorf("Status = %d, want SpanOK", spans[0].Status)
	}
	if spans[0].EndTime.Before(spans[0].StartTime) {
		t.Error("EndTime should not be before StartTime")
	}
	if spans[0].Attrs["key"] != "val" {
		t.Errorf("Attrs[key] = %q, want %q", spans[0].Attrs["key"], "val")
	}
}

func TestTracer_EndSpan_RecordsError(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()

	span := tr.StartSpan(ctx, "err-op", nil)
	tr.EndSpan(span, errors.New("boom"))

	spans := tr.Spans(1)
	if spans[0].Status != SpanError {
		t.Errorf("Status = %d, want SpanError", spans[0].Status)
	}
	if spans[0].Attrs["error"] != "boom" {
		t.Errorf("error attr = %q, want %q", spans[0].Attrs["error"], "boom")
	}
}

func TestTracer_Disabled(t *testing.T) {
	tr := NewTracer(TracerConfig{Enabled: false, MaxSpans: 100})
	ctx := context.Background()
	span := tr.StartSpan(ctx, "noop", nil)
	tr.EndSpan(span, nil)

	if tr.SpanCount() != 0 {
		t.Errorf("disabled tracer SpanCount() = %d, want 0", tr.SpanCount())
	}
}

func TestTracer_RingBuffer_Overflow(t *testing.T) {
	tr := NewTracer(TracerConfig{Enabled: true, MaxSpans: 3})
	ctx := context.Background()

	// Record 5 spans in a buffer of 3
	for i := 0; i < 5; i++ {
		span := tr.StartSpan(ctx, "op", nil)
		tr.EndSpan(span, nil)
	}

	if tr.SpanCount() != 3 {
		t.Errorf("SpanCount() = %d, want 3 (ring buffer overflow)", tr.SpanCount())
	}
}

func TestTracer_Spans_Limit(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		span := tr.StartSpan(ctx, "op", nil)
		tr.EndSpan(span, nil)
	}

	spans := tr.Spans(3)
	if len(spans) != 3 {
		t.Errorf("Spans(3) returned %d, want 3", len(spans))
	}
}

func TestTracer_Spans_ZeroLimit(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		span := tr.StartSpan(ctx, "op", nil)
		tr.EndSpan(span, nil)
	}

	spans := tr.Spans(0)
	if len(spans) != 5 {
		t.Errorf("Spans(0) returned %d, want all 5", len(spans))
	}
}

func TestTracer_Reset(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()
	span := tr.StartSpan(ctx, "op", nil)
	tr.EndSpan(span, nil)

	tr.Reset()
	if tr.SpanCount() != 0 {
		t.Errorf("SpanCount() after Reset = %d, want 0", tr.SpanCount())
	}
}

// ─── Context Propagation ────────────────────────────────────────────────────

func TestTracer_ContextPropagation(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-abc")
	ctx = WithSpanID(ctx, "span-123")

	span := tr.StartSpan(ctx, "child-op", nil)
	tr.EndSpan(span, nil)

	spans := tr.Spans(1)
	if spans[0].TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want %q", spans[0].TraceID, "trace-abc")
	}
	if spans[0].ParentID != "span-123" {
		t.Errorf("ParentID = %q, want %q", spans[0].ParentID, "span-123")
	}
}

func TestTracer_AutoGeneratesTraceID(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background() // no trace ID in context

	span := tr.StartSpan(ctx, "root-op", nil)
	tr.EndSpan(span, nil)

	spans := tr.Spans(1)
	if spans[0].TraceID == "" {
		t.Error("TraceID should be auto-generated, got empty")
	}
}

func TestTracer_SpanIDUnique(t *testing.T) {
	tr := NewTracer(DefaultTracerConfig())
	ctx := context.Background()

	span1 := tr.StartSpan(ctx, "op1", nil)
	span2 := tr.StartSpan(ctx, "op2", nil)

	if span1.SpanID == span2.SpanID {
		t.Errorf("SpanIDs should be unique, both = %q", span1.SpanID)
	}

	tr.EndSpan(span1, nil)
	tr.EndSpan(span2, nil)
}
