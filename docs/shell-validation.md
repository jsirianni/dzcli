# Shell Validation Package

The public `dzcli/shellvalidate` Go package targets POSIX.1-2024 shell and Bash
5.3 source without executing it. This package is a
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

CFG joins and iterative fixed-point propagation across branches and loops are
not implemented. The analyzer limits itself to local definite claims in those
cases, and the semantic catalog records that limitation explicitly.

Support is defined by the checked-in feature, rule, and deterministic-test
catalogs. The package does not claim to model every runtime effect of arbitrary
commands, aliases, dynamically generated code, or extensions outside those
catalogs. Syntax recovery excludes incomplete subtrees from semantic analysis.

Production code uses only the Go standard library. It does not spawn a shell,
load native code, access the network, traverse the filesystem, or read sourced
files implicitly.

The package test suite uses specification-traceable catalogs, stable AST
snapshots, explicitly bounded local-domain enumeration, deterministic
interaction arrays, input mutations, replayable fuzz regressions, and real
source mutations. Exhaustive claims apply only to a named finite local domain;
they do not cover the unbounded set of shell programs. Deep pull-request CI
enforces a 90.0% package statement-coverage floor and uploads commit-specific
coverage, mutation, and conformance evidence. See `shellvalidate/TESTING.md`
for the exact counts, bounds, exclusions, and CI tiers.
