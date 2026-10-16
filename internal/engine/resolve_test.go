package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

// newTestLib returns an empty library under a temp dir, with HOME pointed
// at another temp dir and the registry installed.
func newTestLib(t *testing.T) (*library.Library, *targets.Registry, string) {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	lib := &library.Library{Root: root, Manifest: manifest.New([]string{"agents"}),
		Lock: &lock.Lock{Version: 1, Packs: map[string]*lock.Pack{}, Links: map[string]*lock.Link{}}}
	reg, err := targets.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	SetRegistry(reg)
	return lib, reg, home
}

func TestResolveIsolatesProblems(t *testing.T) {
	lib, reg, home := newTestLib(t)
	writeSkill(t, filepath.Join(lib.Root, "skills", "fine"), "fine")
	writeSkill(t, filepath.Join(lib.Root, "skills", "clash"), "clash")
	writeSkill(t, filepath.Join(lib.Root, "private", "clash"), "clash")
	writeSkill(t, filepath.Join(lib.Root, "skills", "typo"), "typo")
	bad := []string{"agents", "nosuchagent"}
	lib.Manifest.Locals = append(lib.Manifest.Locals, &manifest.Local{Name: "typo", Targets: &bad})
	lib.Manifest.Packs = append(lib.Manifest.Packs, &manifest.Pack{Repo: "o/unfetched", Use: []string{"*"}})

	entries, warnings, err := Resolve(lib, reg)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(warnings, "\n")
	for _, want := range []string{"nosuchagent", "skd update o/unfetched", filepath.Join(lib.Root, "skills", "clash"), filepath.Join(lib.Root, "private", "clash")} {
		if !strings.Contains(all, want) {
			t.Errorf("warnings missing %q:\n%s", want, all)
		}
	}

	// A clash's existing link must survive; everything else is linked.
	agents := filepath.Join(home, ".agents", "skills")
	os.MkdirAll(agents, 0o755)
	os.Symlink(filepath.Join(lib.Root, "skills", "clash"), filepath.Join(agents, "clash"))
	lib.Lock.Links["agents/clash"] = &lock.Link{Mode: "link", Source: "skills/clash"}

	plan := MakePlan(lib, entries, false)
	got := map[string]string{}
	for _, op := range plan.Ops {
		got[op.Target.Name+"/"+op.Name+"/"+op.Action] = op.Reason
	}
	for _, k := range []string{"agents/fine/create", "agents/typo/create", "nosuchagent/typo/skip", "agents/clash/skip"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing op %s in %v", k, got)
		}
	}
	if _, err := Apply(lib, plan); err != nil {
		t.Fatal(err)
	}
	if !isLinkTo(filepath.Join(agents, "clash"), filepath.Join(lib.Root, "skills", "clash")) {
		t.Error("existing link for a conflicting name was touched")
	}
	if !isLinkTo(filepath.Join(agents, "fine"), filepath.Join(lib.Root, "skills", "fine")) {
		t.Error("unrelated skill not linked")
	}
}

func TestOrphanLinkOfRemovedTarget(t *testing.T) {
	lib, _, home := newTestLib(t)
	gone := filepath.Join(home, "custom")
	os.MkdirAll(gone, 0o755)
	writeSkill(t, filepath.Join(lib.Root, "skills", "x"), "x")
	os.Symlink(filepath.Join(lib.Root, "skills", "x"), filepath.Join(gone, "x"))
	lib.Lock.Links["mine/x"] = &lock.Link{Mode: "link", Source: "skills/x", Path: filepath.Join(gone, "x")}
	lib.Lock.Links["old/y"] = &lock.Link{Mode: "link", Source: "skills/y"}
	lib.Manifest.Targets.Default = []string{}

	plan := MakePlan(lib, nil, false)
	if _, err := Apply(lib, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(gone, "x")); !os.IsNotExist(err) {
		t.Error("recorded link of a removed target was not cleaned up")
	}
	if len(lib.Lock.Links) != 0 {
		t.Errorf("lock still has %v", lib.Lock.LinkKeys())
	}
}
