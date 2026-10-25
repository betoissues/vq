package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// failWriter fails every write with err.
type failWriter struct{ err error }

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

func testVault(t *testing.T, notes int) string {
	t.Helper()
	root := t.TempDir()
	for i := range notes {
		p := filepath.Join(root, fmt.Sprintf("note%05d.md", i))
		if err := os.WriteFile(p, []byte("#t\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRunWriteError(t *testing.T) {
	small := testVault(t, 2)   // output fits the buffer: fails at Flush
	large := testVault(t, 600) // output exceeds the buffer: fails at Write
	tests := []struct {
		name   string
		root   string
		err    error
		code   int
		stderr string
	}{
		{"flush, disk full", small, syscall.ENOSPC, 2, "vq: write error: " + syscall.ENOSPC.Error() + "\n"},
		{"write, bad descriptor", large, &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EBADF},
			2, "vq: write error: " + syscall.EBADF.Error() + "\n"},
		{"flush, broken pipe", small, syscall.EPIPE, 0, ""},
		{"write, broken pipe", large, &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run([]string{"--vault", tt.root}, failWriter{tt.err}, &stderr)
			if code != tt.code || stderr.String() != tt.stderr {
				t.Errorf("code %d, stderr %q; want %d, %q", code, stderr.String(), tt.code, tt.stderr)
			}
		})
	}
}

func TestRunPathFilterSkipsSymlinkStat(t *testing.T) {
	root := testVault(t, 0)
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "ok.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nowhere.md", filepath.Join(root, "broken.md")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--vault", root, "--path", "d"}, &stdout, &stderr)
	if code != 0 || stdout.String() != "d/ok.md\n" || stderr.Len() != 0 {
		t.Errorf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	stderr.Reset()
	run([]string{"--vault", root}, &stdout, &stderr)
	if !strings.HasPrefix(stderr.String(), "vq: cannot read: broken.md: ") {
		t.Errorf("without --path: stderr %q", stderr.String())
	}
}

func TestNotReadable(t *testing.T) {
	err := notReadable("/v", &os.PathError{Op: "open", Path: "/v/", Err: syscall.EACCES})
	if want := "not a readable directory: /v: " + syscall.EACCES.Error(); err.Error() != want {
		t.Errorf("got %q, want %q", err, want)
	}
	err = notReadable("./nope", errors.New("gone"))
	if want := "not a readable directory: ./nope: gone"; err.Error() != want {
		t.Errorf("got %q, want %q", err, want)
	}
}

func TestRunNotReadableRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nope")
	code := run([]string{"--vault", missing}, &stdout, &stderr)
	want := "vq: not a readable directory: " + missing + ": " + syscall.ENOENT.Error() + "\n"
	if code != 2 || stderr.String() != want {
		t.Errorf("code %d, stderr %q; want 2, %q", code, stderr.String(), want)
	}
}

func TestRunNFC(t *testing.T) {
	root := testVault(t, 0)
	dir := filepath.Join(root, "café")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := "---\ntitle: Café\n---\n#café\n"
	if err := os.WriteFile(filepath.Join(dir, "n.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"--vault", root, "--path", "café", "--tag", "café", "--where", "title=café", "--json"}
	code := run(args, &stdout, &stderr)
	// Path and frontmatter as on disk, tags in NFC.
	want := "{\"path\":\"café/n.md\",\"frontmatter\":{\"title\":\"Café\"},\"tags\":[\"café\"]}\n"
	if code != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Errorf("code %d, stdout %q, stderr %q; want %q", code, stdout.String(), stderr.String(), want)
	}
}
