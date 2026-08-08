# Server and Gameplay Usage

These commands inspect and modify `serverDZ.cfg` and `cfggameplay.json`. Use [commands.md](commands.md) for the full flag reference.

## Inspect serverDZ.cfg

List all supported server fields from a file:

```sh
dzcli get server --file ./serverDZ.cfg
```

List one field:

```sh
dzcli get server hostname --file ./serverDZ.cfg
```

## Update serverDZ.cfg

Preview a scalar field update:

```sh
dzcli update server hostname --file ./serverDZ.cfg --value "My DayZ Server" --dry-run
```

Apply the update without an overwrite prompt:

```sh
dzcli update server hostname --file ./serverDZ.cfg --value "My DayZ Server" --force
```

Set an array-style field by repeating `--value`:

```sh
dzcli update server motd --file ./serverDZ.cfg --value "Welcome" --value "Read the rules" --dry-run
```

Allow an undocumented field:

```sh
dzcli update server customField --file ./serverDZ.cfg --value 1 --allow-unknown --dry-run
```

Delete a field:

```sh
dzcli delete server disableVoN --file ./serverDZ.cfg --dry-run
```

Validate after writing:

```sh
dzcli validate server ./serverDZ.cfg
```

## Prompts and Dry Runs

- `--dry-run` prints the modified file and does not write.
- Existing server fields prompt before overwrite unless `--force` or `--dry-run` is used.
- `--allow-unknown` is required before inserting a field that is not part of the built-in server config field list.

## Inspect cfggameplay.json

List gameplay fields:

```sh
dzcli get gameplay --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json
```

List one gameplay field:

```sh
dzcli get gameplay PlayerData.disablePersonalLight --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json
```

## Update cfggameplay.json

Preview a scalar field update:

```sh
dzcli update gameplay PlayerData.disablePersonalLight --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json --value true --dry-run
```

Apply the update without an overwrite prompt:

```sh
dzcli update gameplay PlayerData.disablePersonalLight --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json --value true --force
```

Set an array field by repeating `--value`:

```sh
dzcli update gameplay PlayerData.spawnGearPresetFiles --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json --value custom/spawngear.json --value custom/eventgear.json --dry-run
```

Clear an array field:

```sh
dzcli update gameplay PlayerData.spawnGearPresetFiles --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json --clear --dry-run
```

Delete a field:

```sh
dzcli delete gameplay PlayerData.disablePersonalLight --file ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json --dry-run
```

Validate after writing:

```sh
dzcli validate gameplay ./mpmissions/dayzOffline.chernarusplus/cfggameplay.json
```

## Field Names

Use `get` before `update` when field spelling is uncertain. Gameplay fields use dotted paths such as `PlayerData.disablePersonalLight`. Server fields use server config names such as `hostname` or `motd`.

