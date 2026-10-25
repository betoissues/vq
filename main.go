// Command vq finds notes in an Obsidian-style vault by tags, frontmatter
// fields and paths.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/betoissues/vq/internal/query"
	"github.com/betoissues/vq/internal/vault"
)

const help = `Usage: vq [options]

Find notes in an Obsidian-style vault (Markdown files with YAML frontmatter)
by tags, frontmatter fields and paths, and print their paths. All filters
combine with AND; with no filters every note matches.

Options:
  --vault DIR     Vault root. Default: the nearest directory containing
                  .obsidian, from the current directory up; then $VQ_VAULT.
                  A leading ~ is expanded to the home directory.
  --tag T         Note has tag T or a subtag T/... (repeatable).
  --where EXPR    Frontmatter condition (repeatable):
                    KEY          key is present (not null or empty)
                    !KEY         key is not present
                    KEY=VALUE    some value equals VALUE
                    KEY!=VALUE   no value equals VALUE
                    KEY~REGEX    some value matches the RE2 regex
                    KEY>VALUE    also <, >=, <=: string comparison
                  Comparisons are case-insensitive. List values match if
                  any item matches.
  --path P        Vault-relative file or folder, matching it and everything
                  inside it (journal matches journal/x.md, not journal2.md),
                  or a glob if P contains *, ? or [ (* also matches /).
                  Repeatable; several are ORed.
  --fields a,b    Print these frontmatter fields after the path,
                  tab-separated.
  --json          Print one JSON object per note: path, frontmatter, tags.
  --abs           Print absolute paths.
  -0, --null      Terminate each path with NUL instead of newline. Cannot be
                  combined with --json or --fields. Only -0 and --json are
                  safe for file names containing a tab or a newline.
  -h, --help      Show this help.

Exit status: 0 if a note matched, 1 if none did, 2 on usage errors, when no
vault is found or on an error writing the output.

Examples:
  vq --tag project
  vq --tag project --where status!=done
  vq --where 'updated>=2025-01' --fields title,updated
  vq --path journal --where 'mood~^(good|great)$'
  vq --path '*/drafts/*.md' --json
  vq --tag recipe -0 | xargs -0 grep -il garlic
`

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(s string) error { *l = append(*l, s); return nil }

type options struct {
	vault     string
	vaultSet  bool
	fields    []string
	fieldsSet bool
	json      bool
	abs       bool
	null      bool
	query     query.Query
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, showHelp, err := parseArgs(args)
	if err != nil {
		// A message quoting the user's input must stay on one line.
		msg := strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(err.Error())
		fmt.Fprintf(stderr, "vq: %s\nTry 'vq --help'.\n", msg)
		return 2
	}
	if showHelp {
		fmt.Fprint(stdout, help)
		return 0
	}
	root, err := resolveVault(opts)
	if err != nil {
		fmt.Fprintf(stderr, "vq: %v\n", err)
		return 2
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(stderr, "vq: %v\n", err)
		return 2
	}

	display := func(rel string) string {
		if opts.abs {
			return filepath.Join(absRoot, filepath.FromSlash(rel))
		}
		return rel
	}
	warn := func(format, rel string, args ...any) {
		fmt.Fprintf(stderr, "vq: "+format+"\n", append([]any{display(rel)}, args...)...)
	}

	out := bufio.NewWriter(stdout)
	matched := false
	var writeErr error
	var line []byte
	walkErr := vault.Walk(root, opts.query.MatchPath, func(rel, path string) bool {
		note, err := vault.ReadNote(path)
		if err != nil {
			warn("cannot read: %s: %v", rel, reason(err))
			return true
		}
		if note.BadFrontmatter {
			warn("bad frontmatter: %s", rel)
		}
		if note.TagsNotList {
			warn("tags should be a list: %s", rel)
		}
		if !opts.query.MatchNote(note) {
			return true
		}
		matched = true
		line = formatRecord(line[:0], display(rel), note, opts)
		if _, err := out.Write(line); err != nil {
			writeErr = err
			return false
		}
		return true
	}, func(rel string, err error) {
		warn("cannot read: %s: %v", rel, reason(err))
	})
	if walkErr != nil {
		fmt.Fprintf(stderr, "vq: %v\n", notReadable(root, walkErr))
		return 2
	}
	if writeErr == nil {
		writeErr = out.Flush()
	}
	// Closing stdout early (vq | head) normally kills vq with SIGPIPE before
	// this point; an EPIPE seen here means SIGPIPE is ignored, and vq stops
	// quietly all the same.
	if writeErr != nil && !errors.Is(writeErr, syscall.EPIPE) {
		fmt.Fprintf(stderr, "vq: write error: %v\n", reason(writeErr))
		return 2
	}
	if !matched {
		return 1
	}
	return 0
}

// reason strips the path from filesystem errors, since warnings print it.
func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func parseArgs(args []string) (*options, bool, error) {
	fs := flag.NewFlagSet("vq", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	var opts options
	var tags, wheres, paths stringList
	var fields string
	var showHelp bool
	fs.StringVar(&opts.vault, "vault", "", "")
	fs.Var(&tags, "tag", "")
	fs.Var(&wheres, "where", "")
	fs.Var(&paths, "path", "")
	fs.StringVar(&fields, "fields", "", "")
	fs.BoolVar(&opts.json, "json", false, "")
	fs.BoolVar(&opts.abs, "abs", false, "")
	fs.BoolVar(&opts.null, "0", false, "")
	fs.BoolVar(&opts.null, "null", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showHelp, "help", false, "")

	if err := fs.Parse(args); err != nil {
		return nil, false, err
	}
	if fs.NArg() > 0 {
		return nil, false, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	if showHelp {
		return nil, true, nil
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "vault":
			opts.vaultSet = true
		case "fields":
			opts.fieldsSet = true
		}
	})
	if opts.vaultSet && opts.vault == "" {
		return nil, false, errors.New("--vault: empty path")
	}
	for _, f := range strings.Split(fields, ",") {
		if f = strings.TrimSpace(f); f != "" {
			opts.fields = append(opts.fields, f)
		}
	}
	if opts.null && opts.json {
		return nil, false, errors.New("-0 cannot be combined with --json")
	}
	if opts.null && opts.fieldsSet {
		return nil, false, errors.New("-0 cannot be combined with --fields")
	}

	for _, s := range tags {
		t, err := query.ParseTag(s)
		if err != nil {
			return nil, false, err
		}
		opts.query.Tags = append(opts.query.Tags, t)
	}
	for _, s := range wheres {
		w, err := query.ParseWhere(s)
		if err != nil {
			return nil, false, err
		}
		opts.query.Wheres = append(opts.query.Wheres, w)
	}
	for _, s := range paths {
		p, err := query.ParsePath(s)
		if err != nil {
			return nil, false, err
		}
		opts.query.Paths = append(opts.query.Paths, p)
	}
	return &opts, false, nil
}

// resolveVault returns the vault root: --vault, else the nearest directory
// with a .obsidian directory, else $VQ_VAULT.
func resolveVault(opts *options) (string, error) {
	var root string
	switch {
	case opts.vaultSet:
		root = opts.vault
	default:
		if found, ok := findObsidianRoot(); ok {
			root = found
		} else if env := os.Getenv("VQ_VAULT"); env != "" {
			root = env
		} else {
			return "", errors.New("no vault found: use --vault, run inside a vault (a directory with .obsidian) or set VQ_VAULT")
		}
	}
	root, err := expandHome(root)
	if err != nil {
		return "", err
	}
	if err := checkDir(root); err != nil {
		return "", notReadable(root, err)
	}
	return root, nil
}

// notReadable is the error for a vault root that is not a readable directory,
// found before or during the walk.
func notReadable(root string, err error) error {
	return fmt.Errorf("not a readable directory: %s: %v", root, reason(err))
}

func findObsidianRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ".obsidian")); err == nil && info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[1:]), nil
}

func checkDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	if _, err := f.ReadDir(1); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func formatRecord(dst []byte, path string, note *vault.Note, opts *options) []byte {
	switch {
	case opts.null:
		dst = append(dst, path...)
		return append(dst, 0)
	case opts.json:
		return formatJSON(dst, path, note, opts)
	}
	dst = append(dst, path...)
	for _, f := range opts.fields {
		dst = append(dst, '\t')
		v, _ := note.Frontmatter.Get(f)
		dst = append(dst, formatField(v)...)
	}
	return append(dst, '\n')
}

var fieldReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// formatField formats a field value for the default output.
func formatField(v any) string {
	var s string
	switch v := v.(type) {
	case nil:
	case string:
		s = v
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			switch item := item.(type) {
			case nil:
			case string:
				parts[i] = item
			default:
				parts[i] = string(vault.AppendJSON(nil, item))
			}
		}
		s = strings.Join(parts, ", ")
	default:
		s = string(vault.AppendJSON(nil, v))
	}
	return fieldReplacer.Replace(s)
}

func formatJSON(dst []byte, path string, note *vault.Note, opts *options) []byte {
	dst = append(dst, `{"path":`...)
	dst = vault.AppendJSONString(dst, path)
	dst = append(dst, `,"frontmatter":`...)
	if opts.fieldsSet {
		fm := vault.NewMap()
		for _, f := range opts.fields {
			if _, dup := fm.Get(f); dup {
				continue
			}
			v, _ := note.Frontmatter.Get(f)
			fm.Set(f, v)
		}
		dst = vault.AppendJSON(dst, fm)
	} else {
		dst = vault.AppendJSON(dst, note.Frontmatter)
	}
	dst = append(dst, `,"tags":[`...)
	for i, t := range note.Tags {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = vault.AppendJSONString(dst, t)
	}
	return append(dst, "]}\n"...)
}
