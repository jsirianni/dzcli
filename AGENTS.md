# AGENTS.md

## Repository Rules

- Use `make gendocs` to regenerate command docs. It runs `go run ./cmd/gendocs`.
- After any Go code change, run `make test`, `make lint`, `make gosec`, and `make gendocs`.
- If a required command cannot be run, say so and include the reason.
