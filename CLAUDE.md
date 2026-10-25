# vq

CLI that finds notes in an Obsidian-style vault (Markdown + YAML frontmatter) by tags, frontmatter fields and paths,
without Obsidian. Single Go binary. Used from the terminal and by Claude agents working in vaults.

## Status

- `SPEC.md` is the specification for v1. Code and tests follow it.
- v1 replaced an earlier Python prototype, which has been removed. Behavior is defined only by `SPEC.md`.
- Pending work in `todo.md`.
- Two remotes: Forgejo (`aurora-01`) and GitHub. Module path `github.com/betoissues/vq` (for `go install`).

## Layout

- `main.go`: flags, vault resolution, output.
- `internal/vault/`: walking, frontmatter (`go.yaml.in/yaml/v3` via `yaml.Node`), tags.
- `internal/query/`: `--tag`, `--where`, `--path`.
- `testdata/vault/`: fixture vault with invented notes, one per case.
- Dependencies: standard library plus `go.yaml.in/yaml/v3` and `golang.org/x/text` (NFC normalization only). Flags use the standard `flag` package.

## Rules

- Never commit or push: prepare the changes and give Beto the commit message.
- Any behavior change is proposed first as a change to `SPEC.md`, and applied only after Beto approves it.
- The README is general, for any vault: no mention of Beto's vault or folders.
- All repo text (code, comments, docs, help, messages) is in English.
