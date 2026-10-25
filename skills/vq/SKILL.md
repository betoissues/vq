---
name: vq
description: "Find notes in an Obsidian-style vault (Markdown with YAML frontmatter) by tags, frontmatter fields and folders with the vq CLI. Use when asked which notes have a tag, a property value, a date range or live under a folder, or to list or read notes selected that way. Not for searching body text: use rg for that, optionally on vq's output."
---

# vq

`vq` prints the notes of a vault that match filters on tags, frontmatter and paths. It never searches body text and
never modifies files. Run `vq --help` for the full option list; `SPEC.md` in the vq repo defines every rule.

## Choosing the vault

vq finds the vault by itself when the current directory is inside one (a directory with `.obsidian/`). Outside a
vault, pass `--vault DIR`; `$VQ_VAULT` is only a fallback. If vq exits 2 with `no vault found`, add `--vault`.

Paths are always relative to the vault root, whatever the current directory. Use `--abs` when another tool needs to
open the files from elsewhere.

## Filters

All filters combine with AND; repeat an option to add conditions. With no filters every note matches.

- `--tag T`: frontmatter `tags` or inline `#T`, case-insensitive. Matches subtags too (`--tag project` matches
  `project/alpha`).
- `--where EXPR`: frontmatter condition. `KEY` (present), `!KEY` (absent), `KEY=V`, `KEY!=V`, `KEY~REGEX` (RE2),
  `KEY>V`, `<`, `>=`, `<=` (string comparison, good for ISO dates). Values are case-insensitive; keys are not. A list
  matches if any item matches. Quote the whole expression for the shell: `--where 'updated>=2025-01'`.
- `--path P`: a vault-relative folder or file (matches everything inside it), or a glob if it contains `*`, `?` or
  `[` (`*` crosses `/`). Several `--path` options are ORed.

## Reading the results

- Default output is one path per line. `--fields a,b` appends tab-separated values.
- Prefer `--json --fields a,b` when you need to reason about frontmatter: one object per note with only those fields
  (missing ones are `null`), which keeps the output small. Plain `--json` prints the whole frontmatter of every
  matching note, so filter first on large vaults.
- Exit status 1 means "no note matched", not an error. Exit 2 is a usage error, no vault, or a write error.
- Warnings on stderr (`bad frontmatter`, `tags should be a list`, `cannot read`) concern single notes and do not stop
  the search. Mention them to the user if they affect the answer.

## Combining with other tools

File names often contain spaces, so pass paths with NUL separators:

```sh
# Text search inside notes selected by metadata
vq --tag recipe -0 | xargs -0 rg -il garlic

# Read the selected notes (from the vault root, or add --abs)
vq --path journal --where 'updated>=2025-06' -0 | xargs -0 cat

# Count notes per status
vq --tag project --json --fields status | jq -r '.frontmatter.status' | sort | uniq -c
```

`-0` cannot be combined with `--json` or `--fields`.

## Examples

```sh
vq --tag project --where status!=done              # open projects
vq --where 'updated>=2025-01' --fields title,updated
vq --path journal --where 'mood~^(good|great)$'
vq --where '!tags' --path inbox                    # no tags property (inline #tags may exist)
vq --path '*/drafts/*.md' --json --fields title
```
