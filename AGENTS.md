# AGENTS.md

## Repository Rules

- Use `make gendocs` to regenerate command docs. It runs `go run ./cmd/gendocs`.
- Do not edit `docs/commands.md` by hand. Regenerate it with `make gendocs`.
- After user-facing command, behavior, or workflow changes, update `README.md` and the relevant `docs/*.md` files.
- After any Go code change, run `make test`, `make lint`, `make gosec`, and `make gendocs`.
- If a required command cannot be run, say so and include the reason.
