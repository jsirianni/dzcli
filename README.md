# dzcli

`dzcli` is a command-line tool for validating and editing DayZ server configuration files.

It works with server config, gameplay config, weather config, economy XML, DayZ init scripts, and Expansion AI files.

## Help

Show the top-level command list:

```sh
dzcli --help
```

Show help for a specific command:

```sh
dzcli update economy types --help
```

The generated command reference is in `docs/commands.md`.

## Examples

Validate a mission economy folder:

```sh
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
```

Preview an economy type update without writing the file:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --nominal 20 --min 5 --add-usage Military --dry-run
```

Create an Expansion AI loadout file:

```sh
dzcli create expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --class-name SurvivorM_Mirek --chance 1 --quantity-min 1 --quantity-max 1
```
