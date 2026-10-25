package main_test

// End-to-end tests for vq, written from SPEC.md alone. They build the binary,
// run it with arguments, environment and working directory, and compare
// stdout, stderr and the exit code. Expected outputs are written by hand.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	vqBin    string
	buildErr error
	repoRoot string
	homeDir  string
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "vq-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	repoRoot, err = os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	homeDir = filepath.Join(tmp, "home")
	if err := os.Mkdir(homeDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	vqBin = filepath.Join(tmp, "vq")
	out, err := exec.Command("go", "build", "-o", vqBin, ".").CombinedOutput()
	if err != nil {
		buildErr = fmt.Errorf("go build: %v\n%s", err, out)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

type tc struct {
	name     string
	args     []string
	env      []string // extra KEY=VALUE entries; HOME defaults to an empty temp dir and VQ_VAULT is unset
	dir      string   // working directory, absolute or relative to the repo root; default the repo root
	stdout   string
	stdoutRe string // if set, stdout must match this instead of stdout
	stderr   string
	stderrRe string // if set, stderr must match this instead of stderr
	code     int
}

func run(t *testing.T, c tc) {
	t.Helper()
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, vqBin, c.args...)
	switch {
	case c.dir == "":
		cmd.Dir = repoRoot
	case filepath.IsAbs(c.dir):
		cmd.Dir = c.dir
	default:
		cmd.Dir = filepath.Join(repoRoot, c.dir)
	}
	cmd.Env = append([]string{"HOME=" + homeDir}, c.env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run %v: %v", c.args, err)
		}
		code = ee.ExitCode()
	}
	if code != c.code {
		t.Errorf("args %q: exit code %d, want %d", c.args, code, c.code)
	}
	if c.stdoutRe != "" {
		if !regexp.MustCompile(c.stdoutRe).MatchString(stdout.String()) {
			t.Errorf("args %q: stdout\n%q\ndoes not match\n%q", c.args, stdout.String(), c.stdoutRe)
		}
	} else if stdout.String() != c.stdout {
		t.Errorf("args %q: stdout\n%q\nwant\n%q", c.args, stdout.String(), c.stdout)
	}
	if c.stderrRe != "" {
		if !regexp.MustCompile(c.stderrRe).MatchString(stderr.String()) {
			t.Errorf("args %q: stderr\n%q\ndoes not match\n%q", c.args, stderr.String(), c.stderrRe)
		}
	} else if stderr.String() != c.stderr {
		t.Errorf("args %q: stderr\n%q\nwant\n%q", c.args, stderr.String(), c.stderr)
	}
}

func runAll(t *testing.T, cases []tc) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { run(t, c) })
	}
}

// fx prepends --vault for the fixture vault.
func fx(args ...string) []string {
	return append([]string{"--vault", "testdata/vault"}, args...)
}

// wx prepends --vault for the warnings fixture vault.
func wx(args ...string) []string {
	return append([]string{"--vault", "testdata/warn"}, args...)
}

func lines(ss ...string) string {
	if len(ss) == 0 {
		return ""
	}
	return strings.Join(ss, "\n") + "\n"
}

// usageRe matches a usage error: one "vq: " message line containing sub,
// followed by the "Try 'vq --help'." line.
func usageRe(sub string) string {
	return `^vq: [^\n]*` + regexp.QuoteMeta(sub) + `[^\n]*\nTry 'vq --help'\.\n$`
}

// noVaultRe matches the error printed when no vault root is found: one line,
// no Try line.
const noVaultRe = `^vq: no vault found: [^\n]*\n$`

// badVaultRe matches the single line printed when the chosen root at path is
// not a readable directory: no Try line.
func badVaultRe(path string) string {
	return `^vq: not a readable directory: ` + regexp.QuoteMeta(path) + `: [^\n]+\n$`
}

// mkFiles writes files under root, creating parent directories.
func mkFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mkDirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
}

// allNotes is every note of testdata/vault in walk order.
var allNotes = lines(
	".hidden.md",
	"fields/escape.md",
	"fields/f1.md",
	"fields/html.md",
	"fm/alias.md",
	"fm/blank.md",
	"fm/block.md",
	"fm/comment-only.md",
	"fm/comment.md",
	"fm/empty.md",
	"fm/indented-close.md",
	"fm/nested.md",
	"fm/none.md",
	"fm/notfirst.md",
	"fm/nulls.md",
	"fm/order.md",
	"fm/scalars.md",
	"fm/trailing.md",
	"fm/unclosed.md",
	"folder.md/inner.md",
	"path/Case.md",
	"path/a/b.md",
	"path/a.md",
	"path/ab.md",
	"path/q[1.md",
	"sub/deep/leaf.md",
	"sub/note.md",
	"tags/comment-unclosed-html.md",
	"tags/comment-unclosed-obs.md",
	"tags/comments.md",
	"tags/fm-dedup.md",
	"tags/fm-list.md",
	"tags/fm-nested.md",
	"tags/fm-null.md",
	"tags/fm-numeric.md",
	"tags/fm-scalar.md",
	"tags/fm-trim.md",
	"tags/heading.md",
	"tags/inline-basic.md",
	"tags/inline-code-deleted.md",
	"tags/inline-code.md",
	"tags/inline-digits-unicode.md",
	"tags/inline-digits.md",
	"tags/inline-escape-hash.md",
	"tags/inline-escape-prev.md",
	"tags/inline-escape.md",
	"tags/inline-fence-info.md",
	"tags/inline-fence.md",
	"tags/inline-preceded.md",
	"tags/inline-trailing.md",
	"tags/inline-unclosed-fence.md",
	"tags/inline-unicode.md",
	"tags/links.md",
	"tags/nfd.md",
	"where/w1.md",
	"where/w2.md",
	"where/w3.md",
	"where/w4.md",
)

func TestWalk(t *testing.T) {
	runAll(t, []tc{
		{name: "no filters lists every note in walk order", args: fx(), stdout: allNotes},
		{name: "dot dir at root skipped", args: fx("--tag", "skipped"), code: 1},
		{name: "trash skipped", args: fx("--tag", "deleted"), code: 1},
		{name: "dot dir at depth skipped", args: fx("--tag", "cached"), code: 1},
		{name: "dot file is a note", args: fx("--tag", "dotfile"), stdout: lines(".hidden.md")},
		{name: "txt is not a note", args: fx("--tag", "readme"), code: 1},
		{name: "upper MD is not a note", args: fx("--tag", "upper"), code: 1},
		{name: "txt in subdir is not a note", args: fx("--tag", "ctxt"), code: 1},
		{name: "directory named .md is walked", args: fx("--tag", "inner"), stdout: lines("folder.md/inner.md")},
	})
}

func TestWalkTemp(t *testing.T) {
	root := t.TempDir()
	v := filepath.Join(root, "v")
	mkFiles(t, v, map[string]string{
		"C.md":           "",
		"_.md":           "",
		"a/x.md":         "",
		"a-b.md":         "",
		"a.md":           "",
		"b.md":           "",
		".git/x.md":      "",
		"d/.dot.md":      "",
		"d/.hidden/y.md": "",
		"e.MD":           "",
		"f.markdown":     "",
		"g.md/h.md":      "",
	})
	dv := filepath.Join(root, ".dotvault")
	mkFiles(t, dv, map[string]string{"d.md": ""})
	mkDirs(t, dv, ".obsidian", "sub")
	runAll(t, []tc{
		{name: "byte order and skipping", args: []string{"--vault", v}, stdout: lines(
			"C.md", "_.md", "a/x.md", "a-b.md", "a.md", "b.md", "d/.dot.md", "g.md/h.md")},
		{name: "dot-named root via --vault", args: []string{"--vault", dv}, stdout: lines("d.md")},
		{name: "dot-named root via detection", dir: filepath.Join(dv, "sub"), stdout: lines("d.md")},
	})
}

func TestSymlinks(t *testing.T) {
	root := t.TempDir()
	mkFiles(t, root, map[string]string{
		"outside/real.md":       "---\ntags: [real]\n---\n",
		"outside/dir/inside.md": "#inside\n",
	})
	v := filepath.Join(root, "v")
	mkFiles(t, v, map[string]string{"a.md": "", "z.md": ""})
	mkDirs(t, v, ".obsidian")
	for link, target := range map[string]string{
		"link.md":   "../outside/real.md",
		"dirlink":   "../outside/dir",
		"broken.md": "nowhere.md",
		"dirmd.md":  "../outside/dir",
	} {
		if err := os.Symlink(target, filepath.Join(v, link)); err != nil {
			t.Fatal(err)
		}
	}
	runAll(t, []tc{
		{name: "file symlink read, dir symlink not followed, broken symlink warned",
			args: []string{"--vault", v, "--json"},
			stdout: lines(
				`{"path":"a.md","frontmatter":{},"tags":[]}`,
				`{"path":"link.md","frontmatter":{"tags":["real"]},"tags":["real"]}`,
				`{"path":"z.md","frontmatter":{},"tags":[]}`),
			stderrRe: `^vq: cannot read: broken\.md: [^\n]+\n$`},
		{name: "broken symlink excluded by --path is not read", args: []string{"--vault", v, "--path", "a.md"},
			stdout: lines("a.md")},
		{name: "file symlink content is matched", args: []string{"--vault", v, "--tag", "real"},
			stdout: lines("link.md"), stderrRe: `^vq: cannot read: broken\.md: [^\n]+\n$`},
		{name: "dir symlink not followed", args: []string{"--vault", v, "--tag", "inside"},
			code: 1, stderrRe: `^vq: cannot read: broken\.md: [^\n]+\n$`},
		{name: "abs does not resolve symlinks", args: []string{"--vault", v, "--abs", "--tag", "real"},
			stdout:   lines(v + "/link.md"),
			stderrRe: `^vq: cannot read: ` + regexp.QuoteMeta(v+"/broken.md") + `: [^\n]+\n$`},
	})
}

func TestUnreadable(t *testing.T) {
	skipIfRoot(t)
	v := t.TempDir()
	mkFiles(t, v, map[string]string{"ok.md": "#ok\n", "secret.md": "#secret\n"})
	if err := os.Chmod(filepath.Join(v, "secret.md"), 0); err != nil {
		t.Fatal(err)
	}
	runAll(t, []tc{
		{name: "warned and skipped", args: []string{"--vault", v, "--json"},
			stdout:   lines(`{"path":"ok.md","frontmatter":{},"tags":["ok"]}`),
			stderrRe: `^vq: cannot read: secret\.md: [^\n]+\n$`},
		{name: "warning does not change exit code", args: []string{"--vault", v, "--tag", "secret"},
			code: 1, stderrRe: `^vq: cannot read: secret\.md: [^\n]+\n$`},
		{name: "warning uses abs path", args: []string{"--vault", v, "--abs", "--tag", "ok"},
			stdout:   lines(v + "/ok.md"),
			stderrRe: `^vq: cannot read: ` + regexp.QuoteMeta(v+"/secret.md") + `: [^\n]+\n$`},
		{name: "where errors are reported before the vault is read",
			args: []string{"--vault", v, "--where", "a~("}, code: 2, stderrRe: usageRe("")},
		{name: "plain listing reads every note", args: []string{"--vault", v},
			stdout: lines("ok.md"), stderrRe: `^vq: cannot read: secret\.md: [^\n]+\n$`},
		{name: "note excluded by --path is not read", args: []string{"--vault", v, "--path", "ok.md"},
			stdout: lines("ok.md")},
	})
}

func TestUnlistableDir(t *testing.T) {
	skipIfRoot(t)
	v := t.TempDir()
	mkFiles(t, v, map[string]string{"a.md": "", "locked/l.md": "", "z.md": ""})
	if err := os.Chmod(filepath.Join(v, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(v, "locked"), 0o755)
	runAll(t, []tc{
		{name: "warned and skipped", args: []string{"--vault", v},
			stdout: lines("a.md", "z.md"), stderrRe: `^vq: cannot read: locked: [^\n]+\n$`},
		{name: "abs path in warning", args: []string{"--vault", v, "--abs", "--path", "a.md"},
			stdout: lines(v + "/a.md"), stderrRe: `^vq: cannot read: ` + regexp.QuoteMeta(v+"/locked") + `: [^\n]+\n$`},
	})
}

func TestFifo(t *testing.T) {
	v := t.TempDir()
	mkFiles(t, v, map[string]string{"a.md": ""})
	if out, err := exec.Command("mkfifo", filepath.Join(v, "pipe.md")).CombinedOutput(); err != nil {
		t.Skipf("mkfifo: %v %s", err, out)
	}
	run(t, tc{name: "fifo skipped silently", args: []string{"--vault", v, "--json"},
		stdout: lines(`{"path":"a.md","frontmatter":{},"tags":[]}`)})
}

func TestVaultResolution(t *testing.T) {
	root := t.TempDir()
	mkFiles(t, root, map[string]string{
		"home/v/home-note.md":  "",
		"outer/outer.md":       "",
		"outer/inner/inner.md": "",
		"plain/p.md":           "",
		"fakeobs/.obsidian":    "not a directory\n",
		"fakeobs/f.md":         "",
		"envv/e.md":            "",
		"file.md":              "",
		"locked/l.md":          "",
	})
	mkDirs(t, root, "home/v/.obsidian", "outer/.obsidian", "outer/inner/.obsidian",
		"outer/inner/sub/deeper", "envv/.obsidian", "nowhere")
	home := filepath.Join(root, "home")
	p := func(s string) string { return filepath.Join(root, s) }
	cases := []tc{
		{name: "--vault absolute, no .obsidian needed", args: []string{"--vault", p("plain")}, stdout: lines("p.md")},
		{name: "--vault relative to cwd", dir: root, args: []string{"--vault", "plain"}, stdout: lines("p.md")},
		{name: "--vault= form", dir: root, args: []string{"--vault=plain"}, stdout: lines("p.md")},
		{name: "--vault ~/ expanded", env: []string{"HOME=" + home}, args: []string{"--vault", "~/v"}, stdout: lines("home-note.md")},
		{name: "--vault ~ expanded", env: []string{"HOME=" + p("home/v")}, args: []string{"--vault", "~"}, stdout: lines("home-note.md")},
		{name: "--vault ~/ with --abs", env: []string{"HOME=" + home}, args: []string{"--vault", "~/v", "--abs"},
			stdout: lines(p("home/v/home-note.md"))},
		{name: "detection from nested subdir finds nearest", dir: p("outer/inner/sub/deeper"), stdout: lines("inner.md")},
		{name: "detection at root covers nested vault", dir: p("outer"), stdout: lines("inner/inner.md", "outer.md")},
		{name: "detection beats VQ_VAULT", dir: p("outer/inner"), env: []string{"VQ_VAULT=" + p("envv")}, stdout: lines("inner.md")},
		{name: "--vault beats detection", dir: p("outer"), args: []string{"--vault", p("plain")}, stdout: lines("p.md")},
		{name: "VQ_VAULT used outside a vault", dir: p("nowhere"), env: []string{"VQ_VAULT=" + p("envv")}, stdout: lines("e.md")},
		{name: "VQ_VAULT relative to cwd", dir: root, env: []string{"VQ_VAULT=envv"}, stdout: lines("e.md")},
		{name: "VQ_VAULT ~ expanded", dir: p("nowhere"), env: []string{"HOME=" + home, "VQ_VAULT=~/v"}, stdout: lines("home-note.md")},
		{name: "--vault beats VQ_VAULT", dir: p("nowhere"), env: []string{"VQ_VAULT=" + p("envv")}, args: []string{"--vault", p("plain")}, stdout: lines("p.md")},
		{name: "no vault", dir: p("nowhere"), code: 2, stderrRe: noVaultRe},
		{name: "empty VQ_VAULT", dir: p("nowhere"), env: []string{"VQ_VAULT="}, code: 2, stderrRe: noVaultRe},
		{name: ".obsidian file does not mark a vault", dir: p("fakeobs"), code: 2, stderrRe: noVaultRe},
		{name: "--vault missing dir", args: []string{"--vault", p("missing")}, code: 2, stderrRe: badVaultRe(p("missing"))},
		{name: "--vault is a file", args: []string{"--vault", p("file.md")}, code: 2, stderrRe: badVaultRe(p("file.md"))},
		{name: "VQ_VAULT missing dir", dir: p("nowhere"), env: []string{"VQ_VAULT=" + p("missing")}, code: 2, stderrRe: badVaultRe(p("missing"))},
		{name: "bad root reported as given (./)", dir: root, args: []string{"--vault", "./missing"},
			code: 2, stderrRe: badVaultRe("./missing")},
		{name: "bad root reported as given (trailing /)", dir: root, args: []string{"--vault", "missing/"},
			code: 2, stderrRe: badVaultRe("missing/")},
		{name: "relative file root reported as given", dir: root, args: []string{"--vault", "file.md"},
			code: 2, stderrRe: badVaultRe("file.md")},
		{name: "relative VQ_VAULT reported as given", dir: p("nowhere"), env: []string{"VQ_VAULT=../missing"},
			code: 2, stderrRe: badVaultRe("../missing")},
		{name: "bad --vault ~/ reported after expansion", env: []string{"HOME=" + home}, args: []string{"--vault", "~/nope"},
			code: 2, stderrRe: badVaultRe(home + "/nope")},
		{name: "bad VQ_VAULT ~/ reported after expansion", dir: p("nowhere"), env: []string{"HOME=" + home, "VQ_VAULT=~/nope"},
			code: 2, stderrRe: badVaultRe(home + "/nope")},
		{name: "--vault '' is a usage error", dir: p("outer"), args: []string{"--vault", ""}, code: 2, stderrRe: usageRe("")},
		{name: "--vault= is a usage error", dir: p("outer"), env: []string{"VQ_VAULT=" + p("envv")}, args: []string{"--vault="},
			code: 2, stderrRe: usageRe("")},
	}
	runAll(t, cases)

	t.Run("unreadable root", func(t *testing.T) {
		skipIfRoot(t)
		if err := os.Chmod(p("locked"), 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(p("locked"), 0o755)
		run(t, tc{args: []string{"--vault", p("locked")}, code: 2, stderrRe: badVaultRe(p("locked"))})
	})
}

func TestVaultFixture(t *testing.T) {
	abs := filepath.Join(repoRoot, "testdata", "vault")
	runAll(t, []tc{
		{name: "detection from subdir covers whole vault", dir: "testdata/vault/sub/deep", stdout: allNotes},
		{name: "paths relative to vault root, not cwd", dir: "testdata/vault/sub/deep", args: []string{"--tag", "leaf"},
			stdout: lines("sub/deep/leaf.md")},
		{name: "abs with detection", dir: "testdata/vault/sub", args: []string{"--abs", "--path", "fm/none.md"},
			stdout: lines(abs + "/fm/none.md")},
		{name: "abs with relative --vault", args: []string{"--vault", "./testdata//vault/", "--abs", "--path", "fm/none.md"},
			stdout: lines(abs + "/fm/none.md")},
		{name: "last --vault wins", args: []string{"--vault", "/nonexistent-vq-vault", "--vault", "testdata/vault", "--tag", "basic"},
			stdout: lines("tags/inline-basic.md")},
	})
}

func TestParsingBytes(t *testing.T) {
	v := t.TempDir()
	mkFiles(t, v, map[string]string{
		"bom.md":      "\xEF\xBB\xBF---\nstatus: bom\n---\n#bomtag\n",
		"bom-body.md": "\xEF\xBB\xBF#bomstart\n",
		"crlf.md":     "---\r\nstatus: crlf\r\nlit: |\r\n  a\r\n  b\r\n---\r\n```\r\n#incode\r\n```\r\n#crlftag\r\n",
		"cr.md":       "---\rstatus: cr\r---\r#crtag\r",
		"invalid.md":  "---\ntitle: \"a\xffb\"\n---\n#ok\xffx\n",
	})
	j := func(path string) []string { return []string{"--vault", v, "--json", "--path", path} }
	runAll(t, []tc{
		{name: "BOM before frontmatter", args: j("bom.md"),
			stdout: lines(`{"path":"bom.md","frontmatter":{"status":"bom"},"tags":["bomtag"]}`)},
		{name: "BOM before body tag", args: j("bom-body.md"),
			stdout: lines(`{"path":"bom-body.md","frontmatter":{},"tags":["bomstart"]}`)},
		{name: "CRLF", args: j("crlf.md"),
			stdout: lines(`{"path":"crlf.md","frontmatter":{"status":"crlf","lit":"a\nb\n"},"tags":["crlftag"]}`)},
		{name: "CR", args: j("cr.md"),
			stdout: lines(`{"path":"cr.md","frontmatter":{"status":"cr"},"tags":["crtag"]}`)},
		{name: "invalid UTF-8 becomes U+FFFD", args: j("invalid.md"),
			stdout: lines(`{"path":"invalid.md","frontmatter":{"title":"a` + "�" + `b"},"tags":["ok"]}`)},
	})
}

func TestFrontmatter(t *testing.T) {
	j := func(path string) []string { return fx("--json", "--path", path) }
	runAll(t, []tc{
		{name: "none", args: j("fm/none.md"), stdout: lines(`{"path":"fm/none.md","frontmatter":{},"tags":[]}`)},
		{name: "empty", args: j("fm/empty.md"), stdout: lines(`{"path":"fm/empty.md","frontmatter":{},"tags":[]}`)},
		{name: "whitespace only", args: j("fm/blank.md"), stdout: lines(`{"path":"fm/blank.md","frontmatter":{},"tags":[]}`)},
		{name: "unclosed is body", args: j("fm/unclosed.md"),
			stdout: lines(`{"path":"fm/unclosed.md","frontmatter":{},"tags":["fm-unclosed"]}`)},
		{name: "opening not on first line", args: j("fm/notfirst.md"),
			stdout: lines(`{"path":"fm/notfirst.md","frontmatter":{},"tags":[]}`)},
		{name: "indented closing line does not close", args: j("fm/indented-close.md"),
			stdout: lines(`{"path":"fm/indented-close.md","frontmatter":{},"tags":[]}`)},
		{name: "trailing spaces and tabs on delimiters", args: j("fm/trailing.md"),
			stdout: lines(`{"path":"fm/trailing.md","frontmatter":{"status":"trailing"},"tags":[]}`)},
		{name: "comment-only is empty", args: j("fm/comment-only.md"),
			stdout: lines(`{"path":"fm/comment-only.md","frontmatter":{},"tags":[]}`)},
		{name: "YAML comment is not body", args: j("fm/comment.md"),
			stdout: lines(`{"path":"fm/comment.md","frontmatter":{"status":"c"},"tags":[]}`)},
		{name: "scalars not converted", args: j("fm/scalars.md"),
			stdout: lines(`{"path":"fm/scalars.md","frontmatter":{"date":"2024-01-01","code":"007","flag":"true","num":"1.50","hex":"0x1F","tab":"a\tb","single":"it's","esc":"café"},"tags":[]}`)},
		{name: "nulls and quoted null", args: j("fm/nulls.md"),
			stdout: lines(`{"path":"fm/nulls.md","frontmatter":{"a":null,"b":null,"c":null,"d":null,"e":"null","f":"null"},"tags":[]}`)},
		{name: "block scalars", args: j("fm/block.md"),
			stdout: lines(`{"path":"fm/block.md","frontmatter":{"literal":"line1\nline2\n","folded":"a b\n","strip":"x"},"tags":[]}`)},
		{name: "nested values", args: j("fm/nested.md"),
			stdout: lines(`{"path":"fm/nested.md","frontmatter":{"list":["a","b"],"map":{"k":"v","a":"1"},"mixed":["x",{"y":"1"},["z"],null],"emptylist":[],"emptymap":{}},"tags":[]}`)},
		{name: "aliases resolved, merge key kept", args: j("fm/alias.md"),
			stdout: lines(`{"path":"fm/alias.md","frontmatter":{"base":"hello","ref":"hello","defaults":{"x":"1"},"obj":{"<<":{"x":"1"},"y":"2"}},"tags":[]}`)},
		{name: "key order kept", args: j("fm/order.md"),
			stdout: lines(`{"path":"fm/order.md","frontmatter":{"zeta":"1","alpha":"2","mid":"3"},"tags":[]}`)},
		{name: "only notes with real frontmatter have status", args: fx("--path", "fm", "--where", "status"),
			stdout: lines("fm/comment.md", "fm/trailing.md")},
	})
}

// warnAll is every warning of testdata/warn in walk order.
// warnBad is the bad frontmatter warnings of testdata/warn, in walk order.
const warnBad = "vq: bad frontmatter: bad-clash.md\n" +
	"vq: bad frontmatter: bad-cycle.md\n" +
	"vq: bad frontmatter: bad-dup.md\n" +
	"vq: bad frontmatter: bad-key.md\n" +
	"vq: bad frontmatter: bad-laughs.md\n" +
	"vq: bad frontmatter: bad-list-item-dup.md\n" +
	"vq: bad frontmatter: bad-list.md\n" +
	"vq: bad frontmatter: bad-nested-dup.md\n" +
	"vq: bad frontmatter: bad-nested-key.md\n" +
	"vq: bad frontmatter: bad-null-doc.md\n" +
	"vq: bad frontmatter: bad-scalar.md\n" +
	"vq: bad frontmatter: bad-syntax.md\n"

// warnAll is every warning of testdata/warn in walk order.
const warnAll = warnBad +
	"vq: tags should be a list: tags-comma.md\n" +
	"vq: tags should be a list: tags-list-nested.md\n" +
	"vq: tags should be a list: tags-map.md\n" +
	"vq: tags should be a list: tags-space.md\n"

var warnNotes = lines(
	"bad-clash.md", "bad-cycle.md", "bad-dup.md", "bad-key.md", "bad-laughs.md", "bad-list-item-dup.md", "bad-list.md",
	"bad-nested-dup.md", "bad-nested-key.md", "bad-null-doc.md", "bad-scalar.md", "bad-syntax.md",
	"clean.md", "tags-comma.md", "tags-list-nested.md", "tags-map.md", "tags-space.md")

func TestWarnings(t *testing.T) {
	abs := filepath.Join(repoRoot, "testdata", "warn")
	runAll(t, []tc{
		{name: "json over warning vault", args: wx("--json"),
			stdout: lines(
				`{"path":"bad-clash.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-cycle.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-dup.md","frontmatter":{},"tags":["dupbody"]}`,
				`{"path":"bad-key.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-laughs.md","frontmatter":{},"tags":["laughbody"]}`,
				`{"path":"bad-list-item-dup.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-list.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-nested-dup.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-nested-key.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-null-doc.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-scalar.md","frontmatter":{},"tags":[]}`,
				`{"path":"bad-syntax.md","frontmatter":{},"tags":["stillread"]}`,
				`{"path":"clean.md","frontmatter":{"tags":["clean"]},"tags":["clean"]}`,
				`{"path":"tags-comma.md","frontmatter":{"tags":"x, y"},"tags":["x, y"]}`,
				`{"path":"tags-list-nested.md","frontmatter":{"tags":["ok",["no"],{"k":"v"}]},"tags":["ok"]}`,
				`{"path":"tags-map.md","frontmatter":{"tags":{"k":"v"}},"tags":["inlinekept"]}`,
				`{"path":"tags-space.md","frontmatter":{"tags":"foo bar"},"tags":["foo bar"]}`),
			stderr: warnAll},
		{name: "plain listing still warns", args: wx(), stdout: warnNotes, stderr: warnAll},
		{name: "where filter still warns", args: wx("--where", "!a"), stdout: warnNotes, stderr: warnAll},
		{name: "fields still warn", args: wx("--path", "tags-map.md", "--fields", "x"),
			stdout: "tags-map.md\t\n", stderr: "vq: tags should be a list: tags-map.md\n"},
		{name: "tag filter still warns", args: wx("--tag", "clean"), stdout: lines("clean.md"), stderr: warnAll},
		{name: "body read after bad frontmatter", args: wx("--tag", "stillread"), stdout: lines("bad-syntax.md"), stderr: warnAll},
		{name: "scalar tag with comma used as is", args: wx("--tag", "x, y"), stdout: lines("tags-comma.md"), stderr: warnAll},
		{name: "warnings do not change exit code", args: wx("--tag", "nomatch"), code: 1, stderr: warnAll},
		{name: "notes excluded by --path give no warnings", args: wx("--path", "clean.md"), stdout: lines("clean.md")},
		{name: "--path glob limits warnings", args: wx("--path", "bad-*", "--tag", "nomatch"), code: 1, stderr: warnBad},
		{name: "--path prefix limits warnings", args: wx("--path", "tags-comma.md"),
			stdout: lines("tags-comma.md"), stderr: "vq: tags should be a list: tags-comma.md\n"},
		{name: "warnings use abs paths", args: wx("--abs", "--tag", "clean"),
			stdout: lines(abs + "/clean.md"),
			stderr: absWarn(abs)},
	})
}

func absWarn(abs string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(warnAll, "\n") {
		if l == "" {
			continue
		}
		i := strings.LastIndex(l, ": ")
		b.WriteString(l[:i+2] + abs + "/" + l[i+2:])
	}
	return b.String()
}

func TestTags(t *testing.T) {
	j := func(path string) []string { return fx("--json", "--path", path) }
	runAll(t, []tc{
		{name: "fm list: null, empty and # items", args: j("tags/fm-list.md"),
			stdout: lines(`{"path":"tags/fm-list.md","frontmatter":{"tags":["Alpha","#beta",null,"","#","##Gamma"]},"tags":["alpha","beta","gamma"]}`)},
		{name: "fm scalar", args: j("tags/fm-scalar.md"),
			stdout: lines(`{"path":"tags/fm-scalar.md","frontmatter":{"tags":"Solo"},"tags":["solo"]}`)},
		{name: "fm digits-only scalar ignored", args: j("tags/fm-numeric.md"),
			stdout: lines(`{"path":"tags/fm-numeric.md","frontmatter":{"tags":"2024"},"tags":[]}`)},
		{name: "fm trailing - and / trimmed before digits and empty checks", args: j("tags/fm-trim.md"),
			stdout: lines(`{"path":"tags/fm-trim.md","frontmatter":{"tags":["2024","2025-","x","y-/","#Z/","-","/","2026/"]},"tags":["x","y","z"]}`)},
		{name: "escaped # is not a tag, escaped backslash frees it", args: j("tags/inline-escape-hash.md"),
			stdout: lines(`{"path":"tags/inline-escape-hash.md","frontmatter":{},"tags":["escyes"]}`)},
		{name: "escaped character still counts before #", args: j("tags/inline-escape-prev.md"),
			stdout: lines(`{"path":"tags/inline-escape-prev.md","frontmatter":{},"tags":["escparen"]}`)},
		{name: "backslash escapes outside spans", args: j("tags/inline-escape.md"),
			stdout: lines(`{"path":"tags/inline-escape.md","frontmatter":{},"tags":["esca","escb","outside"]}`)},
		{name: "union lowercased and deduplicated", args: j("tags/fm-dedup.md"),
			stdout: lines(`{"path":"tags/fm-dedup.md","frontmatter":{"tags":["Dup","dup"]},"tags":["dup"]}`)},
		{name: "fm nested tags", args: j("tags/fm-nested.md"),
			stdout: lines(`{"path":"tags/fm-nested.md","frontmatter":{"tags":["a/b","#A/B/C"]},"tags":["a/b","a/b/c"]}`)},
		{name: "headings are not tags", args: j("tags/heading.md"),
			stdout: lines(`{"path":"tags/heading.md","frontmatter":{},"tags":["realtag"]}`)},
		{name: "inline basic", args: j("tags/inline-basic.md"),
			stdout: lines(`{"path":"tags/inline-basic.md","frontmatter":{},"tags":["basic","nested/sub/leaf","start","under_score","with-dash"]}`)},
		{name: "inline code spans, unmatched runs literal", args: j("tags/inline-code.md"),
			stdout: lines(`{"path":"tags/inline-code.md","frontmatter":{},"tags":["crossline","opentick"]}`)},
		{name: "inline code deleted, double-backtick span, run without closer", args: j("tags/inline-code-deleted.md"),
			stdout: lines(`{"path":"tags/inline-code-deleted.md","frontmatter":{},"tags":["afterspace","lit","spaced"]}`)},
		{name: "backtick in info string does not open a fence", args: j("tags/inline-fence-info.md"),
			stdout: lines(`{"path":"tags/inline-fence-info.md","frontmatter":{},"tags":["end","notfenced"]}`)},
		{name: "unicode digits", args: j("tags/inline-digits-unicode.md"),
			stdout: lines(`{"path":"tags/inline-digits-unicode.md","frontmatter":{},"tags":["²","٣a"]}`)},
		{name: "Obsidian and HTML comments", args: j("tags/comments.md"),
			stdout: lines(`{"path":"tags/comments.md","frontmatter":{},"tags":["c-after","c-after-fence","c-before","c-code-pct","c-html-after"]}`)},
		{name: "unclosed Obsidian comment runs to end", args: j("tags/comment-unclosed-obs.md"),
			stdout: lines(`{"path":"tags/comment-unclosed-obs.md","frontmatter":{},"tags":["u-before"]}`)},
		{name: "unclosed HTML comment runs to end", args: j("tags/comment-unclosed-html.md"),
			stdout: lines(`{"path":"tags/comment-unclosed-html.md","frontmatter":{},"tags":["h-before"]}`)},
		{name: "wikilinks removed whole, link destinations removed", args: j("tags/links.md"),
			stdout: lines(`{"path":"tags/links.md","frontmatter":{},"tags":["l-aftercomment","l-dest-next","l-nextline","l-text"]}`)},
		{name: "combining marks inside tags, NFC in tags only, mark blocks a tag", args: j("tags/nfd.md"),
			stdout: lines("{\"path\":\"tags/nfd.md\",\"frontmatter\":{\"tags\":[\"Jose\u0301\"],\"nfdname\":\"Jose\u0301\"},\"tags\":[\"caf\u00e9\",\"jos\u00e9\",\"\u00f1o\"]}")},
		{name: "null tags key", args: j("tags/fm-null.md"),
			stdout: lines(`{"path":"tags/fm-null.md","frontmatter":{"tags":null},"tags":["nullbody"]}`)},
		{name: "digits", args: j("tags/inline-digits.md"),
			stdout: lines(`{"path":"tags/inline-digits.md","frontmatter":{},"tags":["1-2","123abc","2025/05","_","v2"]}`)},
		{name: "fences", args: j("tags/inline-fence.md"),
			stdout: lines(`{"path":"tags/inline-fence.md","frontmatter":{},"tags":["after","outside"]}`)},
		{name: "unclosed fence runs to end", args: j("tags/inline-unclosed-fence.md"),
			stdout: lines(`{"path":"tags/inline-unclosed-fence.md","frontmatter":{},"tags":["visible"]}`)},
		{name: "preceding characters", args: j("tags/inline-preceded.md"),
			stdout: lines(`{"path":"tags/inline-preceded.md","frontmatter":{},"tags":["dash","dot","paren","quote"]}`)},
		{name: "trailing - and / trimmed", args: j("tags/inline-trailing.md"),
			stdout: lines(`{"path":"tags/inline-trailing.md","frontmatter":{},"tags":["both","idea","k/-m","path","x--y"]}`)},
		{name: "unicode, sorted by bytes", args: j("tags/inline-unicode.md"),
			stdout: lines(`{"path":"tags/inline-unicode.md","frontmatter":{},"tags":["café_2","cuál","ñandú","日本"]}`)},
		{name: "dot file tags", args: j(".hidden.md"),
			stdout: lines(`{"path":".hidden.md","frontmatter":{"tags":["dotfile"]},"tags":["dotfile"]}`)},
	})
}

func TestTagFilter(t *testing.T) {
	runAll(t, []tc{
		{name: "exact", args: fx("--tag", "basic"), stdout: lines("tags/inline-basic.md")},
		{name: "leading # and case", args: fx("--tag", "#BASIC"), stdout: lines("tags/inline-basic.md")},
		{name: "= form", args: fx("--tag=basic"), stdout: lines("tags/inline-basic.md")},
		{name: "single dash", args: fx("-tag", "basic"), stdout: lines("tags/inline-basic.md")},
		{name: "parent matches subtag", args: fx("--tag", "nested"), stdout: lines("tags/inline-basic.md")},
		{name: "middle level", args: fx("--tag", "nested/sub"), stdout: lines("tags/inline-basic.md")},
		{name: "prefix is not a parent", args: fx("--tag", "nest"), code: 1},
		{name: "fm hierarchy", args: fx("--tag", "a"), stdout: lines("tags/fm-nested.md")},
		{name: "fm hierarchy case", args: fx("--tag", "A/B"), stdout: lines("tags/fm-nested.md")},
		{name: "deeper than tag", args: fx("--tag", "a/b/c/d"), code: 1},
		{name: "unicode lowercased", args: fx("--tag", "CUÁL"), stdout: lines("tags/inline-unicode.md")},
		{name: "trailing / stripped", args: fx("--tag", "nested/"), stdout: lines("tags/inline-basic.md")},
		{name: "trailing / stripped, deeper", args: fx("--tag", "#A/B/"), stdout: lines("tags/fm-nested.md")},
		{name: "NFD --tag matches", args: fx("--tag", "CAFE\u0301"), stdout: lines("tags/nfd.md")},
		{name: "NFC --tag matches NFD tag", args: fx("--tag", "caf\u00e9"), stdout: lines("tags/nfd.md")},
		{name: "NFC --tag matches NFD frontmatter tag", args: fx("--tag", "JOS\u00c9"), stdout: lines("tags/nfd.md")},
		{name: "NFD --tag matches NFC tag", args: fx("--tag", "CUA\u0301L"), stdout: lines("tags/inline-unicode.md")},
		{name: "NFC is not accent stripping", args: fx("--tag", "cafe"), code: 1},
		{name: "combining mark before # blocks a tag", args: fx("--tag", "markblock"), code: 1},
		{name: "tag in Obsidian comment", args: fx("--tag", "c-obs"), code: 1},
		{name: "tag in multi-line Obsidian comment", args: fx("--tag", "c-obsmulti"), code: 1},
		{name: "tag in HTML comment", args: fx("--tag", "c-html"), code: 1},
		{name: "tag in multi-line HTML comment", args: fx("--tag", "c-htmlmulti"), code: 1},
		{name: "comment deleted, not replaced by space", args: fx("--tag", "c-joined"), code: 1},
		{name: "tag after unclosed comment", args: fx("--tag", "u-hidden"), code: 1},
		{name: "wikilink heading", args: fx("--tag", "heading"), code: 1},
		{name: "wikilink display text", args: fx("--tag", "l-display"), code: 1},
		{name: "link anchor", args: fx("--tag", "l-anchor"), code: 1},
		{name: "link text kept", args: fx("--tag", "l-text"), stdout: lines("tags/links.md")},
		{name: "wikilink deleted, not replaced by space", args: fx("--tag", "l-joined"), code: 1},
		{name: "link destination deleted with its ], not replaced by space", args: fx("--tag", "l-joined2"), code: 1},
		{name: "wikilink in code", args: fx("--tag", "l-code"), code: 1},
		{name: "digits-only fm tag ignored", args: fx("--tag", "2024"), code: 1},
		{name: "trimmed fm tag", args: fx("--tag", "y"), stdout: lines("tags/fm-trim.md")},
		{name: "escaped backslash lets backtick open a span", args: fx("--tag", "hidden"), code: 1},
		{name: "escaped # is not a tag", args: fx("--tag", "escnot"), code: 1},
		{name: "escaped # after escaped backslash is not a tag", args: fx("--tag", "esctriple"), code: 1},
		{name: "# after escaped # is not a tag", args: fx("--tag", "escdouble"), code: 1},
		{name: "# after escaped & is not a tag", args: fx("--tag", "escamp"), code: 1},
		{name: "# after escaped ( is a tag", args: fx("--tag", "escparen"), stdout: lines("tags/inline-escape-prev.md")},
		{name: "# after escaped backslash is a tag", args: fx("--tag", "escyes"), stdout: lines("tags/inline-escape-hash.md")},
		{name: "multiple tags AND", args: fx("--tag", "outside", "--tag", "after"), stdout: lines("tags/inline-fence.md")},
		{name: "multiple tags AND no match", args: fx("--tag", "alpha", "--tag", "solo"), code: 1},
		{name: "tag in fence", args: fx("--tag", "infence"), code: 1},
		{name: "tag in nested fence", args: fx("--tag", "nestedfence"), code: 1},
		{name: "tag in YAML comment", args: fx("--tag", "fmcomment"), code: 1},
		{name: "empty tag", args: fx("--tag", ""), code: 2, stderrRe: usageRe("")},
		{name: "tag empty after removing #", args: fx("--tag", "#"), code: 2, stderrRe: usageRe("")},
		{name: "tag empty after removing /", args: fx("--tag", "/"), code: 2, stderrRe: usageRe("")},
		{name: "tag empty after removing # and /", args: fx("--tag", "#/"), code: 2, stderrRe: usageRe("")},
		{name: "tag after deleted code span", args: fx("--tag", "joined"), code: 1},
		{name: "tag inside double-backtick span", args: fx("--tag", "in"), code: 1},
		{name: "unicode digits only", args: fx("--tag", "٢٠٢٤"), code: 1},
	})
}

func TestWhere(t *testing.T) {
	w := func(args ...string) []string { return fx(append([]string{"--path", "where"}, args...)...) }
	all := lines("where/w1.md", "where/w2.md", "where/w3.md", "where/w4.md")
	runAll(t, []tc{
		{name: "=", args: w("--where", "status=open"), stdout: lines("where/w1.md")},
		{name: "spaces ignored", args: w("--where", " status = open "), stdout: lines("where/w1.md")},
		{name: "= case-insensitive", args: w("--where", "status=OPEN"), stdout: lines("where/w1.md")},
		{name: "!= includes missing", args: w("--where", "status!=open"), stdout: lines("where/w2.md", "where/w3.md", "where/w4.md")},
		{name: "present", args: w("--where", "status"), stdout: lines("where/w1.md", "where/w2.md")},
		{name: "not present", args: w("--where", "!status"), stdout: lines("where/w3.md", "where/w4.md")},
		{name: "regex", args: w("--where", "status~^(open|done)$"), stdout: lines("where/w1.md", "where/w2.md")},
		{name: "regex case-insensitive", args: w("--where", "status~^OP"), stdout: lines("where/w1.md")},
		{name: "value with space", args: w("--where", "title=Hello World"), stdout: lines("where/w1.md")},
		{name: "single quotes removed", args: w("--where", "title='hello world'"), stdout: lines("where/w1.md")},
		{name: "double quotes removed", args: w("--where", `title="Hello World"`), stdout: lines("where/w1.md")},
		{name: "mismatched quotes kept", args: w("--where", `title="Hello World'`), code: 1},
		{name: "one pair removed", args: w("--where", "owner='it's'"), stdout: lines("where/w1.md")},
		{name: "value containing =", args: w("--where", "eq=x=y"), stdout: lines("where/w1.md")},
		{name: ">=", args: w("--where", "updated>=2025-01"), stdout: lines("where/w1.md")},
		{name: "<", args: w("--where", "updated<2025"), stdout: lines("where/w2.md")},
		{name: "<=", args: w("--where", "updated<=2024-12-31"), stdout: lines("where/w2.md")},
		{name: ">", args: w("--where", "updated>2024-12-31"), stdout: lines("where/w1.md")},
		{name: "string comparison, not numeric", args: w("--where", "priority>10"), stdout: lines("where/w1.md")},
		{name: "string comparison, not numeric (<)", args: w("--where", "priority<10"), code: 1},
		{name: "list item =", args: w("--where", "aliases=two"), stdout: lines("where/w2.md")},
		{name: "list item !=", args: w("--where", "aliases!=one"), stdout: lines("where/w1.md", "where/w3.md", "where/w4.md")},
		{name: "null list item is empty string", args: w("--where", "aliases<a"), stdout: lines("where/w2.md")},
		{name: "map is present", args: w("--where", "meta"), stdout: lines("where/w2.md")},
		{name: "map has no values", args: w("--where", "meta=v"), code: 1},
		{name: "map != is true", args: w("--where", "meta!=v"), stdout: all},
		{name: "empty list not present", args: w("--where", "labels"), code: 1},
		{name: "!empty list", args: w("--where", "!labels"), stdout: all},
		{name: "empty string not present", args: w("--where", "empty"), code: 1},
		{name: "null not present", args: w("--where", "nul"), code: 1},
		{name: "\\w is ASCII-only", args: w("--where", `name~^\w`), code: 1},
		{name: "regex unicode case folding", args: w("--where", "name~^ñu$"), stdout: lines("where/w4.md")},
		{name: "= unicode case-insensitive", args: w("--where", "name=ñu"), stdout: lines("where/w4.md")},
		{name: "byte order of lowercased", args: w("--where", "name>z"), stdout: lines("where/w4.md")},
		{name: "AND", args: w("--where", "status", "--where", "priority=2"), stdout: lines("where/w1.md")},
		{name: "keys case-sensitive", args: w("--where", "Name=other"), stdout: lines("where/w4.md")},
		{name: "keys case-sensitive (lower)", args: w("--where", "name=other"), code: 1},
		{name: "keys case-sensitive (presence)", args: w("--where", "STATUS"), code: 1},
		{name: "empty VALUE matches null list item", args: w("--where", "aliases="), stdout: lines("where/w2.md")},
		{name: "quoted empty VALUE", args: w("--where", "aliases=''"), stdout: lines("where/w2.md")},
		{name: "empty VALUE: empty string key has no values", args: w("--where", "empty="), code: 1},
		{name: "empty VALUE no match", args: w("--where", "status="), code: 1},
		{name: "!= empty VALUE", args: w("--where", "status!="), stdout: all},
		{name: "empty regex", args: w("--where", "title~"), stdout: lines("where/w1.md")},
		{name: "AND no match", args: w("--where", "status=open", "--where", "status=done"), code: 1},
		{name: "date not converted", args: fx("--where", "date=2024-01-01"), stdout: lines("fm/scalars.md")},
		{name: "leading zeros kept", args: fx("--where", "code=007"), stdout: lines("fm/scalars.md")},
		{name: "no numeric equality", args: fx("--where", "code=7"), code: 1},
		{name: "bool as string", args: fx("--where", "flag=true"), stdout: lines("fm/scalars.md")},
		{name: "quoted null is a string", args: fx("--where", "e=null"), stdout: lines("fm/nulls.md")},
		{name: "null is not the string null", args: fx("--path", "fm/nulls.md", "--where", "a=null"), code: 1},
		{name: "!null key", args: fx("--path", "fm/nulls.md", "--where", "!a"), stdout: lines("fm/nulls.md")},
		{name: "alias value", args: fx("--where", "ref=hello"), stdout: lines("fm/alias.md")},
		{name: "scalar items of mixed list", args: fx("--where", "mixed=x"), stdout: lines("fields/f1.md", "fm/nested.md")},
		{name: "! with operator", args: fx("--where", "!status=done"), code: 2, stderrRe: usageRe("status!=done")},
		{name: "empty key", args: fx("--where", "=open"), code: 2, stderrRe: usageRe("")},
		{name: "empty expression", args: fx("--where", ""), code: 2, stderrRe: usageRe("")},
		{name: "blank expression", args: fx("--where", "  "), code: 2, stderrRe: usageRe("")},
		{name: "bare !", args: fx("--where", "!"), code: 2, stderrRe: usageRe("")},
		{name: "invalid regex", args: fx("--where", "status~("), code: 2, stderrRe: usageRe("")},
		{name: "NFD value, NFC VALUE", args: fx("--where", "nfdname=JOS\u00c9"), stdout: lines("tags/nfd.md")},
		{name: "NFD value, NFC regex", args: fx("--where", "nfdname~^jos\u00e9$"), stdout: lines("tags/nfd.md")},
		{name: "NFC value, NFD VALUE", args: w("--where", "name=N\u0303U"), stdout: lines("where/w4.md")},
		{name: "NFC is not accent stripping (where)", args: fx("--where", "nfdname=jose"), code: 1},
		{name: "! with >", args: fx("--where", "!a>1"), code: 2, stderrRe: usageRe("")},
		{name: "! with ~", args: fx("--where", "!a~x"), code: 2, stderrRe: usageRe("")},
		{name: "! with <=", args: fx("--where", "!a<=1"), code: 2, stderrRe: usageRe("")},
		{name: "! with !=", args: fx("--where", "!a!=x"), code: 2, stderrRe: usageRe("")},
		{name: "empty key before vault", args: []string{"--vault", "/nonexistent-vq-vault", "--where", "=x"},
			code: 2, stderrRe: usageRe("")},
		{name: "! with operator before vault", args: []string{"--vault", "/nonexistent-vq-vault", "--where", "!a>1"},
			code: 2, stderrRe: usageRe("")},
		{name: "invalid regex before vault", args: []string{"--vault", "/nonexistent-vq-vault", "--where", "status~("},
			code: 2, stderrRe: usageRe("")},
	})
}

func TestPath(t *testing.T) {
	runAll(t, []tc{
		{name: "prefix is a directory boundary", args: fx("--path", "path/a"), stdout: lines("path/a/b.md")},
		{name: "slashes trimmed", args: fx("--path", "/path/a/"), stdout: lines("path/a/b.md")},
		{name: "equal to file path", args: fx("--path", "path/a.md"), stdout: lines("path/a.md")},
		{name: "directory prefix", args: fx("--path", "path"),
			stdout: lines("path/Case.md", "path/a/b.md", "path/a.md", "path/ab.md", "path/q[1.md")},
		{name: "prefix is not a suffix", args: fx("--path", "b.md"), code: 1},
		{name: "case-sensitive", args: fx("--path", "path/case.md"), code: 1},
		{name: "* crosses /", args: fx("--path", "path/*.md"),
			stdout: lines("path/Case.md", "path/a/b.md", "path/a.md", "path/ab.md", "path/q[1.md")},
		{name: "glob over whole path", args: fx("--path", "a*"), code: 1},
		{name: "glob with trailing *", args: fx("--path", "path/a*"), stdout: lines("path/a/b.md", "path/a.md", "path/ab.md")},
		{name: "glob suffix with leading slash", args: fx("--path", "/path/*b.md"), stdout: lines("path/a/b.md", "path/ab.md")},
		{name: "? matches /", args: fx("--path", "path/a?b.md"), stdout: lines("path/a/b.md")},
		{name: "? single char", args: fx("--path", "path/?.md"), stdout: lines("path/a.md")},
		{name: "class", args: fx("--path", "path/[Cq]*"), stdout: lines("path/Case.md", "path/q[1.md")},
		{name: "negated class", args: fx("--path", "path/[!a]*"), stdout: lines("path/Case.md", "path/q[1.md")},
		{name: "unclosed [ is literal", args: fx("--path", "path/q[1.md"), stdout: lines("path/q[1.md")},
		{name: "several --path ORed", args: fx("--path", "path/a", "--path", "path/ab.md"), stdout: lines("path/a/b.md", "path/ab.md")},
		{name: "combined with --tag", args: fx("--path", "sub", "--tag", "leaf"), stdout: lines("sub/deep/leaf.md")},
		{name: "leading ./ stripped", args: fx("--path", "./path/a"), stdout: lines("path/a/b.md")},
		{name: "repeated ./ and trailing / stripped", args: fx("--path", "././path/a/"), stdout: lines("path/a/b.md")},
		{name: "leading ./ stripped from glob", args: fx("--path", "./path/*b.md"), stdout: lines("path/a/b.md", "path/ab.md")},
		{name: ". is the whole vault", args: fx("--path", "."), stdout: allNotes},
		{name: "./ is the whole vault", args: fx("--path", "./"), stdout: allNotes},
		{name: "/. is the whole vault", args: fx("--path", "/."), stdout: allNotes},
		{name: "././ is the whole vault", args: fx("--path", "././"), stdout: allNotes},
		{name: ". ORed with a prefix", args: fx("--path", ".", "--path", "path"), stdout: allNotes},
		{name: "empty", args: fx("--path", ""), code: 2, stderrRe: usageRe("")},
		{name: "only slashes", args: fx("--path", "/"), code: 2, stderrRe: usageRe("")},
	})
}

func TestFields(t *testing.T) {
	f := func(args ...string) []string { return fx(append([]string{"--path", "fields/f1.md"}, args...)...) }
	runAll(t, []tc{
		{name: "scalar and list", args: f("--fields", "status,list"), stdout: "fields/f1.md\topen\ta, b, c\n"},
		{name: "trimmed names, empty dropped, missing and null empty",
			args: f("--fields", " status , ,missing,nul "), stdout: "fields/f1.md\topen\t\t\n"},
		{name: "map as compact JSON", args: f("--fields", "map"), stdout: "fields/f1.md\t{\"k\":\"v\",\"n\":[\"1\",\"2\"]}\n"},
		{name: "non-scalar list items as JSON", args: f("--fields", "mixed"), stdout: "fields/f1.md\tx, {\"y\":\"z\"}, [\"p\"]\n"},
		{name: "tab, newline, CR replaced", args: f("--fields", "multi"), stdout: "fields/f1.md\tline1 line2 x y\n"},
		{name: "numbers as written", args: f("--fields", "num"), stdout: "fields/f1.md\t10\n"},
		{name: "null list items print nothing", args: f("--fields", "nulllist"), stdout: "fields/f1.md\ta, , b\n"},
		{name: "name given twice prints twice", args: f("--fields", "status,status"), stdout: "fields/f1.md\topen\topen\n"},
		{name: "no field names", args: f("--fields", ""), stdout: "fields/f1.md\n"},
		{name: "only empty field names", args: f("--fields", " , "), stdout: "fields/f1.md\n"},
		{name: "last --fields wins", args: f("--fields", "status", "--fields", "list"), stdout: "fields/f1.md\ta, b, c\n"},
		{name: "block scalars newlines replaced", args: fx("--path", "fm/block.md", "--fields", "literal,folded"),
			stdout: "fm/block.md\tline1 line2 \ta b \n"},
		{name: "tab in scalar", args: fx("--path", "fm/scalars.md", "--fields", "tab,code,date"),
			stdout: "fm/scalars.md\ta b\t007\t2024-01-01\n"},
		{name: "no frontmatter", args: fx("--path", "fm/none.md", "--fields", "status"), stdout: "fm/none.md\t\n"},
		{name: "several notes", args: fx("--path", "where", "--fields", "status"),
			stdout: "where/w1.md\topen\nwhere/w2.md\tdone\nwhere/w3.md\t\nwhere/w4.md\t\n"},
		{name: "values printed as on disk (NFD)", args: fx("--path", "tags/nfd.md", "--fields", "nfdname,tags"),
			stdout: "tags/nfd.md\tJose\u0301\tJose\u0301\n"},
		{name: "html not escaped", args: fx("--path", "fields/html.md", "--fields", "title"), stdout: "fields/html.md\t<b> & </b>\n"},
		{name: "with --abs", args: f("--abs", "--fields", "status"),
			stdout: filepath.Join(repoRoot, "testdata/vault/fields/f1.md") + "\topen\n"},
	})
}

func TestJSON(t *testing.T) {
	absNone := filepath.Join(repoRoot, "testdata/vault/fm/none.md")
	runAll(t, []tc{
		{name: "full frontmatter", args: fx("--json", "--path", "fields/f1.md"),
			stdout: lines(`{"path":"fields/f1.md","frontmatter":{"status":"open","list":["a","b","c"],"map":{"k":"v","n":["1","2"]},"multi":"line1\nline2\tx\ry","mixed":["x",{"y":"z"},["p"]],"nul":null,"num":"10","nulllist":["a",null,"b"]},"tags":[]}`)},
		{name: "fields in --fields order, missing is null", args: fx("--json", "--path", "fields/f1.md", "--fields", "list,missing,status"),
			stdout: lines(`{"path":"fields/f1.md","frontmatter":{"list":["a","b","c"],"missing":null,"status":"open"},"tags":[]}`)},
		{name: "fields keep tags", args: fx("--json", "--path", "tags/fm-scalar.md", "--fields", "status"),
			stdout: lines(`{"path":"tags/fm-scalar.md","frontmatter":{"status":null},"tags":["solo"]}`)},
		{name: "field given twice appears once", args: fx("--json", "--path", "fields/f1.md", "--fields", "status,list,status"),
			stdout: lines(`{"path":"fields/f1.md","frontmatter":{"status":"open","list":["a","b","c"]},"tags":[]}`)},
		{name: "no field names gives {}", args: fx("--json", "--path", "fields/f1.md", "--fields", ","),
			stdout: lines(`{"path":"fields/f1.md","frontmatter":{},"tags":[]}`)},
		{name: "Go string escaping", args: fx("--json", "--path", "fields/escape.md"),
			stdout: lines(`{"path":"fields/escape.md","frontmatter":{"s":"line\u2028sep\u0001ctl\"q\\b\bend"},"tags":[]}`)},
		{name: "html not escaped", args: fx("--json", "--path", "fields/html.md"),
			stdout: lines(`{"path":"fields/html.md","frontmatter":{"title":"<b> & </b>"},"tags":[]}`)},
		{name: "one line per note in walk order", args: fx("--json", "--path", "where"),
			stdout: lines(
				`{"path":"where/w1.md","frontmatter":{"status":"open","priority":"2","updated":"2025-03-10","title":"Hello World","owner":"it's","eq":"x=y"},"tags":[]}`,
				`{"path":"where/w2.md","frontmatter":{"status":"done","updated":"2024-12-31","aliases":["One","Two",null],"labels":[],"meta":{"k":"v"},"empty":"","nul":null},"tags":[]}`,
				`{"path":"where/w3.md","frontmatter":{},"tags":[]}`,
				`{"path":"where/w4.md","frontmatter":{"name":"Ñu","Name":"Other"},"tags":[]}`)},
		{name: "abs path", args: fx("--json", "--abs", "--path", "fm/none.md"),
			stdout: lines(`{"path":"` + absNone + `","frontmatter":{},"tags":[]}`)},
		{name: "bool option with =value", args: fx("--json=true", "--path", "fm/none.md"),
			stdout: lines(`{"path":"fm/none.md","frontmatter":{},"tags":[]}`)},
	})
}

func TestNull(t *testing.T) {
	runAll(t, []tc{
		{name: "-0", args: fx("-0", "--path", "path/a"), stdout: "path/a/b.md\x00"},
		{name: "--null", args: fx("--null", "--path", "where", "--where", "status"), stdout: "where/w1.md\x00where/w2.md\x00"},
		{name: "-0 with --abs", args: fx("-0", "--abs", "--path", "fm/none.md"),
			stdout: filepath.Join(repoRoot, "testdata/vault/fm/none.md") + "\x00"},
		{name: "-0 no match", args: fx("-0", "--tag", "nomatch"), code: 1},
		{name: "-0 with --json", args: fx("-0", "--json"), code: 2, stderrRe: usageRe("")},
		{name: "--null with --fields", args: fx("--null", "--fields", "status"), code: 2, stderrRe: usageRe("")},
		{name: "-0 with empty --fields", args: fx("-0", "--fields", ""), code: 2, stderrRe: usageRe("")},
		{name: "-0 with --fields naming nothing", args: fx("-0", "--fields", " , "), code: 2, stderrRe: usageRe("")},
	})
}

func TestOptions(t *testing.T) {
	runAll(t, []tc{
		{name: "--help", args: []string{"--help"}, stdoutRe: `(?s).+`},
		{name: "-h", args: []string{"-h"}, stdoutRe: `(?s).+`},
		{name: "no match exits 1 silently", args: fx("--tag", "nomatch"), code: 1},
		{name: "positional argument", args: fx("foo"), code: 2, stderrRe: usageRe("")},
		{name: "positional after options", args: fx("--tag", "basic", "foo"), code: 2, stderrRe: usageRe("")},
		{name: "positional before options", args: []string{"foo", "--vault", "testdata/vault"}, code: 2, stderrRe: usageRe("")},
		{name: "positional after --", args: fx("--", "--tag"), code: 2, stderrRe: usageRe("")},
		{name: "positional without vault", dir: os.TempDir(), args: []string{"foo"}, code: 2, stderrRe: usageRe("")},
		{name: "unknown option", args: fx("--nope"), code: 2, stderrRe: usageRe("")},
		{name: "missing option value", args: fx("--tag"), code: 2, stderrRe: usageRe("")},
		{name: "filters combine with AND", args: fx("--path", "where", "--where", "status", "--tag", "nomatch"), code: 1},
	})
}

func TestStdoutClosedEarly(t *testing.T) {
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	v := t.TempDir()
	pad := strings.Repeat("x", 80)
	files := map[string]string{}
	for i := 0; i < 3000; i++ {
		files[fmt.Sprintf("n%04d-%s.md", i, pad)] = ""
	}
	mkFiles(t, v, files)
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(vqBin, "--vault", v)
	cmd.Env = []string{"HOME=" + homeDir}
	var stderr bytes.Buffer
	cmd.Stdout = pw
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pw.Close()
	if _, err := io.ReadFull(pr, make([]byte, 10)); err != nil {
		t.Fatal(err)
	}
	pr.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("vq did not stop after stdout was closed")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr %q, want empty", stderr.String())
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
		t.Errorf("process state %v, want killed by SIGPIPE", cmd.ProcessState)
	}
}

func TestStdoutWriteError(t *testing.T) {
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to redirect stdout")
	}
	cmd := exec.Command(sh, "-c", `exec "$0" "$@" 1</dev/null`, vqBin, "--vault", "testdata/vault")
	cmd.Dir = repoRoot
	cmd.Env = []string{"HOME=" + homeDir}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Errorf("exit code %d, want 2 (process state %v)", code, cmd.ProcessState)
	}
	if !regexp.MustCompile(`^vq: write error: [^\n]+\n$`).MatchString(stderr.String()) {
		t.Errorf("stderr %q, want one vq: write error line", stderr.String())
	}
}

func TestOddFileNames(t *testing.T) {
	v := t.TempDir()
	mkFiles(t, v, map[string]string{"n\nl.md": "", "t\tab.md": ""})
	runAll(t, []tc{
		{name: "printed as they are", args: []string{"--vault", v}, stdout: "n\nl.md\nt\tab.md\n"},
		{name: "-0", args: []string{"--vault", v, "-0"}, stdout: "n\nl.md\x00t\tab.md\x00"},
		{name: "json escapes", args: []string{"--vault", v, "--json"}, stdout: lines(
			`{"path":"n\nl.md","frontmatter":{},"tags":[]}`,
			`{"path":"t\tab.md","frontmatter":{},"tags":[]}`)},
	})
}

func TestPathNormalization(t *testing.T) {
	v := t.TempDir()
	nfdName := "cafe\u0301.md"  // NFD on disk
	nfcName := "jos\u00e9/n.md" // NFC on disk
	mkFiles(t, v, map[string]string{nfdName: "", nfcName: ""})
	got, err := os.ReadDir(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name() != nfdName {
		t.Skip("file system does not preserve NFD file names")
	}
	runAll(t, []tc{
		{name: "NFC prefix matches NFD file, printed as on disk", args: []string{"--vault", v, "--path", "caf\u00e9.md"},
			stdout: lines(nfdName)},
		{name: "NFD prefix matches NFC dir", args: []string{"--vault", v, "--path", "jose\u0301"},
			stdout: lines(nfcName)},
		{name: "glob ? matches one NFC character", args: []string{"--vault", v, "--path", "caf?.md"},
			stdout: lines(nfdName)},
		{name: "abs path printed as on disk", args: []string{"--vault", v, "--abs", "--path", "caf\u00e9.md"},
			stdout: lines(v + "/" + nfdName)},
		{name: "json path printed as on disk", args: []string{"--vault", v, "--json", "--path", "caf\u00e9.md"},
			stdout: lines(`{"path":"` + nfdName + `","frontmatter":{},"tags":[]}`)},
	})
}
