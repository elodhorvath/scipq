// Tests for the dead verb: entry-point classification, dead/test-only
// computation against a dedicated synthetic index (planted dead, live,
// test-only, exported, main/init, local, and package-clause symbols),
// human and JSON rendering in both the default and --include-exported
// views, and flag/exit-code handling.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/elodhorvath/scipq/internal/index"
)

// Fixture symbol identities, file-level so the builder and the tests
// share one source of truth.
var (
	deadAnimalType    = "go github.com/example/animal Animal."
	deadAnimalSpeak   = "go github.com/example/animal Animal#Speak()."
	deadMethodSym     = "go github.com/example/animal Animal#deadMethod()."
	deadExportSym     = "go github.com/example/animal Animal#DeadExport()."
	testOnlyExportSym = "go github.com/example/animal Animal#TestOnlyExport()."
	testOnlyHelperSym = "go github.com/example/animal Animal#testOnlyHelper()."
	deadHelperFn      = "go github.com/example/util helper()."
	deadOrphanFn      = "go github.com/example/util orphan()."
	deadMainFn        = "go github.com/example/main main."
	deadInitFn        = "go github.com/example/main init."
	deadMethodInit    = "go github.com/example/main Program#init()."
	// Real scip-go package-level shapes (issue #8 pre-commit review,
	// blocker 2): backticked package path + "()." descriptor.
	deadMainRealShape = "scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/cmd/scipq`/main()."
	deadInitRealShape = "scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/internal/index.test`/init()."
	deadLocalSym      = "local 0"
	// Referenced local inside a test file: vacuously test-only, filtered
	// (issue #8 pre-commit review, blocker 1).
	deadTestLocalSym = "local 7"
	deadPkgClause    = "go github.com/example/util `github.com/example/util`/"
	// Go test-entry functions (issue #50): runtime-invoked by the testing
	// framework, never statically referenced. Defined in *_test.go
	// documents — excluded unconditionally, same rationale as main/init.
	deadTestEntryFn = "go github.com/example/animal TestAnimal()."
	deadBenchmarkFn = "go github.com/example/animal BenchmarkAnimal()."
	deadFuzzFn      = "go github.com/example/animal FuzzAnimal()."
	deadExampleFn   = "go github.com/example/animal ExampleAnimal()."
	deadTestMainFn  = "go github.com/example/animal TestMain()."
	// Real scip-go shape: backticked .test package path + "()."
	// descriptor (the .test binary package scip-go records).
	deadTestEntryRealShape = "scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/cmd/scipq.test`/TestParseUnifiedDiff()."
	// NOT entry points: same name-prefix class, wrong context — a
	// function named TestOrdinary in a non-test .go file is ordinary
	// code (distinct symbol from deadTestEntryFn so the two defs cannot
	// merge); a method named TestHelper is not an entry point
	// (package-level rule only). Both are capitalized, so they surface
	// as (exported) rows in the flag view.
	deadTestNamedNonTestFile = "go github.com/example/animal TestOrdinary()."
	deadTestMethodEntry      = "go github.com/example/animal Animal#TestHelper()."
)

// buildDeadFixtureIndex builds a dedicated synthetic index for the dead
// verb. It plants every classification dimension without touching the
// shared fixture (whose map counts are pinned elsewhere):
//
//	animal.go        live type + live method; dead unexported method;
//	                 dead exported method; exported∧test-only method;
//	                 unexported test-only method
//	util/helper.go   live unexported function (referenced from services)
//	util/orphan.go   dead unexported function (subdirectory group)
//	main.go          package-level main + init, both shapes (excluded
//	                 unconditionally); method named init (kept — the
//	                 entry-point rule is package-level only)
//	locals.go        SCIP local "local 0" (kept — strongest dead signal)
//	                 + bare package clause (excluded)
//	services/zoo.go  refs only
//	animal_test.go   refs only (drives the test-only classifications)
//	                 + a referenced local, def AND ref planted here (the
//	                 def makes the referenced-locals filter load-bearing:
//	                 without it the symbol enters computeDead's DefsAll
//	                 walk and must be filtered)
//	                 + test-entry functions (issue #50): Test*/Benchmark*/
//	                 Fuzz*/Example*/TestMain, both synthetic and real
//	                 scip-go .test-package shapes — excluded
//	                 unconditionally; plus a method named TestHelper
//	                 (NOT an entry point — classifies normally)
//	nontest_testname.go  a Test*-prefixed function in a non-test file
//	                 (NOT an entry point — the file-suffix scoping is
//	                 what makes the rule load-bearing)
func buildDeadFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
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
				Occurrences: []*scip.Occurrence{
					def(deadAnimalType, 1),
					def(deadAnimalSpeak, 2),
					def(deadMethodSym, 4),
					def(deadExportSym, 6),
					def(testOnlyExportSym, 8),
					def(testOnlyHelperSym, 10),
				},
			},
			{
				RelativePath: "util/helper.go",
				Occurrences:  []*scip.Occurrence{def(deadHelperFn, 1)},
			},
			{
				RelativePath: "util/orphan.go",
				Occurrences:  []*scip.Occurrence{def(deadOrphanFn, 0)},
			},
			{
				RelativePath: "main.go",
				Occurrences: []*scip.Occurrence{
					def(deadMainFn, 1),
					def(deadInitFn, 2),
					def(deadMethodInit, 5),
					def(deadMainRealShape, 8),
					def(deadInitRealShape, 9),
				},
			},
			{
				RelativePath: "locals.go",
				Occurrences: []*scip.Occurrence{
					def(deadLocalSym, 1),
					def(deadPkgClause, 2),
				},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(deadAnimalType, 3),
					ref(deadAnimalSpeak, 7),
					ref(deadHelperFn, 9),
				},
			},
			{
				RelativePath: "animal_test.go",
				Occurrences: []*scip.Occurrence{
					ref(deadAnimalSpeak, 2),
					ref(testOnlyExportSym, 4),
					ref(testOnlyHelperSym, 6),
					// Referenced local: def AND ref in a test file. The def
					// is what makes the referenced-locals filter load-bearing
					// — computeDead walks DefsAll(), so a ref-only symbol
					// never enters the loop and the filter assertion would
					// be vacuous (PR #48 review round 2).
					def(deadTestLocalSym, 7),
					ref(deadTestLocalSym, 8),
					// Test-entry functions: zero-ref defs in a *_test.go
					// document (issue #50). Excluded unconditionally.
					def(deadTestEntryFn, 10),
					def(deadBenchmarkFn, 12),
					def(deadFuzzFn, 14),
					def(deadExampleFn, 16),
					def(deadTestMainFn, 18),
					def(deadTestEntryRealShape, 20),
					// A method named TestHelper in a test file: NOT an
					// entry point (package-level rule only) — classifies
					// normally (zero-ref dead unexported).
					def(deadTestMethodEntry, 22),
				},
			},
			{
				RelativePath: "nontest_testname.go",
				Occurrences: []*scip.Occurrence{
					// Test* name, but the file does not end in "_test.go" —
					// ordinary code, must classify normally (zero-ref dead
					// exported). Distinct symbol from the test-file plant so
					// the two defs cannot merge; this is the plant that makes
					// the file-suffix scoping load-bearing.
					def(deadTestNamedNonTestFile, 3),
				},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal dead fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write dead fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load dead fixture: %v", err)
	}
	return ri
}

// buildLiveOnlyFixtureIndex builds an index whose only definition is
// referenced from another document — the no-dead-symbols case.
func buildLiveOnlyFixtureIndex(t *testing.T) *index.ReverseIndex {
	t.Helper()
	animalSpeak := "go github.com/example/animal Animal#Speak()."
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
				Occurrences:  []*scip.Occurrence{ref(animalSpeak, 10)},
			},
		},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal live-only fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "index.scip")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write live-only fixture: %v", err)
	}
	ri, err := index.Load(path)
	if err != nil {
		t.Fatalf("load live-only fixture: %v", err)
	}
	return ri
}

// findDead locates sym in the result's groups, returning nil when absent.
func findDead(res DeadResult, sym string) *DeadSymbol {
	for _, g := range res.Groups {
		for _, s := range g.Symbols {
			if s.Symbol == sym {
				s := s
				return &s
			}
		}
	}
	return nil
}

func TestIsEntryPoint(t *testing.T) {
	tests := []struct {
		sym  string
		want bool
	}{
		{"go github.com/example/main main.", true},
		{"go github.com/example/main init.", true},
		{"go github.com/example/main init().", true},
		// Member symbols are never entry points, however they are named:
		// the rule is package-level only (issue #8 review pin).
		{"go github.com/example/main Program#init().", false},
		{"go github.com/example/main Program#Main().", false},
		// Real scip-go package-level shapes (issue #8 pre-commit review,
		// blocker 2): backticked package path + "()." descriptor.
		{"scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/cmd/scipq`/main().", true},
		{"scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/cmd/scipq.test`/main().", true},
		{"scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/internal/index.test`/init().", true},
		// Same shape, but not an entry point.
		{"scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/cmd/scipq`/helper().", false},
		{"go github.com/example/util helper().", false},
		{"go github.com/example/animal Animal#Speak().", false},
		{"local 0", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isEntryPoint(tt.sym); got != tt.want {
			t.Errorf("isEntryPoint(%q) = %v, want %v", tt.sym, got, tt.want)
		}
	}
}

// TestIsTestEntryPoint pins the test-entry predicate (issue #50): the
// Test*/Benchmark*/Fuzz*/Example* prefix on a package-level symbol whose
// defining file is a *_test.go document. The file-suffix scoping is what
// keeps a function named TestFoo in a non-test .go file classifying
// normally, and the no-'#' guard keeps methods out of the class.
func TestIsTestEntryPoint(t *testing.T) {
	tests := []struct {
		sym  string
		file string
		want bool
	}{
		// The four classes + TestMain, in a test file.
		{"go github.com/example/animal TestAnimal().", "animal_test.go", true},
		{"go github.com/example/animal BenchmarkAnimal().", "animal_test.go", true},
		{"go github.com/example/animal FuzzAnimal().", "animal_test.go", true},
		{"go github.com/example/animal ExampleAnimal().", "animal_test.go", true},
		{"go github.com/example/animal TestMain().", "animal_test.go", true},
		// Real scip-go .test-package shape.
		{"scip-go gomod github.com/elodhorvath/scipq `github.com/elodhorvath/scipq/cmd/scipq.test`/TestParseUnifiedDiff().", "parse_test.go", true},
		// Trailing-dot descriptor form.
		{"go github.com/example/animal TestAnimal.", "animal_test.go", true},
		// Wrong context: non-test file — ordinary code.
		{"go github.com/example/animal TestAnimal().", "animal.go", false},
		{"go github.com/example/animal TestOrdinary().", "nontest_testname.go", false},
		// Wrong context: member symbols are never entry points.
		{"go github.com/example/animal Animal#TestHelper().", "animal_test.go", false},
		// Not a test-entry name.
		{"go github.com/example/animal helper().", "animal_test.go", false},
		{"go github.com/example/animal Testing().", "animal_test.go", false},
		{"local 0", "animal_test.go", false},
		{"", "animal_test.go", false},
	}
	for _, tt := range tests {
		if got := isTestEntryPoint(tt.sym, tt.file); got != tt.want {
			t.Errorf("isTestEntryPoint(%q, %q) = %v, want %v", tt.sym, tt.file, got, tt.want)
		}
	}
}

func TestDeadMarkers(t *testing.T) {
	tests := []struct {
		name string
		sym  DeadSymbol
		want string
	}{
		{"no markers", DeadSymbol{}, ""},
		{"test-only", DeadSymbol{TestOnly: true}, "(test-only)"},
		{"exported", DeadSymbol{Exported: true}, "(exported)"},
		{"both, deterministic order", DeadSymbol{TestOnly: true, Exported: true}, "(test-only) (exported)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deadMarkers(tt.sym); got != tt.want {
				t.Errorf("deadMarkers(%+v) = %q, want %q", tt.sym, got, tt.want)
			}
		})
	}
}

func TestComputeDead(t *testing.T) {
	ri := buildDeadFixtureIndex(t)

	t.Run("default view: classification", func(t *testing.T) {
		res := computeDead(ri, false)
		if res.Total != 5 {
			t.Fatalf("Total = %d, want 5\n%+v", res.Total, res.Groups)
		}
		if res.ExportedHidden != 4 {
			t.Errorf("ExportedHidden = %d, want 4 (DeadExport + TestOnlyExport + TestHelper + TestOrdinary)", res.ExportedHidden)
		}
		// Dead unexported: zero refs, listed plain.
		s := findDead(res, deadMethodSym)
		if s == nil {
			t.Fatalf("deadMethod not listed: %+v", res.Groups)
		}
		if s.TestOnly || s.Exported {
			t.Errorf("deadMethod markers = test-only:%v exported:%v, want neither", s.TestOnly, s.Exported)
		}
		if s.Name != "deadMethod" || s.Kind != "method" || s.File != "animal.go" || s.Line != 5 {
			t.Errorf("deadMethod = %+v, want deadMethod/method/animal.go:5", s)
		}
		// Test-only unexported: listed with the marker, not dropped.
		s = findDead(res, testOnlyHelperSym)
		if s == nil {
			t.Fatalf("testOnlyHelper not listed: %+v", res.Groups)
		}
		if !s.TestOnly || s.Exported {
			t.Errorf("testOnlyHelper = %+v, want test-only, not exported", s)
		}
		// Locals stay in the default list when unreferenced (strongest
		// dead signal).
		s = findDead(res, deadLocalSym)
		if s == nil {
			t.Errorf("local 0 not listed (locals must stay in): %+v", res.Groups)
		} else if s.Name != "local 0" {
			t.Errorf("local 0 Name = %q, want %q", s.Name, "local 0")
		}
		// A referenced local is filtered: vacuously test-only, no dead
		// signal (issue #8 pre-commit review, blocker 1).
		if findDead(res, deadTestLocalSym) != nil {
			t.Errorf("referenced test-file local listed (must be filtered): %+v", res.Groups)
		}
		// Method named init is kept: the entry-point rule is
		// package-level only.
		s = findDead(res, deadMethodInit)
		if s == nil {
			t.Errorf("Program#init not listed (methods must classify normally): %+v", res.Groups)
		}
		// Exported dead candidates are hidden by default.
		if findDead(res, deadExportSym) != nil {
			t.Errorf("DeadExport listed in default view (must be hidden): %+v", res.Groups)
		}
		if findDead(res, testOnlyExportSym) != nil {
			t.Errorf("TestOnlyExport listed in default view (exported filter dominates): %+v", res.Groups)
		}
		// Live symbols never appear.
		for _, live := range []string{deadAnimalType, deadAnimalSpeak, deadHelperFn} {
			if findDead(res, live) != nil {
				t.Errorf("live symbol %q listed as dead", live)
			}
		}
		// Unconditional exclusions never appear — both entry-point shapes
		// and every test-entry class (issue #50).
		for _, excluded := range []string{
			deadMainFn, deadInitFn, deadMainRealShape, deadInitRealShape, deadPkgClause,
			deadTestEntryFn, deadBenchmarkFn, deadFuzzFn, deadExampleFn, deadTestMainFn, deadTestEntryRealShape,
		} {
			if findDead(res, excluded) != nil {
				t.Errorf("excluded symbol %q listed: %+v", excluded, res.Groups)
			}
		}
		// Same name-prefix class, wrong context: both classify normally.
		// TestOrdinary (non-test file) is exported — hidden by default,
		// listed in the flag view; TestHelper (method) likewise.
		if findDead(res, deadTestNamedNonTestFile) != nil {
			t.Errorf("TestOrdinary listed in default view (exported filter must dominate): %+v", res.Groups)
		}
		if findDead(res, deadTestMethodEntry) != nil {
			t.Errorf("TestHelper listed in default view (exported filter must dominate): %+v", res.Groups)
		}
	})

	t.Run("default view: groups sorted, symbols sorted within", func(t *testing.T) {
		res := computeDead(ri, false)
		if len(res.Groups) != 2 {
			t.Fatalf("Groups = %d, want 2\n%+v", len(res.Groups), res.Groups)
		}
		if res.Groups[0].Dir != "." || res.Groups[1].Dir != "util" {
			t.Errorf("group dirs = %q, %q; want %q, %q", res.Groups[0].Dir, res.Groups[1].Dir, ".", "util")
		}
		if res.Groups[0].Count != 4 || res.Groups[1].Count != 1 {
			t.Errorf("counts = %d, %d; want 4, 1", res.Groups[0].Count, res.Groups[1].Count)
		}
		wantOrder := []string{deadMethodSym, testOnlyHelperSym, deadLocalSym, deadMethodInit}
		for i, want := range wantOrder {
			if res.Groups[0].Symbols[i].Symbol != want {
				t.Errorf("Symbols[%d] = %q, want %q", i, res.Groups[0].Symbols[i].Symbol, want)
			}
		}
	})

	t.Run("include-exported view: exported rows listed with markers", func(t *testing.T) {
		res := computeDead(ri, true)
		if res.Total != 9 {
			t.Fatalf("Total = %d, want 9\n%+v", res.Total, res.Groups)
		}
		if res.ExportedHidden != 0 {
			t.Errorf("ExportedHidden = %d, want 0 (nothing filtered)", res.ExportedHidden)
		}
		s := findDead(res, deadExportSym)
		if s == nil {
			t.Fatalf("DeadExport not listed: %+v", res.Groups)
		}
		if !s.Exported || s.TestOnly {
			t.Errorf("DeadExport = %+v, want exported only", s)
		}
		if s.Line != 7 {
			t.Errorf("DeadExport Line = %d, want 7 (1-based)", s.Line)
		}
		// Test-entry functions are excluded from the flag view too —
		// unconditional means unconditional (issue #50).
		for _, excluded := range []string{deadTestEntryFn, deadBenchmarkFn, deadFuzzFn, deadExampleFn, deadTestMainFn, deadTestEntryRealShape} {
			if findDead(res, excluded) != nil {
				t.Errorf("test-entry symbol %q listed in flag view (must be excluded): %+v", excluded, res.Groups)
			}
		}
		// The wrong-context plants DO appear in the flag view: TestOrdinary
		// (non-test file) and TestHelper (method) are exported zero-ref
		// symbols — honestly labeled, not entry points.
		s = findDead(res, deadTestNamedNonTestFile)
		if s == nil {
			t.Errorf("TestOrdinary missing from flag view (non-test file must classify normally): %+v", res.Groups)
		} else if s.File != "nontest_testname.go" || !s.Exported {
			t.Errorf("TestOrdinary = %+v, want exported row in nontest_testname.go", s)
		}
		s = findDead(res, deadTestMethodEntry)
		if s == nil {
			t.Errorf("TestHelper missing in flag view (method must classify normally): %+v", res.Groups)
		} else if !s.Exported || s.TestOnly {
			t.Errorf("TestHelper = %+v, want exported only", s)
		}
	})

	t.Run("exported and test-only: hidden by default, both markers in flag view", func(t *testing.T) {
		def := computeDead(ri, false)
		if findDead(def, testOnlyExportSym) != nil {
			t.Errorf("default view listed exported∧test-only symbol (exported filter must dominate)")
		}
		flag := computeDead(ri, true)
		s := findDead(flag, testOnlyExportSym)
		if s == nil {
			t.Fatalf("flag view missing exported∧test-only symbol: %+v", flag.Groups)
		}
		if !s.TestOnly || !s.Exported {
			t.Errorf("exported∧test-only booleans = %v/%v, want both true", s.TestOnly, s.Exported)
		}
		if s.Line != 9 {
			t.Errorf("TestOnlyExport Line = %d, want 9 (1-based)", s.Line)
		}
	})

	t.Run("count equals len(symbols)", func(t *testing.T) {
		res := computeDead(ri, true)
		for _, g := range res.Groups {
			if g.Count != len(g.Symbols) {
				t.Errorf("group %q Count = %d, len(Symbols) = %d", g.Dir, g.Count, len(g.Symbols))
			}
		}
	})

	t.Run("live-only index yields empty result", func(t *testing.T) {
		ri2 := buildLiveOnlyFixtureIndex(t)
		res := computeDead(ri2, false)
		if res.Total != 0 || res.ExportedHidden != 0 || len(res.Groups) != 0 {
			t.Errorf("live-only result = %+v, want all zero/empty", res)
		}
	})
}

func TestRunDeadHuman(t *testing.T) {
	ri := buildDeadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"dead"}, ri)
	if code != exitOK {
		t.Fatalf("dead exit = %d, want %d", code, exitOK)
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
		"5 dead-code candidates · 2 groups (4 exported hidden)",
		"deadMethod  animal.go:5",
		"testOnlyHelper  animal.go:11  (test-only)",
		"local 0  locals.go:2",
		"init  main.go:6",
		"util/",
		"orphan  util/orphan.go:1",
	} {
		if !contains(out.String(), want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out.String())
		}
	}
	// Exported candidates stay hidden in the default view.
	if contains(out.String(), "DeadExport") || contains(out.String(), "TestOnlyExport") {
		t.Errorf("default view leaked exported candidates:\n%s", out.String())
	}
}

func TestRunDeadHumanIncludeExported(t *testing.T) {
	ri := buildDeadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"dead", "--include-exported"}, ri)
	if code != exitOK {
		t.Fatalf("dead exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	for _, want := range []string{
		"9 dead-code candidates · 2 groups",
		"DeadExport  animal.go:7  (exported)",
		// Both markers, deterministic order (issue #8 review pin).
		"TestOnlyExport  animal.go:9  (test-only) (exported)",
	} {
		if !contains(out.String(), want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, out.String())
		}
	}
	// Nothing is filtered, so the hidden count disappears from the header.
	if contains(out.String(), "exported hidden") {
		t.Errorf("header still reports hidden count with flag on:\n%s", out.String())
	}
}

func TestRunDeadHumanNoDeadSymbols(t *testing.T) {
	ri := buildLiveOnlyFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"dead"}, ri)
	if code != exitOK {
		t.Fatalf("dead exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	if !contains(out.String(), "no dead symbols") {
		t.Errorf("output missing empty-result line:\n%s", out.String())
	}
}

func TestRunDeadJSON(t *testing.T) {
	ri := buildDeadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"dead", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("dead exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	var res DeadResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if res.Total != 5 {
		t.Errorf("JSON total = %d, want 5", res.Total)
	}
	if res.ExportedHidden != 4 {
		t.Errorf("JSON exportedHidden = %d, want 4", res.ExportedHidden)
	}
	if len(res.Groups) != 2 {
		t.Fatalf("JSON groups = %d, want 2\n%s", len(res.Groups), out.String())
	}
	first := res.Groups[0]
	if first.Dir != "." || first.Count != 4 || len(first.Symbols) != 4 {
		t.Errorf("JSON groups[0] = %+v, want dir \".\" count 4", first)
	}
	s := findDead(res, deadMethodSym)
	if s == nil {
		t.Fatalf("JSON missing deadMethod:\n%s", out.String())
	}
	if s.Name != "deadMethod" || s.Kind != "method" || s.File != "animal.go" || s.Line != 5 {
		t.Errorf("JSON deadMethod = %+v, want deadMethod/method/animal.go:5", s)
	}
	if s.TestOnly || s.Exported {
		t.Errorf("JSON deadMethod markers = %v/%v, want both false", s.TestOnly, s.Exported)
	}
}

func TestRunDeadJSONIncludeExported(t *testing.T) {
	ri := buildDeadFixtureIndex(t)
	out, errb := captureWriter(t)

	code := runWith(t, []string{"dead", "--include-exported", "--json"}, ri)
	if code != exitOK {
		t.Fatalf("dead exit = %d, want %d", code, exitOK)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
	var res DeadResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if res.Total != 9 || res.ExportedHidden != 0 {
		t.Errorf("JSON totals = %d/%d, want 9/0", res.Total, res.ExportedHidden)
	}
	// The exported∧test-only combination: both booleans true, unambiguous
	// in JSON regardless of marker rendering (issue #8 review pin).
	s := findDead(res, testOnlyExportSym)
	if s == nil {
		t.Fatalf("JSON missing TestOnlyExport:\n%s", out.String())
	}
	if !s.TestOnly || !s.Exported {
		t.Errorf("JSON TestOnlyExport = %+v, want both booleans true", s)
	}
}

func TestRunDeadArgHandling(t *testing.T) {
	ri := buildDeadFixtureIndex(t)

	tests := []struct {
		name string
		args []string
	}{
		{"positional arg", []string{"extra"}},
		{"two positional args", []string{"a", "b"}},
		{"unknown flag", []string{"--bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errb := captureWriter(t)
			code := runWith(t, append([]string{"dead"}, tt.args...), ri)
			if code != exitUsage {
				t.Errorf("exit = %d, want %d", code, exitUsage)
			}
			if errb.Len() == 0 {
				t.Error("expected diagnostic on stderr")
			}
		})
	}
}

func TestRunDeadMissingIndex(t *testing.T) {
	_, errb := captureWriter(t)
	cmd := newRootCommand(func(string) (*index.ReverseIndex, int) { return nil, exitNoIndex })
	err := cmd.Run(context.Background(), []string{"scipq", "dead"})
	if code := runError(err); code != exitNoIndex {
		t.Errorf("exit = %d, want %d (stderr: %q)", code, exitNoIndex, errb.String())
	}
}
