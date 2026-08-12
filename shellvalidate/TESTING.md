# Deterministic conformance testing

The `shellvalidate` suite claims complete, machine-audited coverage of its
declared model. It does not claim to enumerate every possible shell program.
The input language is unbounded, and the catalogs explicitly classify
unsupported and dynamically unknowable behavior.

## Deterministic evidence

The normal `go test ./...` acceptance suite includes:

- separate lexical, grammar, dialect, semantic, rule, and robustness catalogs;
- an audit that rejects malformed catalogs and missing test mappings;
- exact stable AST snapshots and byte-oriented diagnostic spans;
- 285 generated vectors across nine versioned models;
- complete local enumeration through compound depth 3, pipeline length 3,
  four word parts, and two redirections per command;
- deterministic pairwise interaction vectors with achieved-tuple auditing;
- trigger and nearest non-trigger decisions for all 23 diagnostics;
- deterministic truncation, token deletion, byte corruption, and resource
  boundary mutations;
- replay of every committed native-fuzz seed in both dialects; and
- 14 stable critical implementation-mutant probes, all of which must be
  killed.

Generated vector counts and SHA-256 fingerprints are recorded in
`testdata/spec/generated_models.json`. Changing a model requires an intentional
manifest update. No generator reads wall-clock time, process state, host shell,
locale, network data, or uncontrolled randomness.

## CI tiers

Pull requests run the repository `test`, `gosec`, and `revive` jobs. The
scheduled `Shell validator deep tests` workflow replays the largest
deterministic models twice, tests with `CGO_ENABLED=0`, and runs the race suite.
Native Go fuzzing remains a discovery channel. A discovered failure is only
part of deterministic coverage after it is minimized, linked to a feature,
and committed to `testdata/spec/fuzz_regressions.json`.

## Interpretation

“Exhaustive” means complete enumeration through the declared finite bounds and
complete traceability for the declared catalog. It does not imply complete
POSIX.1-2024 or Bash 5.3 conformance outside the behavior catalog. Increasing a
bound or adding a language production changes the model version, generated
count, and fingerprint.
