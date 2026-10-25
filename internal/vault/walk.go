package vault

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Walk calls visit for every note under root, in walk order, with its
// vault-relative path (using '/') and its filesystem path, and stops when
// visit returns false. Notes for which include returns false are skipped
// before they are looked at, so they produce no warnings. Entries that cannot
// be read are reported to warn with their vault-relative path and are skipped.
// Directories whose name starts with a dot are skipped, except root, and
// symlinks to directories are not followed.
func Walk(root string, include func(rel string) bool, visit func(rel, path string) bool, warn func(rel string, err error)) error {
	// A trailing separator makes WalkDir follow root when it is a symlink.
	walkRoot := root
	if !strings.HasSuffix(walkRoot, string(filepath.Separator)) {
		walkRoot += string(filepath.Separator)
	}
	return filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		isRoot := path == walkRoot
		rel := relPath(walkRoot, path)
		if err != nil {
			if isRoot {
				return err
			}
			warn(rel, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !isRoot && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") || !include(rel) {
			return nil
		}
		mode := d.Type()
		if mode&fs.ModeSymlink != 0 {
			info, err := os.Stat(path)
			if err != nil {
				warn(rel, err)
				return nil
			}
			mode = info.Mode().Type()
		}
		if mode.IsRegular() && !visit(rel, path) {
			return fs.SkipAll
		}
		return nil
	})
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	return filepath.ToSlash(rel)
}

// ReadNote reads and parses the note at path.
func ReadNote(path string) (*Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}
