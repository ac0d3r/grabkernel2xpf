package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var segmentRe = regexp.MustCompile(`^[A-Za-z0-9._,+-]+$`)

type FileInfo struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	OS            string              `json:"os"`
	Build         string              `json:"build"`
	Version       string              `json:"version,omitempty"`
	Identifier    string              `json:"identifier"`
	Board         string              `json:"board"`
	FirmwareURL   string              `json:"firmwareURL,omitempty"`
	IsOTA         bool                `json:"isOTA,omitempty"`
	DownloadedAt  string              `json:"downloadedAt"`
	KernelVersion string              `json:"kernelVersion,omitempty"`
	KernelBase    string              `json:"kernelBase,omitempty"`
	KernelEntry   string              `json:"kernelEntry,omitempty"`
	Files         map[string]FileInfo `json:"files"`
	Offsets       map[string]string   `json:"offsets,omitempty"`
}

type Store struct {
	Root string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func New(root string) *Store {
	return &Store{Root: root, locks: make(map[string]*sync.Mutex)}
}

func NormalizeBoard(board string) string {
	return strings.ToLower(strings.TrimSpace(board))
}

func ValidateSegment(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if !segmentRe.MatchString(value) {
		return fmt.Errorf("invalid %s %q", name, value)
	}
	return nil
}

func (s *Store) Dir(identifier, build, board string) (string, error) {
	if err := ValidateSegment("identifier", identifier); err != nil {
		return "", err
	}
	if err := ValidateSegment("build", build); err != nil {
		return "", err
	}
	board = NormalizeBoard(board)
	if err := ValidateSegment("board", board); err != nil {
		return "", err
	}
	return filepath.Join(s.Root, identifier, build, board), nil
}

func (s *Store) lock(dir string) func() {
	s.mu.Lock()
	m, ok := s.locks[dir]
	if !ok {
		m = &sync.Mutex{}
		s.locks[dir] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (s *Store) WithLock(dir string, fn func() error) error {
	unlock := s.lock(dir)
	defer unlock()
	return fn()
}

func (s *Store) KernelPath(dir string) string { return filepath.Join(dir, "kernelcache") }
func (s *Store) SPTMPath(dir string) string   { return filepath.Join(dir, "sptm.im4p") }
func (s *Store) TXMPath(dir string) string    { return filepath.Join(dir, "txm.im4p") }
func (s *Store) ManifestPath(dir string) string {
	return filepath.Join(dir, "MANIFEST.json")
}

func FileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func HashFile(path string) (FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileInfo{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return FileInfo{}, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return FileInfo{}, err
	}
	return FileInfo{SHA256: hex.EncodeToString(h.Sum(nil)), Size: st.Size()}, nil
}

func (s *Store) HasKernel(dir string) bool {
	return FileExists(s.KernelPath(dir))
}

func (m *Manifest) HasOffsets() bool {
	return m != nil && len(m.Offsets) > 0
}

func (m *Manifest) ClearExtract() {
	if m == nil {
		return
	}
	m.KernelVersion = ""
	m.KernelBase = ""
	m.KernelEntry = ""
	m.Offsets = nil
}

func (s *Store) WriteManifest(dir string, m Manifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]FileInfo{}
	type named struct {
		key, path string
	}
	for _, n := range []named{
		{"kernelcache", s.KernelPath(dir)},
		{"sptm", s.SPTMPath(dir)},
		{"txm", s.TXMPath(dir)},
	} {
		if !FileExists(n.path) {
			continue
		}
		info, err := HashFile(n.path)
		if err != nil {
			return err
		}
		files[n.key] = info
	}
	if _, ok := files["kernelcache"]; !ok {
		return fmt.Errorf("kernelcache missing in %s", dir)
	}
	if m.DownloadedAt == "" {
		m.DownloadedAt = time.Now().UTC().Format(time.RFC3339)
	}
	m.Board = NormalizeBoard(m.Board)
	m.Files = files

	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := s.ManifestPath(dir) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.ManifestPath(dir))
}

func (s *Store) ReadManifest(dir string) (*Manifest, error) {
	raw, err := os.ReadFile(s.ManifestPath(dir))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) Get(identifier, build, board string) (*Manifest, error) {
	dir, err := s.Dir(identifier, build, board)
	if err != nil {
		return nil, err
	}
	return s.ReadManifest(dir)
}

func (s *Store) List() ([]Manifest, error) {
	var out []Manifest
	if _, err := os.Stat(s.Root); os.IsNotExist(err) {
		return out, nil
	}
	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "MANIFEST.json" {
			return nil
		}
		m, err := s.ReadManifest(filepath.Dir(path))
		if err != nil {
			return nil
		}
		out = append(out, *m)
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].DownloadedAt != out[j].DownloadedAt {
			return out[i].DownloadedAt > out[j].DownloadedAt
		}
		if out[i].Identifier != out[j].Identifier {
			return out[i].Identifier < out[j].Identifier
		}
		if out[i].Build != out[j].Build {
			return out[i].Build > out[j].Build
		}
		return out[i].Board < out[j].Board
	})
	return out, err
}
