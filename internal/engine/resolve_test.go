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
	reg, err := targets.NewRegistry("", nil)
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

// A record of a target that is gone is forgotten, and the link stays on
// disk: the lock comes with the library and may be forged, so a target
// not in use never has anything removed. The plan says where it is.
func TestOrphanLinkOfRemovedTarget(t *testing.T) {
	lib, _, home := newTestLib(t)
	gone := filepath.Join(home, "custom")
	os.MkdirAll(gone, 0o755)
	writeSkill(t, filepath.Join(lib.Root, "skills", "x"), "x")
	os.Symlink(filepath.Join(lib.Root, "skills", "x"), filepath.Join(gone, "x"))
	lib.Lock.Links["mine/x"] = &lock.Link{Mode: "link", Source: "skills/x", Path: filepath.Join(gone, "x")}
	lib.Lock.Links["old/y"] = &lock.Link{Mode: "link", Source: "skills/y"}
	lib.Manifest.Targets.Default = []string{}

	for _, force := range []bool{false, true} {
		plan := MakePlan(lib, nil, force)
		for _, op := range plan.Ops {
			if op.Action != "forget" {
				t.Errorf("force=%v: %+v", force, op)
			}
			if op.Name == "x" && !strings.Contains(op.Reason, filepath.Join(gone, "x")) {
				t.Errorf("the plan does not say where the link is: %s", op.Reason)
			}
		}
	}
	if _, err := Apply(lib, MakePlan(lib, nil, true)); err != nil {
		t.Fatal(err)
	}
	if !isLinkTo(filepath.Join(gone, "x"), filepath.Join(lib.Root, "skills", "x")) {
		t.Error("the link of a removed target was removed")
	}
	if len(lib.Lock.Links) != 0 {
		t.Errorf("lock still has %v", lib.Lock.LinkKeys())
	}
}

type (
	lockPack  = lock.Pack
	lockSkill = lock.Skill
)

// A lock record whose path was pointed outside every target directory
// (tampered, or a target that is gone) never lets apply --force move a
// real directory there; the record is forgotten.
func TestOrphanPathOutsideTargetsIsForgotten(t *testing.T) {
	lib, _, home := newTestLib(t)
	victim := filepath.Join(home, "Documents", "x")
	writeSkill(t, victim, "x")
	os.WriteFile(filepath.Join(victim, "thesis.txt"), []byte("precious"), 0o644)
	lib.Lock.Links["gone/x"] = &lock.Link{Mode: "link", Source: "skills/x", Path: victim}
	lib.Manifest.Targets.Default = []string{}
	for _, force := range []bool{false, true} {
		plan := MakePlan(lib, nil, force)
		if len(plan.Ops) != 1 || plan.Ops[0].Action != "forget" || !strings.Contains(plan.Ops[0].Reason, victim) {
			t.Fatalf("force=%v: plan %+v", force, plan.Ops)
		}
	}
	if _, err := Apply(lib, MakePlan(lib, nil, true)); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(victim, "thesis.txt")); err != nil || string(b) != "precious" {
		t.Fatal("directory outside the targets was touched")
	}
	if len(lib.Lock.Links) != 0 {
		t.Fatal("record not forgotten")
	}
}

// The same name for two skills is fine when they go to different agents.
func TestSameNameDisjointTargets(t *testing.T) {
	lib, reg, home := newTestLib(t)
	writeSkill(t, filepath.Join(lib.VendorDir(), "o", "r", "good"), "good")
	lib.Lock.Packs["o/r"] = &lock.Pack{Commit: "c", Skills: map[string]*lock.Skill{"good": {Hash: "h"}}}
	agentsOnly, claudeOnly := []string{"agents"}, []string{"claude"}
	lib.Manifest.Packs = []*manifest.Pack{{Repo: "o/r", Use: []string{"*"}, Targets: &agentsOnly}}
	writeSkill(t, filepath.Join(lib.Root, "skills", "good"), "good")
	lib.Manifest.Locals = []*manifest.Local{{Name: "good", Targets: &claudeOnly}}

	entries, warnings, _ := Resolve(lib, reg)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if _, err := Apply(lib, MakePlan(lib, entries, false)); err != nil {
		t.Fatal(err)
	}
	if !isLinkTo(filepath.Join(home, ".agents", "skills", "good"), filepath.Join(lib.VendorDir(), "o", "r", "good")) {
		t.Error("vendor good not linked into agents")
	}
	if !isLinkTo(filepath.Join(home, ".claude", "skills", "good"), filepath.Join(lib.Root, "skills", "good")) {
		t.Error("local good not linked into claude")
	}

	// Overlapping targets still clash.
	both := []string{"agents", "claude"}
	lib.Manifest.Locals[0].Targets = &both
	if _, warnings, _ := Resolve(lib, reg); len(warnings) != 1 {
		t.Fatalf("overlap: warnings %v", warnings)
	}
}

// Recorded paths with ".." (through a missing directory or through a
// symlink) or through a symlinked directory never reach a real directory
// outside the targets, even with --force.
func TestOrphanPathTraversalVariants(t *testing.T) {
	lib, _, home := newTestLib(t)
	outside := filepath.Join(home, "outside")
	victim := filepath.Join(outside, "victim")
	writeSkill(t, victim, "victim")
	os.WriteFile(filepath.Join(victim, "thesis.txt"), []byte("precious"), 0o644)
	deep := filepath.Join(home, "deep", "er")
	os.MkdirAll(deep, 0o755)
	os.Symlink(deep, filepath.Join(outside, "lnk")) // lnk/.. resolves to home/deep
	os.Symlink(outside, filepath.Join(home, "via")) // via/victim is victim
	writeSkill(t, filepath.Join(home, "deep", "victim"), "victim")
	os.WriteFile(filepath.Join(home, "deep", "victim", "thesis.txt"), []byte("precious"), 0o644)
	lib.Manifest.Targets.Default = []string{}
	for _, p := range []string{
		outside + "/nope/../victim",
		outside + "/lnk/../victim",
		home + "/via/victim",
		"relative/victim",
		outside + "//victim",
	} {
		for _, force := range []bool{false, true} {
			lib.Lock.Links["gone/victim"] = &lock.Link{Mode: "link", Source: "skills/victim", Path: p}
			plan := MakePlan(lib, nil, force)
			if len(plan.Ops) != 1 || plan.Ops[0].Action != "forget" {
				t.Errorf("%s force=%v: %+v", p, force, plan.Ops)
			}
			if _, err := Apply(lib, plan); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, v := range []string{victim, filepath.Join(home, "deep", "victim")} {
		if b, err := os.ReadFile(filepath.Join(v, "thesis.txt")); err != nil || string(b) != "precious" {
			t.Errorf("%s was touched", v)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(lib.Root, TrashDir, "*")); len(m) != 0 {
		t.Errorf("trash: %v", m)
	}
}

// An orphan record outside the targets whose source was rewritten to
// point outside the library cannot be used to delete someone's symlink.
func TestOrphanSourceOutsideLibrary(t *testing.T) {
	lib, _, home := newTestLib(t)
	elsewhere := filepath.Join(home, "elsewhere")
	writeSkill(t, filepath.Join(elsewhere, "real"), "real")
	dir := filepath.Join(home, "tools")
	os.MkdirAll(dir, 0o755)
	userLink := filepath.Join(dir, "x")
	os.Symlink(filepath.Join(elsewhere, "real"), userLink)
	rel, _ := filepath.Rel(lib.Root, filepath.Join(elsewhere, "real"))
	lib.Manifest.Targets.Default = []string{}
	for _, src := range []string{filepath.Join(elsewhere, "real"), filepath.ToSlash(rel)} {
		for _, force := range []bool{false, true} {
			lib.Lock.Links["gone/x"] = &lock.Link{Mode: "link", Source: src, Path: userLink}
			plan := MakePlan(lib, nil, force)
			if len(plan.Ops) != 1 || plan.Ops[0].Action != "forget" {
				t.Errorf("source %s force=%v: %+v", src, force, plan.Ops)
			}
			Apply(lib, plan)
			if _, err := os.Lstat(userLink); err != nil {
				t.Fatalf("source %s force=%v: the user's symlink was deleted", src, force)
			}
		}
	}
	// Our own link (source inside the library) is only forgotten too.
	writeSkill(t, filepath.Join(lib.Root, "skills", "y"), "y")
	ours := filepath.Join(dir, "y")
	os.Symlink(filepath.Join(lib.Root, "skills", "y"), ours)
	lib.Lock.Links["gone/y"] = &lock.Link{Mode: "link", Source: "skills/y", Path: ours}
	Apply(lib, MakePlan(lib, nil, true))
	if _, err := os.Lstat(ours); err != nil || lib.Lock.Links["gone/y"] != nil {
		t.Errorf("our own orphan link: %v, record %v", err, lib.Lock.Links["gone/y"])
	}
}

// Links into a custom target that has since been removed, an app link
// (its source is the [[app]] path, outside the library), a vendored one,
// and tampered ones, are all only forgotten: nothing in a target not in
// use is removed, with --force too.
func TestOrphanAppLinkOfRemovedTarget(t *testing.T) {
	lib, _, home := newTestLib(t)
	app := filepath.Join(home, "Applications", "Surge.app", "skills", "surge")
	writeSkill(t, app, "surge")
	lib.Manifest.Apps = append(lib.Manifest.Apps, &manifest.App{Name: "surge", Path: "~/Applications/Surge.app/skills/surge"})
	none := []string{}
	lib.Manifest.Apps[0].Targets = &none
	writeSkill(t, filepath.Join(lib.Root, "vendor", "o", "r", "tdd"), "tdd")
	gone := filepath.Join(home, "custom")
	os.MkdirAll(gone, 0o755)
	os.Symlink(app, filepath.Join(gone, "surge"))
	os.Symlink(filepath.Join(lib.Root, "vendor", "o", "r", "tdd"), filepath.Join(gone, "tdd"))
	lib.Lock.Links["mine/surge"] = &lock.Link{Mode: "link", Source: lib.Rel(app), Path: filepath.Join(gone, "surge")}
	lib.Lock.Links["mine/tdd"] = &lock.Link{Mode: "link", Source: "vendor/o/r/tdd", Path: filepath.Join(gone, "tdd")}

	// Tampered: a source outside the library that is not an [[app]], and
	// an [[app]] source recorded under another link name.
	elsewhere := filepath.Join(home, "elsewhere", "real")
	writeSkill(t, elsewhere, "real")
	os.Symlink(elsewhere, filepath.Join(gone, "real"))
	lib.Lock.Links["mine/real"] = &lock.Link{Mode: "link", Source: elsewhere, Path: filepath.Join(gone, "real")}
	os.Symlink(app, filepath.Join(gone, "alias"))
	lib.Lock.Links["mine/alias"] = &lock.Link{Mode: "link", Source: app, Path: filepath.Join(gone, "alias")}
	lib.Manifest.Targets.Default = []string{}

	entries, _, err := Resolve(lib, registryPtr.Load())
	if err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		got := map[string]string{}
		for _, op := range MakePlan(lib, entries, force).Ops {
			got[op.Target.Name+"/"+op.Name] = op.Action
		}
		want := map[string]string{"mine/surge": "forget", "mine/tdd": "forget", "mine/real": "forget", "mine/alias": "forget"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("force=%v %s: %q, want %q (plan %v)", force, k, got[k], v, got)
			}
		}
	}
	if _, err := Apply(lib, MakePlan(lib, entries, true)); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"surge", "tdd", "real", "alias"} {
		if _, err := os.Lstat(filepath.Join(gone, n)); err != nil {
			t.Errorf("the user's %s link was deleted", n)
		}
	}
	if _, err := os.Stat(filepath.Join(app, "SKILL.md")); err != nil {
		t.Error("the app's own files were touched")
	}
	if len(lib.Lock.Links) != 0 {
		t.Errorf("lock still has %v", lib.Lock.LinkKeys())
	}
}
