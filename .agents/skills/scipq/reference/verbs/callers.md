# `callers` — exact reference sites for a symbol

Who uses this symbol, where, and via what relation. Resolves by full
symbol string or by name suffix. Direct references are labeled `call`;
references to implementors are labeled `implements (via <symbol>)`,
transitively through implements chains.

## Invocation

```bash
scipq callers <SYMBOL> [--json] [--index <path>]
```

Takes exactly one positional argument (the symbol query).

## Symbol resolution

All of these resolve to the same symbol:

- Full symbol string: `go github.com/example/animal Animal#Speak().`
- Descriptor tail: `Animal#Speak().`
- Type-qualified: `Animal#Speak`
- Bare member name: `Speak` — **only if unambiguous**; a query matching
  several defined symbols is ambiguous (see exit codes).

## JSON schema

```json
{
  "symbol": "go github.com/example/animal Animal#Speak().",
  "sites": [
    {
      "file": "services/handler.go",
      "line": 5,
      "relation": "call"
    },
    {
      "file": "services/handler.go",
      "line": 6,
      "relation": "implements (via Dog#Speak())"
    }
  ]
}
```

- `line` is 1-based.
- `sites` sorted by (file, line, relation).
- `relation` is `call` for direct references, or
  `implements (via <implementing symbol>)` for references through
  implements chains (transitive).

## Exit codes

- `0` — success.
- `1` — usage error: wrong arg count, empty symbol, ambiguous query
  (all matches listed on stderr with defining files), or unknown symbol.
- `2` — missing index.

## Worked example

Run against the repo's committed `testdata/index.scip`:

```bash
$ scipq callers Animal#Speak --index testdata/index.scip
go github.com/example/animal Animal#Speak().  (3 sites)
services/handler.go:5  call
services/zoo.go:11     call
services/zoo.go:15     call
```

Reading: three direct call sites. The `implements (via <symbol>)`
relation appears when a referencing site goes through an implementor —
e.g. with a `Dog#Speak()` reference in the index (see
`buildTransitiveFixtureIndex` in `cmd/scipq/callers_test.go`), the same
query reports `services/handler.go:6  implements (via Dog#Speak())`.
Changing `Animal#Speak()`'s contract affects every listed site.
