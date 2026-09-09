// Tests for the callers verb: symbol resolution (exact, suffix, ambiguous,
// unknown), site computation (direct + transitive implements), human and
// JSON rendering, and flag handling.
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/elodhorvath/scipq/internal/index"
)

func TestNameMatches(t *testing.T) {
	tests := []struct {
		sym   string
		query string
		want  bool
	}{
		{"go github.com/example/animal Animal#Speak().", "go github.com/example/animal Animal#Speak().", true},
		{"go github.com/example/animal Animal#Speak().", "Animal#Speak().", true},
		{"go github.com/example/animal Animal#Speak().", "Animal#Speak", true},
		{"go github.com/example/animal Animal#Speak().", "Speak", true},
		{"go github.com/example/animal Animal#Speak().", "Speak()", true},
		{"go github.com/example/animal Animal.", "Animal", true},
		{"go github.com/example/animal Animal.", "Speak", false},
		{"go github.com/example/animal Animal#Speak().", "eak", false},
		{"go github.com/example/animal Animal#Speak().", "animal", false},
		{"", "", true},
		{"", "x", false},
	}
	for _, tt := range tests {
		if got := nameMatches(tt.sym, tt.query); got != tt.want {
			t.Errorf("nameMatches(%q, %q) = %v, want %v", tt.sym, tt.query, got, tt.want)
		}
	}
}

func TestResolveSymbol(t *testing.T) {
	ri := loadFixtureIndex(t)

	t.Run("full symbol string", func(t *testing.T) {
		sym, matches := resolveSymbol(ri, "go github.com/example/animal Animal#Speak().")
		if sym != "go github.com/example/animal Animal#Speak()." || matches != nil {
			t.Errorf("got (%q, %v), want resolved symbol", sym, matches)
		}
	})

	t.Run("member name suffix", func(t *testing.T) {
		// "Speak" collides across Animal/Dog/Puppy — ambiguity is the
		// correct outcome; the listing is asserted below.
		sym, matches := resolveSymbol(ri, "Speak")
		if sym != "" || len(matches) != 3 {
			t.Errorf("got (%q, %d matches), want ambiguous with 3 matches", sym, len(matches))
		}
	})

	t.Run("qualified member suffix", func(t *testing.T) {
		// Type-qualified query resolves uniquely.
		sym, matches := resolveSymbol(ri, "Animal#Speak")
		if sym != "go github.com/example/animal Animal#Speak()." || matches != nil {
			t.Errorf("got (%q, %v), want resolved symbol", sym, matches)
		}
	})

	t.Run("type name suffix", func(t *testing.T) {
		sym, matches := resolveSymbol(ri, "Animal")
		if sym != "go github.com/example/animal Animal." || matches != nil {
			t.Errorf("got (%q, %v), want resolved symbol", sym, matches)
		}
	})

	t.Run("unique name resolves", func(t *testing.T) {
		sym, matches := resolveSymbol(ri, "Helper")
		if sym != "go github.com/example/util Unrelated#Helper()." || matches != nil {
			t.Errorf("got (%q, %v), want resolved symbol", sym, matches)
		}
	})

	t.Run("ambiguous fixture query", func(t *testing.T) {
		// "Speak" collides across the three Speak members.
		sym, matches := resolveSymbol(ri, "Speak")
		if sym != "" {
			t.Errorf("sym = %q, want empty for ambiguous query", sym)
		}
		if len(matches) != 3 {
			t.Fatalf("got %d matches, want 3", len(matches))
		}
		if matches[0].Symbol != "go github.com/example/animal Animal#Speak()." || matches[0].DefinedIn != "animal.go" {
			t.Errorf("match[0] = %+v, want Animal#Speak in animal.go", matches[0])
		}
	})

	t.Run("unknown symbol", func(t *testing.T) {
		sym, matches := resolveSymbol(ri, "Nonexistent")
		if sym != "" || matches != nil {
			t.Errorf("got (%q, %v), want empty/empty", sym, matches)
		}
	})
}

// buildTransitiveFixtureIndex builds an index shaped like the committed
// fixture plus one reference to Dog#Speak() (services/handler.go line 5,
// 0-based), so the transitive implements path is exercisable without
// changing the shared fixture's hub counts.
func buildTransitiveFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	animalSpeak := "go github.com/example/animal Animal#Speak()."
	dogSpeak := "go github.com/example/animal Dog#Speak()."
	puppySpeak := "go github.com/example/animal Puppy#Speak()."
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
				RelativePath: "animal.go",
				Occurrences:  []*scip.Occurrence{def(animalSpeak, 2)},
			},
			{
				RelativePath: "dog.go",
				Occurrences:  []*scip.Occurrence{def(dogSpeak, 3), def(puppySpeak, 9)},
				// Implements edges are declared on the implementing
				// symbol and point at the symbol it implements (SCIP spec).
				Symbols: []*scip.SymbolInformation{
					{
						Symbol: dogSpeak,
						Relationships: []*scip.Relationship{{
							Symbol:           animalSpeak,
							IsImplementation: true,
						}},
					},
					{
						Symbol: puppySpeak,
						Relationships: []*scip.Relationship{{
							Symbol:           dogSpeak,
							IsImplementation: true,
						}},
					},
				},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 10),
					ref(animalSpeak, 14),
				},
			},
			{
				RelativePath: "services/handler.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 4),
					ref(dogSpeak, 5),
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal transitive fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write transitive fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load transitive fixture: %v", err)
	}
	return ri
}

func TestComputeCallers(t *testing.T) {
	ri := loadFixtureIndex(t)

	t.Run("direct refs", func(t *testing.T) {
		res := computeCallers(ri, "go github.com/example/util Unrelated#Helper().")
		if res.Symbol != "go github.com/example/util Unrelated#Helper()." {
			t.Errorf("Symbol = %q", res.Symbol)
		}
		want := []CallerSite{
			{File: "services/zoo.go", Line: 13, Relation: "call"},
		}
		if len(res.Sites) != len(want) {
			t.Fatalf("Sites = %+v, want %+v", res.Sites, want)
		}
		for i := range want {
			if res.Sites[i] != want[i] {
				t.Errorf("Sites[%d] = %+v, want %+v", i, res.Sites[i], want[i])
			}
		}
	})

	t.Run("transitive implements chain", func(t *testing.T) {
		// Animal#Speak is referenced 3× directly; Dog#Speak (implements
		// Animal) is referenced once more. Puppy#Speak implements Dog, so
		// it is in the transitive closure but has no references of its own.
		ri := buildTransitiveFixtureIndex(t)
		res := computeCallers(ri, "go github.com/example/animal Animal#Speak().")
		want := []CallerSite{
			{File: "services/handler.go", Line: 5, Relation: "call"},
			{File: "services/handler.go", Line: 6, Relation: "implements (via Dog#Speak())"},
			{File: "services/zoo.go", Line: 11, Relation: "call"},
			{File: "services/zoo.go", Line: 15, Relation: "call"},
		}
		if len(res.Sites) != len(want) {
			t.Fatalf("Sites = %+v, want %+v", res.Sites, want)
		}
		for i := range want {
			if res.Sites[i] != want[i] {
				t.Errorf("Sites[%d] = %+v, want %+v", i, res.Sites[i], want[i])
			}
		}
	})

	t.Run("no refs yields empty sites", func(t *testing.T) {
		res := computeCallers(ri, "go github.com/example/animal Puppy#Speak().")
		if len(res.Sites) != 0 {
			t.Errorf("Sites = %+v, want none", res.Sites)
		}
	})
}

func TestRunCallersHuman(t *testing.T) {
	ri := buildTransitiveFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runCallers([]string{"Animal#Speak"}, ri, false)
	if code != exitOK {
		t.Fatalf("runCallers exit = %d, want %d", code, exitOK)
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
		"go github.com/example/animal Animal#Speak().  (4 sites)",
		"services/handler.go:5  call",
		"services/handler.go:6  implements (via Dog#Speak())",
		"services/zoo.go:11     call",
		"services/zoo.go:15     call",
	} {
		if !contains(out.String(), want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out.String())
		}
	}
}

func TestRunCallersJSON(t *testing.T) {
	ri := buildTransitiveFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runCallers([]string{"Animal#Speak", "--json"}, ri, true)
	if code != exitOK {
		t.Fatalf("runCallers exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	var res CallersResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if res.Symbol != "go github.com/example/animal Animal#Speak()." {
		t.Errorf("JSON symbol = %q", res.Symbol)
	}
	if len(res.Sites) != 4 {
		t.Fatalf("JSON sites = %d, want 4\n%s", len(res.Sites), out.String())
	}
	// Transitive implements site present in JSON.
	found := 0
	for _, s := range res.Sites {
		if s.File == "services/handler.go" && s.Line == 6 && s.Relation == "implements (via Dog#Speak())" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("JSON transitive implements sites = %d, want 1\n%s", found, out.String())
	}
}

func TestRunCallersAmbiguous(t *testing.T) {
	// Two distinct symbols whose tails both end in "Helper" at a boundary:
	// Unrelated#Helper() and Unrelated#Helper(int) — different descriptors,
	// same member name.
	ri := buildCallersFixtureIndex(t)
	_, errb := captureWriter(t)

	code := runCallers([]string{"Helper"}, ri, false)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	diag := errb.String()
	for _, want := range []string{
		`"Helper" is ambiguous; 2 matching symbols:`,
		"go github.com/example/util Unrelated#Helper().",
		"go github.com/example/util Unrelated#Helper(int).",
	} {
		if !contains(diag, want) {
			t.Errorf("stderr missing %q\n--- got ---\n%s", want, diag)
		}
	}
}

func TestRunCallersUnknown(t *testing.T) {
	ri := loadFixtureIndex(t)
	_, errb := captureWriter(t)

	code := runCallers([]string{"Nonexistent"}, ri, false)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if !contains(errb.String(), "no symbol matching") {
		t.Errorf("stderr missing unknown-symbol diagnostic:\n%s", errb.String())
	}
}

func TestRunCallersMissingIndex(t *testing.T) {
	// The missing-index path exits 2 in loadIndex before the verb runs.
	// Verify the sentinel the CLI maps to exit 2 is reported for a missing
	// file, matching the exit-code contract (0 success, 1 usage, 2 missing
	// index).
	_, err := index.Load(filepath.Join(t.TempDir(), "nope", "index.scip"))
	if err == nil {
		t.Fatal("Load succeeded, want error")
	}
	if !errors.Is(err, index.ErrNotFound) {
		t.Errorf("error %v does not wrap ErrNotFound", err)
	}
}

func TestRunCallersArgHandling(t *testing.T) {
	ri := loadFixtureIndex(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"two args", []string{"Speak", "Helper"}},
		{"empty arg", []string{""}},
		{"unknown flag", []string{"--bogus", "Speak"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errb := captureWriter(t)
			code := runCallers(tt.args, ri, false)
			if code != exitUsage {
				t.Errorf("exit = %d, want %d", code, exitUsage)
			}
			if errb.Len() == 0 {
				t.Error("expected diagnostic on stderr")
			}
		})
	}
}

// buildCallersFixtureIndex builds an index with two same-named members for
// the ambiguity test: Unrelated#Helper() and Unrelated#Helper(int) —
// different descriptors, same member name, so the query "Helper" collides.
func buildCallersFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	helper1 := "go github.com/example/util Unrelated#Helper()."
	helper2 := "go github.com/example/util Unrelated#Helper(int)."
	def := func(sym string, line int32) *scip.Occurrence {
		return &scip.Occurrence{
			Range:       []int32{line, 0, 10},
			Symbol:      sym,
			SymbolRoles: int32(scip.SymbolRole_Definition),
		}
	}
	idx := &scip.Index{
		Metadata: &scip.Metadata{
			ToolInfo:    &scip.ToolInfo{Name: "scipq-test", Version: "0.0.0"},
			ProjectRoot: "file:///synthetic",
		},
		Documents: []*scip.Document{
			{
				RelativePath: "util/helper.go",
				Occurrences: []*scip.Occurrence{
					def(helper1, 1),
					def(helper2, 2),
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal callers fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write callers fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load callers fixture: %v", err)
	}
	return ri
}

func TestRunCallersJSONFlagStripped(t *testing.T) {
	// --json must not be treated as the symbol argument.
	ri := loadFixtureIndex(t)
	out, _ := captureWriter(t)

	code := runCallers([]string{"--json", "Helper"}, ri, true)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if !contains(out.String(), `"symbol"`) {
		t.Errorf("output is not JSON:\n%s", out.String())
	}
}
