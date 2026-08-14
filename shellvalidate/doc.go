// Package shellvalidate parses and statically analyzes POSIX shell and Bash
// programs without executing them.
//
// The package is byte-oriented. Diagnostic offsets and columns count bytes,
// invalid UTF-8 is preserved, and all returned source ranges are half-open.
// Script defects are returned as diagnostics; Go errors are reserved for
// invalid API use, cancellation, resolver failures, and internal failures.
package shellvalidate
