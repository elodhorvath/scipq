package main

// map verb — one-screen repo orientation: per-directory clusters with hub
// symbols ranked by reference in-degree, overall totals, and repo-wide
// hotspots. Human-readable by default, --json for machines.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/elodhorvath/scipq/internal/index"

	"github.com/urfave/cli/v3"
)

const (
	defaultClusterLimit = 10
	defaultHubsPerDir   = 3
	defaultHotspotLimit = 5
)

// Hub is a symbol ranked by how often it is referenced (in-degree).
type Hub struct {
	Symbol string `json:"symbol"`
	Refs   int    `json:"refs"`
}

// Cluster groups the files of one directory with its hub symbols.
type Cluster struct {
	Dir     string `json:"dir"`
	Files   int    `json:"files"`
	Symbols int    `json:"symbols"`
	Hubs    []Hub  `json:"hubs"`
}

// Hotspot is a file that attracts the most references repo-wide.
type Hotspot struct {
	File string `json:"file"`
	Refs int    `json:"refs"`
}

// MapResult is the full map output: totals, clusters, hotspots.
type MapResult struct {
	Files     int       `json:"files"`
	Symbols   int       `json:"symbols"`
	Clusters  []Cluster `json:"clusters"`
	Hotspots  []Hotspot `json:"hotspots"`
	Truncated bool      `json:"truncated"`
}

// dirOf extracts the directory portion of a document path; root-level files
// cluster under ".".
func dirOf(p string) string {
	return path.Dir(p)
}

// shortSymbol renders a symbol for display: the last meaningful segment of
// the SCIP identifier, so output stays token-budgeted.
//
// Shapes handled:
//
//   - Member symbols ("go <mod> <pkg> <Name>#<Member>()."): the member
//     name after '#'.
//   - Type-level symbols ("...Assert#"): empty member position falls
//     back to the last path segment of the descriptor.
//   - Package-level scip-go symbols ("...`pkg/path`/name.",
//     "...`pkg/path`/"): no '#' — the last path segment of the
//     backtick-stripped descriptor.
//   - scip-go local symbols ("local 8"): no extractable name — the whole
//     symbol is the display (a bare digit would read as a count).
func shortSymbol(sym string) string {
	// Descriptor tail: everything after the last space.
	tail := sym
	if i := strings.LastIndex(tail, " "); i >= 0 {
		tail = tail[i+1:]
	}
	// Member symbols carry the name after '#'. Prefer the member name.
	if i := strings.Index(tail, "#"); i >= 0 {
		if member := strings.TrimRight(tail[i+1:], "()."); member != "" {
			return member
		}
		// Empty member position (e.g. "...Assert#"): fall through to the
		// path-segment logic on the trimmed tail.
		tail = strings.TrimRight(tail, "#().")
	}
	// No '#' — package-level scip-go symbols and locals. Display the last
	// path segment of the descriptor, backticks stripped.
	s := strings.ReplaceAll(tail, "`", "")
	s = strings.TrimRight(s, "().")
	s = strings.TrimRight(s, "/")
	if j := strings.LastIndex(s, "/"); j >= 0 {
		s = s[j+1:]
	}
	if s == "" || isAllDigits(s) {
		// Nothing extractable, or the tail is a bare digit (scip-go
		// "local 8"): the whole symbol is the most informative
		// deterministic rendering.
		return sym
	}
	return s
}

// isAllDigits reports whether s is non-empty and consists only of ASCII
// digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isParamFragment reports whether a symbol is a parameter occurrence
// (scip-dotnet style "...(param)" suffix) — noise in orientation output.
func isParamFragment(sym string) bool {
	return strings.HasSuffix(sym, "(param)")
}

// computeMap builds the map result from the reverse index: per-directory
// clusters with top hubs, repo-wide hotspots, and overall totals. Clusters
// are sorted by descending symbol count then name; hubs by descending refs
// then symbol; hotspots by descending refs then file.
func computeMap(ri *index.ReverseIndex, clusterLimit int) MapResult {
	files := ri.Files()
	symbols := ri.DefinedSymbols()
	refCounts := make(map[string]int, len(symbols))
	for _, sym := range symbols {
		refCounts[sym] = ri.RefCount(sym)
	}

	// Bucket symbols by directory of their first definition site.
	symDir := map[string]string{}
	for _, sym := range symbols {
		for _, d := range ri.Defs(sym) {
			symDir[sym] = dirOf(d.File)
			break
		}
	}

	// Bucket files by directory.
	filesByDir := map[string][]string{}
	for _, f := range files {
		d := dirOf(f)
		filesByDir[d] = append(filesByDir[d], f)
	}

	// Count references received per (directory, symbol) pair. Parameter
	// fragments are excluded — they are noise in orientation output.
	hubCounts := map[string]map[string]int{}
	for sym, sites := range ri.RefsAll() {
		if isParamFragment(sym) {
			continue
		}
		dir, ok := symDir[sym]
		if !ok {
			continue // referenced but never defined in this index
		}
		if hubCounts[dir] == nil {
			hubCounts[dir] = map[string]int{}
		}
		hubCounts[dir][sym] += len(sites)
	}

	clusters := make([]Cluster, 0, len(filesByDir))
	for dir, dirFiles := range filesByDir {
		c := Cluster{Dir: dir, Files: len(dirFiles)}
		for sym := range symDir {
			if symDir[sym] == dir {
				c.Symbols++
			}
		}
		for sym, n := range hubCounts[dir] {
			if n > 0 {
				c.Hubs = append(c.Hubs, Hub{Symbol: shortSymbol(sym), Refs: n})
			}
		}
		slices.SortFunc(c.Hubs, func(a, b Hub) int {
			if a.Refs != b.Refs {
				return cmp.Compare(b.Refs, a.Refs)
			}
			return strings.Compare(a.Symbol, b.Symbol)
		})
		if len(c.Hubs) > defaultHubsPerDir {
			c.Hubs = c.Hubs[:defaultHubsPerDir]
		}
		clusters = append(clusters, c)
	}
	slices.SortFunc(clusters, func(a, b Cluster) int {
		if a.Symbols != b.Symbols {
			return cmp.Compare(b.Symbols, a.Symbols)
		}
		return strings.Compare(a.Dir, b.Dir)
	})

	truncated := false
	if clusterLimit >= 0 && len(clusters) > clusterLimit {
		clusters = clusters[:clusterLimit]
		truncated = true
	}

	// Repo-wide hotspots: files by reference count.
	fileRefCounts := ri.FileRefCounts()
	hotspots := make([]Hotspot, 0, len(fileRefCounts))
	for f, n := range fileRefCounts {
		hotspots = append(hotspots, Hotspot{File: f, Refs: n})
	}
	slices.SortFunc(hotspots, func(a, b Hotspot) int {
		if a.Refs != b.Refs {
			return cmp.Compare(b.Refs, a.Refs)
		}
		return strings.Compare(a.File, b.File)
	})
	if len(hotspots) > defaultHotspotLimit {
		hotspots = hotspots[:defaultHotspotLimit]
	}

	return MapResult{
		Files:     len(files),
		Symbols:   len(symbols),
		Clusters:  clusters,
		Hotspots:  hotspots,
		Truncated: truncated,
	}
}

// renderMapHuman writes the human-readable map to w.
func renderMapHuman(w *writer, res MapResult) {
	fmt.Fprintf(w.out, "%d files · %d symbols\n", res.Files, res.Symbols)
	for _, c := range res.Clusters {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s/  %d files · %d symbols", c.Dir, c.Files, c.Symbols)
		if len(c.Hubs) > 0 {
			hubs := make([]string, 0, len(c.Hubs))
			for _, h := range c.Hubs {
				hubs = append(hubs, fmt.Sprintf("%s (%d←)", h.Symbol, h.Refs))
			}
			fmt.Fprintf(&sb, "   hubs: %s", strings.Join(hubs, ", "))
		}
		fmt.Fprintln(w.out, sb.String())
	}
	if len(res.Hotspots) > 0 {
		fmt.Fprintln(w.out, "")
		fmt.Fprintln(w.out, "hotspots:")
		for _, h := range res.Hotspots {
			fmt.Fprintf(w.out, "%s (%d refs)\n", h.File, h.Refs)
		}
	}
	if res.Truncated {
		fmt.Fprintf(w.out, "(clusters truncated; raise --limit to see more)\n")
	}
}

// mapCommand builds the map verb. The --limit flag is verb-local; --index
// and --json arrive as persistent root flags read via lineage lookup.
func mapCommand(load indexLoader) *cli.Command {
	return &cli.Command{
		Name:  "map",
		Usage: "per-directory clusters, hub symbols, and repo-wide hotspots",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "limit",
				Usage: "max directory clusters shown (-1 for all)",
				Value: defaultClusterLimit,
			},
		},
		OnUsageError: usageError,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.NArg() > 0 {
				fmt.Fprintf(stderr, "scipq: map takes no positional arguments (got %q)\n", cmd.Args().Slice())
				return &exitError{code: exitUsage}
			}
			ri, code := load(cmd.String("index"))
			if code != exitOK {
				return &exitError{code: code}
			}

			res := computeMap(ri, cmd.Int("limit"))

			w := newWriter(cmd.Bool("json"))
			if cmd.Bool("json") {
				enc := json.NewEncoder(w.out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					fmt.Fprintf(w.err, "scipq: encode json: %v\n", err)
					return &exitError{code: exitUsage}
				}
			} else {
				renderMapHuman(w, res)
			}
			return nil
		},
	}
}
