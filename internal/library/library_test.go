package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteManifestOffsets(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	dir := filepath.Join(root, "iPhone15,2", "21E219", "d83ap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.KernelPath(dir), []byte("kernel"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := Manifest{
		OS:            "iOS",
		Build:         "21E219",
		Version:       "17.4",
		Identifier:    "iPhone15,2",
		Board:         "D83AP",
		KernelVersion: "Darwin Kernel Version 23.4.0",
		KernelBase:    "0xFFFFFFF007004000",
		KernelEntry:   "0xFFFFFFF0070ABCDE",
		Offsets: map[string]string{
			"kernelSymbol.gPhysBase":   "0x0000000fffffc000",
			"kernelConstant.T1SZ_BOOT": "0x0000000000000019",
		},
	}
	if err := s.WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	got, err := s.ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Board != "d83ap" {
		t.Fatalf("board: %q", got.Board)
	}
	if got.KernelBase != m.KernelBase {
		t.Fatalf("kernelBase: %q", got.KernelBase)
	}
	if got.Offsets["kernelSymbol.gPhysBase"] != "0x0000000fffffc000" {
		t.Fatalf("offsets: %#v", got.Offsets)
	}
	if _, ok := got.Files["kernelcache"]; !ok {
		t.Fatalf("files: %#v", got.Files)
	}
	if !got.HasOffsets() {
		t.Fatal("expected offsets")
	}

	got.ClearExtract()
	if got.HasOffsets() || got.KernelVersion != "" {
		t.Fatalf("clear: %#v", got)
	}
	if err := s.WriteManifest(dir, *got); err != nil {
		t.Fatal(err)
	}
	cleared, err := s.ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.HasOffsets() {
		t.Fatalf("offsets persisted after clear: %#v", cleared.Offsets)
	}
}

func writeKernel(t *testing.T, s *Store, identifier, build, board string) string {
	t.Helper()
	dir, err := s.Dir(identifier, build, board)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.KernelPath(dir), []byte("kernel"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGetAndList(t *testing.T) {
	s := New(t.TempDir())
	older := writeKernel(t, s, "iPhone14,2", "20A", "n54ap")
	newer := writeKernel(t, s, "iPhone15,2", "21E219", "d83ap")
	if err := s.WriteManifest(older, Manifest{
		OS: "iOS", Identifier: "iPhone14,2", Build: "20A", Board: "n54ap",
		DownloadedAt: "2026-01-01T00:00:00Z",
		Offsets:      map[string]string{"a": "0x1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteManifest(newer, Manifest{
		OS: "iOS", Identifier: "iPhone15,2", Build: "21E219", Board: "d83ap",
		DownloadedAt: "2026-09-07T00:00:00Z",
		Offsets:      map[string]string{"b": "0x2"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("iPhone15,2", "21E219", "D83AP")
	if err != nil {
		t.Fatal(err)
	}
	if got.Offsets["b"] != "0x2" {
		t.Fatalf("get: %#v", got.Offsets)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Identifier != "iPhone15,2" {
		t.Fatalf("list: %#v", list)
	}
}
