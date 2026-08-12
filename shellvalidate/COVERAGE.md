# Coverage Catalog

Generated from the machine-readable files under `testdata/`. Run `go generate ./shellvalidate` after catalog changes.

## Specification catalogs

| Catalog | Entries | Required | Extensions | Classified |
|---|---:|---:|---:|---:|
| lexical | 15 | 11 | 3 | 1 |
| grammar | 15 | 10 | 4 | 1 |
| dialects | 6 | 1 | 1 | 4 |
| semantics | 10 | 7 | 0 | 3 |
| robustness | 10 | 10 | 0 | 0 |

## Syntax features

- `bash.arithmetic`
- `bash.commands`
- `bash.conditional`
- `bash.expansion`
- `bash.redirection`
- `common.expansion`
- `common.input`
- `common.quoting`
- `common.redirection`
- `posix.commands`
- `posix.compound`
- `posix.heredoc`
- `recovery.structural`

## Diagnostic rules

- `SHB1001`
- `SHB1002`
- `SHC1001`
- `SHC1002`
- `SHD1001`
- `SHD1002`
- `SHE1001`
- `SHE1002`
- `SHI1001`
- `SHP1001`
- `SHP1002`
- `SHR1001`
- `SHR1002`
- `SHS1001`
- `SHS1002`
- `SHS1003`
- `SHS1004`
- `SHS1005`
- `SHS1006`
- `SHV1001`
- `SHV1002`
- `SHX1001`
- `SHX1002`

## Deterministic generated models

Model version: `shellvalidate-generated-v2`.

This is the expected deterministic-test contract. Runtime CI evidence, including observed coverage and mutation statuses, is published as workflow artifacts.

| Model | Vectors | Obligations | SHA-256 | Strength and bounds |
|---|---:|---:|---|---|
| `arithmetic` | 31 | 31 | `1409d6fd1e0073723d4d36b5e153fbb4de1138362d4b0a2c28c2fa3d128efe23` | exhaustive normative binary-operator inventory |
| `bounded-recursive` | 171 | 171 | `e86739f8cc04c6440e4e68d2d08e6fd81af8b4c53a50285c5224166708e8d1e4` | exhaustive within each local domain; no full cross-domain Cartesian claim; pipelineAlphabet=3, pipelineLength=3, redirectKinds=3, redirectOwners=2, redirectPositions=2, wordPartAlphabet=3, wordParts=4 |
| `conditional` | 15 | 15 | `115546cfa26374adcffe7c8e435d62768f2042ee161675f9bf86e66f5d6e83ab` | exhaustive normative binary-comparison inventory |
| `delimiter-deletion` | 10 | 10 | `4e851e3142b77abb6a5b5d641d2dc0bcd21a808cc3dbc3c68d37e595e549b04f` | exhaustive declared delimiter-deletion classes |
| `heredocs` | 6 | 6 | `4e951de0e33b2e0063bbd047cf31cb969ac73013054ffb32e1aa6337be79c02c` | full delimiter-quote by tab-strip Cartesian product |
| `operators` | 58 | 58 | `a072aa378753757c87246142fdb071e1131fbfa813f3eaab3ec32f1ca3681fd0` | full dialect by normative lexical-operator inventory |
| `pairwise` | 14 | 50 | `5fb5f4ff0469dcdfeabd8b111e9c59a4758b0eb6c919307b93bbe264886fcdbe` | t=2 across all feasible dialect, quote, part, and context value pairs; excluded: single-quote-suppresses-expansion=18 |
| `redirections` | 12 | 12 | `40c1d9c66ee9d74c310c4251c6fff4c4339c90c19dbbe27571ef9b31bb6473ea` | exhaustive declared redirection-operator inventory |
| `separators` | 10 | 10 | `4928400d261631d430e3930ffe7d623168308da97b7c84738dc87c51580331ee` | exhaustive declared separator-context inventory |

Total generated vectors: **327**.

## Expected critical mutation contract

These declarations identify the required source mutations and their anchored tests. They do not assert that a mutation was killed; only a mutation-runner result artifact is observed evidence.

- `LEX001` (critical): maximal-munch operator selection; required test anchor `TestMutationBehavior/LEX001`
- `LEX002` (critical): NUL detection in nested lexical modes; required test anchor `TestMutationBehavior/LEX002`
- `PAR001` (critical): required compound closer; required test anchor `TestMutationBehavior/PAR001`
- `PAR002` (critical): right-associative arithmetic assignment; required test anchor `TestMutationBehavior/PAR002`
- `PAR003` (critical): arithmetic multiplication precedence; required test anchor `TestMutationBehavior/PAR003`
- `PAR004` (critical): POSIX dialect gate; required test anchor `TestMutationBehavior/PAR004`
- `PAR005` (critical): parser-owned here-document recognition; required test anchor `TestMutationBehavior/PAR005`
- `ANA001` (critical): control diagnostic emission; required test anchor `TestMutationBehavior/ANA001`
- `ANA002` (critical): quoted-expansion non-trigger; required test anchor `TestMutationBehavior/ANA002`
- `ANA003` (critical): dynamic-analysis uncertainty propagation; required test anchor `TestMutationBehavior/ANA003`
- `API001` (critical): diagnostic half-open end span; required test anchor `TestMutationBehavior/API001`
- `API002` (critical): diagnostic cap enforcement; required test anchor `TestMutationBehavior/API002`
- `REC001` (critical): recovery progress after unexpected closer; required test anchor `TestMutationBehavior/REC001`
- `SRC001` (critical): source-cycle uncertainty propagation; required test anchor `TestMutationBehavior/SRC001`
