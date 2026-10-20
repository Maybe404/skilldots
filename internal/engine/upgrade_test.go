package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// A library like the author's own, linked by a skd that kept no placed
// record: built-in targets only, link mode, skills/, private/ (its own
// git repository), a vendored pack and one app shipped in an
// application bundle outside the home directory, no symbolic link
// inside the library. After the upgrade nothing changes on disk: every
// entry is kept, nothing is skipped, warned about or trashed; the first
// command (app.Open) records the app's links and the next apply every
// other link, and disabling the app then removes its links outright.
func TestUpgradeOfATypicalLibraryChangesNothing(t *testing.T) {
	lib, reg, home := newTestLib(t)
	all := []string{"agents", "claude", "codex", "gemini", "grok"}
	lib.Manifest.Targets.Default = all
	for _, d := range []string{".codex", ".gemini", ".grok"} {
		os.MkdirAll(filepath.Join(home, d), 0o755) // installed here
	}
	writeSkill(t, filepath.Join(lib.Root, "skills", "mine"), "mine")
	writeSkill(t, filepath.Join(lib.Root, "private", "secret"), "secret")
	os.MkdirAll(filepath.Join(lib.Root, "private", ".git"), 0o755)
	os.WriteFile(filepath.Join(lib.Root, "private", ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	vd := filepath.Join(lib.Root, "vendor", "acme", "skills", "eng", "tdd")
	writeSkill(t, vd, "tdd")
	h, _ := fsutil.HashTree(vd)
	lib.Manifest.Packs = append(lib.Manifest.Packs, &manifest.Pack{Repo: "acme/skills", Use: []string{"*"}})
	lib.Lock.Packs["acme/skills"] = &lock.Pack{Skills: map[string]*lock.Skill{"tdd": {Category: "eng", Hash: h}}}
	surge := filepath.Join(t.TempDir(), "Applications", "Surge.app", "Contents", "Resources", "Skills", "surge")
	writeSkill(t, surge, "surge")
	os.MkdirAll(filepath.Join(surge, "references"), 0o755)
	os.WriteFile(filepath.Join(surge, "references", "cli.md"), []byte("x"), 0o644)
	lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "surge", Path: surge})

	// Linked by the previous version: no placed record.
	entries, warnings, _ := Resolve(lib, reg)
	if len(warnings) != 0 {
		t.Fatalf("warnings %v", warnings)
	}
	if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(home, "placed.json"))
	links := len(lib.Lock.Links)
	if links != 4*len(all) {
		t.Fatalf("links %v", lib.Lock.LinkKeys())
	}

	entries, warnings, _ = Resolve(lib, reg)
	if !OlderSkd() {
		t.Fatal("no placed file, yet not taken for an older skd's")
	}
	if n, _ := AdoptOlderAppLinks(lib, entries); n != len(all) {
		t.Fatalf("adopted %d app links", n)
	}
	if OlderSkd() {
		t.Fatal("still looking for an older skd's links")
	}
	plan := MakePlan(lib, entries, false)
	if len(warnings) != 0 || plan.Changes() {
		t.Fatalf("after upgrade: %v %+v", warnings, plan.ChangeList())
	}
	for _, o := range plan.Ops {
		if o.Action != "keep" {
			t.Fatalf("after upgrade: %+v", o)
		}
	}
	Remember(lib, plan)
	if m := loadPlaced(); len(m) != links {
		t.Fatalf("recorded %d of %d: %v", len(m), links, m)
	}

	lib.Manifest.Apps = nil
	entries, _, _ = Resolve(lib, reg)
	res, err := Apply(lib, MakePlan(lib, entries, false))
	if err != nil || len(res.Backups) != 0 || len(res.Refused) != 0 {
		t.Fatalf("%v %v %v", res.Backups, res.Refused, err)
	}
	for _, n := range all {
		tg, err := reg.Get(n)
		if err != nil {
			t.Fatal(err)
		}
		d := filepath.Join(tg.Path, "surge")
		if fsutil.Lexists(d) {
			t.Fatalf("%s left", d)
		}
	}
	if items, _ := ListTrash(lib, reg); len(items) != 0 {
		t.Fatalf("trash %+v", items)
	}
	if !fileExists(filepath.Join(surge, "references", "cli.md")) {
		t.Fatal("the app's files went")
	}
}
