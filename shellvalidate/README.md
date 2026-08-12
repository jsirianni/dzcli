# shellvalidate

`shellvalidate` is a pure-Go, importable parser and static analyzer targeting
the POSIX.1-2024 Shell Command Language and Bash 5.3. It never executes source,
spawns a shell, or reads sourced files unless a caller explicitly supplies a
`SourceResolver` and enables sourced-file analysis.

The package import path is `dzcli/shellvalidate`. It is intentionally not
integrated with the `dzcli` command in this change.

## API

Use `Parse` for syntax and an immutable AST, `Analyze` for an already parsed
file, or `Check` for both. Source positions are byte-oriented and ranges are
half-open. Script defects are diagnostics rather than Go errors.

`DialectAuto` selects Bash for a recognized Bash interpreter, POSIX for a
recognized `sh` interpreter, and POSIX when no recognized interpreter exists.
An explicit dialect always wins.

## Analysis limits

Static analysis is conservative. Dynamic evaluation, dynamic source paths,
unknown command substitutions, runtime-generated names, aliases, and unknown
external behavior can make `Result.AnalysisExact` false. The package reports
that uncertainty instead of claiming a complete analysis.

The current analyzer does not implement CFG joins or iterative fixed-point
propagation across branches and loops. It emits only local definite claims for
those constructs; the audited semantic catalog classifies this capability as
unsupported instead of mapping it to an unrelated control-scope test.

The parser preserves command, pipeline, compound-command, expression,
redirection, and here-document structure. The analyzer implements the
diagnostic families listed in `COVERAGE.md`; it is not a proof that every
runtime shell behavior is known. Incomplete recovered subtrees are excluded
from semantic passes and make the analysis inexact.

The implementation accepts at most 8 MiB per source, 256 levels of syntactic
nesting, and 16 recursive literal `eval` or sourced-file analyses.

## Dependencies and safety

Production code uses only the Go standard library and supports
`CGO_ENABLED=0`. Normal package operation and tests do not invoke external
programs or access the network.

## Test model

The deterministic conformance suite is documented in [TESTING.md](TESTING.md).
It provides machine-audited traceability, explicitly bounded local-domain
enumeration, interaction coverage, input mutation, fuzz-regression replay, and
real deterministic source mutation. “Exhaustive” is limited to a named finite
local domain and does not mean enumeration of every possible shell script.
Deep CI enforces a 90.0% package statement-coverage floor and publishes runtime
evidence; `COVERAGE.md` records expectations rather than observed CI results.
