package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tutu-network/tutu/internal/domain"
	"github.com/tutu-network/tutu/internal/infra/sqlite"
)

// Manager implements domain.ModelManager.
// It manages content-addressed blobs in a local directory and tracks
// metadata in SQLite.
type Manager struct {
	dir string // Root models directory (contains blobs/ and manifests/)
	db  *sqlite.DB
}

// NewManager creates a Manager rooted at dir.
func NewManager(dir string, db *sqlite.DB) *Manager {
	return &Manager{dir: dir, db: db}
}

// Init ensures the directory structure exists.
func (m *Manager) Init() error {
	dirs := []string{
		filepath.Join(m.dir, "blobs"),
		filepath.Join(m.dir, "manifests"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

// BlobPath returns the filesystem path for a content-addressed blob.
func (m *Manager) BlobPath(digest string) string {
	// digest is "sha256:<hex>" → store as blobs/sha256-<hex>
	safe := strings.ReplaceAll(digest, ":", "-")
	return filepath.Join(m.dir, "blobs", safe)
}

// ManifestPath returns the path for a model manifest file.
func (m *Manager) ManifestPath(ref domain.ModelRef) string {
	name := ref.Name
	tag := ref.Tag
	if tag == "" {
		tag = "latest"
	}
	return filepath.Join(m.dir, "manifests", name, tag)
}

// HasLocal checks whether a model exists locally.
func (m *Manager) HasLocal(ref domain.ModelRef) (bool, error) {
	info, err := m.db.GetModel(ref.String())
	if err != nil {
		return false, err
	}
	return info != nil, nil
}

// Resolve returns the path to the primary weights blob for a model.
// This is used by the engine pool to load a model.
func (m *Manager) Resolve(name string) (string, error) {
	ref := ParseRef(name)

	info, err := m.db.GetModel(ref.String())
	if err != nil {
		return "", fmt.Errorf("query model %s: %w", ref, err)
	}
	if info == nil {
		return "", domain.ErrModelNotFound
	}

	// Touch to update last-used
	_ = m.db.TouchModel(ref.String())

	// Load manifest
	manifest, err := m.loadManifest(ref)
	if err != nil {
		return "", err
	}

	// Find the weights layer (typically the largest layer or type "model")
	for _, layer := range manifest.Layers {
		if layer.MediaType == "application/vnd.tutu.model" ||
			strings.Contains(layer.MediaType, "model") ||
			strings.HasSuffix(layer.Digest, ".gguf") {
			path := m.BlobPath(layer.Digest)
			if _, err := os.Stat(path); err != nil {
				return "", fmt.Errorf("blob missing for %s: %w", layer.Digest, domain.ErrModelCorrupted)
			}
			return path, nil
		}
	}

	// Fallback: return first layer
	if len(manifest.Layers) > 0 {
		path := m.BlobPath(manifest.Layers[0].Digest)
		return path, nil
	}

	return "", fmt.Errorf("model %s has no layers: %w", ref, domain.ErrModelCorrupted)
}

// List returns all locally stored models.
func (m *Manager) List() ([]domain.ModelInfo, error) {
	return m.db.ListModels()
}

// Remove deletes a model from local storage.
func (m *Manager) Remove(name string) error {
	ref := ParseRef(name)

	// Load manifest to find blobs
	manifest, err := m.loadManifest(ref)
	if err == nil {
		// Best-effort blob cleanup
		for _, layer := range manifest.Layers {
			_ = os.Remove(m.BlobPath(layer.Digest))
		}
	}

	// Remove manifest file
	mpath := m.ManifestPath(ref)
	_ = os.Remove(mpath)

	// Remove from DB
	return m.db.DeleteModel(ref.String())
}

// Show returns detailed info about a model.
func (m *Manager) Show(name string) (*domain.ModelInfo, error) {
	ref := ParseRef(name)
	info, err := m.db.GetModel(ref.String())
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, domain.ErrModelNotFound
	}
	return info, nil
}

// Pull downloads a model from the registry. For Phase 0, this creates
// a placeholder model locally for testing purposes. Real registry pull
// will be implemented in Phase 1 when the network layer is ready.
func (m *Manager) Pull(name string, progress func(status string, pct float64)) error {
	ref := ParseRef(name)

	if err := m.Init(); err != nil {
		return err
	}

	if progress != nil {
		progress("resolving "+ref.String(), 0)
	}

	// Check if already exists
	exists, err := m.HasLocal(ref)
	if err != nil {
		return err
	}
	if exists {
		if progress != nil {
			progress("already exists", 100)
		}
		return nil
	}

	if progress != nil {
		progress("creating placeholder model", 50)
	}

	// Phase 0: Create a placeholder GGUF-like blob
	blobContent := []byte(fmt.Sprintf("TUTU-PLACEHOLDER-MODEL:%s\n", ref.String()))
	digest := "sha256:" + computeSHA256(blobContent)

	blobPath := m.BlobPath(digest)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(blobPath, blobContent, 0o644); err != nil {
		return err
	}

	// Create manifest
	manifest := domain.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.tutu.manifest.v1+json",
		Layers: []domain.Layer{
			{
				MediaType: "application/vnd.tutu.model",
				Digest:    digest,
				Size:      int64(len(blobContent)),
			},
		},
	}

	if err := m.saveManifest(ref, manifest); err != nil {
		return err
	}

	// Store in DB
	now := time.Now()
	info := domain.ModelInfo{
		Name:         ref.String(),
		SizeBytes:    int64(len(blobContent)),
		Digest:       digest,
		PulledAt:     now,
		Quantization: "Q4_K_M",
		Format:       "gguf",
	}
	if err := m.db.UpsertModel(info); err != nil {
		return err
	}

	if progress != nil {
		progress("done", 100)
	}
	return nil
}

// CreateFromTuTufile creates a model from a TuTufile.
func (m *Manager) CreateFromTuTufile(name string, tf domain.TuTufile) error {
	ref := ParseRef(name)

	if err := m.Init(); err != nil {
		return err
	}

	// Use the base model if FROM is specified (for now, just record it)
	blobContent := []byte(fmt.Sprintf("TUTU-CUSTOM-MODEL:%s:FROM:%s\n", ref.String(), tf.From))
	digest := "sha256:" + computeSHA256(blobContent)

	blobPath := m.BlobPath(digest)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(blobPath, blobContent, 0o644); err != nil {
		return err
	}

	layers := []domain.Layer{
		{
			MediaType: "application/vnd.tutu.model",
			Digest:    digest,
			Size:      int64(len(blobContent)),
		},
	}

	// Store system prompt as a layer if present
	if tf.System != "" {
		sysContent := []byte(tf.System)
		sysDigest := "sha256:" + computeSHA256(sysContent)
		sysPath := m.BlobPath(sysDigest)
		if err := os.WriteFile(sysPath, sysContent, 0o644); err != nil {
			return err
		}
		layers = append(layers, domain.Layer{
			MediaType: "application/vnd.tutu.system-prompt",
			Digest:    sysDigest,
			Size:      int64(len(sysContent)),
		})
	}

	manifest := domain.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.tutu.manifest.v1+json",
		Layers:        layers,
	}

	if err := m.saveManifest(ref, manifest); err != nil {
		return err
	}

	totalSize := int64(0)
	for _, l := range layers {
		totalSize += l.Size
	}

	now := time.Now()
	info := domain.ModelInfo{
		Name:      ref.String(),
		SizeBytes: totalSize,
		Digest:    digest,
		PulledAt:  now,
		Format:    "gguf",
	}
	return m.db.UpsertModel(info)
}

// --- Internal helpers ---

func (m *Manager) loadManifest(ref domain.ModelRef) (domain.Manifest, error) {
	mpath := m.ManifestPath(ref)
	data, err := os.ReadFile(mpath)
	if err != nil {
		return domain.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest domain.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return domain.Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func (m *Manager) saveManifest(ref domain.ModelRef, manifest domain.Manifest) error {
	mpath := m.ManifestPath(ref)
	if err := os.MkdirAll(filepath.Dir(mpath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mpath, data, 0o644)
}

// ParseRef parses a "name:tag" string into a ModelRef.
func ParseRef(s string) domain.ModelRef {
	parts := strings.SplitN(s, ":", 2)
	ref := domain.ModelRef{Name: parts[0]}
	if len(parts) == 2 {
		ref.Tag = parts[1]
	} else {
		ref.Tag = "latest"
	}
	return ref
}

func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
