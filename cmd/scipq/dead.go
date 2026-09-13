// dead verb — dead-code candidates: definitions with zero reference
// sites anywhere in the index. Test-only symbols (all refs from
// *_test.go documents) are listed with a marker, not dropped. Exported
// symbols are excluded by default — refs may exist outside the index —
// and revealed by --include-exported with an (exported) marker. Locals
// stay in the default list: an unreferenced local is the strongest dead
// signal there is. Human-readable by default, --json for machines.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/elodhorvath/scipq/internal/index"

	"github.com/urfave/cli/v3"
)

// DeadSymbol is one dead-code candidate: display name, best-effort kind,
// definition site (1-based line), and the classification markers.
type DeadSymbol struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	File   string `json:"file"`
	Line   int32  `json:"line"` // 1-based
	// TestOnly marks symbols whose every reference originates from a
	// *_test.go document — referenced, but only by tests.
	TestOnly bool `json:"testOnly"`
	// Exported marks symbols hidden from the default view (the
	// case-based exported heuristic). Set only in --include-exported
	// output, where the row means "exported and unreferenced in-index,"
	// not "dead."
	Exported bool `json:"exported"`
}

// DeadGroup is one directory's slice of the dead-code candidates.
type DeadGroup struct {
	Dir     string       `json:"dir"`
	Count   int          `json:"count"`
	Symbols []DeadSymbol `json:"symbols"`
}

// DeadResult is the full dead output. ExportedHidden reports how many
// exported symbols the default view filtered — honest accounting of what
// the headline count does not include.
type DeadResult struct {
	Total          int         `json:"total"`
	ExportedHidden int         `json:"exportedHidden"`
	Groups         []DeadGroup `json:"groups"`
}

// isEntryPoint reports whether sym is a package-level Go entry point
// (main or init): invoked by the runtime, never referenced in the index,
// so it presents as zero-ref on every codebase that has one. Member
// symbols (with a '#' segment) are never entry points — a method named
// init classifies normally. The real scip-go package-level shape carries
// a backticked package path and a "()." descriptor
// ("scip-go gomod … `pkg/path`/main()."), so backticks are stripped and
// the last /-segment is compared — the trailing-dot form ("go … main
// main.") matches the same way.
func isEntryPoint(sym string) bool {
	tail := symbolTail(sym)
	if strings.Contains(tail, "#") {
		return false
	}
	tail = strings.ReplaceAll(tail, "`", "")
	tail = strings.TrimRight(tail, "().")
	if i := strings.LastIndex(tail, "/"); i >= 0 {
		tail = tail[i+1:]
	}
	return tail == "main" || tail == "init"
}

// deadMarkers renders the human markers for one dead symbol in
// deterministic order: (test-only) then (exported).
func deadMarkers(s DeadSymbol) string {
	var b strings.Builder
	if s.TestOnly {
		b.WriteString("(test-only)")
	}
	if s.Exported {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("(exported)")
	}
	return b.String()
}

// computeDead builds the dead result from the reverse index: definitions
// with zero references anywhere in the index, grouped by the directory of
// their definition file. Test-only symbols (all refs from *_test.go) are
// listed with the marker. Exported symbols are excluded unless
// includeExported is set, in which case they are listed with the
// (exported) marker — the opt-in view reports "exported and unreferenced
// in-index," not "dead." Locals are kept only when unreferenced: an
// unreferenced local is the strongest dead signal there is (deliberate
// divergence from skeleton, which drops them), but a referenced local is
// either live or vacuously test-only — its uses are definitionally
// test-side — so it is not a dead-code candidate. Package clauses and
// package-level main/init are excluded unconditionally. Groups sort by
// dir; symbols sort by (file, line, symbol) — deterministic.
func computeDead(ri *index.ReverseIndex, includeExported bool) DeadResult {
	defs := ri.DefsAll()
	groupsMap := map[string][]DeadSymbol{}
	total := 0
	exportedHidden := 0
	for sym, sites := range defs {
		if len(sites) == 0 || isBarePackageSymbol(sym) || isEntryPoint(sym) {
			continue
		}
		// Test-only heuristic (same as blast's untested flag): every
		// reference originates from a *_test.go document. A referenced
		// symbol whose refs are all test-side is listed with the marker,
		// not dropped; a zero-ref symbol is plain dead, not test-only;
		// anything else referenced is live.
		testOnly := false
		refCount := ri.RefCount(sym)
		if refCount > 0 {
			testOnly = true
			for _, s := range ri.Refs(sym) {
				if !strings.HasSuffix(s.File, "_test.go") {
					testOnly = false
					break
				}
			}
		}
		if refCount != 0 && !testOnly {
			continue // referenced from non-test code: live
		}
		// Locals enter the default list only unreferenced: a referenced
		// local is either live (non-test refs) or vacuously test-only
		// (its uses are definitionally test-side) — either way it carries
		// no dead signal (issue #8 pre-commit review, blocker 1).
		if isLocalSymbol(sym) && refCount != 0 {
			continue
		}
		recorded := ri.SymbolInfo(sym)
		name := deriveName(sym, recorded.DisplayName)
		exported := isExported(name)
		if exported && !includeExported {
			exportedHidden++
			continue
		}
		ds := DeadSymbol{
			Symbol:   sym,
			Name:     name,
			Kind:     deriveKind(sym, recorded.Kind),
			File:     sites[0].File,
			Line:     sites[0].Line + 1,
			TestOnly: testOnly,
			Exported: exported,
		}
		dir := dirOf(sites[0].File)
		groupsMap[dir] = append(groupsMap[dir], ds)
		total++
	}
	groups := make([]DeadGroup, 0, len(groupsMap))
	for dir, syms := range groupsMap {
		slices.SortFunc(syms, func(a, b DeadSymbol) int {
			if c := strings.Compare(a.File, b.File); c != 0 {
				return c
			}
			if a.Line != b.Line {
				return cmp.Compare(a.Line, b.Line)
			}
			return strings.Compare(a.Symbol, b.Symbol)
		})
		groups = append(groups, DeadGroup{Dir: dir, Count: len(syms), Symbols: syms})
	}
	slices.SortFunc(groups, func(a, b DeadGroup) int {
		return strings.Compare(a.Dir, b.Dir)
	})
	return DeadResult{Total: total, ExportedHidden: exportedHidden, Groups: groups}
}

// renderDeadHuman writes the human-readable dead output to w: a header
// with the candidate and hidden-exported counts, then one block per
// directory group with its symbols, test-only and exported symbols
// flagged.
func renderDeadHuman(w *writer, res DeadResult) {
	if res.Total == 0 {
		fmt.Fprintln(w.out, "no dead symbols")
		return
	}
	header := fmt.Sprintf("%d dead-code candidates · %d groups", res.Total, len(res.Groups))
	if res.ExportedHidden > 0 {
		header += fmt.Sprintf(" (%d exported hidden)", res.ExportedHidden)
	}
	fmt.Fprintln(w.out, header)
	for _, g := range res.Groups {
		fmt.Fprintf(w.out, "%s/\n", g.Dir)
		for _, s := range g.Symbols {
			marker := deadMarkers(s)
			if marker != "" {
				marker = "  " + marker
			}
			fmt.Fprintf(w.out, "  %s  %s:%d%s\n", s.Name, s.File, s.Line, marker)
		}
	}
}

// deadCommand builds the dead verb. It takes no positional arguments;
// --include-exported is verb-local; --index and --json arrive as
// persistent root flags.
func deadCommand(load indexLoader) *cli.Command {
	return &cli.Command{
		Name:  "dead",
		Usage: "dead-code candidates: symbols defined but never referenced",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "include-exported",
				Usage: "also list exported symbols (unreferenced in-index; may be consumed outside the index)",
			},
		},
		OnUsageError: usageError,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.NArg() > 0 {
				fmt.Fprintf(stderr, "scipq: dead takes no positional arguments (got %q)\n", cmd.Args().Slice())
				return &exitError{code: exitUsage}
			}
			ri, code := load(cmd.String("index"))
			if code != exitOK {
				return &exitError{code: code}
			}

			res := computeDead(ri, cmd.Bool("include-exported"))

			w := newWriter(cmd.Bool("json"))
			if cmd.Bool("json") {
				enc := json.NewEncoder(w.out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(w.err, "scipq: encode json: %v\n", err)
					return &exitError{code: exitUsage}
				}
			} else {
				renderDeadHuman(w, res)
			}
			return nil
		},
	}
}
