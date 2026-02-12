package catalog

import "testing"

func TestLookupExistingModel(t *testing.T) {
	tests := []struct {
		query    string
		wantName string
	}{
		{"llama3", "llama3"},
		{"llama3:latest", "llama3"},
		{"llama3:1b", "llama3"},
		{"llama3.2", "llama3"},
		{"llama3:8b", "llama3:8b"},
		{"tinyllama", "tinyllama"},
		{"phi3", "phi3"},
		{"qwen2.5", "qwen2.5"},
		{"gemma2", "gemma2"},
		{"smollm2", "smollm2"},
		{"mistral", "mistral"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			entry := Lookup(tt.query)
			if entry == nil {
				t.Fatalf("Lookup(%q) returned nil, want %q", tt.query, tt.wantName)
			}
			if entry.Name != tt.wantName {
				t.Errorf("Lookup(%q).Name = %q, want %q", tt.query, entry.Name, tt.wantName)
			}
		})
	}
}

func TestLookupUnknownModel(t *testing.T) {
	entry := Lookup("nonexistent-model")
	if entry != nil {
		t.Errorf("Lookup(nonexistent) = %v, want nil", entry)
	}
}

func TestDownloadURL(t *testing.T) {
	entry := Lookup("tinyllama")
	if entry == nil {
		t.Fatal("tinyllama not found in catalog")
	}

	url := entry.DownloadURL()
	want := "https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf"
	if url != want {
		t.Errorf("DownloadURL() = %q, want %q", url, want)
	}
}

func TestCatalogNotEmpty(t *testing.T) {
	if len(Catalog) == 0 {
		t.Fatal("Catalog is empty")
	}
}

func TestAllEntriesHaveTags(t *testing.T) {
	for _, entry := range Catalog {
		if len(entry.Tags) == 0 {
			t.Errorf("model %q has no tags", entry.Name)
		}
		if entry.HFRepo == "" {
			t.Errorf("model %q has empty HFRepo", entry.Name)
		}
		if entry.HFFile == "" {
			t.Errorf("model %q has empty HFFile", entry.Name)
		}
		if entry.SizeBytes == 0 {
			t.Errorf("model %q has zero SizeBytes", entry.Name)
		}
	}
}

func TestAllEntriesHaveValidFormat(t *testing.T) {
	for _, entry := range Catalog {
		if entry.Format != "gguf" {
			t.Errorf("model %q format = %q, want gguf", entry.Name, entry.Format)
		}
		if entry.ContextSize == 0 {
			t.Errorf("model %q has zero ContextSize", entry.Name)
		}
		if entry.ChatTemplate == "" {
			t.Errorf("model %q has empty ChatTemplate", entry.Name)
		}
	}
}
