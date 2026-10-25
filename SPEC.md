# vq specification (v1)

vq finds notes in an Obsidian-style vault (Markdown files with YAML frontmatter) by tags, frontmatter fields and
paths, without Obsidian. It prints the matching files. It does not search body text: use a text search tool on its
output for that (`vq --tag x -0 | xargs -0 rg -il foo`).

## Invocation

```
vq [--vault DIR] [--tag T]... [--where EXPR]... [--path P]... [--fields a,b] [--json] [--abs] [-0]
```

Options are parsed with Go's `flag` package: `--opt value` and `--opt=value` are accepted (a single dash also works
but is not documented). Repeatable options: `--tag`, `--where`, `--path`. For the others the last occurrence wins.
Any positional argument is a usage error.

| Option | Meaning |
|---|---|
| `--vault DIR` | Vault root (see [Vault resolution](#vault-resolution)). |
| `--tag T` | Note has tag `T` or a subtag `T/...`. |
| `--where EXPR` | Frontmatter condition (see [`--where`](#--where)). |
| `--path P` | Vault-relative path prefix or glob. Several `--path` options are ORed. |
| `--fields a,b` | Print these frontmatter fields after the path, tab-separated. |
| `--json` | Print one JSON object per note. |
| `--abs` | Print absolute paths instead of vault-relative ones. |
| `-0`, `--null` | Terminate each path with NUL instead of newline. |
| `-h`, `--help` | Print help to stdout and exit 0. |

All filters combine with AND. With no filters every note matches.

## Vault resolution

The vault root is the first of:

1. `--vault DIR`.
2. The nearest directory containing a `.obsidian` directory, starting at the current directory and walking up to `/`.
3. `$VQ_VAULT`, if set and non-empty. It acts as a default for when the current directory is not inside a vault, so
   it can be set globally without disabling detection.

A leading `~` or `~/` in `--vault` and `$VQ_VAULT` is expanded to the home directory. A relative path is resolved
against the current directory. If no root is found, or the chosen root is not a readable directory, vq prints an
error and exits 2. There is no default vault.

The search always covers the whole vault, whatever the current directory, and paths are reported relative to the
vault root (or absolute with `--abs`). Use `--path` to narrow the search.

## Walking the vault

- Every file whose name ends in `.md` (case-sensitive) is a note, including names starting with a dot.
- Directories whose name starts with a dot (`.obsidian`, `.trash`, `.git`, ...) are skipped at every depth. The root
  itself is never skipped.
- Symlinks to files are read. Symlinks to directories are not followed.
- A `.md` entry that is neither a regular file nor a symlink to one (fifo, device, symlink to a directory) is
  skipped silently.
- Order: entries of each directory are sorted by name (byte order), and a subdirectory's notes appear where the
  subdirectory sorts among its siblings (`a.md`, `b/x.md`, `c.md`). This is the order of Go's `filepath.WalkDir`.
- A file that cannot be read (permissions, broken symlink) produces the warning `vq: cannot read: PATH: REASON` on
  stderr and is skipped. A
  subdirectory that cannot be listed gets the same warning and is skipped.
- Notes excluded by `--path` are not read, so they produce no warnings. Every other note is read and its warnings
  are printed, even when no filter needs its contents.

## Parsing a note

The file is read as UTF-8; invalid bytes become U+FFFD. `\r\n` and `\r` are normalized to `\n`. A leading UTF-8 BOM
is removed.

### Frontmatter

- The note has frontmatter if its first line is `---` (trailing spaces or tabs allowed). It ends at the next line
  that is `---` (same tolerance). The body is everything after the closing line.
- With no opening line, or no closing line, the note has no frontmatter and the whole file is the body.
- The text between the two lines is parsed as YAML with `go.yaml.in/yaml/v3` (as a `yaml.Node`). Empty or
  whitespace-only or comment-only text (including `---` immediately followed by `---`) is an empty frontmatter, with
  no warning. A document that is an explicit null (`~`) is invalid.
- The YAML must be a single mapping with scalar keys and no duplicate keys. Anything else (syntax error, a
  top-level list or scalar, duplicate keys, non-scalar keys, an alias cycle) is invalid. Key rules apply to nested
  mappings too, and keys are compared after quoting is resolved (`a` and `"a"` clash). Frontmatter whose aliases
  would expand to more than 10 000 nodes in total is invalid too (it guards against "billion laughs" notes). For
  invalid frontmatter vq prints `vq: bad frontmatter: PATH` on stderr, treats the frontmatter as empty and still
  reads the body.
- Values are never type-converted:
  - A scalar becomes the string YAML yields after quoting and escapes are resolved (`2024-01-01`, `007`, `true` stay
    as written; `"a\tb"` contains a tab; block scalars `|` and `>` are folded as YAML defines).
  - A null (`~`, `null`, an empty value, `!!null`) is null. A quoted `"null"` is the string `null`.
  - A sequence is a list; a mapping is a map that keeps key order. Aliases are resolved. Merge keys (`<<`) are not
    expanded: `<<` is an ordinary key.

### Tags

A note's tags are the union of its frontmatter tags and its inline tags, lowercased and deduplicated.

Frontmatter tags come from the `tags` key:

- A list: each scalar item is a tag. Null items are ignored.
- A scalar: the whole value is one tag. It is not split. If it contains a comma or whitespace, vq prints
  `vq: tags should be a list: PATH` on stderr and still uses the value as is.
- A map, or a list with non-scalar items: same warning; non-scalar items are ignored.
- Leading `#` and trailing `-` and `/` characters are removed from each tag, as for inline tags. Empty tags are
  ignored, and so are tags made only of digits (`tags: [2024, 2025-, x]` gives only `x`), with no warning.
- A null `tags` value means no frontmatter tags, with no warning.

Inline tags are read from the body after removing code, then comments and link targets. Removed text is deleted,
not replaced by a space.

- Fenced code blocks follow CommonMark, except that any indentation is allowed (so fences inside nested lists are
  recognized): an opening line with leading whitespace and at least 3 backticks or tildes; the block ends at a line
  with leading whitespace, the same character repeated at least as many times, and nothing else but whitespace. An
  unclosed block runs to the end of the body. As in CommonMark, a backtick fence whose info string contains a
  backtick does not open a block.
- Inline code follows CommonMark code spans, limited to one line: a run of N backticks opens a span that ends at the
  next run of exactly N backticks on the same line (``` ``a`b`` ``` is one span). A run with no matching closer is
  literal text. Outside a span, backslash escapes follow CommonMark: a backslash before ASCII punctuation escapes it,
  so `` \` `` is a literal backtick that cannot open a span (in `` \`#a` #b `` both `a` and `b` are tags), while in
  `` \\` `` the backslash is escaped and the backtick can open one. Inside a span, backslashes escape nothing. In
  ``a`x`#b`` the span is deleted, so `#b` follows `a` and is not a tag.
- Comments, outside code: Obsidian comments `%% ... %%` and HTML comments `<!-- ... -->`, which may span lines. An
  unclosed comment runs to the end of the body.
- Links, outside code and comments: a wikilink `[[...]]` on one line is removed whole, target and display text
  (`[[#Heading]]`, `[[Note#Part|#x]]`); a Markdown link destination `](...)` on one line is removed, keeping the link
  text (`[see #x](#anchor)` keeps `#x`).
- An inline tag is `#` followed by a letter, number or `_`, then any run of letters, numbers, combining marks, `_`,
  `-` or `/`. Letters, numbers and marks are Unicode (`#Cuál` is the tag `cuál` whether `á` is one code point or
  `a` followed by a combining accent).
- The `#` must not be preceded by a letter, number, combining mark, `_`, `/`, `&` or `#` (so URL fragments, `a#b`, `&#123;` and
  `##x` are not tags). A `#` escaped with a backslash (`\#x`, per the CommonMark escapes above) is literal and does
  not start a tag; in `\\#x` the backslash is escaped and `#x` is a tag. An escaped character still counts as the
  character before the next `#`, so `\##x` and `\&#x` are not tags.
- Trailing `-` and `/` characters are removed (`#idea-` is `idea`).
- A tag made only of digits after that is ignored (`#2024`). Digits are Unicode decimal digits (`#٢٠٢٤` is
  ignored, `#²` is kept).

## Unicode normalization

All comparisons use Unicode NFC (via `golang.org/x/text/unicode/norm`), so text in NFD (`e` followed by a combining
accent, common in files and file names from macOS) matches the same text in NFC:

- Tags from frontmatter and body, and the `T` of `--tag`, are normalized to NFC before lowercasing. Tags in
  `--json` are printed in NFC.
- `--where` normalizes frontmatter values and `VALUE` before comparing or matching.
- `--path` normalizes `P` and the vault-relative path before matching.
- Output is not normalized: paths and frontmatter values are printed as they are on disk.

## Filters

### `--tag`

`--tag T` removes leading `#` and trailing `/` from `T` and lowercases it (`--tag proj/` is `proj`). A note matches if one of its tags equals `T` or starts with
`T/`. An empty `T`, checked after trimming (so `--tag '#'` and `--tag /` too), is a usage
error.

### `--where`

Syntax: `KEY`, `!KEY`, or `KEY OP VALUE` where `OP` is one of `=`, `!=`, `~`, `>`, `<`, `>=`, `<=`. Spaces around
`KEY`, `OP` and `VALUE` are ignored. `KEY` is the text before the first `!`, `=`, `~`, `<` or `>` and must not be
empty. Keys are case-sensitive. `VALUE` may be empty. If `VALUE` is wrapped in a matching pair of `"` or `'`, that
one pair is removed (`'it's'` is `it's`).

- A key is **present** if it exists and its value is not null, not the empty string, not an empty list and not an
  empty map.
- `KEY`: the key is present. `!KEY`: it is not.
- `!` combined with an operator (`!status=done`) is a usage error that suggests `status!=done`.
- The **values** of a key are: its string for a scalar; the strings of its scalar items for a list (null items count
  as the empty string); nothing for a map or for non-scalar items. A key that is not present has no values.
- Comparisons are case-insensitive:
  - `=`: some value equals `VALUE`.
  - `!=`: no value equals `VALUE` (true when the key is not present).
  - `~`: some value matches the RE2 regular expression `VALUE`. The pattern is compiled as is with the `(?i)` flag
    and applied to the original value. RE2 has no lookarounds or backreferences, and `\w`, `\d`, `\s`, `\b` are
    ASCII-only.
  - `>`, `<`, `>=`, `<=`: some value compares that way to `VALUE` as strings (byte order of the lowercased values).
    This works for ISO dates (`--where 'updated>=2025-01'`).
- A malformed expression or an invalid regex is a usage error, reported before the vault is read.

### `--path`

Leading `./` segments and leading and trailing `/` are removed from `P` (`./journal/` is `journal`, as shell
completion writes it from the vault root). A `P` that reduces to `.` after that
(`.`, `./`, `/.`, `././`) matches the whole vault. Any other empty result is a
usage error. Matching is case-sensitive against
the vault-relative path.

- If `P` contains `*`, `?` or `[`, it is a glob over the whole path, like Python's `fnmatch`: `*` matches any
  sequence including `/`, `?` any single character including `/`, `[abc]` and `[!abc]` a character class. An unclosed
  `[` is a literal character.
- Otherwise it is a prefix: the path equals `P` or starts with `P/`.

## Output

Each matching note produces one record, in walk order. The path is vault-relative with `/` separators, or absolute
with `--abs` (the absolute vault root joined with the relative path; symlinks are not resolved). Warnings on stderr
use the same path form. Paths are printed as they are; only `-0` and `--json` are safe for file names that contain
a tab or a newline.

- **Default:** the path, then for each `--fields` field a tab and the field's value, then a newline.
  - Field names are split on commas and trimmed; empty names are dropped. A name given twice prints twice.
  - A scalar prints as is; null or a missing key prints nothing; a list prints its items joined by `, ` (a null item
    prints nothing); a map, or a non-scalar list item, prints as compact JSON.
  - Tabs, `\n` and `\r` inside a value are replaced by a space.
- **`-0`:** the path followed by NUL. It is a usage error together with `--json` or with any `--fields`, even one
  that names no fields.
- **`--json`:** one compact JSON object per line:
  `{"path":"...","frontmatter":{...},"tags":[...]}`. `frontmatter` keeps key order and uses `null` for nulls; it is
  `{}` when missing or invalid. `tags` is sorted by byte order. Strings are escaped as Go's `encoding/json` does,
  except that HTML characters (`<`, `>`, `&`) are not escaped.
  With `--fields`, `frontmatter` contains only those fields, in `--fields` order; a field given twice appears once,
  and a field the note lacks is `null`. A `--fields` that names no fields gives `{}`. Example:
  `{"path":"a.md","frontmatter":{"status":"open","updated":null},"tags":[]}`.

## Errors and exit codes

| Code | When |
|---|---|
| 0 | At least one note matched. |
| 1 | No note matched. |
| 2 | Usage error (bad option or expression, positional argument, incompatible options), no usable vault, or an error writing stdout. |

Usage errors print one `vq: MESSAGE` line and a `Try 'vq --help'.` line to stderr. `--vault ''` is a usage error.

When no root is found vq prints only `vq: no vault found: ...`. When the chosen root is not a readable directory
(also if it stops being one during the walk) it prints only `vq: not a readable directory: PATH: REASON`, with `PATH`
as given after `~` expansion (`./nope`, `~/nope` as `/home/user/nope`). Neither is followed by the Try line.

If stdout is closed early (`vq | head`), vq stops without printing an error; the process dies of SIGPIPE (shell
status 141), as `grep` and `find` do. Any other error writing stdout (a full disk, for example) prints
`vq: write error: REASON` and exits 2.

Warnings never change the exit code. The content of `--help` is free-form.

## Not in v1

Concurrent walking (only if a run takes over 50 ms on a ~600 note vault), `--sort`, `--limit`, shell completions,
`vq tags` / `vq fields`, `path:line` output, dotted keys in `--where` (`author.name=x`), full-text search.

