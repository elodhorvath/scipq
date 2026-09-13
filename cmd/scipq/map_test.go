package main

// Tests for the map verb: cluster/hub/hotspot computation against the
// synthetic fixture, human and JSON rendering, and flag handling.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/elodhorvath/scipq/internal/index"
)

// loadFixtureIndex loads the committed synthetic fixture.
func loadFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	ri, err := index.Load("../../testdata/index.scip")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return ri
}

// runWith runs a full scipq invocation against a stub index loader that
// returns ri regardless of the --index value. It is the entry-point seam
// for verb tests: the same path main() takes, with index loading stubbed.
// Error mapping goes through runError so tests exercise the same
// print-and-map contract as production (issue #29).
func runWith(t *testing.T, argv []string, ri *index.ReverseIndex) int {
	t.Helper()
	cmd := newRootCommand(func(string) (*index.ReverseIndex, int) {
		return ri, exitOK
	})
	err := cmd.Run(context.Background(), append([]string{"scipq"}, argv...))
	return runError(err)
}

// captureWriter redirects the process writers to buffers for the duration
// of f, returning what was written to each stream.
func captureWriter(t *testing.T) (out, errb *bytes.Buffer) {
	t.Helper()
	out = &bytes.Buffer{}
	errb = &bytes.Buffer{}
	stdout, stderr = out, errb
	t.Cleanup(func() {
		stdout, stderr = nil, nil
	})
	return out, errb
}

func TestComputeMap(t *testing.T) {
	ri := loadFixtureIndex(t)

	tests := []struct {
		name         string
		limit        int
		wantFiles    int
		wantSymbols  int
		wantClusters int
		wantTrunc    bool
	}{
		{"all clusters", -1, 5, 5, 3, false},
		{"default limit", defaultClusterLimit, 5, 5, 3, false},
		{"limit 1 truncates", 1, 5, 5, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := computeMap(ri, tt.limit)
			if res.Files != tt.wantFiles {
				t.Errorf("Files = %d, want %d", res.Files, tt.wantFiles)
			}
			if res.Symbols != tt.wantSymbols {
				t.Errorf("Symbols = %d, want %d", res.Symbols, tt.wantSymbols)
			}
			if len(res.Clusters) != tt.wantClusters {
				t.Errorf("len(Clusters) = %d, want %d", len(res.Clusters), tt.wantClusters)
			}
			if res.Truncated != tt.wantTrunc {
				t.Errorf("Truncated = %v, want %v", res.Truncated, tt.wantTrunc)
			}
		})
	}
}

func TestComputeMapClusters(t *testing.T) {
	ri := loadFixtureIndex(t)
	res := computeMap(ri, -1)

	tests := []struct {
		dir        string
		files      int
		symbols    int
		topHub     string
		topHubRefs int
	}{
		{".", 2, 4, "Speak", 3},
		{"util", 1, 1, "Helper", 1},
		{"services", 2, 0, "", 0},
	}
	if len(res.Clusters) != len(tests) {
		t.Fatalf("got %d clusters, want %d", len(res.Clusters), len(tests))
	}
	for i, tt := range tests {
		c := res.Clusters[i]
		if c.Dir != tt.dir {
			t.Errorf("cluster[%d].Dir = %q, want %q", i, c.Dir, tt.dir)
		}
		if c.Files != tt.files {
			t.Errorf("cluster[%d].Files = %d, want %d", i, c.Files, tt.files)
		}
		if c.Symbols != tt.symbols {
			t.Errorf("cluster[%d].Symbols = %d, want %d", i, c.Symbols, tt.symbols)
		}
		if tt.topHub == "" {
			if len(c.Hubs) != 0 {
				t.Errorf("cluster[%d].Hubs = %v, want none", i, c.Hubs)
			}
			continue
		}
		if len(c.Hubs) == 0 || c.Hubs[0].Symbol != tt.topHub || c.Hubs[0].Refs != tt.topHubRefs {
			t.Errorf("cluster[%d].Hubs = %+v, want top %s (%d←)", i, c.Hubs, tt.topHub, tt.topHubRefs)
		}
	}
}

func TestComputeMapHotspots(t *testing.T) {
	ri := loadFixtureIndex(t)
	res := computeMap(ri, -1)

	want := []Hotspot{
		{"services/zoo.go", 3},
		{"services/handler.go", 1},
	}
	if len(res.Hotspots) != len(want) {
		t.Fatalf("got %d hotspots, want %d", len(res.Hotspots), len(want))
	}
	for i, h := range want {
		if res.Hotspots[i] != h {
			t.Errorf("hotspot[%d] = %+v, want %+v", i, res.Hotspots[i], h)
		}
	}
}

func TestRunMapHuman(t *testing.T) {
	ri := loadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"map"}, ri)
	if code != exitOK {
		t.Fatalf("map exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	// Data-only stdout contract: no agent-directed instructions.
	for _, banned := range []string{"you should", "consider", "recommend"} {
		if containsFold(out.String(), banned) {
			t.Errorf("stdout contains banned instruction-like text %q", banned)
		}
	}
	for _, want := range []string{
		"5 files · 5 symbols",
		"./  2 files · 4 symbols   hubs: Speak (3←)",
		"util/  1 files · 1 symbols   hubs: Helper (1←)",
		"services/  2 files · 0 symbols",
		"hotspots:",
		"services/zoo.go (3 refs)",
	} {
		if !contains(out.String(), want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out.String())
		}
	}
}

// buildOutOfRootFixtureIndex builds an index with one in-root document and
// one "../"-escaping build-cache document (def + refs), mirroring the
// scip-go test-compile leakage from issue #42.
func buildOutOfRootFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	inRoot := "go github.com/example/inroot Thing."
	cacheSym := "go github.com/example/cache Cached."
	def := func(sym string, line int32) *scip.Occurrence {
		return &scip.Occurrence{
			Range:       []int32{line, 0, 10},
			Symbol:      sym,
			SymbolRoles: int32(scip.SymbolRole_Definition),
		}
	}
	ref := func(sym string, line int32) *scip.Occurrence {
		return &scip.Occurrence{Range: []int32{line, 0, 10}, Symbol: sym}
	}
	idx := &scip.Index{
		Metadata: &scip.Metadata{
			ToolInfo:    &scip.ToolInfo{Name: "scipq-test", Version: "0.0.0"},
			ProjectRoot: "file:///synthetic",
		},
		Documents: []*scip.Document{
			{
				RelativePath: "inroot.go",
				Occurrences: []*scip.Occurrence{
					def(inRoot, 1),
					ref(inRoot, 5),
				},
			},
			{
				RelativePath: "../../.cache/go-build/08/x.go",
				Occurrences: []*scip.Occurrence{
					def(cacheSym, 0),
					ref(inRoot, 4),
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal out-of-root fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write out-of-root fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load out-of-root fixture: %v", err)
	}
	return ri
}

// TestComputeMapOutOfRoot pins the map-level effect of the index-layer
// filter: the cache directory and file never appear in clusters or
// hotspots, totals describe the filtered project, and the hidden count is
// surfaced.
func TestComputeMapOutOfRoot(t *testing.T) {
	ri := buildOutOfRootFixtureIndex(t)

	res := computeMap(ri, -1)

	if res.Files != 1 {
		t.Errorf("Files = %d, want 1 (out-of-root doc excluded)", res.Files)
	}
	if res.Symbols != 1 {
		t.Errorf("Symbols = %d, want 1 (cache symbol excluded)", res.Symbols)
	}
	if res.ExternalDocsHidden != 1 {
		t.Errorf("ExternalDocsHidden = %d, want 1", res.ExternalDocsHidden)
	}
	for _, c := range res.Clusters {
		if contains(c.Dir, ".cache") {
			t.Errorf("clusters contain cache dir %q", c.Dir)
		}
	}
	for _, h := range res.Hotspots {
		if contains(h.File, ".cache") {
			t.Errorf("hotspots contain cache file %q", h.File)
		}
	}
	// The in-root document's own reference is the only hotspot source.
	if len(res.Hotspots) != 1 || res.Hotspots[0].File != "inroot.go" || res.Hotspots[0].Refs != 1 {
		t.Errorf("Hotspots = %+v, want [inroot.go (1 refs)]", res.Hotspots)
	}
}

// TestRunMapHumanOutOfRoot pins the honesty header: the human output
// carries the hidden-docs suffix when documents were dropped, and the
// suffix is absent when none were.
func TestRunMapHumanOutOfRoot(t *testing.T) {
	t.Run("suffix present when docs dropped", func(t *testing.T) {
		ri := buildOutOfRootFixtureIndex(t)
		out, errb := captureWriter(t)

		code := runWith(t, []string{"map"}, ri)
		if code != exitOK {
			t.Fatalf("map exit = %d, want %d", code, exitOK)
		}
		if errb.Len() != 0 {
			t.Errorf("stderr not empty: %q", errb.String())
		}
		for _, banned := range []string{"you should", "consider", "recommend"} {
			if containsFold(out.String(), banned) {
				t.Errorf("stdout contains banned instruction-like text %q", banned)
			}
		}
		if want := "1 files · 1 symbols (1 external docs hidden)"; !contains(out.String(), want) {
			t.Errorf("output missing header %q\n--- got ---\n%s", want, out.String())
		}
		if contains(out.String(), ".cache") {
			t.Errorf("output contains cache path\n%s", out.String())
		}
	})

	t.Run("suffix absent on conformant fixture", func(t *testing.T) {
		ri := loadFixtureIndex(t)
		out, _ := captureWriter(t)

		code := runWith(t, []string{"map"}, ri)
		if code != exitOK {
			t.Fatalf("map exit = %d, want %d", code, exitOK)
		}
		if contains(out.String(), "external docs hidden") {
			t.Errorf("header reports hidden docs on conformant index:\n%s", out.String())
		}
	})
}

// TestRunMapJSONOutOfRoot pins the JSON contract: externalDocsHidden
// round-trips through --json.
func TestRunMapJSONOutOfRoot(t *testing.T) {
	ri := buildOutOfRootFixtureIndex(t)
	out, _ := captureWriter(t)

	code := runWith(t, []string{"map", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("map --json exit = %d, want %d", code, exitOK)
	}
	var res MapResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if res.ExternalDocsHidden != 1 {
		t.Errorf("JSON externalDocsHidden = %d, want 1", res.ExternalDocsHidden)
	}
	if res.Files != 1 || res.Symbols != 1 {
		t.Errorf("JSON totals = %d/%d, want 1/1", res.Files, res.Symbols)
	}
	if len(res.Clusters) != 1 || res.Clusters[0].Dir != "." {
		t.Errorf("JSON clusters = %+v, want single \".\" cluster", res.Clusters)
	}
}

// TestRunMapAllOutOfRoot pins the pathological empty state: an index whose
// every document escapes the root yields an empty map with honest totals.
func TestRunMapAllOutOfRoot(t *testing.T) {
	cacheSym := "go github.com/example/cache Cached."
	idx := &scip.Index{
		Metadata: &scip.Metadata{
			ToolInfo:    &scip.ToolInfo{Name: "scipq-test", Version: "0.0.0"},
			ProjectRoot: "file:///synthetic",
		},
		Documents: []*scip.Document{{
			RelativePath: "../../.cache/go-build/08/x.go",
			Occurrences: []*scip.Occurrence{{
				Range:       []int32{0, 0, 10},
				Symbol:      cacheSym,
				SymbolRoles: int32(scip.SymbolRole_Definition),
			}},
		}},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal all-out-of-root fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write all-out-of-root fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load all-out-of-root fixture: %v", err)
	}

	out, _ := captureWriter(t)
	code := runWith(t, []string{"map"}, ri)
	if code != exitOK {
		t.Fatalf("map exit = %d, want %d", code, exitOK)
	}
	if want := "0 files · 0 symbols (1 external docs hidden)"; !contains(out.String(), want) {
		t.Errorf("output missing honest empty header %q\n--- got ---\n%s", want, out.String())
	}
}

func TestRunMapJSON(t *testing.T) {
	ri := loadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"map", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("map exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.Len())
	}
	var res MapResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if res.Files != 5 || res.Symbols != 5 {
		t.Errorf("JSON totals = %d/%d, want 5/5", res.Files, res.Symbols)
	}
	if len(res.Clusters) != 3 {
		t.Errorf("JSON clusters = %d, want 3", len(res.Clusters))
	}
	if len(res.Hotspots) != 2 {
		t.Errorf("JSON hotspots = %d, want 2", len(res.Hotspots))
	}
}

func TestRunMapLimit(t *testing.T) {
	ri := loadFixtureIndex(t)

	t.Run("limit 1 truncates", func(t *testing.T) {
		out, _ := captureWriter(t)
		code := runWith(t, []string{"map", "--limit", "1"}, ri)
		if code != exitOK {
			t.Fatalf("exit = %d, want %d", code, exitOK)
		}
		if !contains(out.String(), "(clusters truncated") {
			t.Errorf("output missing truncation notice:\n%s", out.String())
		}
		if !contains(out.String(), "./  2 files") {
			t.Errorf("output missing top cluster:\n%s", out.String())
		}
		if contains(out.String(), "util/") {
			t.Errorf("output shows cluster beyond limit:\n%s", out.String())
		}
	})

	t.Run("limit -1 shows all", func(t *testing.T) {
		out, _ := captureWriter(t)
		code := runWith(t, []string{"map", "--limit", "-1"}, ri)
		if code != exitOK {
			t.Fatalf("exit = %d, want %d", code, exitOK)
		}
		for _, want := range []string{"./", "util/", "services/"} {
			if !contains(out.String(), want) {
				t.Errorf("output missing cluster %q:\n%s", want, out.String())
			}
		}
		if contains(out.String(), "truncated") {
			t.Errorf("output shows truncation notice without limit:\n%s", out.String())
		}
	})

	t.Run("bad limit is usage error", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := runWith(t, []string{"map", "--limit", "abc"}, ri)
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
		if errb.Len() == 0 {
			t.Error("expected diagnostic on stderr")
		}
	})
}

func TestRunMapPositionalArgs(t *testing.T) {
	ri := loadFixtureIndex(t)
	_, errb := captureWriter(t)

	code := runWith(t, []string{"map", "extra"}, ri)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if errb.Len() == 0 {
		t.Error("expected diagnostic on stderr")
	}
}

func TestRunGlobalIndexFlag(t *testing.T) {
	// --index is a persistent root flag: it must work before the verb as
	// well as after. Regression: extraction used to scan only the args
	// after the verb, so "scipq --index foo.scip map" silently fell back
	// to ./index.scip and exited 2.
	tests := []struct {
		name string
		argv []string
	}{
		{"before verb", []string{"--index", "../../testdata/index.scip", "map"}},
		{"after verb", []string{"map", "--index", "../../testdata/index.scip"}},
		{"equals form before verb", []string{"--index=../../testdata/index.scip", "map"}},
		{"equals form after verb", []string{"map", "--index=../../testdata/index.scip"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, errb := captureWriter(t)
			code := run(tt.argv)
			if code != exitOK {
				t.Errorf("exit = %d, want %d (stderr: %q)", code, exitOK, errb.String())
			}
			if !contains(out.String(), "5 files · 5 symbols") {
				t.Errorf("output missing totals line:\n%s", out.String())
			}
		})
	}

	t.Run("missing index before verb exits 2", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := run([]string{"--index", "/nonexistent/index.scip", "map"})
		if code != exitNoIndex {
			t.Errorf("exit = %d, want %d", code, exitNoIndex)
		}
		if errb.Len() == 0 {
			t.Error("expected diagnostic on stderr")
		}
	})

	t.Run("missing index after verb exits 2", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := run([]string{"map", "--index", "/nonexistent/index.scip"})
		if code != exitNoIndex {
			t.Errorf("exit = %d, want %d", code, exitNoIndex)
		}
		if errb.Len() == 0 {
			t.Error("expected diagnostic on stderr")
		}
	})

	t.Run("no verb is usage error", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := run([]string{"--index", "../../testdata/index.scip"})
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
		if errb.Len() == 0 {
			t.Error("expected usage on stderr")
		}
	})

	t.Run("unknown verb is usage error", func(t *testing.T) {
		_, errb := captureWriter(t)
		code := run([]string{"bogus"})
		if code != exitUsage {
			t.Errorf("exit = %d, want %d", code, exitUsage)
		}
		if !contains(errb.String(), "unknown verb") {
			t.Errorf("stderr missing unknown-verb diagnostic:\n%s", errb.String())
		}
	})
}

func TestShortSymbol(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"go github.com/example/animal Animal#Speak().", "Speak"},
		{"go github.com/example/util Unrelated#Helper().", "Helper"},
		{"go github.com/example/animal Animal", "Animal"},
		// Type-level symbol: empty member position after '#'.
		{"csharp example.assert, Services example.assert.Services/Xunit/Assert#", "Assert"},
		{"go github.com/example/animal Animal#", "Animal"},
		{"", ""},
		// Real scip-go shapes (issue #40, found by the self-index dogfood
		// loop): package-level symbols carry a backticked package path with
		// no '#' separator; local symbols are bare "local N" strings.
		{"scip-go gomod github.com/elodhorvath/scipq 40e534a1c6a0 `github.com/elodhorvath/scipq/cmd/scipq`/exitUsage.", "exitUsage"},
		{"scip-go gomod github.com/elodhorvath/scipq 40e534a1c6a0 `github.com/elodhorvath/scipq/internal/index`/", "index"},
		{"scip-go gomod github.com/elodhorvath/scipq 40e534a1c6a0 `github.com/elodhorvath/scipq/cmd/scipq.test`/benchmarks.", "benchmarks"},
		{"scip-go gomod github.com/elodhorvath/scipq 40e534a1c6a0 `github.com/elodhorvath/scipq/cmd/scipq.test`/", "scipq.test"},
		{"local 8", "local 8"},
		{"local 0", "local 0"},
	}
	for _, tt := range tests {
		if got := shortSymbol(tt.in); got != tt.want {
			t.Errorf("shortSymbol(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHubsAreNonEmpty(t *testing.T) {
	// Review gate: hub names must never render empty, whatever the symbol
	// shape (method, type-level, parameter fragment).
	ri := loadFixtureIndex(t)
	res := computeMap(ri, -1)
	for _, c := range res.Clusters {
		for _, h := range c.Hubs {
			if h.Symbol == "" {
				t.Errorf("cluster %q has a hub with empty name (refs=%d)", c.Dir, h.Refs)
			}
		}
	}
}

func TestParamFragmentsExcluded(t *testing.T) {
	// scip-dotnet parameter fragments ("...(param)") are noise in
	// orientation output and must not appear as hubs.
	ri := loadFixtureIndex(t)
	res := computeMap(ri, -1)
	for _, c := range res.Clusters {
		for _, h := range c.Hubs {
			if contains(h.Symbol, "(param") || contains(h.Symbol, ")") {
				t.Errorf("cluster %q hub %q looks like a parameter fragment", c.Dir, h.Symbol)
			}
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return contains(toLower(s), toLower(sub))
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
