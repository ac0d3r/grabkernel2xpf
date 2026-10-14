package external

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	reStart = regexp.MustCompile(`^Starting XPF with .+ \((.+)\)$`)
	reBase  = regexp.MustCompile(`^Kernel base: (0x[0-9a-fA-F]+)$`)
	reEntry = regexp.MustCompile(`^Kernel entry: (0x[0-9a-fA-F]+)$`)
	reOff   = regexp.MustCompile(`^(0x[0-9a-fA-F]+) <- (.+)$`)
	reErr   = regexp.MustCompile(`^(?:XPF Error|Failed to start XPF): (.+)$`)
	reTime  = regexp.MustCompile(`^XPF finished in ([0-9.]+) seconds$`)

	xpfMu sync.Mutex
)

type Tools struct {
	Dir string
}

func New(dir string) *Tools {
	return &Tools{Dir: dir}
}

func (t *Tools) Ready() error {
	if _, err := t.bin("grabkernel"); err != nil {
		return err
	}
	if _, err := t.bin("xpf_test"); err != nil {
		return err
	}
	return nil
}

func (t *Tools) bin(name string) (string, error) {
	p := filepath.Join(t.Dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return p, nil
}

func (t *Tools) Grab(ctx context.Context, osName, build, identifier, board, outDir string) error {
	bin, err := t.bin("grabkernel")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin,
		"--os", osName,
		"--build", build,
		"--identifier", identifier,
		"--board", board,
		"--out", outDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("grabkernel: %s", msg)
		}
		return fmt.Errorf("grabkernel: %w", err)
	}
	return nil
}

type XPFResult struct {
	KernelVersion string            `json:"kernelVersion,omitempty"`
	KernelBase    string            `json:"kernelBase,omitempty"`
	KernelEntry   string            `json:"kernelEntry,omitempty"`
	Offsets       map[string]string `json:"offsets"`
	ElapsedSec    string            `json:"elapsedSec,omitempty"`
	Output        string            `json:"-"`
}

func (t *Tools) XPF(ctx context.Context, kernel, sptm, txm string) (*XPFResult, error) {
	bin, err := t.bin("xpf_test")
	if err != nil {
		return nil, err
	}
	args := []string{kernel}
	if sptm != "" {
		args = append(args, sptm)
	}
	if txm != "" {
		args = append(args, txm)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	xpfMu.Lock()
	defer xpfMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := buf.String()
	res, parseErr := ParseXPF(out)
	if runErr != nil {
		if strings.TrimSpace(res.KernelVersion) != "" {
			return res, fmt.Errorf("xpf_test: %w", runErr)
		}
		if msg := xpfErrorLine(out); msg != "" {
			return res, errors.New(msg)
		}
		return res, fmt.Errorf("xpf_test: %w\n%s", runErr, strings.TrimSpace(out))
	}
	if parseErr != nil {
		return res, parseErr
	}
	if len(res.Offsets) == 0 {
		if msg := xpfErrorLine(out); msg != "" {
			return res, errors.New(msg)
		}
		return res, errors.New("xpf_test produced no offsets")
	}
	return res, nil
}

func xpfErrorLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if m := reErr.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1]
		}
	}
	return ""
}

func ParseXPF(out string) (*XPFResult, error) {
	res := &XPFResult{Offsets: map[string]string{}, Output: out}
	var xpfErr string
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case reStart.MatchString(line):
			res.KernelVersion = reStart.FindStringSubmatch(line)[1]
		case reBase.MatchString(line):
			res.KernelBase = reBase.FindStringSubmatch(line)[1]
		case reEntry.MatchString(line):
			res.KernelEntry = reEntry.FindStringSubmatch(line)[1]
		case reOff.MatchString(line):
			m := reOff.FindStringSubmatch(line)
			res.Offsets[m[2]] = strings.ToLower(m[1])
		case reErr.MatchString(line):
			xpfErr = reErr.FindStringSubmatch(line)[1]
		case reTime.MatchString(line):
			res.ElapsedSec = reTime.FindStringSubmatch(line)[1]
		}
	}
	if xpfErr != "" && len(res.Offsets) == 0 {
		return res, errors.New(xpfErr)
	}
	return res, nil
}
