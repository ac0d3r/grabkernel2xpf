package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/ac0d3r/grabkernel2xpf/internal/appledb"
	"github.com/ac0d3r/grabkernel2xpf/internal/external"
	"github.com/ac0d3r/grabkernel2xpf/internal/library"
)

type Config struct {
	DataDir     string
	ExternalDir string
	Web         fs.FS
}

type App struct {
	store     *library.Store
	db        *appledb.Client
	tools     *external.Tools
	web       fs.FS
	extractMu sync.Mutex
}

type extractReq struct {
	OS         string `json:"os"`
	Identifier string `json:"identifier"`
	Build      string `json:"build"`
	Board      string `json:"board"`
	Version    string `json:"version"`
	Refresh    bool   `json:"refresh"`
}

type extractResult struct {
	Cached        bool              `json:"cached"`
	Manifest      *library.Manifest `json:"manifest"`
	KernelVersion string            `json:"kernelVersion,omitempty"`
	KernelBase    string            `json:"kernelBase,omitempty"`
	KernelEntry   string            `json:"kernelEntry,omitempty"`
	Offsets       map[string]string `json:"offsets"`
	ElapsedSec    string            `json:"elapsedSec,omitempty"`
}

func New(cfg Config) (*App, error) {
	return &App{
		store: library.New(cfg.DataDir),
		db:    appledb.New(),
		tools: external.New(cfg.ExternalDir),
		web:   cfg.Web,
	}, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices", a.handleDevices)
	mux.HandleFunc("/api/builds", a.handleBuilds)
	mux.HandleFunc("/api/library", a.handleLibrary)
	mux.HandleFunc("/api/extract", a.handleExtract)
	mux.HandleFunc("/api/extract/batch", a.handleExtractBatch)
	if a.web != nil {
		mux.Handle("/", http.FileServer(http.FS(a.web)))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (a *App) handleDevices(w http.ResponseWriter, r *http.Request) {
	osStr := r.URL.Query().Get("os")
	if osStr == "" {
		osStr = "iOS"
	}
	devs, err := a.db.Devices(osStr)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, devs)
}

func (a *App) handleBuilds(w http.ResponseWriter, r *http.Request) {
	osStr := r.URL.Query().Get("os")
	id := r.URL.Query().Get("identifier")
	if osStr == "" || id == "" {
		writeErr(w, http.StatusBadRequest, "os and identifier are required")
		return
	}
	builds, err := a.db.Builds(osStr, id)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, builds)
}

func (a *App) handleLibrary(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("identifier"))
	build := strings.TrimSpace(r.URL.Query().Get("build"))
	board := library.NormalizeBoard(r.URL.Query().Get("board"))
	if id != "" || build != "" || board != "" {
		if id == "" || build == "" || board == "" {
			writeErr(w, http.StatusBadRequest, "identifier, build, and board are required")
			return
		}
		m, err := a.store.Get(id, build, board)
		if err != nil {
			if os.IsNotExist(err) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, m)
		return
	}
	list, err := a.store.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}
