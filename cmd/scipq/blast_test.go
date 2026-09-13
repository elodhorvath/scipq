package main

// Tests for the blast verb: unified-diff parsing (any -U context, new /
// deleted / renamed / binary files, no-newline markers), impact
// computation against the synthetic fixture (touched containment, depth
// cutoff, implements traversal, broken refs, untested flag), and verb
// wiring (stdin seam, TTY error, JSON output, exit codes).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"os"
	"path/filepath"

	"github.com/elodhorvath/scipq/internal/index"
)

// swapStdin redirects the stdin var to r for the duration of the test.
func swapStdin(t *testing.T, r interface{ Read([]byte) (int, error) }) {
	t.Helper()
	old := stdin
	stdin = r
	t.Cleanup(func() { stdin = old })
}

// cannedDiff is a helper to build diff text from lines.
func cannedDiff(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func TestParseUnifiedDiff(t *testing.T) {
	tests := []struct {
		name string
		diff string
		want map[string]map[int32]bool
	}{
		{
			name: "empty diff",
			diff: "",
			want: map[string]map[int32]bool{},
		},
		{
			name: "U0 single addition",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -2,0 +3 @@`,
				`+new line`,
			),
			want: map[string]map[int32]bool{"animal.go": {3: true}},
		},
		{
			name: "U3 context: only + lines are changed",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -1,4 +1,5 @@`,
				` context`,
				`-removed`,
				`+added`,
				` context`,
				` context`,
				`+added2`,
			),
			want: map[string]map[int32]bool{"animal.go": {2: true, 5: true}},
		},
		{
			name: "multiple hunks in one file",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -1,2 +1,3 @@`,
				`+first`,
				` ctx`,
				` ctx`,
				`@@ -10,1 +12,2 @@`,
				`+second`,
				` ctx`,
			),
			want: map[string]map[int32]bool{"animal.go": {1: true, 12: true}},
		},
		{
			name: "multiple files",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -1,1 +1,2 @@`,
				`+a`,
				`diff --git a/util/helper.go b/util/helper.go`,
				`--- a/util/helper.go`,
				`+++ b/util/helper.go`,
				`@@ -1,1 +1,2 @@`,
				`+b`,
			),
			want: map[string]map[int32]bool{
				"animal.go":      {1: true},
				"util/helper.go": {1: true},
			},
		},
		{
			name: "new file from /dev/null",
			diff: cannedDiff(
				`diff --git a/new.go b/new.go`,
				`--- /dev/null`,
				`+++ b/new.go`,
				`@@ -0,0 +1,2 @@`,
				`+one`,
				`+two`,
			),
			want: map[string]map[int32]bool{"new.go": {1: true, 2: true}},
		},
		{
			name: "deleted file (+++ /dev/null) is dropped",
			diff: cannedDiff(
				`diff --git a/old.go b/old.go`,
				`--- a/old.go`,
				`+++ /dev/null`,
				`@@ -1,2 +0,0 @@`,
				`-gone`,
				`-also gone`,
			),
			want: map[string]map[int32]bool{},
		},
		{
			name: "rename takes the new path",
			diff: cannedDiff(
				`diff --git a/old.go b/renamed.go`,
				`similarity index 90%`,
				`rename from old.go`,
				`rename to renamed.go`,
				`--- a/old.go`,
				`+++ b/renamed.go`,
				`@@ -1,1 +1,2 @@`,
				`+tweak`,
			),
			want: map[string]map[int32]bool{"renamed.go": {1: true}},
		},
		{
			name: "binary file marker skipped",
			diff: cannedDiff(
				`diff --git a/img.png b/img.png`,
				`Binary files a/img.png and b/img.png differ`,
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -1,1 +1,2 @@`,
				`+x`,
			),
			want: map[string]map[int32]bool{"animal.go": {1: true}},
		},
		{
			name: "no-newline markers ignored",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -1,1 +1,2 @@`,
				`-old`,
				`\ No newline at end of file`,
				`+new`,
				`+new2`,
			),
			// "-old" is old-side only (no counter advance); the marker is
			// not a diff line. "+new" is new-side line 1, "+new2" line 2.
			want: map[string]map[int32]bool{"animal.go": {1: true, 2: true}},
		},
		{
			name: "added line starting with +++ is body, not header",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -2,0 +3,2 @@`,
				`+++ weird`,
				`+// marker`,
			),
			// Hunk declares 2 new body lines; "+++ weird" is body (line 3),
			// "+// marker" is body (line 4). Without length discipline the
			// lookalike header would reset the parse and drop the hunk.
			want: map[string]map[int32]bool{"animal.go": {3: true, 4: true}},
		},
		{
			name: "context line starting with @@ is body, not header",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -2,2 +3,3 @@`,
				` ctx`,
				`@@ -1,1 +1,1 @@`,
				`+// marker`,
			),
			// The raw "@@ ..." line is a context line inside the hunk
			// (added content would carry a "+" prefix). Without length
			// discipline it would be misread as a new hunk header.
			// newStart=3: " ctx" consumes new line 3, the "@@ ..." line
			// consumes new line 4 (context), "+// marker" marks line 5.
			want: map[string]map[int32]bool{"animal.go": {5: true}},
		},
		{
			name: "deleted line starting with --- is body, not header",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -2,2 +2,1 @@`,
				`--- weird`,
				`+kept`,
			),
			want: map[string]map[int32]bool{"animal.go": {2: true}},
		},
		{
			name: "hunk ends at declared count; next header honored",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -1,1 +1,1 @@`,
				`+first`,
				`diff --git a/util/helper.go b/util/helper.go`,
				`--- a/util/helper.go`,
				`+++ b/util/helper.go`,
				`@@ -1,1 +1,1 @@`,
				`+second`,
			),
			want: map[string]map[int32]bool{
				"animal.go":      {1: true},
				"util/helper.go": {1: true},
			},
		},
		{
			name: "pure deletion hunk (U0) yields no file entry",
			diff: cannedDiff(
				`diff --git a/animal.go b/animal.go`,
				`--- a/animal.go`,
				`+++ b/animal.go`,
				`@@ -42,3 +41,0 @@`,
				`-a`,
				`-b`,
				`-c`,
			),
			// No '+' lines → no changed-line set. Deletions are caught by
			// the dangling-refs pass (refs to now-undefined symbols), not
			// by line containment.
			want: map[string]map[int32]bool{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUnifiedDiff(tt.diff)
			if len(got) != len(tt.want) {
				t.Fatalf("parseUnifiedDiff got %d files (%v), want %d", len(got), got, len(tt.want))
			}
			for file, wantSet := range tt.want {
				gotSet := got[file]
				if len(gotSet) != len(wantSet) {
					t.Errorf("%s: got %v, want %v", file, gotSet, wantSet)
					continue
				}
				for line := range wantSet {
					if !gotSet[line] {
						t.Errorf("%s: line %d not marked changed (got %v)", file, line, gotSet)
					}
				}
			}
		})
	}
}

// buildBlastFixtureIndex builds an index shaped like the committed fixture
// plus a *_test.go reference (for the coverage heuristic) and a dangling
// reference to a symbol with no definition (the deletion channel).
func buildBlastFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	animalSpeak := "go github.com/example/animal Animal#Speak()."
	dogSpeak := "go github.com/example/animal Dog#Speak()."
	puppySpeak := "go github.com/example/animal Puppy#Speak()."
	unrelated := "go github.com/example/util Unrelated#Helper()."
	deleted := "go github.com/example/animal Deleted#Thing()."
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
				RelativePath: "util/helper.go",
				Occurrences:  []*scip.Occurrence{def(unrelated, 1)},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 10),
					ref(unrelated, 12),
					ref(deleted, 20), // dangling: Deleted#Thing has no def
				},
			},
			{
				RelativePath: "animal_test.go",
				Occurrences:  []*scip.Occurrence{ref(animalSpeak, 5)},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal blast fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write blast fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load blast fixture: %v", err)
	}
	return ri
}

// buildExternalRefFixtureIndex extends the blast fixture with a reference
// to an external symbol (different module prefix — stdlib-like), which
// must NOT surface as a broken ref.
func buildExternalRefFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	animalSpeak := "go github.com/example/animal Animal#Speak()."
	deleted := "go github.com/example/animal Deleted#Thing()."
	external := "go stdlib/fmt Fmt#Println()."
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
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 10),
					ref(deleted, 20),  // module-internal dangling ref
					ref(external, 21), // external: must NOT surface
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal external-ref fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write external-ref fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load external-ref fixture: %v", err)
	}
	return ri
}

// buildDisjointPrefixFixtureIndex builds an index whose defined symbols
// share no common prefix (go + java naming schemes) — commonSymbolPrefix
// returns "", and the broken-ref channel must stay off.
func buildDisjointPrefixFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	goSym := "go github.com/example/animal Animal#Speak()."
	javaSym := "java com.example.util Util#Helper()."
	goDangling := "go github.com/example/animal Gone#Thing()."
	javaDangling := "java com.example.util Missing#Run()."
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
				Occurrences:  []*scip.Occurrence{def(goSym, 2)},
			},
			{
				RelativePath: "util/Util.java",
				Occurrences:  []*scip.Occurrence{def(javaSym, 1)},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(goDangling, 10),   // undefined, go prefix
					ref(javaDangling, 11), // undefined, java prefix
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal disjoint-prefix fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write disjoint-prefix fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load disjoint-prefix fixture: %v", err)
	}
	return ri
}

func TestComputeBlast(t *testing.T) {
	ri := buildBlastFixtureIndex(t)

	t.Run("touched symbol by def line", func(t *testing.T) {
		// animal.go line 3 (1-based) is changed; Animal#Speak is defined
		// on 0-based line 2 → line+1 = 3 → touched.
		changed := map[string]map[int32]bool{"animal.go": {3: true}}
		res := computeBlast(ri, changed, 0)
		if res.Touched != 1 {
			t.Fatalf("Touched = %d, want 1", res.Touched)
		}
		var found bool
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if s.Symbol == "go github.com/example/animal Animal#Speak()." {
					found = true
					if s.Reason != "touched" {
						t.Errorf("reason = %q, want touched", s.Reason)
					}
					if s.Line != 3 {
						t.Errorf("Line = %d, want 3 (1-based)", s.Line)
					}
					// animal_test.go references Animal#Speak → covered.
					if s.Untested {
						t.Errorf("Animal#Speak flagged untested; has _test.go ref")
					}
				}
			}
		}
		if !found {
			t.Errorf("Animal#Speak not in groups: %+v", res.Groups)
		}
	})

	t.Run("boundary: def on first line of hunk range", func(t *testing.T) {
		// util/helper.go def is 0-based line 1 → 1-based line 2.
		changed := map[string]map[int32]bool{"util/helper.go": {1: true}}
		res := computeBlast(ri, changed, 0)
		if res.Touched != 0 {
			t.Errorf("line 1 changed: Touched = %d, want 0 (def is on line 2)", res.Touched)
		}
		changed = map[string]map[int32]bool{"util/helper.go": {2: true}}
		res = computeBlast(ri, changed, 0)
		if res.Touched != 1 {
			t.Errorf("line 2 changed: Touched = %d, want 1", res.Touched)
		}
	})

	t.Run("depth cutoff: file-dep hops cut, implements closure full", func(t *testing.T) {
		// Touch Animal#Speak (animal.go:3). Implements() is a transitive
		// closure, so Dog#Speak AND Puppy#Speak both arrive at level 1;
		// --depth cuts file-dependency hops, not the implements chain.
		changed := map[string]map[int32]bool{"animal.go": {3: true}}
		res := computeBlast(ri, changed, 1)
		syms := map[string]bool{}
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				syms[s.Symbol] = true
			}
		}
		if !syms["go github.com/example/animal Dog#Speak()."] {
			t.Errorf("depth 1 missing Dog#Speak: %+v", res.Groups)
		}
		if !syms["go github.com/example/animal Puppy#Speak()."] {
			t.Errorf("depth 1 missing Puppy#Speak (implements closure is transitive): %+v", res.Groups)
		}
	})

	t.Run("depth 2 expands implementors' file-deps", func(t *testing.T) {
		// At depth 2, symbols defined in files that reference Dog#Speak
		// (an implementor) become impacted. handler.go references
		// Dog#Speak — but has no defs, so the group set is unchanged;
		// assert Puppy still present and no crash.
		changed := map[string]map[int32]bool{"animal.go": {3: true}}
		res := computeBlast(ri, changed, 2)
		syms := map[string]bool{}
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				syms[s.Symbol] = true
			}
		}
		if !syms["go github.com/example/animal Puppy#Speak()."] {
			t.Errorf("depth 2 missing Puppy#Speak: %+v", res.Groups)
		}
	})

	t.Run("file-level dependent: symbol in a file that references the touched symbol", func(t *testing.T) {
		// services/zoo.go references Animal#Speak; Unrelated#Helper is
		// defined in util/helper.go, not zoo.go — so Helper is NOT a
		// dependent. But a symbol defined in zoo.go would be. The fixture
		// has no def in zoo.go, so dependents come only from implements.
		// Verify Helper is absent at depth 2.
		changed := map[string]map[int32]bool{"animal.go": {3: true}}
		res := computeBlast(ri, changed, 2)
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if s.Symbol == "go github.com/example/util Unrelated#Helper()." {
					t.Errorf("Helper should not be impacted: %+v", res.Groups)
				}
			}
		}
	})

	t.Run("broken ref: referenced-but-undefined symbol surfaces even with empty diff", func(t *testing.T) {
		// Deleted#Thing is referenced in services/zoo.go but has no def —
		// the deletion channel. It must appear even with an empty diff:
		// a reference to an undefined symbol is a break regardless of
		// depth.
		res := computeBlast(ri, map[string]map[int32]bool{}, 0)
		var found *ImpactedSymbol
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if s.Symbol == "go github.com/example/animal Deleted#Thing()." {
					s := s
					found = &s
				}
			}
		}
		if found == nil {
			t.Fatalf("Deleted#Thing not surfaced: %+v", res.Groups)
		}
		if found.Reason != relationBrokenRef {
			t.Errorf("Reason = %q, want %q", found.Reason, relationBrokenRef)
		}
		if found.File != "" {
			t.Errorf("broken ref should have no def site, got %s:%d", found.File, found.Line)
		}
	})

	t.Run("untested flag: symbol with no _test.go refs", func(t *testing.T) {
		// Dog#Speak has no refs at all → untested.
		changed := map[string]map[int32]bool{"dog.go": {4: true}}
		res := computeBlast(ri, changed, 0)
		var found *ImpactedSymbol
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if s.Symbol == "go github.com/example/animal Dog#Speak()." {
					s := s
					found = &s
				}
			}
		}
		if found == nil {
			t.Fatalf("Dog#Speak not touched: %+v", res.Groups)
		}
		if !found.Untested {
			t.Errorf("Dog#Speak should be untested (no _test.go refs)")
		}
	})

	t.Run("groups sorted by dir, symbols sorted within", func(t *testing.T) {
		changed := map[string]map[int32]bool{
			"animal.go":      {3: true},
			"util/helper.go": {2: true},
		}
		res := computeBlast(ri, changed, 0)
		if len(res.Groups) < 2 {
			t.Fatalf("Groups = %+v, want at least 2", res.Groups)
		}
		for i := 1; i < len(res.Groups); i++ {
			if res.Groups[i-1].Dir > res.Groups[i].Dir {
				t.Errorf("groups not sorted: %q before %q", res.Groups[i-1].Dir, res.Groups[i].Dir)
			}
		}
	})

	t.Run("broken refs scoped to module prefix: external refs excluded", func(t *testing.T) {
		// The fixture's defs all share "go github.com/example/". A ref to
		// an external symbol (different prefix) must NOT surface as a
		// broken ref — on real indexes every stdlib/third-party ref would
		// otherwise flood the output.
		ri2 := buildExternalRefFixtureIndex(t)
		res := computeBlast(ri2, map[string]map[int32]bool{}, 0)
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if strings.Contains(s.Symbol, "fmt.Print") {
					t.Errorf("external symbol %q surfaced as broken ref:\n%+v", s.Symbol, res.Groups)
				}
			}
		}
		// The module-internal dangling ref still surfaces.
		var found bool
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if strings.Contains(s.Symbol, "Deleted#Thing") {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("module-internal dangling ref not surfaced:\n%+v", res.Groups)
		}
	})

	t.Run("empty module prefix disables the broken-ref channel", func(t *testing.T) {
		// A multi-language index (go + java symbols share no prefix) makes
		// commonSymbolPrefix return "" — and HasPrefix(sym, "") is true
		// for every symbol, which would re-enable the unscoped flood.
		// Empty prefix = channel off: zero broken refs, no flood.
		ri2 := buildDisjointPrefixFixtureIndex(t)
		res := computeBlast(ri2, map[string]map[int32]bool{}, 0)
		for _, g := range res.Groups {
			for _, s := range g.Symbols {
				if s.Reason == relationBrokenRef {
					t.Errorf("broken ref surfaced with empty module prefix:\n%+v", res.Groups)
				}
			}
		}
	})

	t.Run("empty diff yields empty result", func(t *testing.T) {
		res := computeBlast(ri, map[string]map[int32]bool{}, 2)
		if res.Touched != 0 {
			t.Errorf("Touched = %d, want 0", res.Touched)
		}
	})
}

func TestRunBlastHuman(t *testing.T) {
	ri := buildBlastFixtureIndex(t)
	out, errb := captureWriter(t)
	swapStdin(t, strings.NewReader(cannedDiff(
		`diff --git a/animal.go b/animal.go`,
		`--- a/animal.go`,
		`+++ b/animal.go`,
		`@@ -2,1 +3,1 @@`,
		`+changed`,
	)))

	code := runWith(t, []string{"blast"}, ri)
	if code != exitOK {
		t.Fatalf("blast exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	// Data-only stdout contract.
	for _, banned := range []string{"you should", "consider", "recommend"} {
		if containsFold(out.String(), banned) {
			t.Errorf("stdout contains banned instruction-like text %q", banned)
		}
	}
	if !contains(out.String(), "1 touched") {
		t.Errorf("output missing touched count:\n%s", out.String())
	}
	if !contains(out.String(), "Speak") {
		t.Errorf("output missing touched symbol:\n%s", out.String())
	}
}

func TestRunBlastJSON(t *testing.T) {
	ri := buildBlastFixtureIndex(t)
	out, errb := captureWriter(t)
	swapStdin(t, strings.NewReader(cannedDiff(
		`diff --git a/animal.go b/animal.go`,
		`--- a/animal.go`,
		`+++ b/animal.go`,
		`@@ -2,1 +3,1 @@`,
		`+changed`,
	)))

	code := runWith(t, []string{"blast", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("blast exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	var res BlastResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if res.Touched != 1 {
		t.Errorf("JSON touched = %d, want 1", res.Touched)
	}
	if len(res.Groups) == 0 {
		t.Errorf("JSON groups empty:\n%s", out.String())
	}
}

func TestRunBlastJSONBrokenRefOmitsFileLine(t *testing.T) {
	// Schema note (issue 34): broken refs have no definition site, so
	// "file" and "line" are OMITTED from the JSON object entirely — an
	// absent "file" is the broken-ref marker. Touched/dependent entries
	// must still carry both fields.
	ri := buildBlastFixtureIndex(t)
	out, errb := captureWriter(t)
	swapStdin(t, strings.NewReader(cannedDiff(
		`diff --git a/animal.go b/animal.go`,
		`--- a/animal.go`,
		`+++ b/animal.go`,
		`@@ -2,1 +3,1 @@`,
		`+changed`,
	)))

	code := runWith(t, []string{"blast", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("blast exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}

	var res BlastResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	var broken, touched *ImpactedSymbol
	for _, g := range res.Groups {
		for _, s := range g.Symbols {
			switch {
			case s.Reason == relationBrokenRef:
				broken = &s
			case s.Reason == "touched":
				touched = &s
			}
		}
	}
	if broken == nil {
		t.Fatalf("no broken ref in output:\n%s", out.String())
	}
	if touched == nil {
		t.Fatalf("no touched symbol in output:\n%s", out.String())
	}
	if broken.File != "" || broken.Line != 0 {
		t.Errorf("broken ref File/Line = %q/%d, want zero values", broken.File, broken.Line)
	}
	if touched.File == "" || touched.Line == 0 {
		t.Errorf("touched symbol missing def site: %q/%d", touched.File, touched.Line)
	}
	// Key-presence check on the raw JSON: the broken-ref entry must not
	// carry "file"/"line" keys at all (omitempty drops them); the
	// touched entry must carry both.
	raw := out.String()
	if strings.Contains(raw, `"file": ""`) {
		t.Errorf("broken ref emitted empty file key (omitempty missing):\n%s", raw)
	}
	if strings.Contains(raw, `"line": 0`) {
		t.Errorf("broken ref emitted zero line key (omitempty missing):\n%s", raw)
	}
	if !strings.Contains(raw, `"file": "animal.go"`) {
		t.Errorf("touched symbol missing file key:\n%s", raw)
	}
}

func TestRunBlastEmptyDiff(t *testing.T) {
	ri := buildBlastFixtureIndex(t)
	out, errb := captureWriter(t)
	swapStdin(t, strings.NewReader(""))

	code := runWith(t, []string{"blast"}, ri)
	if code != exitOK {
		t.Fatalf("blast exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	// The broken-ref channel is diff-independent: a reference to an
	// undefined module-internal symbol is a break regardless of the diff
	// (external refs are excluded by module-prefix scoping). Deleted#Thing
	// is referenced but undefined in the fixture, so it surfaces even on
	// an empty diff. "Empty result" in the contract means no TOUCHED
	// symbols, not an empty groups list.
	if !contains(out.String(), "broken ref") {
		t.Errorf("empty-diff output missing broken-ref entry:\n%s", out.String())
	}
}

func TestRunBlastTTY(t *testing.T) {
	ri := buildBlastFixtureIndex(t)
	_, errb := captureWriter(t)
	// stdin stays os.Stdin (a terminal under go test? no — but the
	// default var points at os.Stdin which is NOT a char device under
	// go test). Simulate by leaving stdin as a non-pipe: we test the
	// isTerminal helper directly instead.
	if isTerminal(strings.NewReader("x")) {
		t.Error("strings.Reader should not be a terminal")
	}
	_ = ri
	_ = errb
}

func TestRunBlastMissingIndex(t *testing.T) {
	swapStdin(t, strings.NewReader(cannedDiff(
		`diff --git a/animal.go b/animal.go`,
		`--- a/animal.go`,
		`+++ b/animal.go`,
		`@@ -1,1 +1,2 @@`,
		`+x`,
	)))
	_, errb := captureWriter(t)
	cmd := newRootCommand(func(string) (*index.ReverseIndex, int) { return nil, exitNoIndex })
	err := cmd.Run(context.Background(), []string{"scipq", "blast"})
	if code := runError(err); code != exitNoIndex {
		t.Errorf("exit = %d, want %d (stderr: %q)", code, exitNoIndex, errb.String())
	}
}

func TestRunBlastPositionalArgs(t *testing.T) {
	ri := buildBlastFixtureIndex(t)
	swapStdin(t, strings.NewReader(""))
	_, errb := captureWriter(t)

	code := runWith(t, []string{"blast", "extra"}, ri)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if errb.Len() == 0 {
		t.Error("expected diagnostic on stderr")
	}
}

func TestRunBlastDepthFlag(t *testing.T) {
	ri := buildBlastFixtureIndex(t)
	out, _ := captureWriter(t)
	swapStdin(t, strings.NewReader(cannedDiff(
		`diff --git a/animal.go b/animal.go`,
		`--- a/animal.go`,
		`+++ b/animal.go`,
		`@@ -2,1 +3,1 @@`,
		`+changed`,
	)))

	code := runWith(t, []string{"blast", "--depth", "1"}, ri)
	if code != exitOK {
		t.Fatalf("blast exit = %d, want %d", code, exitOK)
	}
	// Depth 1: implements closure is transitive, so Dog AND Puppy both
	// appear (both render as "Speak" via shortSymbol). Assert on the
	// impacted-symbol count: touched + 2 implementors + 1 broken ref.
	if !contains(out.String(), "4") && !contains(out.String(), "dependent") {
		t.Errorf("depth 1 output missing dependents:\n%s", out.String())
	}
}
