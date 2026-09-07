package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ac0d3r/grabkernel2xpf/internal/external"
	"github.com/ac0d3r/grabkernel2xpf/internal/library"
)

const maxBatchItems = 100

type batchReq struct {
	Items        []extractReq `json:"items"`
	SkipExisting bool         `json:"skipExisting"`
}

func normalizeExtract(req *extractReq) error {
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

func (a *App) extractOne(ctx context.Context, req extractReq, progress func(stage, msg string)) (*extractResult, error) {
	if err := normalizeExtract(&req); err != nil {
		return nil, err
	}
	dir, err := a.store.Dir(req.Identifier, req.Build, req.Board)
	if err != nil {
		return nil, err
	}

	var result extractResult
	err = a.store.WithLock(dir, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cached := a.store.HasKernel(dir) && !req.Refresh
		m := library.Manifest{
			OS:         req.OS,
			Build:      req.Build,
			Version:    req.Version,
			Identifier: req.Identifier,
			Board:      req.Board,
		}
		if existing, err := a.store.ReadManifest(dir); err == nil {
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
			if err := a.tools.Grab(ctx, req.OS, req.Build, req.Identifier, req.Board, dir); err != nil {
				return err
			}
			m.DownloadedAt = ""
			m.ClearExtract()
		}
		if !a.store.HasKernel(dir) {
			return fmt.Errorf("kernelcache was not written to %s", dir)
		}
		if err := a.store.WriteManifest(dir, m); err != nil {
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
			kernel := a.store.KernelPath(dir)
			sptm, txm := "", ""
			if library.FileExists(a.store.SPTMPath(dir)) {
				sptm = a.store.SPTMPath(dir)
			}
			if library.FileExists(a.store.TXMPath(dir)) {
				txm = a.store.TXMPath(dir)
			}
			progress("extract", "Running xpf_test")
			var err error
			xr, err = a.tools.XPF(kernel, sptm, txm)
			if err != nil {
				return err
			}
			m.KernelVersion = xr.KernelVersion
			m.KernelBase = xr.KernelBase
			m.KernelEntry = xr.KernelEntry
			m.Offsets = xr.Offsets
			if err := a.store.WriteManifest(dir, m); err != nil {
				return err
			}
		}
		manifest, err := a.store.ReadManifest(dir)
		if err != nil {
			return err
		}
		result = extractResult{
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

func startSSE(w http.ResponseWriter) func(event string, v any) {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	return func(event string, v any) {
		raw, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (a *App) handleExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req extractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := normalizeExtract(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := a.store.Dir(req.Identifier, req.Build, req.Board); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	send := startSSE(w)
	progress := func(stage, msg string) {
		log.Printf("%s: %s", stage, msg)
		send("progress", map[string]string{"stage": stage, "message": msg})
	}

	a.extractMu.Lock()
	result, err := a.extractOne(r.Context(), req, progress)
	a.extractMu.Unlock()
	if err != nil {
		send("error", map[string]string{"error": err.Error()})
		return
	}
	progress("done", fmt.Sprintf("Resolved %d offsets", len(result.Offsets)))
	send("result", result)
}

func (a *App) handleExtractBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req batchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "items are required")
		return
	}
	if len(req.Items) > maxBatchItems {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("at most %d items", maxBatchItems))
		return
	}

	items := make([]extractReq, 0, len(req.Items))
	seen := map[string]bool{}
	for i := range req.Items {
		item := req.Items[i]
		if err := normalizeExtract(&item); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("item %d: %s", i, err.Error()))
			return
		}
		if _, err := a.store.Dir(item.Identifier, item.Build, item.Board); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("item %d: %s", i, err.Error()))
			return
		}
		key := item.Identifier + "|" + item.Build + "|" + item.Board
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, item)
	}

	send := startSSE(w)
	ctx := r.Context()
	total := len(items)
	ok, skipped, failed := 0, 0, 0

	a.extractMu.Lock()
	defer a.extractMu.Unlock()

	for i, item := range items {
		if err := ctx.Err(); err != nil {
			send("error", map[string]string{"error": err.Error()})
			return
		}
		label := fmt.Sprintf("%s %s (%s)", item.Identifier, item.Build, item.Board)
		send("item", map[string]any{
			"index":      i,
			"total":      total,
			"os":         item.OS,
			"identifier": item.Identifier,
			"build":      item.Build,
			"board":      item.Board,
			"version":    item.Version,
			"status":     "start",
		})
		progress := func(stage, msg string) {
			log.Printf("[%d/%d] %s: %s", i+1, total, stage, msg)
			send("progress", map[string]any{
				"index":      i,
				"total":      total,
				"stage":      stage,
				"message":    msg,
				"identifier": item.Identifier,
				"build":      item.Build,
				"board":      item.Board,
			})
		}

		if req.SkipExisting && !item.Refresh {
			dir, err := a.store.Dir(item.Identifier, item.Build, item.Board)
			if err == nil {
				if m, err := a.store.ReadManifest(dir); err == nil && m.HasOffsets() {
					skipped++
					n := len(m.Offsets)
					progress("skip", fmt.Sprintf("Already extracted %d offsets for %s", n, label))
					send("item", map[string]any{
						"index":      i,
						"total":      total,
						"identifier": item.Identifier,
						"build":      item.Build,
						"board":      item.Board,
						"status":     "skip",
						"result": extractResult{
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

		result, err := a.extractOne(ctx, item, progress)
		if err != nil {
			failed++
			log.Printf("[%d/%d] error: %s: %s", i+1, total, label, err)
			send("item", map[string]any{
				"index":      i,
				"total":      total,
				"identifier": item.Identifier,
				"build":      item.Build,
				"board":      item.Board,
				"status":     "error",
				"error":      err.Error(),
			})
			continue
		}
		ok++
		progress("done", fmt.Sprintf("Resolved %d offsets", len(result.Offsets)))
		send("item", map[string]any{
			"index":      i,
			"total":      total,
			"identifier": item.Identifier,
			"build":      item.Build,
			"board":      item.Board,
			"status":     "ok",
			"result":     result,
		})
	}

	send("done", map[string]any{
		"total":   total,
		"ok":      ok,
		"skipped": skipped,
		"failed":  failed,
	})
}
