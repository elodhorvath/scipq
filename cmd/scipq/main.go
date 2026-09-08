package main

// scipq — deterministic code-graph queries over any SCIP index.
// v0 skeleton: command routing + exit-code contract only. Verbs land per issue.

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/elodhorvath/scipq/internal/index"
)

const (
	exitOK      = 0 // success
	exitUsage   = 1 // bad arguments
	exitNoIndex = 2 // index file missing/unreadable
)

func usage(w *os.File) {
	fmt.Fprintln(w, "scipq — code-graph queries over a SCIP index")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "usage: scipq <verb> [args] [--json]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "verbs: map, callers, blast, skeleton, dead")
	fmt.Fprintln(w, "run 'scipq <verb> -h' for verb help")
}

// resolveIndexPath determines the index file to load: the --index flag if
// set, otherwise ./index.scip in the current directory.
func resolveIndexPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return "./index.scip"
}

// loadIndex resolves and loads the SCIP index, printing an error and
// exiting with the missing-index code on failure.
func loadIndex(flagValue string) *index.ReverseIndex {
	path := resolveIndexPath(flagValue)
	ri, err := index.Load(path)
	if err != nil {
		if errors.Is(err, index.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "scipq: no index at %s (pass --index <path> or place index.scip in the current directory)\n", path)
			os.Exit(exitNoIndex)
		}
		fmt.Fprintf(os.Stderr, "scipq: %v\n", err)
		os.Exit(exitNoIndex)
	}
	return ri
}

func main() {
	fs := flag.NewFlagSet("scipq", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	indexPath := fs.String("index", "", "path to the SCIP index (default ./index.scip)")

	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(exitUsage)
	}
	verb := os.Args[1]
	// v0: verbs are stubs; the index load path is wired so the exit-code
	// contract is testable before any verb logic lands.
	_ = verb
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(exitUsage)
	}
	_ = loadIndex(*indexPath)
	fmt.Fprintln(os.Stderr, "scipq: verbs not implemented yet (see github.com/elodhorvath/scipq/issues)")
	os.Exit(exitOK)
}
