# Provenance

Access date for the references below: 2026-08-11.

## Language and test references

- POSIX.1-2024, Shell Command Language, sections 2.1 through 2.15:
  <https://pubs.opengroup.org/onlinepubs/9799919799/utilities/V3_chap02.html>
- POSIX.1-2024, Shell and Utilities contents and individual utility pages:
  <https://pubs.opengroup.org/onlinepubs/9799919799/utilities/contents.html>
- POSIX.1-2024 rationale, Shell and Utilities:
  <https://pubs.opengroup.org/onlinepubs/9799919799/xrat/V4_xcu_chap01.html>
- GNU Bash Reference Manual 5.3: Shell Syntax, Shell Commands, Shell
  Parameters, Shell Expansions, Redirections, Executing Commands, Bash
  Conditional Expressions, Shell Arithmetic, Builtins, and Bash POSIX Mode:
  <https://www.gnu.org/software/bash/manual/bash.html>
- Go fuzzing documentation, Writing and Running Fuzz Tests:
  <https://go.dev/doc/security/fuzz/>
- Go `testing` package documentation:
  <https://pkg.go.dev/testing>
- U.S. Copyright Office, Computer Programs:
  <https://www.copyright.gov/register/tx-programs.html>

Command-specific models use the corresponding POSIX.1-2024 utility pages for
`.`, `break`, `cd`, `command`, `continue`, `eval`, `exec`, `exit`, `export`,
`getopts`, `read`, `readonly`, `return`, `set`, `shift`, `trap`, `umask`,
`unset`, `wait`, `find`, `grep`, `sed`, `awk`, `rm`, `cp`, `mv`, `ln`, and
`mkdir`; and the GNU Bash 5.3 builtin sections for Bash-only builtins and
extensions. GNU Findutils, Grep, Sed, Gawk, Coreutils, Tar, OpenSSH, Sudo, and
Xargs official manuals are used only for their respective command extensions.

## Independence statement

The parser, AST, analyzer, diagnostic identifiers and wording, catalogs, and
fixtures in this package were independently authored from the references
listed above. No prohibited GPLv3 source code, tests, fixture corpus, wiki,
generated rule documentation, issue discussion, or implementation-derived
material was consulted. Every checked-in fixture was authored for this
project.

## Dependencies

Production code depends only on the Go standard library (BSD-3-Clause). No
third-party parser, analyzer, native binary, WebAssembly module, or non-Go
runtime is used.
