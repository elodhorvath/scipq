// callers verb — exact reference sites for a symbol: who uses it, where,
// and via what relation (direct reference, or transitive through
// implements edges). Human-readable by default, --json for machines.
package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/elodhorvath/scipq/internal/index"
)

// relation labels used in output and JSON.
const (
	relationCall = "call"
	// rendered as "implements (via <descriptor>)"
	relationImplements = "implements (via %s)"
)

// CallerSite is one reference site attributed to the queried symbol,
// annotated with how the reference relates to it.
type CallerSite struct {
	File     string `json:"file"`
	Line     int32  `json:"line"` // 1-based
	Relation string `json:"relation"`
}

// CallersResult is the callers output: the resolved symbol and its
// reference sites, sorted by (file, line, relation).
type CallersResult struct {
	Symbol string       `json:"symbol"`
	Sites  []CallerSite `json:"sites"`
}

// symbolMatch is a candidate symbol for a query, with the file of its
// first definition site (used in ambiguity listings).
type symbolMatch struct {
	Symbol    string
	DefinedIn string
}

// symbolTail returns the last space-separated segment of a SCIP symbol
// string — its descriptor (e.g. "Animal#Speak()." from
// "go github.com/example/animal Animal#Speak().").
func symbolTail(sym string) string {
	if i := strings.LastIndex(sym, " "); i >= 0 {
		sym = sym[i+1:]
	}
	return sym
}

// nameMatches reports whether a SCIP symbol matches a user query. A query
// matches when it equals the full symbol string, or equals / suffix-matches
// the symbol's descriptor tail at a name boundary ('#', '.', '/'). Both the
// raw tail and the extracted member name are considered, so "Speak",
// "Animal#Speak", "Animal#Speak()" and the full symbol string all resolve
// to the same symbol, and parameterized descriptors like "Helper(int)"
// match the bare member name.
func nameMatches(sym, query string) bool {
	if sym == query {
		return true
	}
	tail := symbolTail(sym)
	for _, cand := range []string{tail, strings.TrimRight(tail, "."), memberName(tail)} {
		if suffixMatches(cand, query) {
			return true
		}
	}
	return false
}

// suffixMatches reports whether cand equals query or ends with query at a
// name boundary (the character preceding the match is '#', '.', or '/').
func suffixMatches(cand, query string) bool {
	if cand == query {
		return true
	}
	if !strings.HasSuffix(cand, query) {
		return false
	}
	i := len(cand) - len(query)
	return i > 0 && (cand[i-1] == '#' || cand[i-1] == '.' || cand[i-1] == '/')
}

// memberName extracts the member/type name from a symbol descriptor tail:
// trailing descriptor punctuation is stripped, then any parameter or
// generic annotation is cut ("Equal<T>(a, b)." → "Equal"), leaving the
// name possibly type-qualified ("Animal#Speak").
func memberName(tail string) string {
	s := strings.TrimRight(tail, ".#")
	if i := strings.IndexAny(s, "(<"); i >= 0 {
		s = s[:i]
	}
	return s
}

// resolveSymbol resolves a query to a single symbol defined in the index.
// On success it returns the resolved symbol and a nil match list. When the
// query matches several symbols it returns "" with all candidates; when it
// matches none, it returns "" with an empty list. Only defined symbols are
// resolvable — references to symbols defined outside the index have no
// defining file to report.
func resolveSymbol(ri *index.ReverseIndex, query string) (string, []symbolMatch) {
	var matches []symbolMatch
	for _, sym := range ri.DefinedSymbols() {
		if nameMatches(sym, query) {
			definedIn := ""
			if defs := ri.Defs(sym); len(defs) > 0 {
				definedIn = defs[0].File
			}
			matches = append(matches, symbolMatch{Symbol: sym, DefinedIn: definedIn})
		}
	}
	if len(matches) == 1 {
		return matches[0].Symbol, nil
	}
	return "", matches
}

// computeCallers collects the reference sites attributed to symbol: direct
// references ("call") plus references to every implementor, transitively
// through implements chains, labeled with the implementing symbol. Sites
// are sorted by (file, line, relation) for deterministic output.
func computeCallers(ri *index.ReverseIndex, symbol string) CallersResult {
	sites := make([]CallerSite, 0, len(ri.Refs(symbol)))
	for _, s := range ri.Refs(symbol) {
		sites = append(sites, CallerSite{File: s.File, Line: s.Line + 1, Relation: relationCall})
	}
	for _, impl := range ri.Implements(symbol) {
		for _, s := range ri.Refs(impl) {
			sites = append(sites, CallerSite{
				File:     s.File,
				Line:     s.Line + 1,
				Relation: fmt.Sprintf(relationImplements, strings.TrimRight(symbolTail(impl), ".")),
			})
		}
	}
	slices.SortFunc(sites, func(a, b CallerSite) int {
		if a.File != b.File {
			return strings.Compare(a.File, b.File)
		}
		if a.Line != b.Line {
			return cmp.Compare(a.Line, b.Line)
		}
		return strings.Compare(a.Relation, b.Relation)
	})
	return CallersResult{Symbol: symbol, Sites: sites}
}

// renderCallersHuman writes the human-readable callers output to w: the
// resolved symbol with its site count, then one line per site with the
// relation column aligned.
func renderCallersHuman(w *writer, res CallersResult) {
	fmt.Fprintf(w.out, "%s  (%d sites)\n", res.Symbol, len(res.Sites))
	width := 0
	for _, s := range res.Sites {
		if n := len(s.File) + 1 + len(fmt.Sprint(s.Line)); n > width {
			width = n
		}
	}
	for _, s := range res.Sites {
		fmt.Fprintf(w.out, "%-*s  %s\n", width, fmt.Sprintf("%s:%d", s.File, s.Line), s.Relation)
	}
}

// runCallers executes the callers verb. jsonOut is pre-detected by the
// caller (hasJSONFlag); the --json flag itself is stripped from args before
// the verb's FlagSet parses, since it is not a verb-level flag.
func runCallers(args []string, ri *index.ReverseIndex, jsonOut bool) int {
	verbArgs := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" || a == "-json" {
			continue
		}
		verbArgs = append(verbArgs, a)
	}
	mfs := flag.NewFlagSet("callers", flag.ContinueOnError)
	mfs.SetOutput(io.Discard)
	if err := mfs.Parse(verbArgs); err != nil {
		fmt.Fprintf(stderr, "scipq: callers: %v\n", err)
		return exitUsage
	}
	if mfs.NArg() != 1 {
		fmt.Fprintf(stderr, "scipq: callers takes exactly one symbol argument (got %d)\n", mfs.NArg())
		return exitUsage
	}
	query := mfs.Arg(0)
	if query == "" {
		fmt.Fprintln(stderr, "scipq: callers: symbol argument must not be empty")
		return exitUsage
	}

	resolved, matches := resolveSymbol(ri, query)
	if resolved == "" {
		if len(matches) == 0 {
			fmt.Fprintf(stderr, "scipq: no symbol matching %q\n", query)
			return exitUsage
		}
		fmt.Fprintf(stderr, "scipq: %q is ambiguous; %d matching symbols:\n", query, len(matches))
		for _, m := range matches {
			fmt.Fprintf(stderr, "  %s  (defined in %s)\n", m.Symbol, m.DefinedIn)
		}
		return exitUsage
	}

	res := computeCallers(ri, resolved)

	w := newWriter(jsonOut)
	if jsonOut {
		enc := json.NewEncoder(w.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(w.err, "scipq: encode json: %v\n", err)
			return exitUsage
		}
	} else {
		renderCallersHuman(w, res)
	}
	return exitOK
}
