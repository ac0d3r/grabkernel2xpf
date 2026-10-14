package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ac0d3r/grabkernel2xpf/internal/appledb"
	"github.com/ac0d3r/grabkernel2xpf/internal/external"
	"github.com/ac0d3r/grabkernel2xpf/internal/extract"
	"github.com/ac0d3r/grabkernel2xpf/internal/library"
)

const usageText = `usage: grabkernel2xpf <command> [flags]

  extract   download kernelcaches and extract offsets
  devices   list device identifiers
  export    write extracted offsets as a plist

grabkernel2xpf extract --os iOS --identifier DEVICE --version SPEC

  --os           iOS or iPadOS (default iOS)
  --identifier   one product identifier or name
                 "iPhone 8" includes CDMA and GSM, not iPhone 8 Plus
                 "iPhone XS Max" includes the China mainland model
  --version      one inclusive release range
                 15-18         15.0.0 through every 18.x
                 15.0-18.7.1   through 18.7.1 exactly
                 26            every 26.x, including 26.0.1
                 16.7          every 16.7.x
                 The device only contributes builds it shipped, so a phone
                 that stopped at 16.7.x yields that last 16.7.x and nothing newer.
  --data         library root (default data)
  --external     directory with grabkernel and xpf_test (default external/_bin)
  --refresh      redownload and re-extract
  --dry-run      print the selected builds and exit

Board config is chosen automatically. Beta and RC builds are skipped.
Completed items are skipped unless --refresh is set.
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Fprint(os.Stdout, usageText)
		return
	}
	switch os.Args[1] {
	case "extract":
		os.Exit(cmdExtract(os.Args[2:]))
	case "devices":
		os.Exit(cmdDevices(os.Args[2:]))
	case "export":
		os.Exit(cmdExport(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usageText)
		os.Exit(2)
	}
}

func cmdExtract(args []string) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usageText) }
	osName := fs.String("os", "iOS", "iOS or iPadOS")
	id := fs.String("identifier", "", "one product identifier or name")
	version := fs.String("version", "", "one inclusive release range")
	dataDir := fs.String("data", "data", "library root")
	externalDir := fs.String("external", "external/_bin", "grabkernel and xpf_test directory")
	refresh := fs.Bool("refresh", false, "redownload and re-extract")
	dry := fs.Bool("dry-run", false, "print the selected builds and exit")
	if err := fs.Parse(args); err != nil {
		return flagStatus(err)
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *osName != "iOS" && *osName != "iPadOS" {
		fmt.Fprintf(os.Stderr, "os must be iOS or iPadOS\n")
		return 2
	}
	identifier := strings.TrimSpace(*id)
	spec := strings.TrimSpace(*version)
	if identifier == "" || spec == "" {
		fs.Usage()
		return 2
	}
	r, err := appledb.ParseVersionRange(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}

	db := appledb.New()
	devices, err := db.Devices(*osName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	items, missing, err := extract.Select(devices, []string{identifier}, *osName, []appledb.VersionRange{r}, db.Builds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	for _, missingID := range missing {
		fmt.Fprintf(os.Stderr, "%s: no release builds in range\n", missingID)
	}
	if len(items) == 0 {
		if len(missing) == 0 {
			fmt.Fprintln(os.Stderr, "no release builds in range")
		}
		return 1
	}
	for i := range items {
		items[i].Refresh = *refresh
	}
	if *dry {
		for _, item := range items {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", item.Identifier, item.Name, item.Version, item.Build, item.Board)
		}
		fmt.Fprintf(os.Stderr, "%d builds\n", len(items))
		return 0
	}

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	data := abs(root, *dataDir)
	binDir := abs(root, *externalDir)
	tools := external.New(binDir)
	if err := tools.Ready(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sum := extract.Run(ctx, library.New(data), tools, items, func(ev extract.Event) {
		prefix := fmt.Sprintf("[%d/%d] ", ev.Index+1, ev.Total)
		switch ev.Kind {
		case "start":
			fmt.Fprintf(os.Stderr, "%sstart  %s %s %s (%s)\n", prefix, ev.Item.Identifier, ev.Item.Version, ev.Item.Build, ev.Item.Board)
		case "progress":
			fmt.Fprintf(os.Stderr, "%s%s  %s\n", prefix, ev.Stage, ev.Message)
		case "error":
			fmt.Fprintf(os.Stderr, "%serror  %s\n", prefix, ev.Message)
		}
	})
	fmt.Fprintf(os.Stderr, "done  ok=%d skipped=%d failed=%d\n", sum.OK, sum.Skipped, sum.Failed)
	if sum.Err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", sum.Err)
		return 1
	}
	if sum.Failed > 0 {
		return 1
	}
	return 0
}

func cmdDevices(args []string) int {
	fs := flag.NewFlagSet("devices", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	osName := fs.String("os", "iOS", "iOS or iPadOS")
	if err := fs.Parse(args); err != nil {
		return flagStatus(err)
	}
	if *osName != "iOS" && *osName != "iPadOS" {
		fmt.Fprintf(os.Stderr, "os must be iOS or iPadOS\n")
		return 2
	}
	devs, err := appledb.New().Devices(*osName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	for _, d := range devs {
		fmt.Printf("%s\t%s\t%s\n", d.Identifier, d.Name, strings.Join(d.Boards, ","))
	}
	return 0
}

func cmdExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data", "data", "library root")
	outPath := fs.String("out", "offsets.plist", "output plist, or - for stdout")
	if err := fs.Parse(args); err != nil {
		return flagStatus(err)
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	items, err := library.New(abs(root, *dataDir)).List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	body := library.OffsetsPlist(items)
	if *outPath == "-" {
		_, err = io.WriteString(os.Stdout, body)
	} else {
		err = os.WriteFile(abs(root, *outPath), []byte(body), 0o644)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	return 0
}

func flagStatus(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}

func abs(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}
