# dzcli

`dzcli` is a command-line tool for validating and editing DayZ server configuration files.

It works with server config, gameplay config, weather config, economy XML, DayZ init scripts, and Expansion AI files.

The module also exposes the independent `dzcli/shellvalidate` Go package,
targeting cataloged POSIX.1-2024 shell and Bash 5.3 syntax and static checks.
The package is not connected to a `dzcli` command.

## Documentation

- [Usage docs](docs/README.md)
- [Generated command reference](docs/commands.md)

## Help

Show the top-level command list:

```sh
dzcli --help
```

Show help for a specific command:

```sh
dzcli update economy types --help
```

Show the installed version:

```sh
dzcli version
```

## Structured Output

Commands use the current human-readable text output by default. Add `-o json` or `--output json` when a script or agent needs one stable JSON envelope on stdout:

```sh
dzcli --output json validate economy ./mpmissions/dayzOffline.chernarusplus
dzcli -o json get server --file ./serverDZ.cfg
```

Every JSON envelope includes `status`, `target_path`, `warnings`, `failures`, `remediation`, and `data`. Validation output may also include nonfatal `notices` with source codes and spans when analysis is intentionally incomplete. Validation commands put per-file results in `data.files`; `get` commands put rows in `data.rows`; mutation dry-runs put generated content under `data.content` with `dry_run: true`.

Validation output compacts repeated similar warnings once a group reaches three items. Compact JSON warnings include a `group` object with `key`, `title`, `count`, example `items`, and `omitted_items` when more examples are hidden. Use `--warnings full` with any `validate` command to print or emit every warning and remediation item individually.

Validate a repository or servers root in one consolidated pass:

```sh
dzcli validate all ./servers
dzcli --output json validate all ./servers
```

Repository-wide validation discovers server roots, `serverDZ.cfg`, mission gameplay/weather/init files, central economy folders with `cfgeconomycore.xml`, Expansion AI config roots, XML trees, and Windows `.bat`/`.cmd` service scripts under each server root. Mission folders without `cfgeconomycore.xml` skip economy validation so partial economy folders do not fail solely for missing economy core, while XML files under those folders are still checked for well-formedness.

Validate one Windows batch file or recursively validate a directory without executing its scripts:

```sh
dzcli validate batch ./servers/example/dayz-service.bat
dzcli --output json validate batch ./servers
```

Batch validation fails only for proven documented violations or read errors. Opaque commands and runtime-dependent expansions remain valid. Default text output marks those files as `ok (analysis incomplete)` without a separate notice; pass `--verbose` to show the notice, or use JSON for every informational diagnostic and source span.

Use command help when building scripts or checking flags:

```sh
dzcli validate economy --help
```

## Using dzcli With AI Agents

`dzcli` is useful when an AI agent needs to inspect, validate, or modify large DayZ XML files without reading or rewriting the whole file by hand.

Recommended workflow:

1. Inspect the command surface with `dzcli --help` and command-specific `--help`.
2. Use `get` and `validate` commands to inspect the target file before changing it.
3. Use scoped update commands instead of manual XML edits.
4. Run mutation commands with `--dry-run` first and review the output.
5. Run the matching `validate` command after writing changes.

## Examples

Validate a mission economy folder:

```sh
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
```

Validate one XML file, or validate XML files recursively under a directory:

```sh
dzcli validate xml ./mpmissions/dayzOffline.chernarusplus/db/types.xml
dzcli validate xml ./mpmissions/dayzOffline.chernarusplus
```

Economy validation checks the full mission-root economy and requires `cfgeconomycore.xml`; use `dzcli validate xml <file-or-dir>` for XML well-formedness checks on partial economy folders.

Preview an economy type update without writing the file:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --nominal 20 --min 5 --add-usage Military --dry-run
```

Inspect duplicate type definitions and compare only their differing fields:

```sh
dzcli get economy types --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --duplicates
dzcli get economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --compare
```

Economy validation warnings print either one or more PowerShell-safe scoped `dzcli` remediation commands or an explicit `validation-only` manual-edit notice. Repeated economy warnings are grouped in compact text and JSON validation output; rerun with `--warnings full` when you need every per-item command. Event-spawn and environment-reference commands support `--dry-run` so agents can preview localized XML edits before applying them; scaffolded territory files are path-contained and rolled back if the environment-reference write fails.

Preview an ordered, classified remediation plan:

```sh
dzcli fix economy ./mpmissions/dayzOffline.chernarusplus
```

`fix economy` applies only unambiguous mechanical changes with `--apply`. Deterministic deletions additionally require `--allow-destructive`; semantic decisions and validation-only placeholders remain review items. Changed economy XML is written without trailing whitespace and with one final newline.

Create an Expansion AI loadout file:

```sh
dzcli create expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --class-name SurvivorM_Mirek --chance 1 --quantity-min 1 --quantity-max 1
```
