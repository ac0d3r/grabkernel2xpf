package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ac0d3r/grabkernel2xpf/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	dataDir := flag.String("data", "data", "library root (data/{identifier}/{build}/{board})")
	externalDir := flag.String("external", "external/_bin", "directory containing grabkernel and xpf_test")
	webDir := flag.String("web", "web", "static frontend directory")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	abs := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(root, p)
	}
	*dataDir = abs(*dataDir)
	*externalDir = abs(*externalDir)
	*webDir = abs(*webDir)
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	app, err := server.New(server.Config{
		DataDir:     *dataDir,
		ExternalDir: *externalDir,
		Web:         os.DirFS(*webDir),
	})
	if err != nil {
		log.Fatal(err)
	}

	s := &http.Server{
		Addr:              *addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on http://%s", *addr)
	log.Printf("data %s", *dataDir)
	log.Printf("external %s", *externalDir)
	log.Fatal(s.ListenAndServe())
}
