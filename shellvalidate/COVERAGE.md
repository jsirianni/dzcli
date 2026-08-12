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

Model version: `shellvalidate-generated-v1`.

| Model | Vectors | SHA-256 | Bounds |
|---|---:|---|---|
| `arithmetic` | 26 | `797264f6e5300deb88b5a8b07e32547898fbb18568cab0176180d94588e9f54f` | - |
| `bounded-recursive` | 144 | `e88b309ee83fab1744fc0311335d583e1cdefcea4cfda5605eeb3b10dc84aad4` | compoundDepth=3, pipelineLength=3, redirections=2, wordParts=4 |
| `conditional` | 7 | `efd472f9f6d40449479c5413fb77ff9b57a0be51f8043de6cfbf2e2594f5972e` | - |
| `delimiter-deletion` | 10 | `4e851e3142b77abb6a5b5d641d2dc0bcd21a808cc3dbc3c68d37e595e549b04f` | - |
| `heredocs` | 6 | `4e951de0e33b2e0063bbd047cf31cb969ac73013054ffb32e1aa6337be79c02c` | - |
| `operators` | 58 | `68a0a3bdbb4e994a20391c81e89a4fe841145829fbcdbb1b8a01e966fff84f51` | - |
| `pairwise` | 12 | `afd46faa96ef72f16283c4a46e45f5ac1fbad0350cd4027f57df9fc1017b28c5` | - |
| `redirections` | 12 | `40c1d9c66ee9d74c310c4251c6fff4c4339c90c19dbbe27571ef9b31bb6473ea` | - |
| `separators` | 10 | `52c642a321ba44ecd5d04da664d95a142dcb930da2b030f7c6ea202afede29d8` | - |

Total generated vectors: **285**.

## Critical implementation mutants

- `LEX001` (critical): maximal-munch operator selection; killed by `TestMutationKillAudit/LEX001`
- `LEX002` (critical): NUL detection in nested lexical modes; killed by `TestMutationKillAudit/LEX002`
- `PAR001` (critical): required compound closer; killed by `TestMutationKillAudit/PAR001`
- `PAR002` (critical): right-associative arithmetic assignment; killed by `TestMutationKillAudit/PAR002`
- `PAR003` (critical): arithmetic multiplication precedence; killed by `TestMutationKillAudit/PAR003`
- `PAR004` (critical): POSIX dialect gate; killed by `TestMutationKillAudit/PAR004`
- `PAR005` (critical): here-document recognition in arithmetic mode; killed by `TestMutationKillAudit/PAR005`
- `ANA001` (critical): control diagnostic emission; killed by `TestMutationKillAudit/ANA001`
- `ANA002` (critical): quoted-expansion non-trigger; killed by `TestMutationKillAudit/ANA002`
- `ANA003` (critical): dynamic-analysis uncertainty propagation; killed by `TestMutationKillAudit/ANA003`
- `API001` (critical): diagnostic half-open end span; killed by `TestMutationKillAudit/API001`
- `API002` (critical): diagnostic cap enforcement; killed by `TestMutationKillAudit/API002`
- `REC001` (critical): recovery progress after unexpected closer; killed by `TestMutationKillAudit/REC001`
- `SRC001` (critical): source-cycle uncertainty propagation; killed by `TestMutationKillAudit/SRC001`
