# Shell Validation Package

The public `dzcli/shellvalidate` Go package parses and statically analyzes
POSIX.1-2024 shell and Bash 5.3 source without executing it. This package is a
library surface for a possible future consumer; no `dzcli` command imports or
invokes it in this change.

Use `Parse` to obtain syntax diagnostics and an immutable byte-oriented AST,
`Analyze` for an already parsed file, or `Check` for both operations. Explicit
POSIX mode diagnoses Bash-only constructs. Automatic mode uses a recognized
first-line interpreter and otherwise selects POSIX.

Analysis covers interpreter consistency, quoting and expansion boundaries,
variables and pipeline scope, control flow, redirection ordering, selected
builtin and command semantics, portability, destructive paths, dynamic
evaluation, and sourced files supplied through a caller-owned resolver.
Dynamic behavior is represented by `AnalysisExact=false` and an informational
diagnostic when it materially limits the result.

Production code uses only the Go standard library. It does not spawn a shell,
load native code, access the network, traverse the filesystem, or read sourced
files implicitly.
