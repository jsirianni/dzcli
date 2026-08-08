# dzcli

`dzcli` is a command-line tool for validating and editing DayZ server configuration files.

It works with server config, gameplay config, weather config, economy XML, DayZ init scripts, and Expansion AI files.

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

Preview an economy type update without writing the file:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --nominal 20 --min 5 --add-usage Military --dry-run
```

Inspect duplicate type definitions and compare only their differing fields:

```sh
dzcli get economy types --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --duplicates
dzcli get economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --compare
```

Economy validation warnings now print either one or more PowerShell-safe scoped `dzcli` remediation commands or an explicit `validation-only` manual-edit notice. Event-spawn and environment-reference commands support `--dry-run` so agents can preview localized XML edits before applying them; scaffolded territory files are path-contained and rolled back if the environment-reference write fails.

Create an Expansion AI loadout file:

```sh
dzcli create expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --class-name SurvivorM_Mirek --chance 1 --quantity-min 1 --quantity-max 1
```
