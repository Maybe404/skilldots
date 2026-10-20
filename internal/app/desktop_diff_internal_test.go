package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
)

func hunkText(h []DiffHunk) string {
	var b strings.Builder
	for _, x := range h {
		b.WriteString("@@\n")
		for _, l := range x.Lines {
			b.WriteString(l.Op + l.Text + "\n")
		}
	}
	return b.String()
}

func TestLineDiff(t *testing.T) {
	a := strings.Split("1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20", " ")
	b := append([]string{}, a...)
	b[1] = "two"
	b = append(b[:18], b[19:]...) // drop 19
	h, added, removed := lineDiff(a, b)
	if added != 1 || removed != 2 || len(h) != 2 {
		t.Fatalf("added %d removed %d hunks %d\n%s", added, removed, len(h), hunkText(h))
	}
	if h[0].OldStart != 1 || h[0].OldLines != 5 || h[0].NewStart != 1 || h[0].NewLines != 5 {
		t.Fatalf("first hunk %+v", h[0])
	}
	if want := "@@\n 1\n-2\n+two\n 3\n 4\n 5\n"; !strings.HasPrefix(hunkText(h), want) {
		t.Fatalf("got\n%s", hunkText(h))
	}
	if h[1].OldStart != 16 || h[1].OldLines != 5 || h[1].NewLines != 4 {
		t.Fatalf("second hunk %+v", h[1])
	}

	// Close changes merge into one hunk; equal files have none.
	c := append([]string{}, a...)
	c[4], c[8] = "five", "nine"
	if h, _, _ := lineDiff(a, c); len(h) != 1 {
		t.Fatalf("merged: %s", hunkText(h))
	}
	if h, ad, rm := lineDiff(a, a); len(h) != 0 || ad+rm != 0 {
		t.Fatal("equal files differ")
	}
	// A new file: everything added, starting after line 0.
	if h, ad, _ := lineDiff(nil, []string{"x", "y"}); ad != 2 || len(h) != 1 || h[0].OldStart != 0 || h[0].NewStart != 1 {
		t.Fatalf("new file: %+v", h)
	}
}

func TestDiffFilesBinaryAndLarge(t *testing.T) {
	big := []byte(strings.Repeat("x\n", maxDiffBytes))
	files, more := diffFiles(
		map[string][]byte{"a.png": {0, 1}, "big.txt": []byte("x\n"), "same.md": []byte("s")},
		map[string][]byte{"a.png": {0, 2}, "big.txt": big, "same.md": []byte("s")},
		nil)
	if more != 0 || len(files) != 2 || !files[0].Binary || !files[1].TooLarge {
		t.Fatalf("%+v", files)
	}
	if hashFiles(map[string][]byte{"b": []byte("1"), "a": []byte("2")}) == hashFiles(map[string][]byte{"a": []byte("1"), "b": []byte("2")}) {
		t.Fatal("hash ignores which file holds what")
	}
}

// hashFiles must agree with the lock's hash (fsutil.HashTree), or no
// vendored version would ever be recognised.
func TestHashFilesMatchesHashTree(t *testing.T) {
	dir := t.TempDir()
	for p, body := range map[string]string{"SKILL.md": "a", "ref/x.md": "b", "ref-y.md": "c", ".DS_Store": "noise", "node_modules/m.js": "m"} {
		f := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(f), 0o755)
		if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := readFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fsutil.HashTree(dir)
	if err != nil || hashFiles(files) != want || len(files) != 3 {
		t.Fatalf("hash %s, HashTree %s (%v), files %v", hashFiles(files), want, err, files)
	}
}
