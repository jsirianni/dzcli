# Deterministic conformance testing

The `shellvalidate` suite provides machine-audited traceability for its
declared model. It does not claim to enumerate every shell program or prove
complete conformance to the full POSIX and Bash languages. The input language
is unbounded, and the catalogs classify unsupported and dynamically
unknowable behavior.

## Deterministic evidence

The normal `go test ./...` acceptance suite includes:

- separate lexical, grammar, dialect, semantic, rule, and robustness catalogs;
- an audit that rejects malformed catalogs and missing test mappings;
- exact stable AST snapshots and byte-oriented diagnostic spans;
- versioned generated vectors across nine models, with counts and digests in
  `COVERAGE.md`;
- separate complete local domains for pipelines through length 3, words
  through four parts, and redirection ownership/placement combinations; these
  domains are not a full Cartesian product and make no compound-depth claim;
- deterministic pairwise interaction vectors with achieved-tuple auditing;
- trigger and nearest non-trigger decisions for every declared diagnostic;
- deterministic truncation, token deletion, byte corruption, and resource
  boundary mutations;
- replay of every committed native-fuzz seed in both dialects; and
- 15 stable critical source-mutation contracts whose observed status is
  determined by the mutation runner, rather than declared by a unit test.

Generated vector counts and SHA-256 fingerprints are recorded in
`testdata/spec/generated_models.json`. Changing a model requires an intentional
manifest update. No generator reads wall-clock time, process state, host shell,
locale, network data, or uncontrolled randomness.

## CI tiers

Pull requests that touch the package, deep workflow, or module files run the
`Shell validator deep tests` workflow. It replays every
`TestGeneratedModel_*` test twice, tests the repository with `CGO_ENABLED=0`,
runs the package race suite, performs real deterministic source mutation, and
enforces at least 90.0% package statement coverage. It uploads the cover
profile, mutation results and per-mutant logs, and JSON/Markdown runtime
evidence tied to the tested commit. The checked-in `COVERAGE.md` is an expected
contract; it is not evidence of a CI run or of mutant kills.

The normal repository `test`, `gosec`, and `revive` checks continue to run
according to the repository CI workflow. Native Go fuzzing remains a discovery
channel. A discovered failure is only part of deterministic coverage after it
is minimized, linked to a feature, and committed to
`testdata/spec/fuzz_regressions.json`.

## Interpretation

“Exhaustive” applies only to a specifically declared finite local domain, such
as all values in one operator inventory. Pairwise models cover audited feasible
2-way tuples and record exclusions; they are not Cartesian enumeration.
Neither term implies complete POSIX.1-2024 or Bash 5.3 conformance outside the
behavior catalog. Increasing a bound or adding a language production changes
the model version, generated count, and fingerprint.
