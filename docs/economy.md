# Economy Usage

The economy commands inspect and modify central economy XML files. Use [commands.md](commands.md) for the full flag reference.

## Inspect Types

List all type names in a `types.xml` file:

```sh
dzcli get economy types --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml
```

Find one type through `cfgeconomycore.xml`:

```sh
dzcli get economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml
```

Use `--cfgeconomycore` when a mission has multiple economy files and the tool should locate the unique file that contains the type.

## Update Type Scalars

Preview scalar changes:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --nominal 20 --min 5 --restock 1800 --dry-run
```

Apply the same change:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --nominal 20 --min 5 --restock 1800
```

Update a type by resolving its file through `cfgeconomycore.xml`:

```sh
dzcli update economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --nominal 20 --min 5 --dry-run
```

Supported scalar fields are `nominal`, `lifetime`, `restock`, `min`, `quantmin`, `quantmax`, and `cost`.

## Update Type Flags

Set flags with `name=0|1`:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --flag count_in_cargo=1 --flag count_in_hoarder=0 --dry-run
```

Remove one flag:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --remove-flag crafted --dry-run
```

Remove the entire flags field:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --remove-flags --dry-run
```

## Update Type Collections

Add collection references:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --add-category weapons --add-usage Military --dry-run
```

Remove collection references:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --remove-usage Police --remove-value Tier1 --dry-run
```

Replace a collection:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --set-usage Military --set-usage Police --dry-run
```

Clear a collection:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --clear-usages --dry-run
```

Collection fields are `category`, `tag`, `usage`, and `value`.

## Rename and Duplicate Types

Rename a type:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --rename M4A1_Black --dry-run
```

Select a duplicate by occurrence:

```sh
dzcli update economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/db/types.xml --occurrence 2 --nominal 10 --dry-run
```

Use `--occurrence` when duplicate `<type name="...">` entries exist and the command must target a specific match.

## Base Limits

List base limits:

```sh
dzcli get economy limits usage --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinition.xml
```

List one base limit:

```sh
dzcli get economy limits usage Military --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinition.xml
```

Create a base limit:

```sh
dzcli create economy limits usage SpecialForces --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinition.xml --dry-run
```

Delete a base limit:

```sh
dzcli delete economy limits usage SpecialForces --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinition.xml --dry-run
```

Base limit kinds are `category`, `tag`, `usage`, and `value`.

## User Limit Groups

List user limit groups:

```sh
dzcli get economy limits group usage --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinitionuser.xml
```

Create a group with members:

```sh
dzcli create economy limits group usage MilitaryGuns --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinitionuser.xml --member M4A1 --member AKM --dry-run
```

Add a group member:

```sh
dzcli create economy limits group member usage MilitaryGuns LAR --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinitionuser.xml --dry-run
```

Delete a group member:

```sh
dzcli delete economy limits group member usage MilitaryGuns LAR --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinitionuser.xml --dry-run
```

Delete a group:

```sh
dzcli delete economy limits group usage MilitaryGuns --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinitionuser.xml --dry-run
```

User limit group kinds are `usage` and `value`.

## Validation Workflow

After economy changes, validate the mission:

```sh
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
```

For a broad XML parse check, also run:

```sh
dzcli validate xml ./mpmissions/dayzOffline.chernarusplus
```
