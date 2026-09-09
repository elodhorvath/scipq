package main

// map verb — one-screen repo orientation: per-directory clusters with hub
// symbols ranked by reference in-degree, overall totals, and repo-wide
// hotspots. Human-readable by default, --json for machines.

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/elodhorvath/scipq/internal/index"
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
func shortSymbol(sym string) string {
	// scip-go style: "go <module> <pkg> <Name>#<Member>()." — keep the
	// trailing name portion after the last space, then trim descriptor
	// punctuation.
	s := sym
	if i := strings.LastIndex(s, " "); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "#"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimRight(s, "().")
	if s == "" {
		// Type-level symbol (e.g. "...xunit.assert Xunit/Assert#"): the
		// member position is empty, so take the last path segment before
		// the trailing descriptor separator.
		head := sym
		if i := strings.LastIndex(head, " "); i >= 0 {
			head = head[i+1:]
		}
		head = strings.TrimRight(head, "#().")
		if j := strings.LastIndexAny(head, "/#"); j >= 0 {
			s = head[j+1:]
		} else {
			s = head
		}
	}
	return s
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

// runMap executes the map verb. jsonOut is pre-detected by the caller
// (hasJSONFlag); the --json flag itself is stripped from args before the
// verb's FlagSet parses, since it is not a verb-level flag.
func runMap(args []string, ri *index.ReverseIndex, jsonOut bool) int {
	verbArgs := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" || a == "-json" {
			continue
		}
		verbArgs = append(verbArgs, a)
	}
	mfs := flag.NewFlagSet("map", flag.ContinueOnError)
	mfs.SetOutput(io.Discard)
	limit := mfs.Int("limit", defaultClusterLimit, "max directory clusters shown (-1 for all)")
	if err := mfs.Parse(verbArgs); err != nil {
		fmt.Fprintf(stderr, "scipq: map: %v\n", err)
		return exitUsage
	}
	if mfs.NArg() > 0 {
		fmt.Fprintf(stderr, "scipq: map takes no positional arguments (got %q)\n", mfs.Args())
		return exitUsage
	}

	res := computeMap(ri, *limit)

	w := newWriter(jsonOut)
	if jsonOut {
		enc := json.NewEncoder(w.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(w.err, "scipq: encode json: %v\n", err)
			return exitUsage
		}
	} else {
		renderMapHuman(w, res)
	}
	return exitOK
}
