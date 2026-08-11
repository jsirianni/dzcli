# Economy Usage

The economy commands inspect and modify central economy XML files. Use [commands.md](commands.md) for the full flag reference.

Add `--output json` to any economy command for a stable envelope. Listings return `data.rows`; validation returns `data.files`; mutation dry-runs include the generated XML in `data.content` instead of writing raw XML to stdout.

Validation output uses compact warning groups by default once three or more similar warnings repeat. Compact JSON emits grouped warning diagnostics with counts and limited example items. Use `dzcli validate --warnings full economy ...` to print or emit every economy warning and remediation command individually.

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

List definitions whose names occur more than once across every loaded types file:

```sh
dzcli get economy types --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --duplicates
```

The duplicate table identifies the canonical definition, each later duplicate, its source file, and its 1-based occurrence within that file. Compare normalized definitions while preserving the distinction between an absent scalar and an explicit zero:

```sh
dzcli get economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --compare
```

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

Delete exactly one duplicate definition:

```sh
dzcli delete economy types M4A1 --file ./mpmissions/dayzOffline.chernarusplus/mods/types.xml --occurrence 1 --dry-run
```

When the selected file contains multiple matching entries, deletion requires `--occurrence`.

## Events and Event Spawns

Inspect fixed/player/uniform positioning and explicit activity in `db/events.xml`:

```sh
dzcli get economy events AmbientBear --file ./mpmissions/dayzOffline.chernarusplus/db/events.xml
```

Inspect explicit spawn positions and zones:

```sh
dzcli get economy event-spawns StaticHeliCrash --file ./mpmissions/dayzOffline.chernarusplus/cfgeventspawns.xml
```

The output includes a `STATUS` column. Structurally well-formed files can be inspected even when individual entries are empty or contain invalid position/zone attributes.

Create an entry with one or more positions, an optional angle/height, or a zone:

```sh
dzcli create economy event-spawns StaticHeliCrash --file ./mpmissions/dayzOffline.chernarusplus/cfgeventspawns.xml --pos 7500,7500,90 --zone 0,1,0,1,45 --dry-run
```

Update positions or the zone without rewriting unknown XML content:

```sh
dzcli update economy event-spawns StaticHeliCrash --file ./mpmissions/dayzOffline.chernarusplus/cfgeventspawns.xml --add-pos 8000,8000 --remove-pos 1 --set-zone 0,2,0,2,60 --dry-run
```

Copy every zone from another valid event, selecting a duplicate source when necessary:

```sh
dzcli update economy event-spawns StaticHeliCrash --file ./mpmissions/dayzOffline.chernarusplus/cfgeventspawns.xml --copy-zone-from StaticPoliceCar --source-occurrence 1 --dry-run
```

Use `--set-pos` to replace all positions, `--remove-zone` to remove all zones, `--rename` to rename an event, and `--occurrence` to select duplicate event entries. An update cannot leave an event with no position and no zone.

Delete one event-spawn entry:

```sh
dzcli delete economy event-spawns StaticHeliCrash --file ./mpmissions/dayzOffline.chernarusplus/cfgeventspawns.xml --occurrence 1 --dry-run
```

Deletion works directly for an empty invalid entry and does not require repairing it first. All-zero zones are rejected unless `--scaffold-placeholder` is supplied; that explicit escape hatch emits a warning because it satisfies validation without promising useful gameplay behavior.

## Environment References

Inspect registered territory paths and territory-scoped usable references, including whether their physical file is available:

```sh
dzcli get economy environment --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml
dzcli get economy environment Bear --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml
```

Register an existing territory file, or safely scaffold a missing one below the mission root:

```sh
dzcli create economy environment path env/bear_territories.xml --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --dry-run
dzcli create economy environment path env/bear_territories.xml --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --scaffold
```

Absolute paths, symlink escapes, and any raw `..` path component are rejected, even when normalization would remain inside the mission root. Scaffolding writes a valid `<territory-type />` document and removes a newly created scaffold if the `cfgenvironment.xml` write fails. Use an identity update to scaffold a file for an already registered path:

```sh
dzcli update economy environment path env/bear_territories.xml --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --set-path env/bear_territories.xml --scaffold
```

Minimal `--scaffold` files are validation-only and contain no live zones. Create an inferred commented template instead:

```sh
dzcli create economy environment path env/hare_territories.xml --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --scaffold-template
```

Or provide one or more complete, map-specific live zones:

```sh
dzcli create economy environment path env/hare_territories.xml --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --scaffold-template --zone Zone_Hare,0,0,0,2,1661.25,2288.75,50
```

Zone input is `name,smin,smax,dmin,dmax,x,z,r`; names must be non-empty, numeric fields finite, and radius positive. dzcli may infer the owning territory name/type for comments, but never invents live map coordinates. Existing files are not overwritten.

Create, rename, or delete a usable reference owned by a territory:

```sh
dzcli create economy environment usable Bear bear_territories --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --dry-run
dzcli update economy environment usable Bear bear_territories --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --set-usable bear_new --dry-run
dzcli delete economy environment usable Bear bear_new --file ./mpmissions/dayzOffline.chernarusplus/cfgenvironment.xml --dry-run
```

Use `--occurrence` for duplicate references and `--territory-occurrence` for duplicate territory names. Updating or deleting a reference never moves or deletes its physical territory XML file.

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

Full mission-root economy validation requires `cfgeconomycore.xml` at the resolved mission root. If a repository contains only a partial economy folder, run `dzcli validate xml <file-or-dir>` for an XML well-formedness check until standalone economy-file semantic validation is available.

Compact validation output groups repeated economy warning classes, including missing fixed event spawns, missing presets or territory files, duplicate type definitions, undefined type limit references, and repeated scalar relationship issues. In full warning output, each warning is followed by a concrete remediation command when dzcli can safely address it; validation-only findings explicitly require manual XML editing. Missing event-spawn warnings exclude disabled fixed events and events backed by a matching, registered, existing environment territory file.

Preview all supported findings as an ordered plan:

```sh
dzcli fix economy ./mpmissions/dayzOffline.chernarusplus
dzcli fix economy ./mpmissions/dayzOffline.chernarusplus --dry-run
```

Apply unambiguous non-destructive actions, or also permit deterministic deletions after review:

```sh
dzcli fix economy ./mpmissions/dayzOffline.chernarusplus --apply
dzcli fix economy ./mpmissions/dayzOffline.chernarusplus --apply --allow-destructive
```

Plan rows identify mechanical, semantic, placeholder, and deletion actions. Application never executes the printed shell text; it dispatches typed operations, revalidates afterward, and returns non-zero while unresolved findings remain.

For a broad XML parse check, also run:

```sh
dzcli validate xml ./mpmissions/dayzOffline.chernarusplus
```
