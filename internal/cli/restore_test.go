package cli_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// findDir is the directory named name under root (the first found).
func findDir(t *testing.T, root, name string) string {
	t.Helper()
	var found string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == name && found == "" {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no %s under %s", name, root)
	}
	return found
}

// Everything skd itself moves into the trash can be restored: what apply
// --force and adopt move out of an agent's directory, what update moves
// aside in vendor/, a removed vendored skill and a purged pack.
func TestRestoreWhatEachCommandParks(t *testing.T) {
	e := newEnv(t)
	lib := e.p("lib")
	e.ok("", "init", lib, "--no-skill")
	for _, n := range []string{"a", "b", "c", "d"} {
		e.skill("up/skills/"+n, n)
	}
	up := e.repo("up")
	// In the way of a's link.
	e.skill(".agents/skills/a", "a")
	e.write(".agents/skills/a/mine.txt", "mine\n")
	e.ok(lib, "add", up, "--all")
	e.ok(lib, "apply", "--force")

	// b changed here and upstream: update moves the local one aside.
	b := findDir(t, filepath.Join(lib, "vendor"), "b")
	c := findDir(t, filepath.Join(lib, "vendor"), "c")
	pack := filepath.Dir(filepath.Dir(b)) // vendor/<pack>/skills/b
	if filepath.Base(filepath.Dir(b)) != "skills" {
		pack = filepath.Dir(b)
	}
	os.WriteFile(filepath.Join(b, "local.txt"), []byte("local\n"), 0o644)
	e.write("up/skills/b/more.md", "upstream\n")
	e.git(up, "add", "-A")
	e.git(up, "commit", "-qm", "b")
	e.ok(lib, "update", "--yes")
	e.ok(lib, "remove", "c", "--yes")
	e.skill(".claude/skills/dirx", "namey")
	e.ok(lib, "adopt", "--yes")
	e.ok(lib, "purge", up, "--yes")

	out, _, _ := e.skd(lib, "trash", "--json")
	var items []struct {
		ID         string `json:"id"`
		Origin     string `json:"origin"`
		Restorable bool   `json:"restorable"`
		Problem    string `json:"problem"`
	}
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := map[string]bool{e.p(".agents/skills/a"): false, b: false, c: false, e.p(".claude/skills/dirx"): false, pack: false}
	for _, it := range items {
		if !it.Restorable {
			t.Errorf("%s from %s is not restorable: %s", it.ID, it.Origin, it.Problem)
		}
		if _, ok := want[it.Origin]; ok {
			want[it.Origin] = true
		}
	}
	for o, seen := range want {
		if !seen {
			t.Errorf("nothing in the trash from %s:\n%s", o, out)
		}
	}
	// Newest first: the pack, then what was parked before it.
	restored := map[string]bool{}
	for _, it := range items {
		if _, err := os.Lstat(it.Origin); err == nil {
			continue // b's new copy came back with the pack
		}
		e.ok(lib, "trash", "restore", it.ID)
		if st, err := os.Lstat(it.Origin); err != nil || !st.IsDir() {
			t.Errorf("%s is not back at %s", it.ID, it.Origin)
		}
		restored[it.Origin] = true
	}
	for _, o := range []string{pack, c, e.p(".agents/skills/a"), e.p(".claude/skills/dirx")} {
		if !restored[o] {
			t.Errorf("%s was not restored", o)
		}
	}
	if b, err := os.ReadFile(e.p(".agents/skills/a/mine.txt")); err != nil || string(b) != "mine\n" {
		t.Error("a's own file did not come back")
	}
}
