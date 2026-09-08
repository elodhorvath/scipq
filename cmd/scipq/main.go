package main

// scipq — deterministic code-graph queries over any SCIP index.
// v0 skeleton: command routing + exit-code contract only. Verbs land per issue.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elodhorvath/scipq/internal/index"
)

const (
	exitOK      = 0 // success
	exitUsage   = 1 // bad arguments
	exitNoIndex = 2 // index file missing/unreadable
)

// stdout and stderr are variables so tests can capture verb output.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
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
			fmt.Fprintf(stderr, "scipq: no index at %s (pass --index <path> or place index.scip in the current directory)\n", path)
			os.Exit(exitNoIndex)
		}
		fmt.Fprintf(stderr, "scipq: %v\n", err)
		os.Exit(exitNoIndex)
	}
	return ri
}

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(exitUsage)
	}
	verb := os.Args[1]
	args := os.Args[2:]

	// --index is a global flag; extract it manually so the verb's FlagSet
	// only sees its own flags (a FlagSet stops at the first unknown flag,
	// which would strand later flags in Args).
	indexPath, rest, err := extractStringFlag(args, "index")
	if err != nil {
		fmt.Fprintf(stderr, "scipq: %v\n", err)
		os.Exit(exitUsage)
	}
	ri := loadIndex(indexPath)

	switch verb {
	case "map":
		os.Exit(runMap(rest, ri, hasJSONFlag(rest)))
	case "callers", "blast", "skeleton", "dead":
		fmt.Fprintf(os.Stderr, "scipq: verb %q not implemented yet (see github.com/elodhorvath/scipq/issues)\n", verb)
		os.Exit(exitUsage)
	default:
		usage(os.Stderr)
		os.Exit(exitUsage)
	}
}

// extractStringFlag removes "--flag value" (or "--flag=value") from args and
// returns the flag's value plus the remaining arguments.
func extractStringFlag(args []string, name string) (value string, rest []string, err error) {
	prefix := "--" + name + "="
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--"+name:
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("flag --%s requires a value", name)
			}
			return args[i+1], append(args[:i:i], args[i+2:]...), nil
		case strings.HasPrefix(args[i], prefix):
			return strings.TrimPrefix(args[i], prefix), append(args[:i:i], args[i+1:]...), nil
		}
	}
	return "", args, nil
}

// hasJSONFlag reports whether the verb args request --json output.
func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" {
			return true
		}
	}
	return false
}
