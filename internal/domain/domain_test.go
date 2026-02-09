package domain

import (
	"testing"
	"time"
)

// ─── ModelRef Tests ─────────────────────────────────────────────────────────

func TestModelRef_String(t *testing.T) {
	tests := []struct {
		name string
		ref  ModelRef
		want string
	}{
		{
			name: "simple name, latest tag omitted",
			ref:  ModelRef{Name: "llama3"},
			want: "llama3",
		},
		{
			name: "with explicit tag",
			ref:  ModelRef{Name: "llama3", Tag: "7b"},
			want: "llama3:7b",
		},
		{
			name: "latest tag not printed",
			ref:  ModelRef{Name: "llama3", Tag: "latest"},
			want: "llama3",
		},
		{
			name: "with namespace",
			ref:  ModelRef{Namespace: "myuser", Name: "mymodel", Tag: "v2"},
			want: "myuser/mymodel:v2",
		},
		{
			name: "library namespace omitted",
			ref:  ModelRef{Namespace: "library", Name: "llama3"},
			want: "llama3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ref.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModelRef_FullPath(t *testing.T) {
	ref := ModelRef{Registry: "registry.tutu.ai", Namespace: "library", Name: "llama3"}
	got := ref.FullPath()
	want := "registry.tutu.ai/library/llama3"
	if got != want {
		t.Errorf("FullPath() = %q, want %q", got, want)
	}
}

// ─── Manifest Tests ─────────────────────────────────────────────────────────

func TestManifest_TotalSize(t *testing.T) {
	m := Manifest{
		Layers: []Layer{
			{Size: 100},
			{Size: 200},
			{Size: 300},
		},
	}
	if got := m.TotalSize(); got != 600 {
		t.Errorf("TotalSize() = %d, want 600", got)
	}
}

func TestManifest_TotalSize_Empty(t *testing.T) {
	m := Manifest{}
	if got := m.TotalSize(); got != 0 {
		t.Errorf("TotalSize() = %d, want 0", got)
	}
}

// ─── Utility Tests ──────────────────────────────────────────────────────────

func TestSHA256Hex(t *testing.T) {
	// Known SHA-256 of "hello"
	got := SHA256Hex([]byte("hello"))
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("SHA256Hex(\"hello\") = %q, want %q", got, want)
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
		{4831838208, "4.5 GB"}, // Typical model size
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := HumanSize(tt.bytes)
			if got != tt.want {
				t.Errorf("HumanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

// ─── LoadedModel Tests ──────────────────────────────────────────────────────

func TestLoadedModel_ExpiresIn(t *testing.T) {
	m := LoadedModel{
		ExpiresAt: time.Now().Add(5*time.Minute + 30*time.Second),
	}
	got := m.ExpiresIn()
	// Should be something like "5m30s"
	if got == "" || got == "expired" {
		t.Errorf("ExpiresIn() = %q, expected non-expired", got)
	}
}

func TestLoadedModel_ExpiresIn_Expired(t *testing.T) {
	m := LoadedModel{
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	got := m.ExpiresIn()
	if got != "expired" {
		t.Errorf("ExpiresIn() = %q, want \"expired\"", got)
	}
}

// ─── Error Tests ────────────────────────────────────────────────────────────

func TestSentinelErrors(t *testing.T) {
	errors := []struct {
		name string
		err  error
	}{
		{"ErrModelNotFound", ErrModelNotFound},
		{"ErrModelExists", ErrModelExists},
		{"ErrModelCorrupted", ErrModelCorrupted},
		{"ErrInferenceTimeout", ErrInferenceTimeout},
		{"ErrNoFromDirective", ErrNoFromDirective},
		{"ErrPoolExhausted", ErrPoolExhausted},
	}

	for _, tt := range errors {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
			if tt.err.Error() == "" {
				t.Errorf("%s.Error() is empty", tt.name)
			}
		})
	}
}

// ─── Credit Type Tests (Phase 1 prep) ───────────────────────────────────────

func TestEntryTypes(t *testing.T) {
	if EntryDebit != "DEBIT" {
		t.Errorf("EntryDebit should be DEBIT, got %s", EntryDebit)
	}
	if EntryCredit != "CREDIT" {
		t.Errorf("EntryCredit should be CREDIT, got %s", EntryCredit)
	}
	if EntryDebit == EntryCredit {
		t.Error("EntryDebit and EntryCredit must be distinct")
	}
}

func TestTransactionTypes(t *testing.T) {
	types := []TransactionType{TxEarn, TxSpend, TxBond, TxRelease, TxPenalty, TxBonus}
	seen := make(map[TransactionType]bool)
	for _, tt := range types {
		if seen[tt] {
			t.Errorf("duplicate TransactionType: %s", tt)
		}
		seen[tt] = true
	}
	if len(seen) != 6 {
		t.Errorf("expected 6 unique TransactionTypes, got %d", len(seen))
	}
}

func TestLedgerEntry(t *testing.T) {
	entry := LedgerEntry{
		ID:          1,
		Type:        TxEarn,
		EntryType:   EntryCredit,
		Account:     "node-abc",
		Amount:      100,
		Description: "test earning",
		Balance:     100,
	}
	if entry.Amount != 100 {
		t.Errorf("expected Amount 100, got %d", entry.Amount)
	}
	if entry.Type != TxEarn {
		t.Errorf("expected TxEarn, got %s", entry.Type)
	}
}
