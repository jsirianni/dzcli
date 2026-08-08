# Using dzcli With Agents

`dzcli` helps agents work on large DayZ configuration files with smaller, scoped operations. Use it to inspect, validate, and modify known resources instead of loading or rewriting entire XML or JSON files.

## Recommended Workflow

1. Start with help.

   ```sh
   dzcli --help
   dzcli update economy types --help
   ```

2. Inspect the current state with `get` commands.

   ```sh
   dzcli get economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml
   dzcli get economy limits usage Military --file ./mpmissions/dayzOffline.chernarusplus/cfglimitsdefinition.xml
   ```

3. Validate before making changes.

   ```sh
   dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
   dzcli validate xml ./mpmissions/dayzOffline.chernarusplus
   ```

4. Preview mutations with `--dry-run`.

   ```sh
   dzcli update economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --nominal 20 --min 5 --dry-run
   ```

5. Apply the same command without `--dry-run`.

   ```sh
   dzcli update economy types M4A1 --cfgeconomycore ./mpmissions/dayzOffline.chernarusplus/cfgeconomycore.xml --nominal 20 --min 5
   ```

6. Revalidate the changed files.

   ```sh
   dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
   ```

## Large File Practices

- Prefer `--cfgeconomycore` for economy type lookups when a mission uses multiple `types.xml` files.
- Use `get` commands to locate a resource before updating it.
- Use `--occurrence` when a file contains duplicate type or patrol names.
- Use `--dry-run` for every write command during planning or review.
- Keep edits scoped to one resource per command when possible.
- Validate the specific file and the containing mission folder after changes.

## Agent Prompt Pattern

Use a short task prompt that includes the target file, intended change, and validation command.

```text
Use dzcli to update M4A1 nominal to 20 and min to 5 in this mission.
Preview with --dry-run, then apply the change and run:
dzcli validate economy ./mpmissions/dayzOffline.chernarusplus
```

## Reference

Use [commands.md](commands.md) for the full generated list of commands, arguments, and flags.

