// Tests for the skeleton verb: file resolution (exact, suffix, ambiguous,
// unknown), def listing against a dedicated synthetic index (locals and
// bare package clauses filtered, kinds derived, exported marker), human
// and JSON rendering, and flag handling.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/elodhorvath/scipq/internal/index"
)

// buildSkeletonFixtureIndex builds a dedicated synthetic index for the
// skeleton verb. It exercises every contract dimension without touching
// the shared fixture (whose map counts are pinned elsewhere):
//
//	animal.go            type Animal (line 1), method Animal#Speak() (line 2), unexported method Animal#speak() (line 3)
//	util/helper.go       package-level const `pkg`/maxSize. (line 1), function helper() (line 2)
//	locals.go            SCIP local "local 0" (line 1) + bare package clause (line 2) — both dropped
//	services/helper.go   same basename as util/helper.go — suffix ambiguity
func buildSkeletonFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	animalType := "go github.com/example/animal Animal."
	animalSpeak := "go github.com/example/animal Animal#Speak()."
	animalSpeakLower := "go github.com/example/animal Animal#speak()."
	maxSize := "go github.com/example/util `github.com/example/util`/maxSize."
	helperFn := "go github.com/example/util helper()."
	localSym := "local 0"
	pkgClause := "go github.com/example/util `github.com/example/util`/"
	otherSym := "go github.com/example/util Other#Thing()."

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
				RelativePath: "animal.go",
				Occurrences: []*scip.Occurrence{
					def(animalType, 1),
					def(animalSpeak, 2),
					def(animalSpeakLower, 3),
				},
				// Indexer-recorded metadata for one symbol: exercises the
				// recorded-kind/display-name path over the grammar fallback.
				Symbols: []*scip.SymbolInformation{
					{
						Symbol:      animalType,
						DisplayName: "Animal",
						Kind:        scip.SymbolInformation_Class,
					},
				},
			},
			{
				RelativePath: "util/helper.go",
				Occurrences: []*scip.Occurrence{
					def(maxSize, 1),
					def(helperFn, 2),
				},
			},
			{
				RelativePath: "services/helper.go",
				Occurrences:  []*scip.Occurrence{def(otherSym, 1)},
			},
			{
				RelativePath: "locals.go",
				Occurrences: []*scip.Occurrence{
					def(localSym, 1),
					def(pkgClause, 2),
				},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					{Range: []int32{10, 0, 10}, Symbol: animalSpeak}, // ref only
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal skeleton fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write skeleton fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load skeleton fixture: %v", err)
	}
	return ri
}

func TestResolveFile(t *testing.T) {
	files := []string{"animal.go", "util/helper.go", "services/helper.go", "services/zoo.go"}

	tests := []struct {
		name  string
		query string
		want  string
		wantN int // matches count when unresolved
	}{
		{"exact match", "animal.go", "animal.go", 0},
		{"exact nested path", "util/helper.go", "util/helper.go", 0},
		{"unique suffix", "zoo.go", "services/zoo.go", 0},
		{"ambiguous suffix", "helper.go", "", 2},
		{"unknown", "nope.go", "", 0},
		{"suffix must be path-bounded", "il/hel", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matches := resolveFile(files, tt.query)
			if got != tt.want {
				t.Errorf("resolveFile(%q) = %q, want %q", tt.query, got, tt.want)
			}
			if tt.want == "" && len(matches) != tt.wantN {
				t.Errorf("resolveFile(%q) matches = %d, want %d", tt.query, len(matches), tt.wantN)
			}
		})
	}
}

func TestIsLocalSymbol(t *testing.T) {
	tests := []struct {
		sym  string
		want bool
	}{
		{"local 8", true},
		{"local 0", true},
		{"local 123", true},
		{"local", false},    // no space-separated rest
		{"local x", false},  // non-digit rest
		{"local 8x", false}, // mixed
		{"go mod Animal#Speak().", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isLocalSymbol(tt.sym); got != tt.want {
			t.Errorf("isLocalSymbol(%q) = %v, want %v", tt.sym, got, tt.want)
		}
	}
}

func TestIsBarePackageSymbol(t *testing.T) {
	tests := []struct {
		sym  string
		want bool
	}{
		{"go github.com/example/util `github.com/example/util`/", true},
		{"go github.com/example/util `github.com/example/util`/maxSize.", false},
		{"go github.com/example/animal Animal#Speak().", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isBarePackageSymbol(tt.sym); got != tt.want {
			t.Errorf("isBarePackageSymbol(%q) = %v, want %v", tt.sym, got, tt.want)
		}
	}
}

func TestDeriveKind(t *testing.T) {
	tests := []struct {
		sym      string
		recorded string
		want     string
	}{
		// Grammar fallback (no recorded kind).
		{"go github.com/example/animal Animal#Speak().", "", "method"},
		{"go github.com/example/animal Animal#speak().", "", "method"},
		{"go github.com/example/animal Animal#Name", "", "property"},
		{"go github.com/example/util helper().", "", "function"},
		{"go github.com/example/animal Animal.", "", "type"},
		{"go github.com/example/util `pkg`/maxSize.", "", "type"},
		{"weird shape", "", "def"},
		// Indexer-recorded kind wins when populated.
		{"go github.com/example/animal Animal.", "Class", "class"},
		{"go github.com/example/animal Animal#Speak().", "Method", "method"},
		// UnspecifiedKind falls through to grammar.
		{"go github.com/example/animal Animal#Speak().", "UnspecifiedKind", "method"},
	}
	for _, tt := range tests {
		if got := deriveKind(tt.sym, tt.recorded); got != tt.want {
			t.Errorf("deriveKind(%q, %q) = %q, want %q", tt.sym, tt.recorded, got, tt.want)
		}
	}
}

func TestIsExported(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Animal", true},
		{"Speak", true},
		{"speak", false},
		{"maxSize", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isExported(tt.name); got != tt.want {
			t.Errorf("isExported(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestComputeSkeleton(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)

	t.Run("animal.go: kinds, lines, exported marker", func(t *testing.T) {
		res := computeSkeleton(ri, "animal.go")
		if res.File != "animal.go" {
			t.Fatalf("File = %q", res.File)
		}
		if len(res.Symbols) != 3 {
			t.Fatalf("Symbols = %d, want 3: %+v", len(res.Symbols), res.Symbols)
		}
		want := []SkeletonSymbol{
			{Symbol: "go github.com/example/animal Animal.", Name: "Animal", Kind: "class", Line: 2, Exported: true},
			{Symbol: "go github.com/example/animal Animal#Speak().", Name: "Speak", Kind: "method", Line: 3, Exported: true},
			{Symbol: "go github.com/example/animal Animal#speak().", Name: "speak", Kind: "method", Line: 4, Exported: false},
		}
		for i := range want {
			if res.Symbols[i] != want[i] {
				t.Errorf("Symbols[%d] = %+v, want %+v", i, res.Symbols[i], want[i])
			}
		}
	})

	t.Run("locals and package clauses dropped", func(t *testing.T) {
		res := computeSkeleton(ri, "locals.go")
		if len(res.Symbols) != 0 {
			t.Errorf("locals.go Symbols = %+v, want none (local + bare package clause dropped)", res.Symbols)
		}
	})

	t.Run("package-level named declaration kept", func(t *testing.T) {
		res := computeSkeleton(ri, "util/helper.go")
		if len(res.Symbols) != 2 {
			t.Fatalf("Symbols = %d, want 2: %+v", len(res.Symbols), res.Symbols)
		}
		// maxSize: package-level const — no '#', no '()' → grammar "type";
		// unexported by case.
		if res.Symbols[0].Name != "maxSize" || res.Symbols[0].Exported {
			t.Errorf("Symbols[0] = %+v, want unexported maxSize", res.Symbols[0])
		}
		if res.Symbols[1].Name != "helper" || res.Symbols[1].Kind != "function" {
			t.Errorf("Symbols[1] = %+v, want function helper", res.Symbols[1])
		}
	})

	t.Run("sorted by line then name", func(t *testing.T) {
		res := computeSkeleton(ri, "animal.go")
		for i := 1; i < len(res.Symbols); i++ {
			if res.Symbols[i-1].Line > res.Symbols[i].Line {
				t.Errorf("symbols not sorted by line: %d before %d", res.Symbols[i-1].Line, res.Symbols[i].Line)
			}
		}
	})

	t.Run("file with only refs yields empty list", func(t *testing.T) {
		res := computeSkeleton(ri, "services/zoo.go")
		if len(res.Symbols) != 0 {
			t.Errorf("Symbols = %+v, want none (refs are not defs)", res.Symbols)
		}
	})
}

func TestRunSkeletonHuman(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"skeleton", "animal.go"}, ri)
	if code != exitOK {
		t.Fatalf("skeleton exit = %d, want %d", code, exitOK)
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
	for _, want := range []string{
		"animal.go  (3 symbols)",
		"class   Animal  +exported",
		"method  Speak  +exported",
		"method  speak",
	} {
		if !contains(out.String(), want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out.String())
		}
	}
}

func TestRunSkeletonJSON(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"skeleton", "animal.go", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("skeleton exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	var res SkeletonResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if res.File != "animal.go" {
		t.Errorf("JSON file = %q", res.File)
	}
	if len(res.Symbols) != 3 {
		t.Fatalf("JSON symbols = %d, want 3\n%s", len(res.Symbols), out.String())
	}
	first := res.Symbols[0]
	if first.Name != "Animal" || first.Kind != "class" || first.Line != 2 || !first.Exported {
		t.Errorf("JSON symbols[0] = %+v, want Animal/class/2/exported", first)
	}
}

func TestRunSkeletonSuffixMatch(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)
	out, _ := captureWriter(t)

	code := runWith(t, []string{"skeleton", "zoo.go"}, ri)
	if code != exitOK {
		t.Fatalf("skeleton exit = %d, want %d", code, exitOK)
	}
	if !contains(out.String(), "services/zoo.go") {
		t.Errorf("suffix match did not resolve to services/zoo.go:\n%s", out.String())
	}
}

func TestRunSkeletonAmbiguous(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)
	_, errb := captureWriter(t)

	code := runWith(t, []string{"skeleton", "helper.go"}, ri)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	diag := errb.String()
	for _, want := range []string{
		`"helper.go" is ambiguous; 2 matching files:`,
		"services/helper.go",
		"util/helper.go",
	} {
		if !contains(diag, want) {
			t.Errorf("stderr missing %q\n--- got ---\n%s", want, diag)
		}
	}
}

func TestRunSkeletonUnknownFile(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)
	_, errb := captureWriter(t)

	code := runWith(t, []string{"skeleton", "nope.go"}, ri)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if !contains(errb.String(), "no indexed file matching") {
		t.Errorf("stderr missing unknown-file diagnostic:\n%s", errb.String())
	}
}

func TestRunSkeletonArgHandling(t *testing.T) {
	ri := buildSkeletonFixtureIndex(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"two args", []string{"a", "b"}},
		{"empty arg", []string{""}},
		{"unknown flag", []string{"--bogus", "animal.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errb := captureWriter(t)
			code := runWith(t, append([]string{"skeleton"}, tt.args...), ri)
			if code != exitUsage {
				t.Errorf("exit = %d, want %d", code, exitUsage)
			}
			if errb.Len() == 0 {
				t.Error("expected diagnostic on stderr")
			}
		})
	}
}

func TestRunSkeletonMissingIndex(t *testing.T) {
	// Missing-index path exits 2 via loadIndex before the verb runs;
	// pinned in the exit-code contract matrix (skeleton missing index).
	_, err := index.Load(filepath.Join(t.TempDir(), "nope", "index.scip"))
	if err == nil {
		t.Fatal("Load succeeded, want error")
	}
}
