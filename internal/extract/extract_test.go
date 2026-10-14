package extract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ac0d3r/grabkernel2xpf/internal/external"
	"github.com/ac0d3r/grabkernel2xpf/internal/library"
)

func TestBatchSkipsFinished(t *testing.T) {
	root := t.TempDir()
	store := library.New(root)
	dir, err := store.Dir("iPhone12,1", "19A346", "n104ap")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kernelcache"), []byte("kernel"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := library.Manifest{
		OS:         "iOS",
		Identifier: "iPhone12,1",
		Build:      "19A346",
		Board:      "n104ap",
		Version:    "15.0",
		Offsets:    map[string]string{"kernelSymbol.gPhysBase": "0x1"},
	}
	if err := store.WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	sum := Run(context.Background(), store, external.New(t.TempDir()), []Request{{
		OS: "iOS", Identifier: "iPhone12,1", Build: "19A346", Board: "n104ap", Version: "15.0",
	}}, nil)
	if sum.Err != nil {
		t.Fatal(sum.Err)
	}
	if sum.Skipped != 1 || sum.OK != 0 || sum.Failed != 0 {
		t.Fatalf("%+v", sum)
	}
}

func TestRunCancelIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sum := Run(ctx, library.New(t.TempDir()), external.New(t.TempDir()), []Request{{
		OS: "iOS", Identifier: "iPhone12,1", Build: "19A346", Board: "n104ap", Version: "15.0",
	}}, nil)
	if !errors.Is(sum.Err, context.Canceled) {
		t.Fatalf("err=%v", sum.Err)
	}
	if sum.Failed != 0 || sum.OK != 0 || sum.Skipped != 0 {
		t.Fatalf("%+v", sum)
	}
}
