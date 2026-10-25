# Todo

## v1

- [x] Implement `SPEC.md` (`main.go`, `internal/vault`, `internal/query`), with unit tests.
- [x] Fixture vault (`testdata/vault/`) and end-to-end tests written from `SPEC.md`, independently of the code.
- [x] Code review against `SPEC.md`.
- [x] README (general, for any vault) and `--help`.
- [x] Measure run time on a ~600 note vault; consider concurrent walking only if over 50 ms. Measured
      0.01 s on 548 notes: not needed.
- [x] goreleaser: GitHub Releases with darwin/linux arm64/amd64 binaries on `vX.Y.Z` tags; Forgejo as mirror. Not validated
      locally (goreleaser not installed); the first tag run validates it.
- [x] Rollout: `go install .`, remove `~/.local/bin/vq` and `ref/vq.py`, create `~/Common/Notes/.obsidian/`, notify
      the `hub` agent.

## After v1

- `path:line` output for editors and quickfix lists.
- Dotted keys in `--where` (`author.name=x`).
- `--sort FIELD`, `--limit N`.
- zsh completions.
- `vq tags` / `vq fields` to list existing tags and frontmatter fields.
- Code span stripping is superlinear on a line with thousands of distinct backtick runs (review L5); index runs by
  length if it ever matters.
