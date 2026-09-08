// Command genfixture writes the committed synthetic SCIP test fixture
// (testdata/index.scip). Run it from the repo root:
//
//	go run ./internal/index/genfixture
//
// The fixture is synthetic — it is not generated from any real codebase.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func main() {
	idx := fixture()
	data, err := proto.Marshal(idx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genfixture: marshal: %v\n", err)
		os.Exit(1)
	}
	out := filepath.Join("testdata", "index.scip")
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "genfixture: write %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(data))
}

// fixture builds the same synthetic index shape as the unit-test fixture:
// an Animal/Dog/Puppy implements chain plus an unrelated helper symbol,
// spread across directories (root, util/, services/) to exercise clustering.
func fixture() *scip.Index {
	animalSpeak := "go github.com/example/animal Animal#Speak()."
	dogSpeak := "go github.com/example/animal Dog#Speak()."
	puppySpeak := "go github.com/example/animal Puppy#Speak()."
	unrelated := "go github.com/example/util Unrelated#Helper()."

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

	return &scip.Index{
		Metadata: &scip.Metadata{
			ToolInfo:    &scip.ToolInfo{Name: "scipq-fixture", Version: "0.0.0"},
			ProjectRoot: "file:///synthetic",
		},
		Documents: []*scip.Document{
			{
				RelativePath: "animal.go",
				Occurrences:  []*scip.Occurrence{def(animalSpeak, 2)},
				Symbols: []*scip.SymbolInformation{{
					Symbol: animalSpeak,
					Relationships: []*scip.Relationship{{
						Symbol:           dogSpeak,
						IsImplementation: true,
					}},
				}},
			},
			{
				RelativePath: "dog.go",
				Occurrences: []*scip.Occurrence{
					def(dogSpeak, 3),
					def(puppySpeak, 9),
				},
				Symbols: []*scip.SymbolInformation{{
					Symbol: dogSpeak,
					Relationships: []*scip.Relationship{{
						Symbol:           puppySpeak,
						IsImplementation: true,
					}},
				}},
			},
			{
				RelativePath: "util/helper.go",
				Occurrences:  []*scip.Occurrence{def(unrelated, 1)},
			},
			{
				RelativePath: "services/zoo.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 10),
					ref(animalSpeak, 14),
					ref(unrelated, 12),
				},
			},
			{
				RelativePath: "services/handler.go",
				Occurrences: []*scip.Occurrence{
					ref(animalSpeak, 4),
				},
			},
		},
	}
}
