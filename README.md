# vq

vq finds notes in an Obsidian-style vault (Markdown files with YAML frontmatter) by tags, frontmatter fields and
paths, without Obsidian. It is a single binary that prints the matching files, so its output composes with other
command-line tools.

## Install

```
go install github.com/betoissues/vq@latest
```

## Usage

```
vq [--vault DIR] [--tag T]... [--where EXPR]... [--path P]... [--fields a,b] [--json] [--abs] [-0]
```

All filters combine with AND. With no filters every note matches. `--tag`, `--where` and `--path` can be repeated.

```sh
# Notes tagged project, or a subtag like project/alpha
vq --tag project

# Open projects
vq --tag project --where status!=done

# Notes updated since January 2025, with two fields as tab-separated columns
vq --where 'updated>=2025-01' --fields title,updated

# Notes under a folder whose mood matches a regex
vq --path journal --where 'mood~^(good|great)$'

# JSON for scripts
vq --path '*/drafts/*.md' --json

# Full-text search on top of vq
vq --tag recipe -0 | xargs -0 grep -il garlic
```

### Finding the vault

The vault root is the first of:

1. `--vault DIR`.
2. The nearest directory containing a `.obsidian` directory, from the current directory up.
3. `$VQ_VAULT`. This is a fallback for when you are not inside a vault, so it can be set globally.

A leading `~` in `--vault` or `$VQ_VAULT` is expanded to your home directory.

The whole vault is always searched and paths are printed relative to its root. Use `--path` to narrow the search
and `--abs` for absolute paths.

### Tags

A note's tags are its frontmatter `tags` (a list, or a single value) plus the inline `#tags` in its body, ignoring
code blocks, inline code, comments (`%% ... %%`, `<!-- ... -->`) and link targets (`[[#Heading]]`,
`[text](#anchor)`). Tags are case-insensitive. `--tag project` matches `project` and `project/alpha`.

### `--where`

| Expression | Matches when |
|---|---|
| `KEY` | the key is present (not null, empty string, empty list or empty map) |
| `!KEY` | the key is not present |
| `KEY=VALUE` | some value equals `VALUE` |
| `KEY!=VALUE` | no value equals `VALUE` |
| `KEY~REGEX` | some value matches the [RE2](https://github.com/google/re2/wiki/Syntax) regex |
| `KEY>VALUE`, `<`, `>=`, `<=` | some value compares that way as a string |

Comparisons are case-insensitive. For a list, each item is a value. Values are compared as written in the YAML, never
converted to numbers or dates; string comparison works for ISO dates (`updated>=2025-01`). Quote `VALUE` with `"` or
`'` to keep surrounding spaces.

### `--path`

A vault-relative file or folder: it matches that path and everything inside it (`--path journal` matches
`journal/x.md`, not `journal2.md`). A leading `./` is ignored, so shell completion from the vault root works. If the
argument contains `*`, `?` or `[` it is a glob over the whole path instead, where `*` also matches `/`. Several
`--path` options are ORed.

### Output

- Default: one path per line; with `--fields a,b`, the field values follow, tab-separated. Lists are joined with `, `.
- `--json`: one object per line, `{"path":"...","frontmatter":{...},"tags":[...]}`. With `--fields`, `frontmatter`
  holds only those fields.
- `-0` or `--null`: paths terminated by NUL, for `xargs -0`. It cannot be combined with `--json` or `--fields`.

Paths are printed as they are, so only `-0` and `--json` are safe for file names that contain a tab or a newline.

Problems with individual notes (unreadable files, invalid frontmatter) are reported on stderr and do not stop the
search.

### Exit status

| Code | Meaning |
|---|---|
| 0 | At least one note matched. |
| 1 | No note matched. |
| 2 | Usage error, no usable vault, or an error writing the output. |

## Agent skill

[`skills/vq/SKILL.md`](skills/vq/SKILL.md) teaches coding agents such as Claude Code when and how to use vq. To
install it for Claude Code, link it into your skills directory from a clone of this repo:

```
ln -s "$PWD/skills/vq" ~/.claude/skills/vq
```

If you set `CLAUDE_CONFIG_DIR`, use `$CLAUDE_CONFIG_DIR/skills` instead of `~/.claude/skills`.

## Specification

[`SPEC.md`](SPEC.md) describes the behavior in full.

## License

See [`LICENSE`](LICENSE).
