package vault

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func walkAll(t *testing.T, root string) (notes, warned []string) {
	t.Helper()
	return walkSome(t, root, func(string) bool { return true })
}

func walkSome(t *testing.T, root string, include func(string) bool) (notes, warned []string) {
	t.Helper()
	err := Walk(root, include, func(rel, path string) bool {
		notes = append(notes, rel)
		return true
	}, func(rel string, err error) {
		warned = append(warned, rel)
	})
	if err != nil {
		t.Fatal(err)
	}
	return notes, warned
}

func TestWalk(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		"c.md", "a.md", "b/x.md", "b/y.txt", "b.md", "Z.md", "upper.MD", ".hidden.md",
		".obsidian/w.md", ".trash/t.md", "d/.git/g.md", "d/e/f.md",
	} {
		writeFile(t, root, f)
	}
	must(t, os.Symlink(filepath.Join(root, "a.md"), filepath.Join(root, "link.md")))
	must(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken.md")))
	must(t, os.Symlink(filepath.Join(root, "b"), filepath.Join(root, "dirlink")))
	must(t, os.Symlink(filepath.Join(root, "b"), filepath.Join(root, "dirlink.md")))

	notes, warned := walkAll(t, root)
	want := []string{".hidden.md", "Z.md", "a.md", "b/x.md", "b.md", "c.md", "d/e/f.md", "link.md"}
	if !slices.Equal(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	if want := []string{"broken.md"}; !slices.Equal(warned, want) {
		t.Errorf("warned = %q, want %q", warned, want)
	}
}

func TestWalkRootIsDotDirOrSymlink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, ".vault")
	writeFile(t, root, "n.md")
	notes, _ := walkAll(t, root)
	if want := []string{"n.md"}; !slices.Equal(notes, want) {
		t.Errorf("dot root: notes = %q, want %q", notes, want)
	}

	link := filepath.Join(base, "link")
	must(t, os.Symlink(root, link))
	notes, _ = walkAll(t, link)
	if want := []string{"n.md"}; !slices.Equal(notes, want) {
		t.Errorf("symlink root: notes = %q, want %q", notes, want)
	}
}

func TestWalkStops(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.md")
	writeFile(t, root, "b.md")
	var notes []string
	err := Walk(root, func(string) bool { return true }, func(rel, path string) bool {
		notes = append(notes, rel)
		return false
	}, func(string, error) {})
	if err != nil || !slices.Equal(notes, []string{"a.md"}) {
		t.Errorf("notes = %q, err = %v", notes, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestWalkIncludeBeforeStat(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "d/ok.md")
	writeFile(t, root, "skip.md")
	must(t, os.Symlink(filepath.Join(root, "nowhere.md"), filepath.Join(root, "broken.md")))
	must(t, os.Symlink("loop.md", filepath.Join(root, "loop.md")))

	var asked []string
	notes, warned := walkSome(t, root, func(rel string) bool {
		asked = append(asked, rel)
		return strings.HasPrefix(rel, "d/")
	})
	if want := []string{"d/ok.md"}; !slices.Equal(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	if len(warned) != 0 {
		t.Errorf("warned = %q, want none", warned)
	}
	// include is asked about notes only, not other files or directories.
	if want := []string{"broken.md", "d/ok.md", "loop.md", "skip.md"}; !slices.Equal(asked, want) {
		t.Errorf("asked = %q, want %q", asked, want)
	}

	_, warned = walkAll(t, root)
	if want := []string{"broken.md", "loop.md"}; !slices.Equal(warned, want) {
		t.Errorf("without filter: warned = %q, want %q", warned, want)
	}
}
