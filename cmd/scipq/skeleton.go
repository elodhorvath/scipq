// skeleton verb — file API surface: every symbol defined in a file, no
// bodies. Language-agnostic: whatever the indexer recorded as definitions
// in that document. Human-readable by default, --json for machines.
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

// SkeletonSymbol is one definition in a file: display name, best-effort
// kind, 1-based start line, and the exported marker.
type SkeletonSymbol struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Line     int32  `json:"line"` // 1-based
	Exported bool   `json:"exported"`
}

// SkeletonResult is the skeleton output: the resolved file and its
// definitions, sorted by (line, name).
type SkeletonResult struct {
	File    string           `json:"file"`
	Symbols []SkeletonSymbol `json:"symbols"`
}

// resolveFile resolves a user-supplied path to an indexed document.
// Exact match wins; otherwise the query suffix-matches indexed paths at a
// "/" boundary (so "helper.go" matches "util/helper.go"). Zero matches or
// several matches are both unresolved: the caller reports them (exit 1).
func resolveFile(files []string, query string) (string, []string) {
	var matches []string
	for _, f := range files {
		if f == query {
			return f, nil // exact match wins outright
		}
	}
	for _, f := range files {
		if strings.HasSuffix(f, query) && len(f) > len(query) && f[len(f)-len(query)-1] == '/' {
			matches = append(matches, f)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", matches
}

// isLocalSymbol reports whether sym is a SCIP local ("local 8" shape):
// the spec's escape hatch for symbols with no stable global identity.
// Locals carry zero API information and are dropped from skeletons.
func isLocalSymbol(sym string) bool {
	_, rest, found := strings.Cut(sym, " ")
	return found && rest != "" && isAllDigits(rest)
}

// isBarePackageSymbol reports whether sym's descriptor is a bare package
// clause ("`pkg/path`/" — no member segment). It is the package
// declaration, not a declaration in the file, and is dropped from
// skeletons.
func isBarePackageSymbol(sym string) bool {
	tail := symbolTail(sym)
	return strings.HasSuffix(tail, "`/") || strings.HasSuffix(tail, "/")
}

// deriveKind renders the best-effort kind for a symbol: the indexer's
// recorded kind when populated, else descriptor-grammar fallback, else
// the honest generic "def". Documented as best-effort (issue #7 design
// comment): SCIP's SymbolInformation carries no kind field, so the
// grammar path is indexer-shape-dependent.
func deriveKind(sym string, recorded string) string {
	if recorded != "" && recorded != "UnspecifiedKind" {
		return strings.ToLower(recorded)
	}
	tail := symbolTail(sym)
	hasMember := strings.Contains(tail, "#")
	hasCall := strings.Contains(tail, "()")
	switch {
	case hasMember && hasCall:
		return "method"
	case hasMember:
		return "property"
	case hasCall:
		return "function"
	case strings.HasSuffix(tail, "."):
		return "type"
	default:
		return "def"
	}
}

// deriveName renders the display name for a symbol: the indexer's
// display name when populated, else the short rendering.
func deriveName(sym, recorded string) string {
	if recorded != "" {
		return recorded
	}
	return shortSymbol(sym)
}

// isExported reports the exported marker for a display name: the
// case-based convention shared by Go and C# (capitalized = exported).
// Documented as a heuristic, not a language service.
func isExported(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	return first >= 'A' && first <= 'Z'
}

// computeSkeleton lists the definitions in file: SCIP locals and bare
// package clauses dropped (issue #7 design comment), the rest rendered
// with best-effort kind, 1-based line, and the exported marker. Sorted
// by (line, name) for deterministic output.
func computeSkeleton(ri *index.ReverseIndex, file string) SkeletonResult {
	defs := ri.DefsInFile(file)
	symbols := make([]SkeletonSymbol, 0, len(defs))
	for _, d := range defs {
		if isLocalSymbol(d.Symbol) || isBarePackageSymbol(d.Symbol) {
			continue
		}
		recorded := ri.SymbolInfo(d.Symbol)
		symbols = append(symbols, SkeletonSymbol{
			Symbol:   d.Symbol,
			Name:     deriveName(d.Symbol, recorded.DisplayName),
			Kind:     deriveKind(d.Symbol, recorded.Kind),
			Line:     d.Line + 1,
			Exported: isExported(deriveName(d.Symbol, recorded.DisplayName)),
		})
	}
	slices.SortFunc(symbols, func(a, b SkeletonSymbol) int {
		if a.Line != b.Line {
			return cmp.Compare(a.Line, b.Line)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return SkeletonResult{File: file, Symbols: symbols}
}

// renderSkeletonHuman writes the human-readable skeleton to w: the file
// with its def count, then one line per symbol with the kind column
// aligned.
func renderSkeletonHuman(w *writer, res SkeletonResult) {
	fmt.Fprintf(w.out, "%s  (%d symbols)\n", res.File, len(res.Symbols))
	kindWidth := 0
	for _, s := range res.Symbols {
		if len(s.Kind) > kindWidth {
			kindWidth = len(s.Kind)
		}
	}
	for _, s := range res.Symbols {
		marker := ""
		if s.Exported {
			marker = "  +exported"
		}
		fmt.Fprintf(w.out, "%6d  %-*s  %s%s\n", s.Line, kindWidth, s.Kind, s.Name, marker)
	}
}

// skeletonCommand builds the skeleton verb. The file argument is a
// required positional; --index and --json arrive as persistent root flags.
func skeletonCommand(load indexLoader) *cli.Command {
	return &cli.Command{
		Name:         "skeleton",
		Usage:        "file API surface: symbols defined in a file, no bodies",
		ArgsUsage:    "FILE",
		OnUsageError: usageError,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.NArg() != 1 {
				fmt.Fprintf(stderr, "scipq: skeleton takes exactly one file argument (got %d)\n", cmd.NArg())
				return &exitError{code: exitUsage}
			}
			query := cmd.Args().First()
			if query == "" {
				fmt.Fprintln(stderr, "scipq: skeleton: file argument must not be empty")
				return &exitError{code: exitUsage}
			}
			ri, code := load(cmd.String("index"))
			if code != exitOK {
				return &exitError{code: code}
			}

			resolved, matches := resolveFile(ri.Files(), query)
			if resolved == "" {
				if len(matches) == 0 {
					fmt.Fprintf(stderr, "scipq: no indexed file matching %q\n", query)
					return &exitError{code: exitUsage}
				}
				fmt.Fprintf(stderr, "scipq: %q is ambiguous; %d matching files:\n", query, len(matches))
				for _, m := range matches {
					fmt.Fprintf(stderr, "  %s\n", m)
				}
				return &exitError{code: exitUsage}
			}

			res := computeSkeleton(ri, resolved)

			w := newWriter(cmd.Bool("json"))
			if cmd.Bool("json") {
				enc := json.NewEncoder(w.out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(w.err, "scipq: encode json: %v\n", err)
					return &exitError{code: exitUsage}
				}
			} else {
				renderSkeletonHuman(w, res)
			}
			return nil
		},
	}
}
