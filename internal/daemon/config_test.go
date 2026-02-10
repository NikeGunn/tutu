package daemon

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.API.Host != "127.0.0.1" {
		t.Errorf("API.Host = %q, want %q", cfg.API.Host, "127.0.0.1")
	}
	if cfg.API.Port != 11434 {
		t.Errorf("API.Port = %d, want %d", cfg.API.Port, 11434)
	}
	if cfg.Models.MaxStorage != "50GB" {
		t.Errorf("Models.MaxStorage = %q, want %q", cfg.Models.MaxStorage, "50GB")
	}
	if cfg.Inference.ContextLength != 4096 {
		t.Errorf("Inference.ContextLength = %d, want %d", cfg.Inference.ContextLength, 4096)
	}

	// Phase 2: MCP config
	if !cfg.MCP.Enabled {
		t.Error("MCP.Enabled should be true by default")
	}
	if cfg.MCP.DefaultTier != "standard" {
		t.Errorf("MCP.DefaultTier = %q, want %q", cfg.MCP.DefaultTier, "standard")
	}
	if cfg.MCP.RateLimitRPM != 300 {
		t.Errorf("MCP.RateLimitRPM = %d, want %d", cfg.MCP.RateLimitRPM, 300)
	}

	// Phase 2: Agent config
	if cfg.Agent.Enabled {
		t.Error("Agent.Enabled should be false by default (opt-in)")
	}
	if cfg.Agent.IdleTimeout != "5m" {
		t.Errorf("Agent.IdleTimeout = %q, want %q", cfg.Agent.IdleTimeout, "5m")
	}
	if cfg.Agent.MaxAgents != 4 {
		t.Errorf("Agent.MaxAgents = %d, want %d", cfg.Agent.MaxAgents, 4)
	}
}

func TestParseStorageSize(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
	}{
		{"50GB", 50 * 1024 * 1024 * 1024},
		{"1TB", 1 * 1024 * 1024 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
		{"", 50 * 1024 * 1024 * 1024}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseStorageSize(tt.input)
			if got != tt.want {
				t.Errorf("parseStorageSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
