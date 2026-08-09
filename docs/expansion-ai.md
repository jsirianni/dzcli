# Expansion AI Usage

Expansion AI commands inspect and modify loadout and patrol JSON files. Use [commands.md](commands.md) for the full flag reference.

## Validate Expansion AI Files

Validate from the current directory:

```sh
dzcli validate expansion ai
```

Validate a server or repository root:

```sh
dzcli validate expansion ai ./dayz-server
```

The validator checks AI settings, patrol settings, location settings, loadouts, and loot drops that it finds under the target path.

## Inspect Patrols

List patrols:

```sh
dzcli get expansion ai patrols --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json
```

List one patrol by name:

```sh
dzcli get expansion ai patrols NorthCheckpoint --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json
```

## Create Patrols

Preview a new patrol:

```sh
dzcli create expansion ai patrols NorthCheckpoint --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --faction West --loadout Guard --number-of-ai 3 --waypoint 7500,0,7500 --waypoint 7600,0,7600 --dry-run
```

Apply the same command without `--dry-run` when the preview is correct.

## Update Patrols

Update by name:

```sh
dzcli update expansion ai patrols NorthCheckpoint --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --number-of-ai 4 --add-unit eAI_SurvivorM_Mirek --dry-run
```

Rename a patrol:

```sh
dzcli update expansion ai patrols NorthCheckpoint --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --set-name NorthMilitaryCheckpoint --dry-run
```

Update a waypoint by 1-based index:

```sh
dzcli update expansion ai patrols NorthCheckpoint --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --set-waypoint 2=7700,0,7700 --dry-run
```

Select a patrol by index:

```sh
dzcli update expansion ai patrols --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --index 3 --chance 0.5 --dry-run
```

Use `--occurrence` when duplicate patrol names exist.

## Delete Patrols

Preview a delete:

```sh
dzcli delete expansion ai patrols NorthCheckpoint --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --dry-run
```

Delete by index:

```sh
dzcli delete expansion ai patrols --file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --index 3 --dry-run
```

## Inspect Loadouts

List loadouts under a server or repository root:

```sh
dzcli get expansion ai loadouts --path ./dayz-server
```

List one loadout:

```sh
dzcli get expansion ai loadouts Guard --path ./dayz-server
```

Inspect one loadout file directly:

```sh
dzcli get expansion ai loadouts --file ./profiles/ExpansionMod/Loadouts/Guard.json
```

## Create Loadouts

Preview a new loadout:

```sh
dzcli create expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --class-name SurvivorM_Mirek --chance 1 --quantity-min 1 --quantity-max 1 --dry-run
```

Set health and construction parts:

```sh
dzcli create expansion ai loadouts Builder --file ./profiles/ExpansionMod/Loadouts/Builder.json --class-name SurvivorM_Mirek --health =0.8,1 --add-construction-part Wall --add-construction-part Gate --dry-run
```

## Update Loadouts

Update the root loadout object:

```sh
dzcli update expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --class-name SurvivorM_Denis --chance 0.75 --dry-run
```

Replace construction parts:

```sh
dzcli update expansion ai loadouts Builder --file ./profiles/ExpansionMod/Loadouts/Builder.json --set-construction-part Wall --set-construction-part Watchtower --dry-run
```

## Loadout Items

Add an item to a loadout container:

```sh
dzcli create expansion ai loadouts item Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --parent root --container cargo --class-name Ammo_556x45 --quantity-min 20 --quantity-max 40 --dry-run
```

Add an attachment to a parent item:

```sh
dzcli create expansion ai loadouts item Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --parent root/cargo:1 --container attachment --slot weaponOptics --class-name ACOGOptic --dry-run
```

Update a nested item by path:

```sh
dzcli update expansion ai loadouts item Guard root/cargo:1 --file ./profiles/ExpansionMod/Loadouts/Guard.json --quantity-min 30 --quantity-max 60 --dry-run
```

Remove a nested item:

```sh
dzcli delete expansion ai loadouts item Guard root/cargo:1 --file ./profiles/ExpansionMod/Loadouts/Guard.json --dry-run
```

Loadout item paths start at `root`. Child segments are `cargo:<n>`, `sets:<n>`, and `attachments:<slot>:<n>`, using 1-based indexes.

## Delete Loadouts

Preview a loadout delete:

```sh
dzcli delete expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --patrols-file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --dry-run
```

Apply with a confirmation prompt:

```sh
dzcli delete expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --patrols-file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json
```

Skip the prompt:

```sh
dzcli delete expansion ai loadouts Guard --file ./profiles/ExpansionMod/Loadouts/Guard.json --patrols-file ./mpmissions/dayzOffline.chernarusplus/expansion/settings/AIPatrolSettings.json --force
```

When `--patrols-file` is set, delete planning reports patrol references before removal.

With `--output json`, loadout delete prompts are disabled so stdout remains one parseable envelope; use `--dry-run` for the delete plan or `--force` to delete.

## Validation Workflow

After loadout or patrol changes, run:

```sh
dzcli validate expansion ai ./dayz-server
```
