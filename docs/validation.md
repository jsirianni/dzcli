# Validation

Use validation commands before and after changes. Validation commands print `ok` on success and return a non-zero exit code on failure.

For exact command syntax, see [commands.md](commands.md).

## Central Economy

Validate a whole mission economy folder:

```sh
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
```

Validate a `cfgeconomycore.xml` file:

```sh
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml
```

Validate one economy file:

```sh
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus/db/types.xml
```

Use this after changing `types.xml`, `cfgeconomycore.xml`, `cfglimitsdefinition.xml`, `cfglimitsdefinitionuser.xml`, or economy files referenced by `cfgeconomycore.xml`.

Economy warnings remain non-fatal. Every warning is followed by either a scoped `dzcli` remediation command or `validation-only; edit the XML manually`. Parsing and schema errors remain fatal and return a non-zero exit code.

Generated remediation commands single-quote dynamic values and paths so mission folders containing spaces or PowerShell metacharacters can be copied safely.

For fixed events, validation requires an explicit `cfgeventspawns.xml` entry unless the event is disabled with `<active>0</active>` or a matching environment territory supplies spawn data through a registered, existing usable file. Matching is case-sensitive and also recognizes one leading `Animal` or `Ambient` prefix.

## XML

Validate XML files recursively from the current directory:

```sh
dzcli validate xml
```

Validate XML files under a target path:

```sh
dzcli validate xml ./mpmissions/dayzOffline.chernarusplus
```

Use this for broad XML well-formedness checks. Use `validate economy` for DayZ economy semantics.

## Server Config

Validate `serverDZ.cfg`:

```sh
dzcli validate server ./serverDZ.cfg
```

Use this after changing server fields with `dzcli update server`.

## Gameplay Config

Validate `cfggameplay.json`:

```sh
dzcli validate gameplay ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json
```

Use this after changing gameplay fields with `dzcli update gameplay`.

## Weather Config

Validate `cfgweather.xml`:

```sh
dzcli validate weather ./mpmissions/dayzOffline.chernarusplus/cfgweather.xml
```

Use this after manual weather edits or before shipping a mission pack.

## Mission Init

Validate `init.c`:

```sh
dzcli validate init ./mpmissions/dayzOffline.chernarusplus/init.c
```

Use this to check DayZ mission init scripts before deploy.

## Expansion AI

Validate Expansion AI files from the current directory:

```sh
dzcli validate expansion ai
```

Validate a server or repository root:

```sh
dzcli validate expansion ai ./dayz-server
```

Use this after changing loadouts, patrol settings, AI settings, AI locations, or loot drops.
