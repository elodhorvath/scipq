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

func usage(w io.Writer) {
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

// loadIndex resolves and loads the SCIP index. On failure it prints the
// diagnostic and returns the exit code the process should use (the
// missing-index code); ri is nil then.
func loadIndex(flagValue string) (*index.ReverseIndex, int) {
	path := resolveIndexPath(flagValue)
	ri, err := index.Load(path)
	if err != nil {
		if errors.Is(err, index.ErrNotFound) {
			fmt.Fprintf(stderr, "scipq: no index at %s (pass --index <path> or place index.scip in the current directory)\n", path)
		} else {
			fmt.Fprintf(stderr, "scipq: %v\n", err)
		}
		return nil, exitNoIndex
	}
	return ri, exitOK
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches a scipq invocation (everything after the program name) and
// returns the process exit code. It is the testable seam for main.
func run(argv []string) int {
	// --index is a global flag: it may appear before or after the verb, so
	// it is extracted from the whole invocation. Extraction is manual
	// because a FlagSet stops at the first unknown flag, which would
	// strand later flags in Args.
	indexPath, rest, err := extractStringFlag(argv, "index")
	if err != nil {
		fmt.Fprintf(stderr, "scipq: %v\n", err)
		return exitUsage
	}
	if len(rest) == 0 {
		usage(stderr)
		return exitUsage
	}
	verb := rest[0]
	args := rest[1:]

	ri, code := loadIndex(indexPath)
	if code != exitOK {
		return code
	}

	switch verb {
	case "map":
		return runMap(args, ri, hasJSONFlag(args))
	case "callers":
		return runCallers(args, ri, hasJSONFlag(args))
	case "blast", "skeleton", "dead":
		fmt.Fprintf(stderr, "scipq: verb %q not implemented yet (see github.com/elodhorvath/scipq/issues)\n", verb)
		return exitUsage
	default:
		usage(stderr)
		return exitUsage
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
