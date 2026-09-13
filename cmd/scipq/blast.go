// blast verb — diff impact analysis: reads a unified diff from stdin,
// maps changed lines to symbols defined on those lines, and walks
// transitive dependents (reverse references + implements chains). A
// reference to a symbol with no definition in the index is surfaced as a
// broken reference — the deletion channel. Human-readable by default,
// --json for machines.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/elodhorvath/scipq/internal/index"

	"github.com/urfave/cli/v3"
)

const (
	defaultBlastDepth = 2
	// relationBrokenRef labels an impacted symbol that is referenced but
	// has no definition in the index — typically deleted by the diff.
	relationBrokenRef = "broken ref"
)

// stdin is the reader blast consumes the diff from. A variable so tests
// can substitute canned diff text.
var stdin io.Reader = os.Stdin

// ImpactedSymbol is one symbol affected by the diff: how it relates to the
// change, and whether it has any test coverage.
type ImpactedSymbol struct {
	Symbol   string `json:"symbol"`
	Short    string `json:"short"`
	File     string `json:"file"`
	Line     int32  `json:"line"` // 1-based
	Reason   string `json:"reason"`
	Untested bool   `json:"untested"`
}

// BlastGroup is one directory's slice of the impact.
type BlastGroup struct {
	Dir     string           `json:"dir"`
	Symbols []ImpactedSymbol `json:"symbols"`
}

// BlastResult is the full blast output: touched count and impacted groups.
type BlastResult struct {
	Touched int          `json:"touched"`
	Groups  []BlastGroup `json:"groups"`
}

// parseUnifiedDiff extracts the set of changed (new-side) line numbers per
// file from standard unified diff text. Lines are 1-based. It enforces
// hunk-length discipline: the "@@ -a,b +c,d @@" header declares how many
// old/new body lines follow, and exactly that many body lines are
// consumed before a new file header is honored — so added content that
// itself starts with "+++ ", "--- ", or "@@ " cannot corrupt the parse.
// /dev/null targets (deletions) are dropped; binary file markers are
// skipped; "\ No newline at end of file" markers are ignored.
func parseUnifiedDiff(text string) map[string]map[int32]bool {
	changed := map[string]map[int32]bool{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var newFile string
	var newLine int32
	var newRemaining int32 // body lines left in the current hunk (new side)
	var oldRemaining int32 // body lines left in the current hunk (old side)
	inHunk := false

	for sc.Scan() {
		line := sc.Text()
		// Inside a hunk, body lines are consumed by count — a body line
		// that looks like a header ("+++ x", "--- x", "@@ ...") is body,
		// period. Only when the hunk is exhausted do headers apply again.
		if inHunk {
			if strings.HasPrefix(line, `\`+" No newline") {
				continue // marker, not a diff line
			}
			if newRemaining <= 0 && oldRemaining <= 0 {
				inHunk = false // hunk exhausted; fall through to header cases
			} else {
				switch {
				case strings.HasPrefix(line, "+"):
					if newRemaining > 0 {
						set := changed[newFile]
						if set == nil {
							set = map[int32]bool{}
							changed[newFile] = set
						}
						set[newLine] = true
						newLine++
						newRemaining--
					}
				case strings.HasPrefix(line, "-"):
					if oldRemaining > 0 {
						oldRemaining--
					}
				default:
					// Context line counts on both sides.
					if newRemaining > 0 {
						newLine++
						newRemaining--
					}
					if oldRemaining > 0 {
						oldRemaining--
					}
				}
				continue
			}
		}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			// "diff --git a/X b/Y" — the new path wins (renames resolve
			// to the new name). Quoted paths (spaces) are skipped.
			newFile = gitPath(line)
			inHunk = false
		case strings.HasPrefix(line, "+++ "):
			target := strings.TrimPrefix(line, "+++ ")
			target = strings.TrimPrefix(target, "b/")
			newFile = ""
			inHunk = false
			if target != "/dev/null" {
				newFile = target
			}
		case strings.HasPrefix(line, "--- "):
			// Old path; nothing to track on the new side.
		case strings.HasPrefix(line, "@@ "):
			// "@@ -a,b +c,d @@ ..." — c is the new-side start (1-based);
			// b and d are the old/new body-line counts.
			inHunk = true
			newLine, newRemaining, oldRemaining = parseHunkHeader(line)
		case strings.HasPrefix(line, "Binary files "):
			inHunk = false
		}
	}
	return changed
}

// gitPath extracts the b/ path from a "diff --git a/X b/Y" header line.
// Returns "" when the line does not carry a plain b/ path.
func gitPath(line string) string {
	parts := strings.Split(line, " ")
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "b/") {
		return strings.TrimPrefix(last, "b/")
	}
	return ""
}

// parseHunkHeader extracts the new-side start line and the old/new
// body-line counts from a hunk header ("@@ -a,b +c,d @@"). Degenerate
// counts (",0" omitted by git for single-line ranges) default to 1;
// malformed headers yield start 1 with zero remaining counts.
func parseHunkHeader(header string) (newStart, newCount, oldCount int32) {
	newStart, newCount, oldCount = 1, 0, 0
	// Split the ranges: "@@ -a,b +c,d @@ ..." → tokens "-a,b" and "+c,d".
	i := strings.Index(header, " -")
	if i < 0 {
		return
	}
	rest := header[i+2:]
	j := strings.Index(rest, " +")
	if j < 0 {
		return
	}
	oldPart := rest[:j]
	rest = rest[j+2:]
	if end := strings.IndexAny(rest, " @"); end >= 0 {
		rest = rest[:end]
	}
	newPart := rest
	oldStart := int32(1)
	oldPart = strings.Split(oldPart, ",")[0]
	if n, err := fmt.Sscanf(oldPart, "%d", new(int)); err == nil {
		_ = n
	}
	var o int
	if _, err := fmt.Sscanf(oldPart, "%d", &o); err == nil && o > 0 {
		oldStart = int32(o)
	}
	_ = oldStart
	var n int
	if _, err := fmt.Sscanf(newPart, "%d", &n); err == nil && n > 0 {
		newStart = int32(n)
	}
	// Counts: "a,b" → b lines (default 1 when the ",b" part is absent).
	oldCount = hunkCount(oldPart)
	newCount = hunkCount(newPart)
	return newStart, newCount, oldCount
}

// hunkCount extracts the count part of a range token ("a,b" → b; "a" → 1).
func hunkCount(part string) int32 {
	_, after, found := strings.Cut(part, ",")
	if !found {
		return 1
	}
	var n int
	if _, err := fmt.Sscanf(after, "%d", &n); err != nil || n < 0 {
		return 1
	}
	return int32(n)
}

// computeBlast builds the blast result: symbols touched by the diff (def
// site on a changed line), their transitive dependents (reverse refs +
// implements chains to depth), and broken references (refs to symbols
// with no definition in the index — the deletion channel). Symbols are
// grouped by directory of their definition file; ordering is
// deterministic.
//
// Line bases: Site.Line is 0-based; the changed set is 1-based, so def
// sites are joined as line+1.
//
// Broken-ref scoping: SCIP indexes define only the indexed project's own
// symbols — every stdlib and third-party reference is "referenced but
// undefined" by construction. To keep the deletion channel from flooding
// with external symbols, a broken ref is reported only when the
// referenced symbol shares the indexed module's symbol prefix (derived
// from the majority prefix of defined symbols). Residual limitation:
// deletions in other modules are invisible.
func computeBlast(ri *index.ReverseIndex, changed map[string]map[int32]bool, depth int) BlastResult {
	defs := ri.DefsAll()

	// Touched symbols: exact containment of a def site in the changed set.
	touched := map[string]bool{}
	for sym, sites := range defs {
		for _, s := range sites {
			if changed[s.File][s.Line+1] {
				touched[sym] = true
				break
			}
		}
	}

	// The indexed module's symbol prefix: the longest common prefix of
	// defined symbols up to the last "/" before the descriptor — e.g.
	// "go github.com/example/animal Animal#Speak()." and
	// "go github.com/example/util Unrelated#Helper()." share
	// "go github.com/example/". Only undefined symbols within this
	// prefix are treated as broken refs.
	modulePrefix := commonSymbolPrefix(defs)

	// Broken references: referenced, undefined, and inside the module.
	broken := map[string]bool{}
	for sym := range ri.RefsAll() {
		if _, defined := defs[sym]; !defined && strings.HasPrefix(sym, modulePrefix) {
			broken[sym] = true
		}
	}

	// Transitive dependents: BFS from touched symbols over reverse refs
	// and implements chains, to depth levels. Depth 0 = only the touched
	// symbols themselves.
	//
	// Two edge types, with different depth semantics:
	//
	//   - File-level dependency: symbol X is depended on by symbol Y when
	//     Y is defined in a file that references X. (SCIP occurrences do
	//     not carry their enclosing symbol, so a direct symbol→symbol
	//     edge is not derivable.) Each BFS level expands one hop.
	//   - Implements: Implements() already returns the TRANSITIVE
	//     closure of implementors, so it is expanded fully at level 1 —
	//     depth does not cut an implements chain mid-way.
	impacted := map[string]bool{}
	for sym := range touched {
		impacted[sym] = true
	}
	// Broken refs are impacted unconditionally: a reference to a symbol
	// with no definition is a break regardless of depth.
	for sym := range broken {
		impacted[sym] = true
	}

	// file → symbols defined in it (one O(defs) pass; answers BFS
	// frontier lookups lazily instead of building the full depGraph
	// up front — O(defs × refFiles) on real indexes was the perf risk).
	defsByFile := map[string][]string{}
	for sym, sites := range defs {
		for _, s := range sites {
			defsByFile[s.File] = append(defsByFile[s.File], sym)
		}
	}
	// referenced symbol → files referencing it.
	refFiles := map[string]map[string]bool{}
	for sym, sites := range ri.RefsAll() {
		for _, s := range sites {
			if refFiles[sym] == nil {
				refFiles[sym] = map[string]bool{}
			}
			refFiles[sym][s.File] = true
		}
	}

	frontier := map[string]bool{}
	for sym := range touched {
		frontier[sym] = true
	}
	visited := map[string]bool{}
	for sym := range impacted {
		visited[sym] = true
	}
	for level := 0; level < depth; level++ {
		next := map[string]bool{}
		for sym := range frontier {
			// File-level deps: symbols defined in files that reference sym.
			for f := range refFiles[sym] {
				for _, dep := range defsByFile[f] {
					if !visited[dep] {
						next[dep] = true
					}
				}
			}
			// Implements is a transitive closure: expand fully once.
			for _, impl := range ri.Implements(sym) {
				if !visited[impl] {
					next[impl] = true
					// Mark visited so the closure is not re-walked;
					// implementors' own file-deps expand on later levels.
					visited[impl] = true
				}
			}
		}
		for sym := range next {
			impacted[sym] = true
			visited[sym] = true
		}
		frontier = next
		if len(frontier) == 0 {
			break
		}
	}

	// Group impacted symbols by directory of their definition file.
	// Broken refs have no def site; they group under their referencing
	// files' directories.
	groupsMap := map[string][]ImpactedSymbol{}
	for sym := range impacted {
		reason := "dependent"
		switch {
		case touched[sym]:
			reason = "touched"
		case broken[sym]:
			reason = relationBrokenRef
		}
		dirs := map[string]bool{}
		if sites := defs[sym]; len(sites) > 0 {
			dirs[dirOf(sites[0].File)] = true
		}
		if len(dirs) == 0 {
			for f := range refFiles[sym] {
				dirs[dirOf(f)] = true
			}
		}
		for dir := range dirs {
			groupsMap[dir] = append(groupsMap[dir], impactedSymbol(ri, defs, sym, reason))
		}
	}

	groups := make([]BlastGroup, 0, len(groupsMap))
	for dir, syms := range groupsMap {
		slices.SortFunc(syms, func(a, b ImpactedSymbol) int {
			return strings.Compare(a.Symbol, b.Symbol)
		})
		groups = append(groups, BlastGroup{Dir: dir, Symbols: syms})
	}
	slices.SortFunc(groups, func(a, b BlastGroup) int {
		return strings.Compare(a.Dir, b.Dir)
	})

	return BlastResult{Touched: len(touched), Groups: groups}
}

// commonSymbolPrefix derives the indexed module's symbol prefix: the
// longest common prefix of all defined symbols, cut back to the last "/"
// (so a partial token never matches). Falls back to "" when symbols share
// no prefix.
func commonSymbolPrefix(defs map[string][]index.Site) string {
	var prefix string
	first := true
	for sym := range defs {
		if first {
			prefix = sym
			first = false
			continue
		}
		for !strings.HasPrefix(sym, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		prefix = prefix[:i+1]
	} else {
		prefix = ""
	}
	return prefix
}

// impactedSymbol renders one impacted symbol for output: display name,
// definition site (1-based), reason, and the untested flag (no refs from
// *_test.go files).
func impactedSymbol(ri *index.ReverseIndex, defs map[string][]index.Site, sym, reason string) ImpactedSymbol {
	im := ImpactedSymbol{
		Symbol: sym,
		Short:  shortSymbol(sym),
		Reason: reason,
	}
	if sites := defs[sym]; len(sites) > 0 {
		im.File = sites[0].File
		im.Line = sites[0].Line + 1
	}
	// Coverage heuristic: untested when no reference originates from a
	// *_test.go file.
	untested := true
	for _, s := range ri.Refs(sym) {
		if strings.HasSuffix(s.File, "_test.go") {
			untested = false
			break
		}
	}
	im.Untested = untested
	return im
}

// renderBlastHuman writes the human-readable blast output to w: one block
// per directory group with its impacted symbols, untested symbols flagged.
func renderBlastHuman(w *writer, res BlastResult) {
	if res.Touched == 0 && len(res.Groups) == 0 {
		fmt.Fprintln(w.out, "no impacted symbols")
		return
	}
	fmt.Fprintf(w.out, "%d touched · %d groups\n", res.Touched, len(res.Groups))
	for _, g := range res.Groups {
		fmt.Fprintf(w.out, "%s/\n", g.Dir)
		for _, s := range g.Symbols {
			flag := ""
			if s.Untested {
				flag = "  (untested)"
			}
			fmt.Fprintf(w.out, "  %s  %s%s\n", s.Short, s.Reason, flag)
		}
	}
}

// blastCommand builds the blast verb. The diff arrives on stdin; --depth
// is verb-local; --index and --json arrive as persistent root flags.
func blastCommand(load indexLoader) *cli.Command {
	return &cli.Command{
		Name:  "blast",
		Usage: "diff impact analysis: unified diff on stdin → impacted symbols",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "depth",
				Usage: "transitive dependent depth (default 2)",
				Value: defaultBlastDepth,
			},
		},
		OnUsageError: usageError,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.NArg() > 0 {
				fmt.Fprintf(stderr, "scipq: blast takes no positional arguments (got %q)\n", cmd.Args().Slice())
				return &exitError{code: exitUsage}
			}
			// A bare `scipq blast` in a terminal would block on stdin;
			// require a pipe or redirect.
			if isTerminal(stdin) {
				fmt.Fprintln(stderr, "scipq: blast reads a unified diff from stdin (pipe: git diff -U0 | scipq blast)")
				return &exitError{code: exitUsage}
			}
			diffText, err := io.ReadAll(stdin)
			if err != nil {
				fmt.Fprintf(stderr, "scipq: blast: read diff: %v\n", err)
				return &exitError{code: exitUsage}
			}
			changed := parseUnifiedDiff(string(diffText))

			ri, code := load(cmd.String("index"))
			if code != exitOK {
				return &exitError{code: code}
			}

			res := computeBlast(ri, changed, cmd.Int("depth"))

			w := newWriter(cmd.Bool("json"))
			if cmd.Bool("json") {
				enc := json.NewEncoder(w.out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(w.err, "scipq: encode json: %v\n", err)
					return &exitError{code: exitUsage}
				}
			} else {
				renderBlastHuman(w, res)
			}
			return nil
		},
	}
}

// isTerminal reports whether r is an interactive terminal. Only *os.File
// can be a terminal; anything else (buffers in tests, pipes) is not.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
