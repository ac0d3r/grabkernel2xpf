package extract

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ac0d3r/grabkernel2xpf/internal/external"
	"github.com/ac0d3r/grabkernel2xpf/internal/library"
)

type Result struct {
	Cached        bool
	Manifest      *library.Manifest
	KernelVersion string
	KernelBase    string
	KernelEntry   string
	Offsets       map[string]string
	ElapsedSec    string
}

type Event struct {
	Index   int
	Total   int
	Kind    string
	Stage   string
	Message string
	Item    Request
	Result  *Result
}

type Summary struct {
	OK      int
	Skipped int
	Failed  int
	Err     error
}

func Run(ctx context.Context, store *library.Store, tools *external.Tools, items []Request, progress func(Event)) Summary {
	if progress == nil {
		progress = func(Event) {}
	}
	var sum Summary
	total := len(items)
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			sum.Err = err
			return sum
		}
		label := fmt.Sprintf("%s %s (%s)", item.Identifier, item.Build, item.Board)
		progress(Event{Index: i, Total: total, Kind: "start", Item: item})

		if !item.Refresh {
			dir, err := store.Dir(item.Identifier, item.Build, item.Board)
			if err == nil {
				if m, err := store.ReadManifest(dir); err == nil && m.HasOffsets() {
					sum.Skipped++
					progress(Event{
						Index: i, Total: total, Kind: "progress", Stage: "skip", Item: item,
						Message: fmt.Sprintf("Already extracted %d offsets for %s", len(m.Offsets), label),
						Result: &Result{
							Cached:        true,
							Manifest:      m,
							KernelVersion: m.KernelVersion,
							KernelBase:    m.KernelBase,
							KernelEntry:   m.KernelEntry,
							Offsets:       m.Offsets,
						},
					})
					continue
				}
			}
		}

		result, err := one(ctx, store, tools, item, func(stage, msg string) {
			progress(Event{Index: i, Total: total, Kind: "progress", Stage: stage, Message: msg, Item: item})
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				sum.Err = ctxErr
				return sum
			}
			sum.Failed++
			progress(Event{Index: i, Total: total, Kind: "error", Message: err.Error(), Item: item})
			continue
		}
		sum.OK++
		progress(Event{
			Index: i, Total: total, Kind: "progress", Stage: "done", Item: item, Result: result,
			Message: fmt.Sprintf("Resolved %d offsets", len(result.Offsets)),
		})
	}
	return sum
}

func one(ctx context.Context, store *library.Store, tools *external.Tools, req Request, progress func(stage, msg string)) (*Result, error) {
	if err := normalize(&req); err != nil {
		return nil, err
	}
	dir, err := store.Dir(req.Identifier, req.Build, req.Board)
	if err != nil {
		return nil, err
	}

	var result Result
	err = store.WithLock(dir, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cached := store.HasKernel(dir) && !req.Refresh
		m := library.Manifest{
			OS:         req.OS,
			Build:      req.Build,
			Version:    req.Version,
			Identifier: req.Identifier,
			Board:      req.Board,
		}
		if existing, err := store.ReadManifest(dir); err == nil {
			m = *existing
			if req.Version != "" {
				m.Version = req.Version
			}
		}
		if cached {
			progress("download", "Using cached kernelcache")
		} else {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			progress("download", fmt.Sprintf("Downloading images for %s %s (%s)", req.Identifier, req.Build, req.Board))
			if err := tools.Grab(ctx, req.OS, req.Build, req.Identifier, req.Board, dir); err != nil {
				return err
			}
			m.DownloadedAt = ""
			m.ClearExtract()
		}
		if !store.HasKernel(dir) {
			return fmt.Errorf("kernelcache was not written to %s", dir)
		}
		if err := store.WriteManifest(dir, m); err != nil {
			return err
		}

		var xr *external.XPFResult
		if cached && m.HasOffsets() {
			progress("extract", "Using cached offsets")
			xr = &external.XPFResult{
				KernelVersion: m.KernelVersion,
				KernelBase:    m.KernelBase,
				KernelEntry:   m.KernelEntry,
				Offsets:       m.Offsets,
			}
		} else {
			kernel := store.KernelPath(dir)
			sptm, txm := "", ""
			if library.FileExists(store.SPTMPath(dir)) {
				sptm = store.SPTMPath(dir)
			}
			if library.FileExists(store.TXMPath(dir)) {
				txm = store.TXMPath(dir)
			}
			progress("extract", "Running xpf_test")
			var err error
			xr, err = tools.XPF(ctx, kernel, sptm, txm)
			if err != nil {
				return err
			}
			m.KernelVersion = xr.KernelVersion
			m.KernelBase = xr.KernelBase
			m.KernelEntry = xr.KernelEntry
			m.Offsets = xr.Offsets
			if err := store.WriteManifest(dir, m); err != nil {
				return err
			}
		}
		manifest, err := store.ReadManifest(dir)
		if err != nil {
			return err
		}
		result = Result{
			Cached:        cached,
			Manifest:      manifest,
			KernelVersion: xr.KernelVersion,
			KernelBase:    xr.KernelBase,
			KernelEntry:   xr.KernelEntry,
			Offsets:       xr.Offsets,
			ElapsedSec:    xr.ElapsedSec,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func normalize(req *Request) error {
	req.OS = strings.TrimSpace(req.OS)
	req.Identifier = strings.TrimSpace(req.Identifier)
	req.Build = strings.TrimSpace(req.Build)
	req.Board = library.NormalizeBoard(req.Board)
	req.Version = strings.TrimSpace(req.Version)
	if req.OS == "" || req.Identifier == "" || req.Build == "" || req.Board == "" {
		return fmt.Errorf("os, identifier, build, and board are required")
	}
	return nil
}
