package main

// scipq — deterministic code-graph queries over any SCIP index.
// Routing is built on urfave/cli v3 (spike for issue #25): --index and
// --json are persistent root flags, so they parse in any position without
// manual extraction. The exit-code contract (0 ok / 1 usage / 2 missing
// index) and the testable run(argv) seam are preserved.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/elodhorvath/scipq/internal/index"

	"github.com/urfave/cli/v3"
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

// exitError carries a scipq exit code through cli.Command.Run's error
// return. The root ExitErrHandler is a no-op, so the code surfaces to
// run() for mapping instead of the framework calling os.Exit.
type exitError struct{ code int }

func (e *exitError) Error() string {
	switch e.code {
	case exitUsage:
		return "usage error"
	case exitNoIndex:
		return "missing index"
	default:
		return fmt.Sprintf("exit %d", e.code)
	}
}

// indexLoader loads a SCIP index and reports the process exit code on
// failure (ri is nil then). Injectable so tests can stub index loading.
type indexLoader func(indexPath string) (*index.ReverseIndex, int)

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

// runError maps a cli.Command.Run error return to the process exit code:
// nil is success, our exitError carries its code, anything else is a
// usage error.
func runError(err error) int {
	if err == nil {
		return exitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return exitUsage
}

// run dispatches a scipq invocation (everything after the program name) and
// returns the process exit code. It is the testable seam for main: the
// framework's error return is mapped back to the 0/1/2 contract here, and
// the ExitErrHandler no-op keeps os.Exit out of the call path.
func run(argv []string) int {
	cmd := newRootCommand(loadIndex)
	// cli.Command.Run treats args[0] as the program name; run() receives
	// only the arguments after it.
	return runError(cmd.Run(context.Background(), append([]string{"scipq"}, argv...)))
}

// usageError prints a scipq-prefixed diagnostic and returns the usage exit
// code as an error, replacing the framework's default "Incorrect Usage"
// banner. Used as OnUsageError on the root and every verb.
func usageError(_ context.Context, _ *cli.Command, err error, _ bool) error {
	fmt.Fprintf(stderr, "scipq: %v\n", err)
	return &exitError{code: exitUsage}
}

// noVerbAction handles invocations with no verb (bare scipq, or flags
// only): print usage and exit 1. Unknown verbs also land here — v3.11.0
// dispatches them to the root action with the name as a positional arg.
func noVerbAction(_ context.Context, cmd *cli.Command) error {
	if args := cmd.Args(); args.Len() > 0 {
		fmt.Fprintf(stderr, "scipq: unknown verb %q\n", args.First())
	}
	usage(stderr)
	return &exitError{code: exitUsage}
}

// usage writes the verb list to w. The framework renders per-verb help
// (scipq <verb> -h); this covers the bare-invocation and unknown-verb
// cases.
func usage(w io.Writer) {
	fmt.Fprintln(w, "scipq — code-graph queries over a SCIP index")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "usage: scipq <verb> [args] [--json]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "verbs: map, callers, blast, skeleton, dead")
	fmt.Fprintln(w, "run 'scipq <verb> -h' for verb help")
}

// stubAction is the action for not-yet-implemented verbs.
func stubAction(verb string) cli.ActionFunc {
	return func(_ context.Context, _ *cli.Command) error {
		fmt.Fprintf(stderr, "scipq: verb %q not implemented yet (see github.com/elodhorvath/scipq/issues)\n", verb)
		return &exitError{code: exitUsage}
	}
}

// newRootCommand builds the scipq command tree. --index and --json are
// root flags with Local unset (false), which makes them persistent: they
// parse in any position and are readable from verb actions via lineage
// lookup. load is injected so tests can stub index loading.
func newRootCommand(load indexLoader) *cli.Command {
	return &cli.Command{
		Name:  "scipq",
		Usage: "code-graph queries over a SCIP index",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "index",
				Usage: "path to the SCIP index (default ./index.scip)",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "emit machine-readable JSON output",
			},
		},
		Commands: []*cli.Command{
			mapCommand(load),
			callersCommand(load),
			{
				Name:   "blast",
				Usage:  "not implemented yet",
				Action: stubAction("blast"),
			},
			{
				Name:   "skeleton",
				Usage:  "not implemented yet",
				Action: stubAction("skeleton"),
			},
			{
				Name:   "dead",
				Usage:  "not implemented yet",
				Action: stubAction("dead"),
			},
		},
		// Keep the verb list clean: no built-in "help" subcommand.
		HideHelpCommand: true,
		Writer:          stdout,
		ErrWriter:       stderr,
		// Route framework usage errors through our diagnostic prefix and
		// exit code instead of the default "Incorrect Usage" banner.
		OnUsageError: usageError,
		// Bare scipq (or an unknown verb) shows usage and exits 1.
		Action: noVerbAction,
		// Neutralize the framework's os.Exit-on-ExitCoder behavior so
		// Run returns the error and run() maps it to the exit code. This
		// is what keeps run(argv) int testable.
		ExitErrHandler: func(_ context.Context, _ *cli.Command, err error) {
			_ = err // handled by run() via Run's return value
		},
	}
}
