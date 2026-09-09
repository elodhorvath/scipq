package main

// Tests for the map verb: cluster/hub/hotspot computation against the
// synthetic fixture, human and JSON rendering, and flag handling.

import (
	"bytes"
	"encoding/json"
	"testing"

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

	code := runMap([]string{}, ri, false)
	if code != exitOK {
		t.Fatalf("runMap exit = %d, want %d", code, exitOK)
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

func TestRunMapJSON(t *testing.T) {
	ri := loadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runMap([]string{"--json"}, ri, true)
	if code != exitOK {
		t.Fatalf("runMap exit = %d, want %d", code, exitOK)
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
		code := runMap([]string{"--limit", "1"}, ri, false)
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
		code := runMap([]string{"--limit", "-1"}, ri, false)
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
		code := runMap([]string{"--limit", "abc"}, ri, false)
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

	code := runMap([]string{"extra"}, ri, false)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if errb.Len() == 0 {
		t.Error("expected diagnostic on stderr")
	}
}

func TestRunGlobalIndexFlag(t *testing.T) {
	// --index is a global flag: it must work before the verb as well as
	// after. Regression: extraction used to scan only the args after the
	// verb, so "scipq --index foo.scip map" silently fell back to
	// ./index.scip and exited 2.
	tests := []struct {
		name string
		argv []string
	}{
		{"before verb", []string{"--index", "../../testdata/index.scip", "map"}},
		{"after verb", []string{"map", "--index", "../../testdata/index.scip"}},
		{"equals form before verb", []string{"--index=../../testdata/index.scip", "map"}},
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

func TestExtractStringFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantValue string
		wantRest  []string
		wantErr   bool
	}{
		{
			name:      "space form",
			args:      []string{"--index", "a.scip", "--json"},
			wantValue: "a.scip",
			wantRest:  []string{"--json"},
		},
		{
			name:      "equals form",
			args:      []string{"--index=b.scip", "other"},
			wantValue: "b.scip",
			wantRest:  []string{"other"},
		},
		{
			name:      "absent",
			args:      []string{"--json"},
			wantValue: "",
			wantRest:  []string{"--json"},
		},
		{
			name:    "missing value",
			args:    []string{"--index"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, rest, err := extractStringFlag(tt.args, "index")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if v != tt.wantValue {
				t.Errorf("value = %q, want %q", v, tt.wantValue)
			}
			if len(rest) != len(tt.wantRest) {
				t.Fatalf("rest = %v, want %v", rest, tt.wantRest)
			}
			for i := range rest {
				if rest[i] != tt.wantRest[i] {
					t.Errorf("rest[%d] = %q, want %q", i, rest[i], tt.wantRest[i])
				}
			}
		})
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
