# Coverage Catalog

This file is checked against `testdata/features.json` and
`testdata/rules.json` by the corpus audit. Each catalog entry names its
authoritative reference and required case classes; paired `.sh` and `.json`
files carry the case-level mapping.

## Syntax features

- `common.input`
- `common.quoting`
- `common.expansion`
- `common.redirection`
- `posix.commands`
- `posix.compound`
- `posix.heredoc`
- `bash.commands`
- `bash.expansion`
- `bash.redirection`
- `bash.conditional`
- `bash.arithmetic`
- `recovery.structural`

## Rule families

- `SHS1001` through `SHS1006`
- `SHD1001`, `SHD1002`
- `SHE1001`, `SHE1002`
- `SHV1001`, `SHV1002`
- `SHC1001`, `SHC1002`
- `SHR1001`, `SHR1002`
- `SHB1001`, `SHB1002`
- `SHP1001`, `SHP1002`
- `SHX1001`, `SHX1002`
- `SHI1001`
