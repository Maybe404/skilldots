package app

import (
	"context"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A file larger than maxDiffBytes is never held in memory: from disk or
// from the library's git history, only its hash is, and the diff says it
// is too large.
func TestDiffNeverHoldsALargeFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	big := strings.Repeat("x", maxDiffBytes+1)
	for p, body := range map[string]string{"s/SKILL.md": "a\n", "s/big.bin": big} {
		f := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, []byte(body), 0o644)
	}
	sum := sha256.Sum256([]byte(big))
	files, err := readFiles(filepath.Join(root, "s"))
	if err != nil || string(files["SKILL.md"]) != "a\n" || string(files["big.bin"]) != string(tooLargeMark(sum[:])) {
		t.Fatalf("readFiles: %v %q", err, files["big.bin"])
	}

	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "x"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	tree, err := gitTreeFiles(context.Background(), root, "HEAD", "s")
	if err != nil || string(tree["SKILL.md"]) != "a\n" || string(tree["big.bin"]) != string(tooLargeMark(sum[:])) {
		t.Fatalf("gitTreeFiles: %v %q", err, tree["big.bin"])
	}

	other := tooLargeMark(make([]byte, 32))
	diff, _ := diffFiles(map[string][]byte{"big.bin": files["big.bin"]}, map[string][]byte{"big.bin": other}, nil)
	if len(diff) != 1 || !diff[0].TooLarge || diff[0].Binary {
		t.Fatalf("%+v", diff)
	}
	if diff, _ := diffFiles(files, tree, nil); len(diff) != 0 {
		t.Fatalf("the same large file differs: %+v", diff)
	}
}
