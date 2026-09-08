package main

// Output plumbing shared by verbs: a writer that targets stdout for data
// and stderr for diagnostics, honoring the repo's data-only stdout contract.

import (
	"io"
)

// writer bundles the output streams a verb may use. Data goes to out
// (stdout); diagnostics go to err (stderr). Tests substitute buffers.
type writer struct {
	out io.Writer
	err io.Writer
}

// newWriter returns the process writer: stdout for data, stderr for
// diagnostics. In --json mode diagnostics still go to stderr so stdout
// stays pure data.
func newWriter(jsonOut bool) *writer {
	_ = jsonOut // both modes write data to stdout; kept for call-site clarity
	return &writer{out: stdout, err: stderr}
}
