package main

// scipq — deterministic code-graph queries over any SCIP index.
// v0 skeleton: command routing + exit-code contract only. Verbs land per issue.

import (
	"fmt"
	"os"
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

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(exitUsage)
	}
	verb := os.Args[1]
	// v0: verbs are stubs; wire the missing-index exit path now so the
	// contract is testable before any verb logic lands.
	_ = verb
	fmt.Fprintln(os.Stderr, "scipq: verbs not implemented yet (see github.com/elodhorvath/scipq/issues)")
	os.Exit(exitNoIndex)
}
